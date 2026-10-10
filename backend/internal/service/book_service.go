package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/formats"
	"ebookshelf/internal/repository"
)

const invalidFileNameChars = `/\:*?"<>|`

type BookService struct {
	bookRepo repository.BookRepository
	readRepo repository.ReadStatusRepository
	files    *FileService
}

func NewBookService(bookRepo repository.BookRepository, readRepo repository.ReadStatusRepository, files *FileService) *BookService {
	return &BookService{bookRepo: bookRepo, readRepo: readRepo, files: files}
}

// ---- catalog: books already added to the library ----

func (s *BookService) List(ctx context.Context, userID int64, req dto.BookFilterRequest, page, pageSize int) (domain.Page[dto.BookSummaryResponse], error) {
	var (
		result domain.Page[dto.BookSummaryResponse]
		books  domain.Page[domain.BookSummary]
		err    error
		filter = domain.BookFilter{
			Query:    strings.TrimSpace(req.Query),
			AuthorID: req.AuthorID,
			TagID:    req.TagID,
			Language: strings.ToLower(req.Language),
			Read:     domain.ReadFilter(req.Read),
			Sort:     domain.BookSort(req.Sort),
		}
	)

	if !filter.Valid() {
		return result, apperrors.InvalidRequest("read must be read or unread and sort must be created or title")
	}

	if books, err = s.bookRepo.FindAll(ctx, userID, filter, page, pageSize); err != nil {
		slog.ErrorContext(ctx, "list books failed", "err", err)
		err = apperrors.Internal("failed to list books")
	} else {
		items := make([]dto.BookSummaryResponse, len(books.Items))
		for i, b := range books.Items {
			items[i] = dto.BookSummaryResponse{ID: b.ID, Title: b.Title, Authors: emptyIfNil(b.Authors), Read: b.Read}
		}
		result = domain.NewPage(items, books.Page, books.PageSize, books.Total)
	}
	return result, err
}

func (s *BookService) Get(ctx context.Context, userID, id int64) (dto.BookResponse, error) {
	var (
		resp dto.BookResponse
		read bool
	)

	book, err := s.findBook(ctx, id)
	if err == nil {
		if read, err = s.readRepo.Exists(ctx, userID, id); err != nil {
			slog.ErrorContext(ctx, "get book: read status failed", "book_id", id, "err", err)
			err = apperrors.Internal("failed to get book")
		}
	}
	if err == nil {
		resp = toBookResponse(book, read, findCoverPath(s.files, id) != "")
	}
	return resp, err
}

func (s *BookService) File(ctx context.Context, id int64) (dto.FileResponse, error) {
	var file dto.FileResponse

	book, err := s.findBook(ctx, id)
	if err == nil {
		file = dto.FileResponse{Path: s.files.Path(s.files.BookPath(id, book.Format)), Name: downloadName(book), Hash: book.FileHash}
	}
	return file, err
}

func (s *BookService) Cover(ctx context.Context, id int64) (string, error) {
	var abs string

	_, err := s.findBook(ctx, id)
	if err == nil {
		if cover := findCoverPath(s.files, id); cover != "" {
			abs = s.files.Path(cover)
		} else {
			err = apperrors.BookNotFound("cover not found")
		}
	}
	return abs, err
}

func (s *BookService) MarkRead(ctx context.Context, userID, id int64) error {
	_, err := s.findBook(ctx, id)
	if err == nil {
		if err = s.readRepo.Create(ctx, userID, id); err != nil {
			slog.ErrorContext(ctx, "mark read failed", "book_id", id, "err", err)
			err = apperrors.Internal("failed to mark book as read")
		}
	}
	return err
}

func (s *BookService) MarkUnread(ctx context.Context, userID, id int64) error {
	_, err := s.findBook(ctx, id)
	if err == nil {
		if err = s.readRepo.Delete(ctx, userID, id); err != nil {
			slog.ErrorContext(ctx, "mark unread failed", "book_id", id, "err", err)
			err = apperrors.Internal("failed to mark book as unread")
		}
	}
	return err
}

// ---- pending books: uploaded, waiting for the user to add them ----

func (s *BookService) Upload(ctx context.Context, actorID int64, filename string, src io.Reader) (dto.BookResponse, error) {
	var (
		resp dto.BookResponse
		meta *formats.Metadata
		book *domain.Book
		hash string
		ext  = fileExt(filename)
		tmp  = s.files.NewTmpPath(ext)
	)

	err := s.files.Save(ctx, tmp, src, domain.BookFilePolicy)
	if err == nil {
		if hash, err = s.files.Hash(tmp); err != nil {
			slog.ErrorContext(ctx, "upload: hash failed", "err", err)
			err = apperrors.Internal("failed to save pending book")
		}
	}
	if err == nil {
		err = s.duplicateOf(ctx, hash)
	}
	if err == nil {
		meta, err = s.readMetadata(ctx, tmp)
	}
	if err == nil {
		book = bookFromMetadata(meta, ext)
		book.FileHash, book.SourceHash = hash, hash
		book.Status, book.UploadedBy = domain.BookPending, &actorID
		book.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		err = s.insertPending(ctx, book, tmp)
	}
	if err == nil {
		s.storeCover(ctx, book.ID, meta.Cover)
		resp = toBookResponse(book, false, findCoverPath(s.files, book.ID) != "")
		slog.InfoContext(ctx, "pending book received", "book_id", book.ID)
	} else if rmErr := s.files.Remove(tmp); rmErr != nil {
		slog.WarnContext(ctx, "upload: cleanup failed", "err", rmErr)
	}
	return resp, err
}

func (s *BookService) ListPending(ctx context.Context, actor dto.UserResponse) ([]dto.BookSummaryResponse, error) {
	var (
		items      []dto.BookSummaryResponse
		uploadedBy int64
	)

	if !actor.Role.IsStaff() {
		uploadedBy = actor.ID
	}
	books, err := s.bookRepo.FindPending(ctx, uploadedBy)
	if err != nil {
		slog.ErrorContext(ctx, "list pending books failed", "err", err)
		err = apperrors.Internal("failed to list pending books")
	} else {
		items = make([]dto.BookSummaryResponse, len(books))
		for i, b := range books {
			items[i] = dto.BookSummaryResponse{ID: b.ID, Title: b.Title, Authors: emptyIfNil(b.Authors)}
		}
	}
	return items, err
}

func (s *BookService) GetPending(ctx context.Context, actor dto.UserResponse, id int64) (dto.BookResponse, error) {
	var resp dto.BookResponse

	book, err := s.findPending(ctx, actor, id)
	if err == nil {
		resp = toBookResponse(book, false, findCoverPath(s.files, id) != "")
	}
	return resp, err
}

func (s *BookService) DiscardPending(ctx context.Context, actor dto.UserResponse, id int64) error {
	_, err := s.findPending(ctx, actor, id)
	if err == nil {
		if err = s.bookRepo.DeletePending(ctx, id); err != nil {
			slog.ErrorContext(ctx, "discard pending book failed", "book_id", id, "err", err)
			err = apperrors.Internal("failed to discard pending book")
		}
	}
	if err == nil {
		s.removeFiles(ctx, id)
		slog.InfoContext(ctx, "pending book discarded", "book_id", id)
	}
	return err
}

func (s *BookService) CommitPending(ctx context.Context, actor dto.UserResponse, id int64) (dto.BookResponse, error) {
	var resp dto.BookResponse

	book, err := s.findPending(ctx, actor, id)
	if err == nil {
		book.Status, book.CreatedAt = domain.BookAdded, time.Now().UTC().Format(time.RFC3339)
		if err = s.bookRepo.MarkAdded(ctx, id, book.CreatedAt); err != nil {
			slog.ErrorContext(ctx, "commit pending book failed", "book_id", id, "err", err)
			err = apperrors.Internal("failed to add book")
		}
	}
	if err == nil {
		resp = toBookResponse(book, false, findCoverPath(s.files, id) != "")
		slog.InfoContext(ctx, "book added", "book_id", id)
	}
	return resp, err
}

func (s *BookService) findPending(ctx context.Context, actor dto.UserResponse, id int64) (*domain.Book, error) {
	book, err := s.bookRepo.FindByID(ctx, id)
	switch {
	case err == nil && book.Status == domain.BookPending && (actor.Role.IsStaff() || book.UploadedBy != nil && *book.UploadedBy == actor.ID):
	case err == nil || errors.Is(err, sql.ErrNoRows):
		err = apperrors.BookNotFound("pending book not found")
	default:
		slog.ErrorContext(ctx, "find pending book failed", "book_id", id, "err", err)
		err = apperrors.Internal("failed to find pending book")
	}
	return book, err
}

// the row and the file move commit together: a failed move or commit leaves neither row nor folder
func (s *BookService) insertPending(ctx context.Context, book *domain.Book, tmp string) error {
	err := s.bookRepo.Create(ctx, book, func(id int64) error {
		err := s.files.Move(tmp, s.files.BookPath(id, book.Format))
		if err == nil {
			err = s.files.Link(s.files.BookPath(id, book.Format), s.files.OriginalPath(id, book.Format))
		}
		return err
	})
	if err != nil && book.ID != 0 {
		if rmErr := s.files.Remove(s.files.BookDir(book.ID)); rmErr != nil {
			slog.WarnContext(ctx, "upload: remove folder failed", "book_id", book.ID, "err", rmErr)
		}
	}
	switch {
	case err == nil:
	case errors.Is(err, repository.ErrConflict):
		if err = s.duplicateOf(ctx, book.FileHash); err == nil {
			err = apperrors.Internal("failed to add book")
		}
	default:
		slog.ErrorContext(ctx, "upload: add book failed", "err", err)
		err = apperrors.Internal("failed to save pending book")
	}
	return err
}

// returns BookDuplicate when a book already has this hash and nil when none does
func (s *BookService) duplicateOf(ctx context.Context, fileHash string) error {
	existing, err := s.bookRepo.FindByFileHash(ctx, fileHash)
	switch {
	case err == nil:
		err = apperrors.BookDuplicate("book already exists", existing.ID)
	case errors.Is(err, sql.ErrNoRows):
		err = nil
	default:
		slog.ErrorContext(ctx, "find duplicate failed", "err", err)
		err = apperrors.Internal("failed to check duplicates")
	}
	return err
}

// the extension was already checked against BookFilePolicy, so formats.For finds a reader
func (s *BookService) readMetadata(ctx context.Context, relPath string) (*formats.Metadata, error) {
	meta, err := formats.For(fileExt(relPath)).Read(s.files.Path(relPath))
	if err != nil {
		slog.WarnContext(ctx, "read book failed", "err", err)
		err = apperrors.UnreadableFile("file is corrupt or not a valid book")
	}
	return meta, err
}

// a cover that breaks the policy is dropped: the book is still valid without it
func (s *BookService) storeCover(ctx context.Context, id int64, data []byte) {
	if ext := detectCoverExt(data); ext != "" {
		if err := s.files.Save(ctx, s.files.CoverPath(id, ext), bytes.NewReader(data), domain.CoverPolicy); err != nil {
			slog.WarnContext(ctx, "upload: cover dropped", "book_id", id, "err", err)
		}
	}
}

func (s *BookService) removeFiles(ctx context.Context, id int64) {
	rels := []string{s.files.BookDir(id)}
	for _, ext := range domain.CoverPolicy.AllowedExts {
		rels = append(rels, s.files.CoverPath(id, ext))
	}
	for _, rel := range rels {
		if err := s.files.Remove(rel); err != nil {
			slog.WarnContext(ctx, "remove book files failed", "path", rel, "err", err)
		}
	}
}

func bookFromMetadata(m *formats.Metadata, ext string) *domain.Book {
	book := &domain.Book{
		Title:       m.Title,
		Language:    nullIfEmpty(m.Language),
		Publisher:   nullIfEmpty(m.Publisher),
		Description: nullIfEmpty(m.Description),
		Format:      ext,
		Authors:     m.Authors,
		Tags:        m.Tags,
		Identifiers: m.Identifiers,
	}
	if m.Year != 0 {
		book.PubYear = &m.Year
	}
	return book
}

func detectCoverExt(data []byte) string {
	var ext string

	switch http.DetectContentType(data) {
	case "image/jpeg":
		ext = domain.CoverJPG
	case "image/png":
		ext = domain.CoverPNG
	}
	return ext
}

// ---- helpers shared by both ----

func (s *BookService) findBook(ctx context.Context, id int64) (*domain.Book, error) {
	book, err := s.bookRepo.FindByID(ctx, id)
	switch {
	case err == nil && book.Status != domain.BookAdded, errors.Is(err, sql.ErrNoRows):
		err = apperrors.BookNotFound("book not found")
	case err != nil:
		slog.ErrorContext(ctx, "find book failed", "book_id", id, "err", err)
		err = apperrors.Internal("failed to find book")
	}
	return book, err
}

func findCoverPath(files *FileService, id int64) string {
	var (
		found string
		exts  = domain.CoverPolicy.AllowedExts
	)

	for i := 0; found == "" && i < len(exts); i++ {
		if p := files.CoverPath(id, exts[i]); files.Exists(p) {
			found = p
		}
	}
	return found
}

func toBookResponse(b *domain.Book, read, hasCover bool) dto.BookResponse {
	return dto.BookResponse{ID: b.ID, Status: b.Status, Read: read, CreatedAt: b.CreatedAt, BookMetadataResponse: toMetadataResponse(b, hasCover)}
}

func toMetadataResponse(b *domain.Book, hasCover bool) dto.BookMetadataResponse {
	return dto.BookMetadataResponse{
		Title:       b.Title,
		Language:    b.Language,
		Publisher:   b.Publisher,
		PubYear:     b.PubYear,
		Description: b.Description,
		Format:      b.Format,
		Authors:     emptyIfNil(b.Authors),
		Tags:        emptyIfNil(b.Tags),
		Identifiers: toIdentifierResponses(b.Identifiers),
		HasCover:    hasCover,
	}
}

func toIdentifierResponses(ids []domain.Identifier) []dto.IdentifierResponse {
	resp := make([]dto.IdentifierResponse, len(ids))
	for i, id := range ids {
		resp[i] = dto.IdentifierResponse{Type: id.Type, Value: id.Value}
	}
	return resp
}

func downloadName(b *domain.Book) string {
	name := b.Title
	if len(b.Authors) > 0 {
		name += " - " + strings.Join(b.Authors, ", ")
	}
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(invalidFileNameChars, r) || unicode.IsControl(r) {
			r = '_'
		}
		return r
	}, name)
	return name + "." + b.Format
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func emptyIfNil(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}

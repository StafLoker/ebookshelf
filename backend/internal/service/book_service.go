package service

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/formats"
	"ebookshelf/internal/repository"
)

const (
	invalidFileNameChars = `/\:*?"<>|`
	libraryRules         = "a book in the library needs a title and an author with a last name; " +
		"a first name is one word and a year is 1 to 9999"
)

type BookService struct {
	bookRepo repository.BookRepository
	readRepo repository.ReadStatusRepository
	files    *FileService
}

func NewBookService(bookRepo repository.BookRepository, readRepo repository.ReadStatusRepository, files *FileService) *BookService {
	return &BookService{bookRepo: bookRepo, readRepo: readRepo, files: files}
}

// ---- create ----

func (s *BookService) Add(ctx context.Context, actorID int64, filename string, src io.Reader) (dto.BookResponse, error) {
	var (
		ext = fileExt(filename)
		tmp = s.files.NewTmpPath(ext)
	)
	// once the book is stored tmp is gone, so this only removes a failed upload
	defer func() {
		if rmErr := s.files.Remove(tmp); rmErr != nil {
			slog.WarnContext(ctx, "add book: cleanup failed", "err", rmErr)
		}
	}()

	if err := s.files.Save(ctx, tmp, src, domain.BookFilePolicy); err != nil {
		return dto.BookResponse{}, err
	}

	hash, err := s.files.Hash(tmp)
	if err != nil {
		slog.ErrorContext(ctx, "add book: hash failed", "err", err)
		return dto.BookResponse{}, apperrors.Internal("failed to save pending book")
	}

	if err = s.duplicateOf(ctx, hash); err != nil {
		return dto.BookResponse{}, err
	}

	meta, err := s.readMetadata(ctx, tmp)
	if err != nil {
		return dto.BookResponse{}, err
	}

	book := bookFromMetadata(meta, ext)
	book.FileHash, book.SourceHash = hash, hash
	book.Status, book.AddedBy = domain.BookPending, &actorID
	book.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	if err = s.insertPending(ctx, book, tmp); err != nil {
		return dto.BookResponse{}, err
	}

	s.storeCover(ctx, book.ID, meta.Cover)
	slog.InfoContext(ctx, "pending book received", "book_id", book.ID)
	return toBookResponse(book, false, findCoverPath(s.files, book.ID) != ""), nil
}

// ---- read ----

// a pending book is listed only to its uploader, admin and root
func (s *BookService) List(ctx context.Context, actor dto.UserResponse, req dto.BookFilterRequest, page, pageSize int) (domain.Page[dto.BookSummaryResponse], error) {
	filter := domain.BookFilter{
		Status:   domain.BookStatus(req.Status),
		Query:    strings.TrimSpace(req.Query),
		AuthorID: req.AuthorID,
		TagID:    req.TagID,
		Language: strings.ToLower(req.Language),
		Read:     domain.ReadFilter(req.Read),
		Sort:     domain.BookSort(req.Sort),
	}

	if filter.Status == "" {
		filter.Status = domain.BookAdded
	}
	if filter.Status == domain.BookPending && !actor.Role.IsStaff() {
		filter.AddedBy = actor.ID
	}
	if !filter.Valid() {
		return domain.Page[dto.BookSummaryResponse]{}, apperrors.InvalidRequest("status must be added or pending, read must be read or unread and sort must be created or title")
	}

	books, err := s.bookRepo.FindAll(ctx, actor.ID, filter, page, pageSize)
	if err != nil {
		slog.ErrorContext(ctx, "list books failed", "err", err)
		return domain.Page[dto.BookSummaryResponse]{}, apperrors.Internal("failed to list books")
	}

	items := make([]dto.BookSummaryResponse, len(books.Items))
	for i, b := range books.Items {
		items[i] = dto.BookSummaryResponse{ID: b.ID, Title: b.Title, Authors: toAuthors(b.Authors), Read: b.Read}
	}
	return domain.NewPage(items, books.Page, books.PageSize, books.Total), nil
}

func (s *BookService) Get(ctx context.Context, actor dto.UserResponse, id int64) (dto.BookResponse, error) {
	book, err := s.find(ctx, actor, id)
	if err != nil {
		return dto.BookResponse{}, err
	}

	read, err := s.readRepo.Exists(ctx, actor.ID, id)
	if err != nil {
		slog.ErrorContext(ctx, "get book: read status failed", "book_id", id, "err", err)
		return dto.BookResponse{}, apperrors.Internal("failed to get book")
	}
	return toBookResponse(book, read, findCoverPath(s.files, id) != ""), nil
}

func (s *BookService) File(ctx context.Context, actor dto.UserResponse, id int64) (dto.FileResponse, error) {
	book, err := s.find(ctx, actor, id)
	if err != nil {
		return dto.FileResponse{}, err
	}
	return dto.FileResponse{Path: s.files.Path(s.files.BookPath(id, book.Format)), Name: downloadName(book), Hash: book.FileHash}, nil
}

func (s *BookService) Cover(ctx context.Context, actor dto.UserResponse, id int64) (string, error) {
	if _, err := s.find(ctx, actor, id); err != nil {
		return "", err
	}

	cover := findCoverPath(s.files, id)
	if cover == "" {
		return "", apperrors.BookNotFound("cover not found")
	}
	return s.files.Path(cover), nil
}

// ---- update ----

func (s *BookService) Update(ctx context.Context, actor dto.UserResponse, id int64, req dto.UpdateBookRequest) (dto.BookResponse, error) {
	book, err := s.findFor(ctx, actor, id, domain.PermEdit)
	if err != nil {
		return dto.BookResponse{}, err
	}

	if err = s.edit(ctx, book, &req, nil); err != nil {
		return dto.BookResponse{}, err
	}
	return s.Get(ctx, actor, id)
}

func (s *BookService) SetCover(ctx context.Context, actor dto.UserResponse, id int64, src io.Reader) (dto.BookResponse, error) {
	book, err := s.findFor(ctx, actor, id, domain.PermEdit)
	if err != nil {
		return dto.BookResponse{}, err
	}

	cover, err := readCover(src)
	if err != nil {
		return dto.BookResponse{}, err
	}

	if err = s.edit(ctx, book, nil, cover); err != nil {
		return dto.BookResponse{}, err
	}
	return s.Get(ctx, actor, id)
}

// ---- delete ----

func (s *BookService) Delete(ctx context.Context, actor dto.UserResponse, id int64) error {
	book, err := s.findFor(ctx, actor, id, domain.PermDelete)
	if err != nil {
		return err
	}

	if err = s.bookRepo.Delete(ctx, id, book.Status); err != nil {
		slog.ErrorContext(ctx, "delete book failed", "book_id", id, "err", err)
		return apperrors.Internal("failed to delete book")
	}
	s.removeFiles(ctx, id)
	slog.InfoContext(ctx, "book deleted", "book_id", id)
	return nil
}

// ---- other actions ----

func (s *BookService) Commit(ctx context.Context, actor dto.UserResponse, id int64) (dto.BookResponse, error) {
	book, err := s.find(ctx, actor, id)
	if err != nil {
		return dto.BookResponse{}, err
	}
	if book.Status != domain.BookPending {
		return dto.BookResponse{}, apperrors.BookNotFound("pending book not found")
	}

	book.Status, book.CreatedAt = domain.BookAdded, time.Now().UTC().Format(time.RFC3339)
	err = s.bookRepo.MarkAdded(ctx, id, book.CreatedAt)
	if errors.Is(err, repository.ErrInvalid) {
		return dto.BookResponse{}, apperrors.InvalidRequest(libraryRules)
	}
	if err != nil {
		slog.ErrorContext(ctx, "commit pending book failed", "book_id", id, "err", err)
		return dto.BookResponse{}, apperrors.Internal("failed to add book")
	}

	slog.InfoContext(ctx, "book added", "book_id", id)
	return toBookResponse(book, false, findCoverPath(s.files, id) != ""), nil
}

func (s *BookService) MarkRead(ctx context.Context, actor dto.UserResponse, id int64) error {
	book, err := s.find(ctx, actor, id)
	if err != nil {
		return err
	}
	if book.Status != domain.BookAdded {
		return apperrors.BookNotFound("book not found")
	}

	if err := s.readRepo.Create(ctx, actor.ID, id); err != nil {
		slog.ErrorContext(ctx, "mark read failed", "book_id", id, "err", err)
		return apperrors.Internal("failed to mark book as read")
	}
	return nil
}

func (s *BookService) MarkUnread(ctx context.Context, actor dto.UserResponse, id int64) error {
	book, err := s.find(ctx, actor, id)
	if err != nil {
		return err
	}
	if book.Status != domain.BookAdded {
		return apperrors.BookNotFound("book not found")
	}

	if err := s.readRepo.Delete(ctx, actor.ID, id); err != nil {
		slog.ErrorContext(ctx, "mark unread failed", "book_id", id, "err", err)
		return apperrors.Internal("failed to mark book as unread")
	}
	return nil
}

// ---- helpers ----

// a pending book exists only for who uploaded it, admin and root
func (s *BookService) find(ctx context.Context, actor dto.UserResponse, id int64) (*domain.Book, error) {
	book, err := s.bookRepo.FindByID(ctx, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.ErrorContext(ctx, "find book failed", "book_id", id, "err", err)
		return book, apperrors.Internal("failed to find book")
	}
	if err != nil || book.Status == domain.BookPending && !actor.Role.IsStaff() && (book.AddedBy == nil || *book.AddedBy != actor.ID) {
		return book, apperrors.BookNotFound("book not found")
	}
	return book, nil
}

// a book in the library needs the permission of the action; a pending one only the permission to add books
func (s *BookService) findFor(ctx context.Context, actor dto.UserResponse, id int64, perm domain.Permission) (*domain.Book, error) {
	book, err := s.find(ctx, actor, id)
	if err != nil {
		return book, err
	}

	if book.Status == domain.BookPending {
		perm = domain.PermAdd
	}
	user := domain.User{Role: actor.Role, CanAdd: actor.CanAdd, CanEdit: actor.CanEdit, CanDelete: actor.CanDelete}
	if !user.Allowed(perm) {
		return book, apperrors.Forbidden("permission required: " + string(perm))
	}
	return book, nil
}

// the row and the file move commit together: a failed move or commit leaves neither row nor folder
func (s *BookService) insertPending(ctx context.Context, book *domain.Book, tmp string) error {
	err := s.bookRepo.Create(ctx, book, func(id int64) error {
		if err := s.files.Move(tmp, s.files.BookPath(id, book.Format)); err != nil {
			return err
		}
		return s.files.Link(s.files.BookPath(id, book.Format), s.files.OriginalPath(id, book.Format))
	})
	if err != nil {
		if book.ID != 0 {
			if rmErr := s.files.Remove(s.files.BookDir(book.ID)); rmErr != nil {
				slog.WarnContext(ctx, "add book: remove folder failed", "book_id", book.ID, "err", rmErr)
			}
		}
		if errors.Is(err, repository.ErrConflict) {
			if dupErr := s.duplicateOf(ctx, book.FileHash); dupErr != nil {
				return dupErr
			}
			return apperrors.Internal("failed to add book")
		}
		slog.ErrorContext(ctx, "add book failed", "err", err)
		return apperrors.Internal("failed to save pending book")
	}
	return nil
}

// returns BookDuplicate when a book already has this hash and nil when none does
func (s *BookService) duplicateOf(ctx context.Context, fileHash string) error {
	existing, err := s.bookRepo.FindByHash(ctx, fileHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "find duplicate failed", "err", err)
		return apperrors.Internal("failed to check duplicates")
	}
	return apperrors.BookDuplicate("book already exists", existing.ID)
}

// the extension was already checked against BookFilePolicy, so formats.For finds a reader
func (s *BookService) readMetadata(ctx context.Context, relPath string) (*formats.Metadata, error) {
	meta, err := formats.For(fileExt(relPath)).Read(s.files.Path(relPath))
	if err != nil {
		slog.WarnContext(ctx, "read book failed", "err", err)
		return meta, apperrors.UnreadableFile("file is corrupt or not a valid book")
	}
	return meta, nil
}

// a cover that breaks the policy is dropped: the book is still valid without it
func (s *BookService) storeCover(ctx context.Context, id int64, data []byte) {
	if detectCoverExt(data) != "" {
		if err := s.replaceCover(ctx, id, data); err != nil {
			slog.WarnContext(ctx, "add book: cover dropped", "book_id", id, "err", err)
		}
	}
}

// the copy is built in tmp/ and moved over the book inside the database transaction, as in the upload,
// so a rejected change leaves file and row as they were. A nil req keeps the metadata
func (s *BookService) edit(ctx context.Context, book *domain.Book, req *dto.UpdateBookRequest, cover []byte) error {
	var (
		next   = *book
		format = formats.For(book.Format)
		rel    = s.files.BookPath(book.ID, book.Format)
		tmp    = s.files.NewTmpPath(book.Format)
	)

	if req != nil {
		if err := applyUpdate(&next, *req); err != nil {
			return err
		}
	}
	if cover == nil && reflect.DeepEqual(*book, next) {
		return nil
	}

	defer func() {
		if rmErr := s.files.Remove(tmp); rmErr != nil {
			slog.WarnContext(ctx, "edit: cleanup failed", "err", rmErr)
		}
	}()

	if err := format.Write(s.files.Path(rel), s.files.Path(tmp), metadataFromBook(&next, cover)); err != nil {
		slog.ErrorContext(ctx, "write book file failed", "book_id", book.ID, "err", err)
		return apperrors.FileWriteFailed("failed to write the book file")
	}
	if _, err := format.Read(s.files.Path(tmp)); err != nil {
		slog.ErrorContext(ctx, "write book file failed", "book_id", book.ID, "err", err)
		return apperrors.FileWriteFailed("failed to write the book file")
	}

	var err error
	if next.FileHash, err = s.files.Hash(tmp); err != nil {
		slog.ErrorContext(ctx, "update book failed", "book_id", book.ID, "err", err)
		return apperrors.Internal("failed to update book")
	}

	err = s.bookRepo.Update(ctx, &next, func() error { return s.files.Move(tmp, rel) })
	if errors.Is(err, repository.ErrInvalid) {
		return apperrors.InvalidRequest(libraryRules)
	}
	if errors.Is(err, repository.ErrConflict) {
		if dupErr := s.duplicateOf(ctx, next.FileHash); dupErr != nil {
			return dupErr
		}
		return apperrors.Internal("failed to update book")
	}
	if err != nil {
		slog.ErrorContext(ctx, "update book failed", "book_id", book.ID, "err", err)
		return apperrors.Internal("failed to update book")
	}

	if cover != nil {
		if err = s.replaceCover(ctx, book.ID, cover); err != nil {
			return err
		}
	}
	slog.InfoContext(ctx, "book edited", "book_id", book.ID)
	return nil
}

// saved in tmp/ and moved into place, so a failed save keeps the old cover
func (s *BookService) replaceCover(ctx context.Context, id int64, data []byte) error {
	var (
		ext = detectCoverExt(data)
		tmp = s.files.NewTmpPath(ext)
	)

	defer func() {
		if rmErr := s.files.Remove(tmp); rmErr != nil {
			slog.WarnContext(ctx, "cover: cleanup failed", "err", rmErr)
		}
	}()

	if err := s.files.Save(ctx, tmp, bytes.NewReader(data), domain.CoverPolicy); err != nil {
		return err
	}
	if err := s.files.Move(tmp, s.files.CoverPath(id, ext)); err != nil {
		slog.ErrorContext(ctx, "save cover failed", "book_id", id, "err", err)
		return apperrors.Internal("failed to save cover")
	}

	for _, old := range domain.CoverPolicy.AllowedExts {
		if old != ext {
			if rmErr := s.files.Remove(s.files.CoverPath(id, old)); rmErr != nil {
				slog.WarnContext(ctx, "remove old cover failed", "book_id", id, "err", rmErr)
			}
		}
	}
	return nil
}

// the request replaces the metadata of book; the rules of a valid book live in the schema, which rejects the update
func applyUpdate(book *domain.Book, req dto.UpdateBookRequest) error {
	book.Title = strings.TrimSpace(req.Title)
	book.Language = nullIfEmpty(formats.ParseLanguage(req.Language))
	book.Publisher = nullIfEmpty(strings.TrimSpace(req.Publisher))
	book.Description = nullIfEmpty(strings.TrimSpace(req.Description))
	book.PubYear = nil
	if req.PubYear != 0 {
		book.PubYear = &req.PubYear
	}
	book.Tags = cleanNames(req.Tags, book.Tags)
	book.Authors = cleanAuthors(req.Authors, book.Authors)
	if book.Language == nil && strings.TrimSpace(req.Language) != "" {
		return apperrors.InvalidRequest("language must be an ISO 639 code")
	}

	var err error
	book.Identifiers, err = cleanIdentifiers(req.Identifiers)
	return err
}

// an empty row is dropped; an author already on the book stays as stored: renaming, case included, is done on the author page
func cleanAuthors(list []dto.Author, current []domain.Author) []domain.Author {
	var authors []domain.Author

	for _, r := range list {
		a := domain.Author{FirstName: strings.TrimSpace(r.FirstName), MiddleName: strings.TrimSpace(r.MiddleName), LastName: strings.TrimSpace(r.LastName)}
		same := func(b domain.Author) bool { return strings.EqualFold(a.SortName(), b.SortName()) }
		if i := slices.IndexFunc(current, same); i >= 0 {
			a = current[i]
		}
		if a.Name() != "" && !slices.ContainsFunc(authors, same) {
			authors = append(authors, a)
		}
	}
	return authors
}

// collapses spaces and drops empty and repeated names; a tag already on the book keeps its spelling
func cleanNames(list, current []string) []string {
	var names []string

	for _, name := range list {
		name = strings.Join(strings.Fields(name), " ")
		same := func(n string) bool { return strings.EqualFold(n, name) }
		if i := slices.IndexFunc(current, same); i >= 0 {
			name = current[i]
		}
		if name != "" && !slices.ContainsFunc(names, same) {
			names = append(names, name)
		}
	}
	return names
}

// normalized like the ones read from a file and sorted like the database returns them
func cleanIdentifiers(list []dto.Identifier) ([]domain.Identifier, error) {
	var (
		ids []domain.Identifier
		err error
	)

	for i := 0; err == nil && i < len(list); i++ {
		parsed := formats.ParseIdentifiers(list[i].Type, list[i].Value)
		if len(parsed) == 0 {
			err = apperrors.InvalidRequest("invalid identifier of type " + list[i].Type)
		}
		for _, id := range parsed {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
	}
	slices.SortFunc(ids, func(a, b domain.Identifier) int {
		return cmp.Or(strings.Compare(a.Type, b.Type), strings.Compare(a.Value, b.Value))
	})
	return ids, err
}

func readCover(src io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(src, domain.CoverPolicy.MaxSizeBytes+1))
	switch {
	case err != nil:
		err = apperrors.InvalidRequest("failed to read the cover")
	case int64(len(data)) > domain.CoverPolicy.MaxSizeBytes:
		err = apperrors.FileTooLarge(fmt.Sprintf("cover must not exceed %d MB", domain.CoverPolicy.MaxSizeBytes/domain.MB))
	case detectCoverExt(data) == "":
		err = apperrors.UnsupportedFormat("cover must be a jpg or png image")
	}
	return data, err
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

func metadataFromBook(b *domain.Book, cover []byte) *formats.Metadata {
	meta := &formats.Metadata{
		Title:       b.Title,
		Language:    textOf(b.Language),
		Publisher:   textOf(b.Publisher),
		Description: textOf(b.Description),
		Authors:     b.Authors,
		Tags:        b.Tags,
		Identifiers: b.Identifiers,
		Cover:       cover,
	}
	if b.PubYear != nil {
		meta.Year = *b.PubYear
	}
	return meta
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
		Authors:     toAuthors(b.Authors),
		Tags:        emptyIfNil(b.Tags),
		Identifiers: toIdentifiers(b.Identifiers),
		HasCover:    hasCover,
	}
}

func toAuthors(authors []domain.Author) []dto.Author {
	resp := make([]dto.Author, len(authors))
	for i, a := range authors {
		resp[i] = dto.Author{ID: a.ID, FirstName: a.FirstName, MiddleName: a.MiddleName, LastName: a.LastName, Name: a.Name()}
	}
	return resp
}

func toIdentifiers(ids []domain.Identifier) []dto.Identifier {
	resp := make([]dto.Identifier, len(ids))
	for i, id := range ids {
		resp[i] = dto.Identifier{Type: id.Type, Value: id.Value}
	}
	return resp
}

func downloadName(b *domain.Book) string {
	name := b.Title
	names := make([]string, len(b.Authors))
	for i, a := range b.Authors {
		names[i] = a.Name()
	}
	if len(names) > 0 {
		name += " - " + strings.Join(names, ", ")
	}
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(invalidFileNameChars, r) || unicode.IsControl(r) {
			r = '_'
		}
		return r
	}, name)
	return name + "." + b.Format
}

func textOf(s *string) string {
	var text string

	if s != nil {
		text = *s
	}
	return text
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

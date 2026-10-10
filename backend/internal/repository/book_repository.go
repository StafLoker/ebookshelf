package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"ebookshelf/internal/domain"
)

type BookRepository interface {
	Create(ctx context.Context, book *domain.Book, beforeCommit func(id int64) error) error
	FindByID(ctx context.Context, id int64) (*domain.Book, error)
	FindByFileHash(ctx context.Context, fileHash string) (*domain.Book, error)
	FindAll(ctx context.Context, userID int64, filter domain.BookFilter, page, pageSize int) (domain.Page[domain.BookSummary], error)
	FindPending(ctx context.Context, uploadedBy int64) ([]domain.BookSummary, error)
	MarkAdded(ctx context.Context, id int64, createdAt string) error
	DeletePending(ctx context.Context, id int64) error
}

type bookRepository struct {
	db *sqlx.DB
}

func NewBookRepository(db *sqlx.DB) BookRepository {
	return &bookRepository{db: db}
}

func (r *bookRepository) Create(ctx context.Context, book *domain.Book, beforeCommit func(id int64) error) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	err = insertBook(ctx, tx, book)
	if err == nil {
		err = linkAuthors(ctx, tx, book)
	}
	if err == nil {
		err = linkTags(ctx, tx, book)
	}
	if err == nil {
		err = linkIdentifiers(ctx, tx, book)
	}
	if err == nil && beforeCommit != nil {
		err = beforeCommit(book.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	return err
}

func insertBook(ctx context.Context, tx *sqlx.Tx, book *domain.Book) error {
	result, err := tx.NamedExecContext(ctx, bookCreate, book)
	if err != nil {
		return mapConflict(err)
	}

	book.ID, err = result.LastInsertId()
	return err
}

func linkAuthors(ctx context.Context, tx *sqlx.Tx, book *domain.Book) error {
	var (
		authorID int64
		err      error
	)

	for i := 0; i < len(book.Authors) && err == nil; i++ {
		if err = tx.GetContext(ctx, &authorID, bookUpsertAuthor, book.Authors[i]); err == nil {
			_, err = tx.ExecContext(ctx, bookCreateAuthorLink, book.ID, authorID, i)
		}
	}
	return err
}

func linkTags(ctx context.Context, tx *sqlx.Tx, book *domain.Book) error {
	var (
		tagID int64
		err   error
	)

	for i := 0; i < len(book.Tags) && err == nil; i++ {
		if err = tx.GetContext(ctx, &tagID, bookUpsertTag, book.Tags[i]); err == nil {
			_, err = tx.ExecContext(ctx, bookCreateTagLink, book.ID, tagID)
		}
	}
	return err
}

func linkIdentifiers(ctx context.Context, tx *sqlx.Tx, book *domain.Book) error {
	var err error

	for i := 0; i < len(book.Identifiers) && err == nil; i++ {
		_, err = tx.ExecContext(ctx, bookCreateIdentifier, book.ID, book.Identifiers[i].Type, book.Identifiers[i].Value)
	}
	return err
}

func (r *bookRepository) FindByID(ctx context.Context, id int64) (*domain.Book, error) {
	var book domain.Book

	err := r.db.GetContext(ctx, &book, bookFindByID, id)
	if err == nil {
		err = r.db.SelectContext(ctx, &book.Authors, bookFindAuthors, id)
	}
	if err == nil {
		err = r.db.SelectContext(ctx, &book.Tags, bookFindTags, id)
	}
	if err == nil {
		err = r.db.SelectContext(ctx, &book.Identifiers, bookFindIdentifiers, id)
	}
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func (r *bookRepository) FindByFileHash(ctx context.Context, fileHash string) (*domain.Book, error) {
	var book domain.Book
	if err := r.db.GetContext(ctx, &book, bookFindByFileHash, fileHash); err != nil {
		return nil, err
	}
	return &book, nil
}

func (r *bookRepository) FindAll(ctx context.Context, userID int64, filter domain.BookFilter, page, pageSize int) (domain.Page[domain.BookSummary], error) {
	var (
		total int
		items = []domain.BookSummary{}
		args  = []any{userID, filter.Query, filter.AuthorID, filter.TagID, filter.Language, filter.Read}
	)

	err := r.db.GetContext(ctx, &total, bookCount, args...)
	if err == nil {
		err = r.db.SelectContext(ctx, &items, bookFindAll, append(args, filter.Sort, pageSize, (page-1)*pageSize)...)
	}
	if err == nil && len(items) > 0 {
		err = r.attachAuthors(ctx, items)
	}
	return domain.NewPage(items, page, pageSize, total), err
}

// uploadedBy 0 lists every pending book
func (r *bookRepository) FindPending(ctx context.Context, uploadedBy int64) ([]domain.BookSummary, error) {
	items := []domain.BookSummary{}

	err := r.db.SelectContext(ctx, &items, bookFindPending, uploadedBy)
	if err == nil && len(items) > 0 {
		err = r.attachAuthors(ctx, items)
	}
	return items, err
}

func (r *bookRepository) MarkAdded(ctx context.Context, id int64, createdAt string) error {
	result, err := r.db.ExecContext(ctx, bookMarkAdded, createdAt, id)
	return requireAffected(result, err)
}

func (r *bookRepository) DeletePending(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, bookDeletePending, id)
	return requireAffected(result, err)
}

type authorRow struct {
	BookID int64  `db:"book_id"`
	Name   string `db:"name"`
}

func (r *bookRepository) attachAuthors(ctx context.Context, items []domain.BookSummary) error {
	var (
		ids     = make([]int64, len(items))
		byBook  = make(map[int64]*domain.BookSummary, len(items))
		authors []authorRow
	)

	for i := range items {
		ids[i] = items[i].ID
		byBook[items[i].ID] = &items[i]
	}
	query, args, err := sqlx.In(bookFindAuthorsByBooks, ids)
	if err == nil {
		err = r.db.SelectContext(ctx, &authors, query, args...)
	}
	for _, a := range authors {
		byBook[a.BookID].Authors = append(byBook[a.BookID].Authors, a.Name)
	}
	return err
}

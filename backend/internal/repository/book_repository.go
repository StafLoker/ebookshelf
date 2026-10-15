package repository

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/jmoiron/sqlx"

	"ebookshelf/internal/domain"
)

type BookRepository interface {
	Create(ctx context.Context, book *domain.Book, beforeCommit func(id int64) error) error
	FindByID(ctx context.Context, id int64) (*domain.Book, error)
	FindByHash(ctx context.Context, hash string) (*domain.Book, error)
	FindAll(ctx context.Context, userID int64, filter domain.BookFilter, page, pageSize int) (domain.Page[domain.BookSummary], error)
	FindPending(ctx context.Context, uploadedBy int64) ([]domain.BookSummary, error)
	Update(ctx context.Context, book *domain.Book, beforeCommit func() error) error
	MarkAdded(ctx context.Context, id int64, createdAt string) error
	Delete(ctx context.Context, id int64, status domain.BookStatus) error
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

	if err = insertBook(ctx, tx, book); err != nil {
		return mapConstraint(err)
	}
	if err = linkAuthors(ctx, tx, book); err != nil {
		return mapConstraint(err)
	}
	if err = linkTags(ctx, tx, book); err != nil {
		return mapConstraint(err)
	}
	if err = linkIdentifiers(ctx, tx, book); err != nil {
		return mapConstraint(err)
	}
	if beforeCommit != nil {
		if err = beforeCommit(book.ID); err != nil {
			return mapConstraint(err)
		}
	}
	return mapConstraint(tx.Commit())
}

func (r *bookRepository) FindByID(ctx context.Context, id int64) (*domain.Book, error) {
	var book domain.Book

	if err := r.db.GetContext(ctx, &book, bookFindByID, id); err != nil {
		return nil, err
	}
	if err := r.db.SelectContext(ctx, &book.Authors, bookFindAuthors, id); err != nil {
		return nil, err
	}
	if err := r.db.SelectContext(ctx, &book.Tags, bookFindTags, id); err != nil {
		return nil, err
	}
	if err := r.db.SelectContext(ctx, &book.Identifiers, bookFindIdentifiers, id); err != nil {
		return nil, err
	}
	return &book, nil
}

func (r *bookRepository) FindByHash(ctx context.Context, hash string) (*domain.Book, error) {
	var book domain.Book
	if err := r.db.GetContext(ctx, &book, bookFindByHash, hash); err != nil {
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

	if err := r.db.GetContext(ctx, &total, bookCount, args...); err != nil {
		return domain.Page[domain.BookSummary]{}, err
	}
	if err := r.db.SelectContext(ctx, &items, bookFindAll, append(args, filter.Sort, pageSize, (page-1)*pageSize)...); err != nil {
		return domain.Page[domain.BookSummary]{}, err
	}
	if len(items) > 0 {
		if err := r.attachAuthors(ctx, items); err != nil {
			return domain.Page[domain.BookSummary]{}, err
		}
	}
	return domain.NewPage(items, page, pageSize, total), nil
}

// uploadedBy 0 lists every pending book
func (r *bookRepository) FindPending(ctx context.Context, uploadedBy int64) ([]domain.BookSummary, error) {
	items := []domain.BookSummary{}

	if err := r.db.SelectContext(ctx, &items, bookFindPending, uploadedBy); err != nil {
		return nil, err
	}
	if len(items) > 0 {
		if err := r.attachAuthors(ctx, items); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// links are pruned instead of recreated: dropping the last link deletes the author or tag, and with it its id
func (r *bookRepository) Update(ctx context.Context, book *domain.Book, beforeCommit func() error) error {
	var (
		result    sql.Result
		authorIDs []byte
		tagNames  []byte
	)

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return mapConstraint(err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err = tx.NamedExecContext(ctx, bookUpdate, book)
	if err = requireAffected(result, err); err != nil {
		return mapConstraint(err)
	}
	if err = linkAuthors(ctx, tx, book); err != nil {
		return mapConstraint(err)
	}

	ids := make([]int64, len(book.Authors))
	for i, a := range book.Authors {
		ids[i] = a.ID
	}
	if authorIDs, err = json.Marshal(ids); err != nil {
		return mapConstraint(err)
	}
	if _, err = tx.ExecContext(ctx, bookDeleteAuthorLinksExcept, book.ID, string(authorIDs)); err != nil {
		return mapConstraint(err)
	}
	if err = linkTags(ctx, tx, book); err != nil {
		return mapConstraint(err)
	}
	if tagNames, err = json.Marshal(append([]string{}, book.Tags...)); err != nil {
		return mapConstraint(err)
	}
	if _, err = tx.ExecContext(ctx, bookDeleteTagLinksExcept, book.ID, string(tagNames)); err != nil {
		return mapConstraint(err)
	}
	if _, err = tx.ExecContext(ctx, bookDeleteIdentifiers, book.ID); err != nil {
		return mapConstraint(err)
	}
	if err = linkIdentifiers(ctx, tx, book); err != nil {
		return mapConstraint(err)
	}
	if beforeCommit != nil {
		if err = beforeCommit(); err != nil {
			return mapConstraint(err)
		}
	}
	return mapConstraint(tx.Commit())
}

func (r *bookRepository) MarkAdded(ctx context.Context, id int64, createdAt string) error {
	result, err := r.db.ExecContext(ctx, bookMarkAdded, createdAt, id)
	return requireAffected(result, mapConstraint(err))
}

// the status guards against a book that changed status since the caller read it
func (r *bookRepository) Delete(ctx context.Context, id int64, status domain.BookStatus) error {
	result, err := r.db.ExecContext(ctx, bookDelete, id, status)
	return requireAffected(result, err)
}

func insertBook(ctx context.Context, tx *sqlx.Tx, book *domain.Book) error {
	result, err := tx.NamedExecContext(ctx, bookCreate, book)
	if err != nil {
		return err
	}

	book.ID, err = result.LastInsertId()
	return err
}

// fills in the id of each author, creating the ones that do not exist yet
func linkAuthors(ctx context.Context, tx *sqlx.Tx, book *domain.Book) error {
	var err error

	for i := 0; i < len(book.Authors) && err == nil; i++ {
		a := &book.Authors[i]
		if err = tx.GetContext(ctx, &a.ID, bookUpsertAuthor, a.FirstName, a.MiddleName, a.LastName); err == nil {
			_, err = tx.ExecContext(ctx, bookCreateAuthorLink, book.ID, a.ID, i)
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

type authorRow struct {
	BookID int64 `db:"book_id"`
	domain.Author
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
	if err != nil {
		return err
	}
	if err = r.db.SelectContext(ctx, &authors, query, args...); err != nil {
		return err
	}
	for _, a := range authors {
		byBook[a.BookID].Authors = append(byBook[a.BookID].Authors, a.Author)
	}
	return nil
}

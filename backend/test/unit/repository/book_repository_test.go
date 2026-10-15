package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/repository"
	"ebookshelf/test/testutil"
)

// authors are given by last name only
func newBook(title, hash string, lastNames, tags []string) *domain.Book {
	lang := "en"
	book := &domain.Book{Title: title, Language: &lang, Format: "epub", FileHash: hash, SourceHash: hash, Status: domain.BookAdded, CreatedAt: "2026-01-01T00:00:00Z", Tags: tags}
	for _, name := range lastNames {
		book.Authors = append(book.Authors, domain.Author{LastName: name})
	}
	return book
}

func lastNames(authors []domain.Author) []string {
	names := make([]string, len(authors))
	for i, a := range authors {
		names[i] = a.LastName
	}
	return names
}

func TestBookRepository_Create_SharesAuthorsIgnoringCase(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)

	require.NoError(t, bookRepo.Create(ctx, newBook("One", "h1", []string{"Ёлкин", "Bob"}, []string{"Fiction"}), nil))
	require.NoError(t, bookRepo.Create(ctx, newBook("Two", "h2", []string{"ЕЛКИН"}, []string{"fiction"}), nil))
	found, err := bookRepo.FindByID(ctx, 1)

	require.NoError(t, err)
	assert.Equal(t, []string{"Ёлкин", "Bob"}, lastNames(found.Authors))
	assert.Equal(t, 2, testutil.Count(t, db, "SELECT COUNT(*) FROM authors"))
	assert.Equal(t, 1, testutil.Count(t, db, "SELECT COUNT(*) FROM tags"))
}

func TestBookRepository_Create_DuplicateHashRollsBack(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)
	require.NoError(t, bookRepo.Create(ctx, newBook("One", "h1", []string{"Ann"}, nil), nil))

	err := bookRepo.Create(ctx, newBook("Two", "h1", []string{"New Author"}, nil), nil)

	assert.ErrorIs(t, err, repository.ErrConflict)
	assert.Equal(t, 1, testutil.Count(t, db, "SELECT COUNT(*) FROM authors"))
}

func TestBookRepository_Create_CallbackErrorRollsBack(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)

	err := bookRepo.Create(ctx, newBook("One", "h1", []string{"Ann"}, []string{"x"}), func(int64) error { return errors.New("move failed") })

	assert.Error(t, err)
	assert.Equal(t, 0, testutil.Count(t, db, "SELECT COUNT(*) FROM books"))
	assert.Equal(t, 0, testutil.Count(t, db, "SELECT COUNT(*) FROM authors"))
}

func TestBookRepository_FindAll_SearchAndReadFilter(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)
	readRepo := repository.NewReadStatusRepository(db)
	user := &domain.User{Username: "u", PasswordHash: "h", Role: domain.RoleUser}
	require.NoError(t, repository.NewUserRepository(db).Create(ctx, user))
	userID := user.ID
	require.NoError(t, bookRepo.Create(ctx, newBook("Борис Ельцин", "h1", []string{"Коржаков"}, nil), nil))
	require.NoError(t, bookRepo.Create(ctx, newBook("Other", "h2", []string{"Ann"}, []string{"history"}), nil))
	require.NoError(t, readRepo.Create(ctx, userID, 2))

	byTitle, err := bookRepo.FindAll(ctx, userID, domain.BookFilter{Status: domain.BookAdded, Query: "ЕЛЬЦИН"}, 1, 24)
	require.NoError(t, err)
	byTag, err := bookRepo.FindAll(ctx, userID, domain.BookFilter{Status: domain.BookAdded, Query: "HIST"}, 1, 24)
	require.NoError(t, err)
	unread, err := bookRepo.FindAll(ctx, userID, domain.BookFilter{Status: domain.BookAdded, Read: domain.ReadUnread}, 1, 24)
	require.NoError(t, err)

	assert.Equal(t, 1, byTitle.Total)
	assert.Equal(t, []string{"Коржаков"}, lastNames(byTitle.Items[0].Authors))
	assert.Equal(t, "Other", byTag.Items[0].Title)
	assert.True(t, byTag.Items[0].Read)
	assert.Equal(t, "Борис Ельцин", unread.Items[0].Title)
	assert.Equal(t, 1, unread.Total)
}

func TestBookRepository_FindAll_TotalPages(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)
	for _, hash := range []string{"h1", "h2", "h3"} {
		require.NoError(t, bookRepo.Create(ctx, newBook(hash, hash, nil, nil), nil))
	}

	second, err := bookRepo.FindAll(ctx, 1, domain.BookFilter{Status: domain.BookAdded}, 2, 2)
	require.NoError(t, err)
	empty, err := bookRepo.FindAll(ctx, 1, domain.BookFilter{Status: domain.BookAdded, Query: "nothing"}, 1, 2)
	require.NoError(t, err)

	assert.Equal(t, 3, second.Total)
	assert.Equal(t, 2, second.TotalPages)
	assert.Len(t, second.Items, 1)
	assert.Equal(t, 0, empty.TotalPages)
	assert.NotNil(t, empty.Items)
}

func TestBookRepository_Identifiers_StoredAndSearchable(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)
	book := newBook("One", "h1", []string{"Ann"}, nil)
	book.Identifiers = []domain.Identifier{{Type: "isbn", Value: "9785389099401"}, {Type: "litres", Value: "42"}, {Type: "isbn", Value: "9785389099401"}}
	require.NoError(t, bookRepo.Create(ctx, book, nil))

	found, err := bookRepo.FindByID(ctx, book.ID)
	require.NoError(t, err)
	byIdentifier, err := bookRepo.FindAll(ctx, 0, domain.BookFilter{Status: domain.BookAdded, Query: "5389099"}, 1, 24)
	require.NoError(t, err)
	byHyphenated, err := bookRepo.FindAll(ctx, 0, domain.BookFilter{Status: domain.BookAdded, Query: "978-5-389-09940-1"}, 1, 24)
	require.NoError(t, err)
	hyphensOnly, err := bookRepo.FindAll(ctx, 0, domain.BookFilter{Status: domain.BookAdded, Query: "-"}, 1, 24)
	require.NoError(t, err)

	assert.Equal(t, []domain.Identifier{{Type: "isbn", Value: "9785389099401"}, {Type: "litres", Value: "42"}}, found.Identifiers)
	assert.Equal(t, 1, byIdentifier.Total)
	assert.Equal(t, 1, byHyphenated.Total)
	assert.Equal(t, 0, hyphensOnly.Total)
}

func TestBookRepository_Update_KeepsAuthorIDsAndDropsOrphans(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)
	book := newBook("One", "h1", []string{"Ёлкин", "Bob"}, []string{"a", "b"})
	book.Identifiers = []domain.Identifier{{Type: "isbn", Value: "1234567890"}}
	require.NoError(t, bookRepo.Create(ctx, book, nil))
	authorID := testutil.Count(t, db, "SELECT id FROM authors WHERE last_name = 'Ёлкин'")

	book.Title, book.FileHash = "New", "h2"
	book.Authors = []domain.Author{{LastName: "Carl"}, {LastName: "ЕЛКИН"}}
	book.Tags, book.Identifiers = []string{"B"}, nil
	require.NoError(t, bookRepo.Update(ctx, book, nil))
	found, err := bookRepo.FindByID(ctx, book.ID)

	require.NoError(t, err)
	assert.Equal(t, "New", found.Title)
	assert.Equal(t, "h2", found.FileHash)
	assert.Equal(t, "h1", found.SourceHash)
	assert.Equal(t, []string{"Carl", "Ёлкин"}, lastNames(found.Authors))
	assert.Equal(t, []string{"b"}, found.Tags)
	assert.Empty(t, found.Identifiers)
	assert.Equal(t, int64(authorID), found.Authors[1].ID)
	assert.Equal(t, 2, testutil.Count(t, db, "SELECT COUNT(*) FROM authors"))
	assert.Equal(t, 1, testutil.Count(t, db, "SELECT COUNT(*) FROM tags"))
}

func TestBookRepository_SchemaRules_LibraryContract(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)
	added := newBook("Added", "h1", []string{"Ann"}, nil)
	require.NoError(t, bookRepo.Create(ctx, added, nil))
	noAuthor := newBook("Pending", "h2", nil, nil)
	noAuthor.Status = domain.BookPending
	require.NoError(t, bookRepo.Create(ctx, noAuthor, nil))

	commitErr := bookRepo.MarkAdded(ctx, noAuthor.ID, "2026-01-01T00:00:00Z")
	added.Authors = nil
	lastAuthorErr := bookRepo.Update(ctx, added, nil)
	added.Authors = []domain.Author{{FirstName: "Mary Ann", LastName: "Smith"}}
	twoWordFirstErr := bookRepo.Update(ctx, added, nil)

	assert.ErrorIs(t, commitErr, repository.ErrInvalid)
	assert.ErrorIs(t, lastAuthorErr, repository.ErrInvalid)
	assert.ErrorIs(t, twoWordFirstErr, repository.ErrInvalid)
}

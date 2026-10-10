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

func newBook(title, hash string, authors, tags []string) *domain.Book {
	return &domain.Book{Title: title, Format: "epub", FileHash: hash, SourceHash: hash, Status: domain.BookAdded, CreatedAt: "2026-01-01T00:00:00Z", Authors: authors, Tags: tags}
}

func TestBookRepository_Create_SharesAuthorsIgnoringCase(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)

	require.NoError(t, bookRepo.Create(ctx, newBook("One", "h1", []string{"Ёлкин", "Bob"}, []string{"Fiction"}), nil))
	require.NoError(t, bookRepo.Create(ctx, newBook("Two", "h2", []string{"ЕЛКИН"}, []string{"fiction"}), nil))
	found, err := bookRepo.FindByID(ctx, 1)

	require.NoError(t, err)
	assert.Equal(t, []string{"Ёлкин", "Bob"}, found.Authors)
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

	byTitle, err := bookRepo.FindAll(ctx, userID, domain.BookFilter{Query: "ЕЛЬЦИН"}, 1, 24)
	require.NoError(t, err)
	byTag, err := bookRepo.FindAll(ctx, userID, domain.BookFilter{Query: "HIST"}, 1, 24)
	require.NoError(t, err)
	unread, err := bookRepo.FindAll(ctx, userID, domain.BookFilter{Read: domain.ReadUnread}, 1, 24)
	require.NoError(t, err)

	assert.Equal(t, 1, byTitle.Total)
	assert.Equal(t, []string{"Коржаков"}, byTitle.Items[0].Authors)
	assert.Equal(t, "Other", byTag.Items[0].Title)
	assert.True(t, byTag.Items[0].Read)
	assert.Equal(t, "Борис Ельцин", unread.Items[0].Title)
	assert.Equal(t, 1, unread.Total)
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

func TestBookRepository_FindAll_TotalPages(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)
	for _, hash := range []string{"h1", "h2", "h3"} {
		require.NoError(t, bookRepo.Create(ctx, newBook(hash, hash, nil, nil), nil))
	}

	second, err := bookRepo.FindAll(ctx, 1, domain.BookFilter{}, 2, 2)
	require.NoError(t, err)
	empty, err := bookRepo.FindAll(ctx, 1, domain.BookFilter{Query: "nothing"}, 1, 2)
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
	byIdentifier, err := bookRepo.FindAll(ctx, 0, domain.BookFilter{Query: "5389099"}, 1, 24)
	require.NoError(t, err)
	byHyphenated, err := bookRepo.FindAll(ctx, 0, domain.BookFilter{Query: "978-5-389-09940-1"}, 1, 24)
	require.NoError(t, err)
	hyphensOnly, err := bookRepo.FindAll(ctx, 0, domain.BookFilter{Query: "-"}, 1, 24)
	require.NoError(t, err)

	assert.Equal(t, []domain.Identifier{{Type: "isbn", Value: "9785389099401"}, {Type: "litres", Value: "42"}}, found.Identifiers)
	assert.Equal(t, 1, byIdentifier.Total)
	assert.Equal(t, 1, byHyphenated.Total)
	assert.Equal(t, 0, hyphensOnly.Total)
}

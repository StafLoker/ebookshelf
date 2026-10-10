package database_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/database"
	"ebookshelf/test/testutil"
)

func TestDatabase_Open_MigratesAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	first, err := database.Open(context.Background(), path, testutil.DiscardLog)
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second := testutil.OpenDB(t, path)

	assert.Equal(t, 1, testutil.UserVersion(t, second))
}

func TestDatabase_Open_SchemaNewer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	db := testutil.OpenDB(t, path)
	db.MustExec("PRAGMA user_version = 99")
	require.NoError(t, db.Close())

	_, err := database.Open(context.Background(), path, testutil.DiscardLog)

	assert.ErrorIs(t, err, database.ErrSchemaNewer)
}

func TestDatabase_Casefold_FindsYolka(t *testing.T) {
	db := testutil.OpenTempDB(t)
	testutil.InsertBook(t, db, "Ёлка", "h1")
	testutil.InsertBook(t, db, "Война и мир", "h2")
	var titles []string

	err := db.Select(&titles, "SELECT title FROM books WHERE casefold(title) LIKE '%' || casefold(?) || '%'", "елка")

	require.NoError(t, err)
	assert.Equal(t, []string{"Ёлка"}, titles)
}

func TestDatabase_Trigger_RemovesOnlyUnsharedAuthor(t *testing.T) {
	db := testutil.OpenTempDB(t)
	book1 := testutil.InsertBook(t, db, "Один", "h1")
	book2 := testutil.InsertBook(t, db, "Два", "h2")
	db.MustExec("INSERT INTO authors (id, name) VALUES (1, 'Общий'), (2, 'Одиночка')")
	db.MustExec("INSERT INTO book_authors VALUES (?, 1, 0), (?, 2, 1), (?, 1, 0)", book1, book1, book2)

	db.MustExec("DELETE FROM books WHERE id = ?", book1)

	assert.Equal(t, 1, testutil.Count(t, db, "SELECT count(*) FROM authors WHERE id = 1"))
	assert.Equal(t, 0, testutil.Count(t, db, "SELECT count(*) FROM authors WHERE id = 2"))
}

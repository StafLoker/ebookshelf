package testutil

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/database"
)

// DiscardLog is a logger that drops everything.
var DiscardLog = slog.New(slog.DiscardHandler)

// OpenDB opens (and migrates) the database at path, closing it on cleanup.
func OpenDB(t *testing.T, path string) *sqlx.DB {
	t.Helper()

	db, err := database.Open(context.Background(), path, DiscardLog)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// OpenTempDB opens a fresh database in a temp dir.
func OpenTempDB(t *testing.T) *sqlx.DB {
	t.Helper()

	return OpenDB(t, filepath.Join(t.TempDir(), "app.db"))
}

// UserVersion returns PRAGMA user_version.
func UserVersion(t *testing.T, db *sqlx.DB) int {
	t.Helper()

	var v int
	require.NoError(t, db.Get(&v, "PRAGMA user_version"))
	return v
}

// InsertBook inserts a minimal book and returns its id.
func InsertBook(t *testing.T, db *sqlx.DB, title, hash string) int64 {
	t.Helper()

	res, err := db.Exec(`INSERT INTO books (title, format, file_hash, source_hash, created_at)
		VALUES (?, 'epub', ?, ?, '2026-01-01T00:00:00Z')`, title, hash, hash)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

// Count runs a single-integer query.
func Count(t *testing.T, db *sqlx.DB, query string, args ...any) int {
	t.Helper()

	var n int
	require.NoError(t, db.Get(&n, query, args...))
	return n
}

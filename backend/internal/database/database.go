package database

import (
	"context"
	"database/sql/driver"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jmoiron/sqlx"
	"golang.org/x/text/cases"
	"modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var ErrSchemaNewer = errors.New("schema is newer than this application")

func Casefold(s string) string {
	return strings.ReplaceAll(cases.Fold().String(s), "ё", "е")
}

func Open(ctx context.Context, path string, log *slog.Logger) (*sqlx.DB, error) {
	var (
		db  *sqlx.DB
		err error
	)

	// the driver only registers functions process-wide; a second Open gets "already registered", which is fine
	_ = sqlite.RegisterDeterministicScalarFunction("casefold", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if s, ok := args[0].(string); ok {
			return Casefold(s), nil
		}
		return args[0], nil
	})
	_ = sqlite.RegisterCollationUtf8("CASEFOLD", func(a, b string) int {
		return strings.Compare(Casefold(a), Casefold(b))
	})

	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	if db, err = sqlx.Open("sqlite", dsn); err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err = db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err = migrate(ctx, db, log); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func migrate(ctx context.Context, db *sqlx.DB, log *slog.Logger) error {
	var current int

	files, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	if err = db.GetContext(ctx, &current, "PRAGMA user_version"); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current > len(files) {
		return fmt.Errorf("%w: version %d, known %d", ErrSchemaNewer, current, len(files))
	}

	for version := current + 1; version <= len(files) && err == nil; version++ {
		err = applyMigration(ctx, db, files[version-1].Name(), version)
		if err != nil {
			err = fmt.Errorf("migration %04d: %w", version, err)
		} else {
			log.InfoContext(ctx, "migration applied", "version", version)
		}
	}
	return err
}

func applyMigration(ctx context.Context, db *sqlx.DB, name string, version int) error {
	body, err := migrationsFS.ReadFile("migrations/" + name)
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err = tx.ExecContext(ctx, string(body)); err != nil {
		return err
	}
	// PRAGMA takes no bound parameters
	if _, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}

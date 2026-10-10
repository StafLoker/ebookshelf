package repository

import (
	"database/sql"
	"errors"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

//go:generate go run ../../scripts/gen_sql_queries.go

var ErrConflict = errors.New("conflict")

// turns a UNIQUE violation into ErrConflict; any other error passes through
func mapConflict(err error) error {
	var sqliteErr *sqlite.Error

	if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return ErrConflict
	}
	return err
}

// sql.ErrNoRows when the statement changed nothing, so a missing row is reported like a failed lookup
func requireAffected(result sql.Result, err error) error {
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

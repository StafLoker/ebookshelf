package repository

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type ReadStatusRepository interface {
	Create(ctx context.Context, userID, bookID int64) error
	Exists(ctx context.Context, userID, bookID int64) (bool, error)
	Delete(ctx context.Context, userID, bookID int64) error
}

type readStatusRepository struct {
	db *sqlx.DB
}

func NewReadStatusRepository(db *sqlx.DB) ReadStatusRepository {
	return &readStatusRepository{db: db}
}

func (r *readStatusRepository) Create(ctx context.Context, userID, bookID int64) error {
	_, err := r.db.ExecContext(ctx, readStatusCreate, userID, bookID)
	return err
}

func (r *readStatusRepository) Exists(ctx context.Context, userID, bookID int64) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, readStatusExists, userID, bookID)
	return exists, err
}

func (r *readStatusRepository) Delete(ctx context.Context, userID, bookID int64) error {
	_, err := r.db.ExecContext(ctx, readStatusDelete, userID, bookID)
	return err
}

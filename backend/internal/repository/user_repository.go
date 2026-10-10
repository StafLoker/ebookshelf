package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"ebookshelf/internal/domain"
)

//go:generate go run ../../scripts/gen_sql_queries.go

var ErrConflict = errors.New("conflict")

type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	FindByID(ctx context.Context, id int64) (*domain.User, error)
	FindByUsername(ctx context.Context, username string) (*domain.User, error)
	FindByRole(ctx context.Context, role domain.Role) (*domain.User, error)
	FindAll(ctx context.Context, page, pageSize int) (domain.Page[domain.User], error)
	Update(ctx context.Context, user *domain.User) error
	ChangePassword(ctx context.Context, id int64, passwordHash string) error
	Delete(ctx context.Context, id int64) error
}

type userRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) UserRepository {
	return &userRepository{db: db}
}

func translate(err error) error {
	var sqliteErr *sqlite.Error

	if errors.As(err, &sqliteErr) && sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
		return ErrConflict
	}
	return err
}

func (r *userRepository) Create(ctx context.Context, user *domain.User) error {
	result, err := r.db.NamedExecContext(ctx, userCreate, user)
	if err != nil {
		return translate(err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	user.ID = id
	user.SessionVersion = 1
	return nil
}

func (r *userRepository) FindByID(ctx context.Context, id int64) (*domain.User, error) {
	var user domain.User
	if err := r.db.GetContext(ctx, &user, userFindByID, id); err != nil {
		return nil, translate(err)
	}
	return &user, nil
}

func (r *userRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	if err := r.db.GetContext(ctx, &user, userFindByUsername, username); err != nil {
		return nil, translate(err)
	}
	return &user, nil
}

func (r *userRepository) FindByRole(ctx context.Context, role domain.Role) (*domain.User, error) {
	var user domain.User
	if err := r.db.GetContext(ctx, &user, userFindByRole, role); err != nil {
		return nil, translate(err)
	}
	return &user, nil
}

func (r *userRepository) FindAll(ctx context.Context, page, pageSize int) (domain.Page[domain.User], error) {
	result := domain.Page[domain.User]{Items: []domain.User{}, Page: page, PageSize: pageSize}

	if err := r.db.GetContext(ctx, &result.Total, userCount); err != nil {
		return result, err
	}
	err := r.db.SelectContext(ctx, &result.Items, userFindAll, pageSize, (page-1)*pageSize)
	return result, err
}

func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	result, err := r.db.ExecContext(ctx, userUpdate, user.Role, user.CanUpload, user.CanEdit, user.CanDelete, user.ID)
	return affectedOne(result, err)
}

func (r *userRepository) ChangePassword(ctx context.Context, id int64, passwordHash string) error {
	result, err := r.db.ExecContext(ctx, userChangePassword, passwordHash, id)
	return affectedOne(result, err)
}

func (r *userRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, userDelete, id)
	return affectedOne(result, err)
}

func affectedOne(result sql.Result, err error) error {
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

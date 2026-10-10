package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"ebookshelf/internal/domain"
)

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

func (r *userRepository) Create(ctx context.Context, user *domain.User) error {
	result, err := r.db.NamedExecContext(ctx, userCreate, user)
	if err != nil {
		return mapConflict(err)
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
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	if err := r.db.GetContext(ctx, &user, userFindByUsername, username); err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindByRole(ctx context.Context, role domain.Role) (*domain.User, error) {
	var user domain.User
	if err := r.db.GetContext(ctx, &user, userFindByRole, role); err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) FindAll(ctx context.Context, page, pageSize int) (domain.Page[domain.User], error) {
	var (
		total int
		items = []domain.User{}
	)

	err := r.db.GetContext(ctx, &total, userCount)
	if err == nil {
		err = r.db.SelectContext(ctx, &items, userFindAll, pageSize, (page-1)*pageSize)
	}
	return domain.NewPage(items, page, pageSize, total), err
}

func (r *userRepository) Update(ctx context.Context, user *domain.User) error {
	result, err := r.db.ExecContext(ctx, userUpdate, user.Role, user.CanUpload, user.CanEdit, user.CanDelete, user.ID)
	return requireAffected(result, err)
}

func (r *userRepository) ChangePassword(ctx context.Context, id int64, passwordHash string) error {
	result, err := r.db.ExecContext(ctx, userChangePassword, passwordHash, id)
	return requireAffected(result, err)
}

func (r *userRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, userDelete, id)
	return requireAffected(result, err)
}

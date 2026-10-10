package repository

import (
	"context"

	"github.com/jmoiron/sqlx"

	"ebookshelf/internal/domain"
)

type PasswordResetTokenRepository interface {
	Create(ctx context.Context, token *domain.PasswordResetToken) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error)
	DeleteByTokenHash(ctx context.Context, tokenHash string) error
}

type passwordResetTokenRepository struct {
	db *sqlx.DB
}

func NewPasswordResetTokenRepository(db *sqlx.DB) PasswordResetTokenRepository {
	return &passwordResetTokenRepository{db: db}
}

func (r *passwordResetTokenRepository) Create(ctx context.Context, token *domain.PasswordResetToken) error {
	_, err := r.db.ExecContext(ctx, passwordResetTokenCreate, token.UserID, token.TokenHash, token.ExpiresAt)
	return err
}

func (r *passwordResetTokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*domain.PasswordResetToken, error) {
	var token domain.PasswordResetToken
	if err := r.db.GetContext(ctx, &token, passwordResetTokenFindByTokenHash, tokenHash); err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *passwordResetTokenRepository) DeleteByTokenHash(ctx context.Context, tokenHash string) error {
	result, err := r.db.ExecContext(ctx, passwordResetTokenDeleteByTokenHash, tokenHash)
	return requireAffected(result, err)
}

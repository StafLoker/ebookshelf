package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/repository"
	"ebookshelf/internal/security"
)

const (
	password    = "Correct-Horse-9"
	newPassword = "Another-Pass-42!"
)

var (
	ctx = context.Background()
	key = []byte("0123456789abcdef0123456789abcdef")
)

func seedUser(t *testing.T, repo repository.UserRepository, username string, role domain.Role) *domain.User {
	t.Helper()

	hash, err := security.HashPassword(password)
	require.NoError(t, err)
	user := &domain.User{Username: username, PasswordHash: hash, Role: role}
	require.NoError(t, repo.Create(ctx, user))
	return user
}

func seedToken(t *testing.T, repo repository.PasswordResetTokenRepository, userID int64, expiresAt string) string {
	t.Helper()

	plain, hash, err := security.NewToken()
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, &domain.PasswordResetToken{UserID: userID, TokenHash: hash, ExpiresAt: expiresAt}))
	return plain
}

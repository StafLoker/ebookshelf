package repository_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/repository"
	"ebookshelf/test/testutil"
)

func TestUserRepository_Create_FindByUsername(t *testing.T) {
	ctx := context.Background()
	userRepo := repository.NewUserRepository(testutil.OpenTempDB(t))
	user := &domain.User{Username: "Alice", PasswordHash: "h", Role: domain.RoleUser, CanAdd: true}
	require.NoError(t, userRepo.Create(ctx, user))

	found, err := userRepo.FindByUsername(ctx, "ALICE")

	require.NoError(t, err)
	assert.Equal(t, *user, *found)
}

func TestUserRepository_Create_DuplicateUsername(t *testing.T) {
	ctx := context.Background()
	userRepo := repository.NewUserRepository(testutil.OpenTempDB(t))
	require.NoError(t, userRepo.Create(ctx, &domain.User{Username: "alice", PasswordHash: "h", Role: domain.RoleUser}))

	err := userRepo.Create(ctx, &domain.User{Username: "ALICE", PasswordHash: "h", Role: domain.RoleUser})

	assert.ErrorIs(t, err, repository.ErrConflict)
}

func TestPasswordResetTokenRepository_DeleteByTokenHash_OnlyOnce(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewPasswordResetTokenRepository(db)
	user := &domain.User{Username: "alice", PasswordHash: "old", Role: domain.RoleUser}
	require.NoError(t, userRepo.Create(ctx, user))
	require.NoError(t, tokenRepo.Create(ctx, &domain.PasswordResetToken{UserID: user.ID, TokenHash: "t1", ExpiresAt: "2999-01-01T00:00:00Z"}))

	errFirst := tokenRepo.DeleteByTokenHash(ctx, "t1")
	errSecond := tokenRepo.DeleteByTokenHash(ctx, "t1")

	require.NoError(t, errFirst)
	assert.ErrorIs(t, errSecond, sql.ErrNoRows)
}

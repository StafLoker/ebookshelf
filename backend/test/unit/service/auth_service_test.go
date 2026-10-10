package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/domain"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/repository"
	"ebookshelf/internal/service"
	"ebookshelf/test/testutil"
)

func TestAuthService_Login_OK(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, repository.NewPasswordResetTokenRepository(db), key)
	seedUser(t, userRepo, "alice", domain.RoleUser)

	login, err := svc.Login(ctx, "alice", password)

	require.NoError(t, err)
	assert.NotEmpty(t, login.Cookie)
	assert.True(t, login.ExpiresAt.After(time.Now()))
	assert.Equal(t, "alice", login.Username)
}

func TestAuthService_Login_InvalidCredentials(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, repository.NewPasswordResetTokenRepository(db), key)
	seedUser(t, userRepo, "alice", domain.RoleUser)

	_, errWrong := svc.Login(ctx, "alice", "wrong")
	_, errUnknown := svc.Login(ctx, "nobody", password)

	assert.ErrorIs(t, errWrong, apperrors.ErrInvalidCredentials)
	assert.Equal(t, errWrong, errUnknown)
}

func TestAuthService_Authenticate_StaleVersion(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewPasswordResetTokenRepository(db)
	svc := service.NewAuthService(userRepo, tokenRepo, key)
	alice := seedUser(t, userRepo, "alice", domain.RoleUser)
	login, err := svc.Login(ctx, "alice", password)
	require.NoError(t, err)

	user, errOK := svc.Authenticate(ctx, login.Cookie)
	require.NoError(t, svc.ResetPassword(ctx, seedToken(t, tokenRepo, alice.ID, "2999-01-01T00:00:00Z"), newPassword))
	_, errStale := svc.Authenticate(ctx, login.Cookie)

	require.NoError(t, errOK)
	assert.Equal(t, alice.ID, user.ID)
	assert.ErrorIs(t, errStale, apperrors.ErrUnauthenticated)
}

func TestAuthService_ResetPassword_OKThenReused(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewPasswordResetTokenRepository(db)
	svc := service.NewAuthService(userRepo, tokenRepo, key)
	alice := seedUser(t, userRepo, "alice", domain.RoleUser)
	token := seedToken(t, tokenRepo, alice.ID, "2999-01-01T00:00:00Z")

	errFirst := svc.ResetPassword(ctx, token, newPassword)
	errSecond := svc.ResetPassword(ctx, token, newPassword)
	_, errLogin := svc.Login(ctx, "alice", newPassword)

	require.NoError(t, errFirst)
	assert.ErrorIs(t, errSecond, apperrors.ErrInvalidToken)
	assert.NoError(t, errLogin)
}

func TestAuthService_ResetPassword_WeakKeepsToken(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewPasswordResetTokenRepository(db)
	svc := service.NewAuthService(userRepo, tokenRepo, key)
	alice := seedUser(t, userRepo, "alice", domain.RoleUser)
	token := seedToken(t, tokenRepo, alice.ID, "2999-01-01T00:00:00Z")

	errWeak := svc.ResetPassword(ctx, token, "short")
	errCheck := svc.CheckResetToken(ctx, token)

	assert.ErrorIs(t, errWeak, apperrors.ErrWeakPassword)
	assert.NoError(t, errCheck)
}

func TestAuthService_ResetPassword_Expired(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewPasswordResetTokenRepository(db)
	svc := service.NewAuthService(userRepo, tokenRepo, key)
	alice := seedUser(t, userRepo, "alice", domain.RoleUser)
	token := seedToken(t, tokenRepo, alice.ID, "2020-01-01T00:00:00Z")

	err := svc.ResetPassword(ctx, token, newPassword)

	assert.ErrorIs(t, err, apperrors.ErrInvalidToken)
}

func TestAuthService_ChangePassword_NewCookie(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewAuthService(userRepo, repository.NewPasswordResetTokenRepository(db), key)
	seedUser(t, userRepo, "alice", domain.RoleUser)
	old, err := svc.Login(ctx, "alice", password)
	require.NoError(t, err)

	_, errWrong := svc.ChangePassword(ctx, old.ID, "wrong", newPassword)
	changed, errChange := svc.ChangePassword(ctx, old.ID, password, newPassword)
	_, errOld := svc.Authenticate(ctx, old.Cookie)
	user, errNew := svc.Authenticate(ctx, changed.Cookie)

	assert.ErrorIs(t, errWrong, apperrors.ErrInvalidCredentials)
	require.NoError(t, errChange)
	assert.ErrorIs(t, errOld, apperrors.ErrUnauthenticated)
	require.NoError(t, errNew)
	assert.Equal(t, old.ID, user.ID)
}

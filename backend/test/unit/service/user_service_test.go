package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/repository"
	"ebookshelf/internal/service"
	"ebookshelf/test/testutil"
)

func TestPermissions_Allowed(t *testing.T) {
	admin := dto.UserResponse{Role: domain.RoleAdmin}
	flagged := dto.UserResponse{Role: domain.RoleUser, CanEdit: true}
	plain := dto.UserResponse{Role: domain.RoleUser}

	assert.True(t, service.Allowed(admin, service.PermDelete))
	assert.True(t, service.Allowed(flagged, service.PermEdit))
	assert.False(t, service.Allowed(flagged, service.PermUpload))
	assert.False(t, service.Allowed(plain, service.PermEdit))
}

func TestPermissions_CanManage(t *testing.T) {
	root := &domain.User{Role: domain.RoleRoot}
	admin := &domain.User{Role: domain.RoleAdmin}
	user := &domain.User{Role: domain.RoleUser}

	assert.True(t, service.CanManage(admin, user))
	assert.False(t, service.CanManage(admin, admin))
	assert.True(t, service.CanManage(root, admin))
	assert.False(t, service.CanManage(root, root))
	assert.False(t, service.CanManage(admin, root))
	assert.False(t, service.CanManage(user, user))
}

func TestUserService_Create_OKAndTokenWorks(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewPasswordResetTokenRepository(db)
	userSvc := service.NewUserService(userRepo, tokenRepo)
	authSvc := service.NewAuthService(userRepo, tokenRepo, key)
	admin := seedUser(t, userRepo, "boss", domain.RoleAdmin)

	created, err := userSvc.Create(ctx, admin.ID, dto.CreateUserRequest{Username: "bob", Role: domain.RoleUser, CanUpload: true})
	require.NoError(t, err)
	errReset := authSvc.ResetPassword(ctx, created.Token, newPassword)
	_, errLogin := authSvc.Login(ctx, "bob", newPassword)

	assert.True(t, created.User.CanUpload)
	assert.NoError(t, errReset)
	assert.NoError(t, errLogin)
}

func TestUserService_Create_AdminCreatingAdminDenied(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewUserService(userRepo, repository.NewPasswordResetTokenRepository(db))
	admin := seedUser(t, userRepo, "boss", domain.RoleAdmin)

	_, err := svc.Create(ctx, admin.ID, dto.CreateUserRequest{Username: "bob", Role: domain.RoleAdmin})

	assert.ErrorIs(t, err, apperrors.ErrForbidden)
}

func TestUserService_Create_DuplicateUsername(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewUserService(userRepo, repository.NewPasswordResetTokenRepository(db))
	admin := seedUser(t, userRepo, "boss", domain.RoleAdmin)

	_, err := svc.Create(ctx, admin.ID, dto.CreateUserRequest{Username: "boss", Role: domain.RoleUser})

	assert.ErrorIs(t, err, apperrors.ErrUsernameTaken)
}

func TestUserService_Update_RoleByAdminDenied(t *testing.T) {
	db := testutil.OpenTempDB(t)
	userRepo := repository.NewUserRepository(db)
	svc := service.NewUserService(userRepo, repository.NewPasswordResetTokenRepository(db))
	admin := seedUser(t, userRepo, "boss", domain.RoleAdmin)
	bob := seedUser(t, userRepo, "bob", domain.RoleUser)
	role := domain.RoleAdmin

	_, err := svc.Update(ctx, admin.ID, bob.ID, dto.UpdateUserRequest{Role: &role})

	assert.ErrorIs(t, err, apperrors.ErrForbidden)
}

func TestUserService_CreateRoot_Twice(t *testing.T) {
	db := testutil.OpenTempDB(t)
	svc := service.NewUserService(repository.NewUserRepository(db), repository.NewPasswordResetTokenRepository(db))

	token, errFirst := svc.CreateRoot(ctx, "root")
	_, errSecond := svc.CreateRoot(ctx, "root2")

	require.NoError(t, errFirst)
	assert.NotEmpty(t, token)
	assert.ErrorIs(t, errSecond, apperrors.ErrRootExists)
}

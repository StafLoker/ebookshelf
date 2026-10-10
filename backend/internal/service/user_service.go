package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/repository"
	"ebookshelf/internal/security"
)

const resetTokenTTL = 60 * time.Minute

type UserService struct {
	userRepo  repository.UserRepository
	tokenRepo repository.PasswordResetTokenRepository
}

func NewUserService(userRepo repository.UserRepository, tokenRepo repository.PasswordResetTokenRepository) *UserService {
	return &UserService{userRepo: userRepo, tokenRepo: tokenRepo}
}

func (s *UserService) Create(ctx context.Context, actorID int64, req dto.CreateUserRequest) (dto.CreateUserResponse, error) {
	actor, err := s.findUser(ctx, actorID)
	if err != nil {
		return dto.CreateUserResponse{}, err
	}
	if actor.Role == domain.RoleUser || req.Role == domain.RoleAdmin && actor.Role != domain.RoleRoot {
		return dto.CreateUserResponse{}, apperrors.Forbidden("not allowed to create this user")
	}
	if req.Role != domain.RoleUser && req.Role != domain.RoleAdmin {
		return dto.CreateUserResponse{}, apperrors.InvalidRequest("role must be admin or user")
	}

	user := &domain.User{Username: req.Username, Role: req.Role, CanUpload: req.CanUpload, CanEdit: req.CanEdit, CanDelete: req.CanDelete}
	token, err := s.createWithToken(ctx, user)
	if err != nil {
		return dto.CreateUserResponse{}, err
	}
	slog.InfoContext(ctx, "user created", "user_id", user.ID, "actor_id", actorID)
	return dto.CreateUserResponse{User: toUserResponse(user), Token: token}, nil
}

func (s *UserService) List(ctx context.Context, page, pageSize int) (domain.Page[dto.UserResponse], error) {
	users, err := s.userRepo.FindAll(ctx, page, pageSize)
	if err != nil {
		slog.ErrorContext(ctx, "list users failed", "err", err)
		return domain.Page[dto.UserResponse]{}, apperrors.Internal("failed to list users")
	}

	items := make([]dto.UserResponse, len(users.Items))
	for i := range users.Items {
		items[i] = toUserResponse(&users.Items[i])
	}
	return domain.Page[dto.UserResponse]{Items: items, Total: users.Total, Page: users.Page, PageSize: users.PageSize}, nil
}

func (s *UserService) Update(ctx context.Context, actorID, id int64, req dto.UpdateUserRequest) (dto.UserResponse, error) {
	actor, target, err := s.manageable(ctx, actorID, id)
	if err != nil {
		return dto.UserResponse{}, err
	}
	if req.Role != nil && *req.Role != target.Role {
		if actor.Role != domain.RoleRoot {
			return dto.UserResponse{}, apperrors.Forbidden("only root can change roles")
		}
		if *req.Role != domain.RoleAdmin && *req.Role != domain.RoleUser {
			return dto.UserResponse{}, apperrors.InvalidRequest("role must be admin or user")
		}
		target.Role = *req.Role
	}
	if req.CanUpload != nil {
		target.CanUpload = *req.CanUpload
	}
	if req.CanEdit != nil {
		target.CanEdit = *req.CanEdit
	}
	if req.CanDelete != nil {
		target.CanDelete = *req.CanDelete
	}

	if err := s.userRepo.Update(ctx, target); err != nil {
		slog.ErrorContext(ctx, "update user failed", "user_id", id, "err", err)
		return dto.UserResponse{}, apperrors.Internal("failed to update user")
	}
	slog.InfoContext(ctx, "user updated", "user_id", id, "actor_id", actorID)
	return toUserResponse(target), nil
}

func (s *UserService) Delete(ctx context.Context, actorID, id int64) error {
	if _, _, err := s.manageable(ctx, actorID, id); err != nil {
		return err
	}
	if err := s.userRepo.Delete(ctx, id); err != nil {
		slog.ErrorContext(ctx, "delete user failed", "user_id", id, "err", err)
		return apperrors.Internal("failed to delete user")
	}
	slog.InfoContext(ctx, "user deleted", "user_id", id, "actor_id", actorID)
	return nil
}

func (s *UserService) IssueResetToken(ctx context.Context, actorID, id int64) (string, error) {
	if _, _, err := s.manageable(ctx, actorID, id); err != nil {
		return "", err
	}
	token, err := s.issueToken(ctx, id)
	if err != nil {
		return "", err
	}
	slog.InfoContext(ctx, "reset token issued", "user_id", id, "actor_id", actorID)
	return token, nil
}

func (s *UserService) CreateRoot(ctx context.Context, username string) (string, error) {
	_, err := s.userRepo.FindByRole(ctx, domain.RoleRoot)
	if err == nil {
		return "", apperrors.RootExists("root user already exists")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		slog.ErrorContext(ctx, "create root: find root failed", "err", err)
		return "", apperrors.Internal("failed to look up root")
	}
	token, err := s.createWithToken(ctx, &domain.User{Username: username, Role: domain.RoleRoot})
	if err == nil {
		slog.InfoContext(ctx, "root user created")
	}
	return token, err
}

func (s *UserService) ResetRootPassword(ctx context.Context) (string, string, error) {
	root, err := s.userRepo.FindByRole(ctx, domain.RoleRoot)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", apperrors.NoRoot("no root user")
	}
	if err != nil {
		slog.ErrorContext(ctx, "reset root password: find root failed", "err", err)
		return "", "", apperrors.Internal("failed to look up root")
	}
	token, err := s.issueToken(ctx, root.ID)
	if err != nil {
		return "", "", err
	}
	slog.InfoContext(ctx, "root reset token issued", "user_id", root.ID)
	return root.Username, token, nil
}

func (s *UserService) manageable(ctx context.Context, actorID, id int64) (*domain.User, *domain.User, error) {
	actor, err := s.findUser(ctx, actorID)
	if err != nil {
		return nil, nil, err
	}
	target, err := s.findUser(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if !CanManage(actor, target) {
		slog.WarnContext(ctx, "user management denied", "actor_id", actorID, "user_id", id)
		return nil, nil, apperrors.Forbidden("not allowed to manage this user")
	}
	return actor, target, nil
}

func (s *UserService) findUser(ctx context.Context, id int64) (*domain.User, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.UserNotFound("user not found")
	}
	if err != nil {
		slog.ErrorContext(ctx, "find user failed", "user_id", id, "err", err)
		return nil, apperrors.Internal("failed to find user")
	}
	return user, nil
}

func toUserResponse(u *domain.User) dto.UserResponse {
	return dto.UserResponse{ID: u.ID, Username: u.Username, Role: u.Role, CanUpload: u.CanUpload, CanEdit: u.CanEdit, CanDelete: u.CanDelete}
}

func (s *UserService) createWithToken(ctx context.Context, user *domain.User) (string, error) {
	var token string

	if !user.ValidUsername() {
		return "", apperrors.InvalidRequest(fmt.Sprintf("username must be %d to %d characters without spaces", domain.UsernameMinLen, domain.UsernameMaxLen))
	}

	password, _, err := security.NewToken()
	if err == nil {
		user.PasswordHash, err = security.HashPassword(password)
	}
	if err != nil {
		slog.ErrorContext(ctx, "create user: random password failed", "err", err)
		return "", apperrors.Internal("failed to create user")
	}

	err = s.userRepo.Create(ctx, user)
	if errors.Is(err, repository.ErrConflict) {
		return "", apperrors.UsernameTaken("username already taken")
	}
	if err != nil {
		slog.ErrorContext(ctx, "create user failed", "err", err)
		return "", apperrors.Internal("failed to create user")
	}

	token, err = s.issueToken(ctx, user.ID)
	if err != nil {
		if delErr := s.userRepo.Delete(ctx, user.ID); delErr != nil {
			slog.WarnContext(ctx, "create user: cleanup failed", "user_id", user.ID, "err", delErr)
		}
		return "", err
	}
	return token, nil
}

func (s *UserService) issueToken(ctx context.Context, userID int64) (string, error) {
	plain, hash, err := security.NewToken()
	if err == nil {
		err = s.tokenRepo.Create(ctx, &domain.PasswordResetToken{UserID: userID, TokenHash: hash, ExpiresAt: time.Now().Add(resetTokenTTL).UTC().Format(time.RFC3339)})
	}
	if err != nil {
		slog.ErrorContext(ctx, "issue reset token failed", "user_id", userID, "err", err)
		return "", apperrors.Internal("failed to issue reset token")
	}
	return plain, nil
}

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
	var (
		resp  dto.CreateUserResponse
		token string
		user  = &domain.User{Username: req.Username, Role: req.Role, CanUpload: req.CanUpload, CanEdit: req.CanEdit, CanDelete: req.CanDelete}
	)

	actor, err := s.getUser(ctx, actorID)
	switch {
	case err != nil:
	case actor.Role == domain.RoleUser || req.Role == domain.RoleAdmin && actor.Role != domain.RoleRoot:
		err = apperrors.Forbidden("not allowed to create this user")
	case !req.Role.IsAssignable():
		err = apperrors.InvalidRequest("role must be admin or user")
	}
	if err == nil {
		if token, err = s.createWithResetToken(ctx, user); err == nil {
			slog.InfoContext(ctx, "user created", "user_id", user.ID, "actor_id", actorID)
			resp = dto.CreateUserResponse{User: toUserResponse(user), Token: token}
		}
	}
	return resp, err
}

func (s *UserService) List(ctx context.Context, page, pageSize int) (domain.Page[dto.UserResponse], error) {
	var result domain.Page[dto.UserResponse]

	users, err := s.userRepo.FindAll(ctx, page, pageSize)
	if err != nil {
		slog.ErrorContext(ctx, "list users failed", "err", err)
		err = apperrors.Internal("failed to list users")
	} else {
		items := make([]dto.UserResponse, len(users.Items))
		for i := range users.Items {
			items[i] = toUserResponse(&users.Items[i])
		}
		result = domain.NewPage(items, users.Page, users.PageSize, users.Total)
	}
	return result, err
}

func (s *UserService) Update(ctx context.Context, actorID, id int64, req dto.UpdateUserRequest) (dto.UserResponse, error) {
	var resp dto.UserResponse

	actor, target, err := s.loadManaged(ctx, actorID, id)
	if err == nil && req.Role != nil && *req.Role != target.Role {
		switch {
		case actor.Role != domain.RoleRoot:
			err = apperrors.Forbidden("only root can change roles")
		case !req.Role.IsAssignable():
			err = apperrors.InvalidRequest("role must be admin or user")
		default:
			target.Role = *req.Role
		}
	}
	if err == nil {
		if req.CanUpload != nil {
			target.CanUpload = *req.CanUpload
		}
		if req.CanEdit != nil {
			target.CanEdit = *req.CanEdit
		}
		if req.CanDelete != nil {
			target.CanDelete = *req.CanDelete
		}
		if err = s.userRepo.Update(ctx, target); err != nil {
			slog.ErrorContext(ctx, "update user failed", "user_id", id, "err", err)
			err = apperrors.Internal("failed to update user")
		} else {
			slog.InfoContext(ctx, "user updated", "user_id", id, "actor_id", actorID)
			resp = toUserResponse(target)
		}
	}
	return resp, err
}

func (s *UserService) Delete(ctx context.Context, actorID, id int64) error {
	_, _, err := s.loadManaged(ctx, actorID, id)
	if err == nil {
		if err = s.userRepo.Delete(ctx, id); err != nil {
			slog.ErrorContext(ctx, "delete user failed", "user_id", id, "err", err)
			err = apperrors.Internal("failed to delete user")
		} else {
			slog.InfoContext(ctx, "user deleted", "user_id", id, "actor_id", actorID)
		}
	}
	return err
}

func (s *UserService) IssueResetToken(ctx context.Context, actorID, id int64) (string, error) {
	var token string

	_, _, err := s.loadManaged(ctx, actorID, id)
	if err == nil {
		if token, err = s.newResetToken(ctx, id); err == nil {
			slog.InfoContext(ctx, "reset token issued", "user_id", id, "actor_id", actorID)
		}
	}
	return token, err
}

func (s *UserService) CreateRoot(ctx context.Context, username string) (string, error) {
	var token string

	_, err := s.userRepo.FindByRole(ctx, domain.RoleRoot)
	switch {
	case err == nil:
		err = apperrors.RootExists("root user already exists")
	case errors.Is(err, sql.ErrNoRows):
		if token, err = s.createWithResetToken(ctx, &domain.User{Username: username, Role: domain.RoleRoot}); err == nil {
			slog.InfoContext(ctx, "root user created")
		}
	default:
		slog.ErrorContext(ctx, "create root: find root failed", "err", err)
		err = apperrors.Internal("failed to look up root")
	}
	return token, err
}

func (s *UserService) ResetRootPassword(ctx context.Context) (username, token string, err error) {
	root, err := s.userRepo.FindByRole(ctx, domain.RoleRoot)
	switch {
	case err == nil:
		if token, err = s.newResetToken(ctx, root.ID); err == nil {
			slog.InfoContext(ctx, "root reset token issued", "user_id", root.ID)
			username = root.Username
		}
	case errors.Is(err, sql.ErrNoRows):
		err = apperrors.NoRoot("no root user")
	default:
		slog.ErrorContext(ctx, "reset root password: find root failed", "err", err)
		err = apperrors.Internal("failed to look up root")
	}
	return username, token, err
}

// loads both users and checks that the actor may manage the target
func (s *UserService) loadManaged(ctx context.Context, actorID, id int64) (actor, target *domain.User, err error) {
	if actor, err = s.getUser(ctx, actorID); err == nil {
		target, err = s.getUser(ctx, id)
	}
	if err == nil && !actor.CanManage(target) {
		slog.WarnContext(ctx, "user management denied", "actor_id", actorID, "user_id", id)
		err = apperrors.Forbidden("not allowed to manage this user")
	}
	return actor, target, err
}

func (s *UserService) getUser(ctx context.Context, id int64) (*domain.User, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	switch {
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		err = apperrors.UserNotFound("user not found")
	default:
		slog.ErrorContext(ctx, "find user failed", "user_id", id, "err", err)
		err = apperrors.Internal("failed to find user")
	}
	return user, err
}

// the user starts with a random password nobody knows; the reset token is how they set their own
func (s *UserService) createWithResetToken(ctx context.Context, user *domain.User) (string, error) {
	var (
		token    string
		password string
		err      error
	)

	if !user.ValidUsername() {
		err = apperrors.InvalidRequest(fmt.Sprintf("username must be %d to %d characters without spaces", domain.UsernameMinLen, domain.UsernameMaxLen))
	}
	if err == nil {
		if password, _, err = security.NewToken(); err == nil {
			user.PasswordHash, err = security.HashPassword(password)
		}
		if err != nil {
			slog.ErrorContext(ctx, "create user: random password failed", "err", err)
			err = apperrors.Internal("failed to create user")
		}
	}
	if err == nil {
		err = s.userRepo.Create(ctx, user)
		switch {
		case err == nil:
		case errors.Is(err, repository.ErrConflict):
			err = apperrors.UsernameTaken("username already taken")
		default:
			slog.ErrorContext(ctx, "create user failed", "err", err)
			err = apperrors.Internal("failed to create user")
		}
	}
	if err == nil {
		if token, err = s.newResetToken(ctx, user.ID); err != nil {
			if delErr := s.userRepo.Delete(ctx, user.ID); delErr != nil {
				slog.WarnContext(ctx, "create user: cleanup failed", "user_id", user.ID, "err", delErr)
			}
		}
	}
	return token, err
}

func (s *UserService) newResetToken(ctx context.Context, userID int64) (string, error) {
	plain, hash, err := security.NewToken()
	if err == nil {
		err = s.tokenRepo.Create(ctx, &domain.PasswordResetToken{UserID: userID, TokenHash: hash, ExpiresAt: time.Now().Add(resetTokenTTL).UTC().Format(time.RFC3339)})
	}
	if err != nil {
		slog.ErrorContext(ctx, "issue reset token failed", "user_id", userID, "err", err)
		plain, err = "", apperrors.Internal("failed to issue reset token")
	}
	return plain, err
}

func toUserResponse(u *domain.User) dto.UserResponse {
	return dto.UserResponse{ID: u.ID, Username: u.Username, Role: u.Role, CanUpload: u.CanUpload, CanEdit: u.CanEdit, CanDelete: u.CanDelete}
}

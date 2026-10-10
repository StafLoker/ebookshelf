package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/repository"
	"ebookshelf/internal/security"
)

const sessionTTL = 30 * 24 * time.Hour

// hash of a throwaway password, verified for unknown users so login takes the same time
const dummyHash = "$argon2id$v=19$m=19456,t=2,p=1$WeDuptYqbg2k/yAVNBRH4g$fHspz5+ok7yFFWKQ1Fl09cofpaTBZXKCX4Vy9o/1diM"

type AuthService struct {
	userRepo   repository.UserRepository
	tokenRepo  repository.PasswordResetTokenRepository
	sessionKey []byte
}

func NewAuthService(userRepo repository.UserRepository, tokenRepo repository.PasswordResetTokenRepository, sessionKey []byte) *AuthService {
	return &AuthService{userRepo: userRepo, tokenRepo: tokenRepo, sessionKey: sessionKey}
}

func (s *AuthService) Login(ctx context.Context, username, password string) (dto.LoginResponse, error) {
	var (
		resp dto.LoginResponse
		hash = dummyHash
		ok   bool
	)

	user, err := s.userRepo.FindByUsername(ctx, username)
	switch {
	case err == nil:
		hash = user.PasswordHash
	case errors.Is(err, sql.ErrNoRows):
		err = nil
	default:
		slog.ErrorContext(ctx, "login: find user failed", "err", err)
		err = apperrors.Internal("failed to log in")
	}
	if err == nil {
		if ok, err = security.VerifyPassword(password, hash); err != nil {
			slog.ErrorContext(ctx, "login: verify password failed", "err", err)
			err = apperrors.Internal("failed to log in")
		}
	}
	switch {
	case err != nil:
	case user == nil || !ok:
		slog.WarnContext(ctx, "login denied")
		err = apperrors.InvalidCredentials("invalid username or password")
	default:
		slog.InfoContext(ctx, "user logged in", "user_id", user.ID)
		resp = s.newLoginResponse(user, user.SessionVersion)
	}
	return resp, err
}

func (s *AuthService) Authenticate(ctx context.Context, cookie string) (dto.UserResponse, error) {
	var (
		resp dto.UserResponse
		user *domain.User
	)

	session, err := security.VerifySession(s.sessionKey, cookie, time.Now())
	if err == nil {
		user, err = s.userRepo.FindByID(ctx, session.UserID)
	}
	switch {
	case err == nil && user.SessionVersion == session.Version:
		resp = toUserResponse(user)
	case err == nil, errors.Is(err, security.ErrInvalidSession), errors.Is(err, security.ErrSessionExpired), errors.Is(err, sql.ErrNoRows):
		err = apperrors.Unauthenticated("invalid session")
	default:
		slog.ErrorContext(ctx, "authenticate: find user failed", "user_id", session.UserID, "err", err)
		err = apperrors.Internal("failed to authenticate")
	}
	return resp, err
}

func (s *AuthService) CheckResetToken(ctx context.Context, token string) error {
	_, err := s.validResetToken(ctx, token)
	return err
}

func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	var (
		hash  string
		found *domain.PasswordResetToken
		err   error
	)

	if missing := domain.MissingPasswordRules(newPassword); len(missing) > 0 {
		err = apperrors.WeakPassword("password does not meet the policy", missing)
	}
	if err == nil {
		if hash, err = security.HashPassword(newPassword); err != nil {
			slog.ErrorContext(ctx, "reset password: hash failed", "err", err)
			err = apperrors.Internal("failed to reset password")
		}
	}
	if err == nil {
		found, err = s.validResetToken(ctx, token)
	}
	if err == nil {
		if err = s.tokenRepo.DeleteByTokenHash(ctx, found.TokenHash); err == nil {
			err = s.userRepo.ChangePassword(ctx, found.UserID, hash)
		}
		switch {
		case err == nil:
			slog.InfoContext(ctx, "password reset", "user_id", found.UserID)
		case errors.Is(err, sql.ErrNoRows):
			err = apperrors.InvalidToken("invalid or expired token")
		default:
			slog.ErrorContext(ctx, "reset password failed", "err", err)
			err = apperrors.Internal("failed to reset password")
		}
	}
	return err
}

func (s *AuthService) ChangePassword(ctx context.Context, userID int64, current, newPassword string) (dto.LoginResponse, error) {
	var (
		resp dto.LoginResponse
		hash string
		ok   bool
	)

	user, err := s.userRepo.FindByID(ctx, userID)
	if err == nil {
		ok, err = security.VerifyPassword(current, user.PasswordHash)
	}
	switch {
	case err != nil:
		slog.ErrorContext(ctx, "change password: check current failed", "user_id", userID, "err", err)
		err = apperrors.Internal("failed to change password")
	case !ok:
		slog.WarnContext(ctx, "change password denied", "user_id", userID)
		err = apperrors.InvalidCredentials("current password is wrong")
	}
	if missing := domain.MissingPasswordRules(newPassword); err == nil && len(missing) > 0 {
		err = apperrors.WeakPassword("password does not meet the policy", missing)
	}
	if err == nil {
		if hash, err = security.HashPassword(newPassword); err == nil {
			err = s.userRepo.ChangePassword(ctx, userID, hash)
		}
		if err != nil {
			slog.ErrorContext(ctx, "change password failed", "user_id", userID, "err", err)
			err = apperrors.Internal("failed to change password")
		}
	}
	if err == nil {
		slog.InfoContext(ctx, "password changed", "user_id", userID)
		resp = s.newLoginResponse(user, user.SessionVersion+1)
	}
	return resp, err
}

// returns InvalidToken when the token does not exist or has expired
func (s *AuthService) validResetToken(ctx context.Context, token string) (*domain.PasswordResetToken, error) {
	found, err := s.tokenRepo.FindByTokenHash(ctx, security.HashToken(token))
	switch {
	case err == nil && found.ExpiresAt > time.Now().UTC().Format(time.RFC3339):
	case err == nil, errors.Is(err, sql.ErrNoRows):
		err = apperrors.InvalidToken("invalid or expired token")
	default:
		slog.ErrorContext(ctx, "find reset token failed", "err", err)
		err = apperrors.Internal("failed to check token")
	}
	return found, err
}

// version is the session version the cookie is valid for: a password change bumps it and ends the other sessions
func (s *AuthService) newLoginResponse(user *domain.User, version int64) dto.LoginResponse {
	expires := time.Now().Add(sessionTTL)

	return dto.LoginResponse{
		UserResponse: toUserResponse(user),
		Cookie:       security.SignSession(s.sessionKey, security.Session{UserID: user.ID, Version: version, ExpiresAt: expires}),
		ExpiresAt:    expires,
	}
}

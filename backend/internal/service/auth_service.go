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
	hash := dummyHash

	user, err := s.userRepo.FindByUsername(ctx, username)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.ErrorContext(ctx, "login: find user failed", "err", err)
		return dto.LoginResponse{}, apperrors.Internal("failed to log in")
	}
	if user != nil {
		hash = user.PasswordHash
	}

	ok, err := security.VerifyPassword(password, hash)
	if err != nil {
		slog.ErrorContext(ctx, "login: verify password failed", "err", err)
		return dto.LoginResponse{}, apperrors.Internal("failed to log in")
	}
	if user == nil || !ok {
		slog.WarnContext(ctx, "login denied")
		return dto.LoginResponse{}, apperrors.InvalidCredentials("invalid username or password")
	}

	slog.InfoContext(ctx, "user logged in", "user_id", user.ID)
	return s.newLoginResponse(user, user.SessionVersion), nil
}

func (s *AuthService) Authenticate(ctx context.Context, cookie string) (dto.UserResponse, error) {
	session, err := security.VerifySession(s.sessionKey, cookie, time.Now())
	if errors.Is(err, security.ErrInvalidSession) || errors.Is(err, security.ErrSessionExpired) {
		return dto.UserResponse{}, apperrors.Unauthenticated("invalid session")
	}
	if err != nil {
		slog.ErrorContext(ctx, "authenticate: find user failed", "user_id", session.UserID, "err", err)
		return dto.UserResponse{}, apperrors.Internal("failed to authenticate")
	}

	user, err := s.userRepo.FindByID(ctx, session.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return dto.UserResponse{}, apperrors.Unauthenticated("invalid session")
	}
	if err != nil {
		slog.ErrorContext(ctx, "authenticate: find user failed", "user_id", session.UserID, "err", err)
		return dto.UserResponse{}, apperrors.Internal("failed to authenticate")
	}
	if user.SessionVersion != session.Version {
		return dto.UserResponse{}, apperrors.Unauthenticated("invalid session")
	}
	return toUserResponse(user), nil
}

func (s *AuthService) CheckResetToken(ctx context.Context, token string) error {
	_, err := s.validResetToken(ctx, token)
	return err
}

func (s *AuthService) ResetPassword(ctx context.Context, token, newPassword string) error {
	if missing := domain.MissingPasswordRules(newPassword); len(missing) > 0 {
		return apperrors.WeakPassword("password does not meet the policy", missing)
	}

	hash, err := security.HashPassword(newPassword)
	if err != nil {
		slog.ErrorContext(ctx, "reset password: hash failed", "err", err)
		return apperrors.Internal("failed to reset password")
	}
	found, err := s.validResetToken(ctx, token)
	if err != nil {
		return err
	}

	err = s.tokenRepo.DeleteByTokenHash(ctx, found.TokenHash)
	if err != nil {
		return resetPasswordError(ctx, err)
	}
	err = s.userRepo.ChangePassword(ctx, found.UserID, hash)
	if err != nil {
		return resetPasswordError(ctx, err)
	}
	slog.InfoContext(ctx, "password reset", "user_id", found.UserID)
	return nil
}

func (s *AuthService) ChangePassword(ctx context.Context, userID int64, current, newPassword string) (dto.LoginResponse, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		slog.ErrorContext(ctx, "change password: check current failed", "user_id", userID, "err", err)
		return dto.LoginResponse{}, apperrors.Internal("failed to change password")
	}
	ok, err := security.VerifyPassword(current, user.PasswordHash)
	if err != nil {
		slog.ErrorContext(ctx, "change password: check current failed", "user_id", userID, "err", err)
		return dto.LoginResponse{}, apperrors.Internal("failed to change password")
	}
	if !ok {
		slog.WarnContext(ctx, "change password denied", "user_id", userID)
		return dto.LoginResponse{}, apperrors.InvalidCredentials("current password is wrong")
	}
	if missing := domain.MissingPasswordRules(newPassword); len(missing) > 0 {
		return dto.LoginResponse{}, apperrors.WeakPassword("password does not meet the policy", missing)
	}

	hash, err := security.HashPassword(newPassword)
	if err != nil {
		slog.ErrorContext(ctx, "change password failed", "user_id", userID, "err", err)
		return dto.LoginResponse{}, apperrors.Internal("failed to change password")
	}
	err = s.userRepo.ChangePassword(ctx, userID, hash)
	if err != nil {
		slog.ErrorContext(ctx, "change password failed", "user_id", userID, "err", err)
		return dto.LoginResponse{}, apperrors.Internal("failed to change password")
	}
	slog.InfoContext(ctx, "password changed", "user_id", userID)
	return s.newLoginResponse(user, user.SessionVersion+1), nil
}

func resetPasswordError(ctx context.Context, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return apperrors.InvalidToken("invalid or expired token")
	}
	slog.ErrorContext(ctx, "reset password failed", "err", err)
	return apperrors.Internal("failed to reset password")
}

// returns InvalidToken when the token does not exist or has expired
func (s *AuthService) validResetToken(ctx context.Context, token string) (*domain.PasswordResetToken, error) {
	found, err := s.tokenRepo.FindByTokenHash(ctx, security.HashToken(token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.InvalidToken("invalid or expired token")
	}
	if err != nil {
		slog.ErrorContext(ctx, "find reset token failed", "err", err)
		return nil, apperrors.Internal("failed to check token")
	}
	if found.ExpiresAt <= time.Now().UTC().Format(time.RFC3339) {
		return nil, apperrors.InvalidToken("invalid or expired token")
	}
	return found, nil
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

package handler

import (
	"net/http"
	"time"

	"ebookshelf/internal/dto"
	"ebookshelf/internal/service"
)

const SessionCookie = "session"

type UserKey struct{}

type AuthHandler struct {
	authSvc *service.AuthService
}

func NewAuthHandler(authSvc *service.AuthService) *AuthHandler {
	return &AuthHandler{authSvc: authSvc}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var (
		req   dto.LoginRequest
		login dto.LoginResponse
	)

	err := decodeJSON(w, r, &req)
	if err == nil {
		login, err = h.authSvc.Login(r.Context(), req.Username, req.Password)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		setSessionCookie(w, r, login.Cookie, login.ExpiresAt)
		writeJSON(w, r, http.StatusOK, dto.Success("Login successful", login))
	}
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	setSessionCookie(w, r, "", time.Unix(0, 0))
	writeJSON(w, r, http.StatusOK, dto.Success("Logout successful", nil))
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, dto.Success("Current user", currentUser(r)))
}

func (h *AuthHandler) CheckResetToken(w http.ResponseWriter, r *http.Request) {
	var req dto.ResetPasswordRequest

	err := decodeJSON(w, r, &req)
	if err == nil {
		err = h.authSvc.CheckResetToken(r.Context(), req.Token)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("Token is valid", nil))
	}
}

func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req dto.ResetPasswordRequest

	err := decodeJSON(w, r, &req)
	if err == nil {
		err = h.authSvc.ResetPassword(r.Context(), req.Token, req.NewPassword)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("Password reset successful", nil))
	}
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	var (
		req   dto.ChangePasswordRequest
		login dto.LoginResponse
	)

	err := decodeJSON(w, r, &req)
	if err == nil {
		login, err = h.authSvc.ChangePassword(r.Context(), currentUser(r).ID, req.CurrentPassword, req.NewPassword)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		setSessionCookie(w, r, login.Cookie, login.ExpiresAt)
		writeJSON(w, r, http.StatusOK, dto.Success("Password changed successfully", nil))
	}
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, value string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})
}

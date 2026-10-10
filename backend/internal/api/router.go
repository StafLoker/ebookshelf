package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"ebookshelf/internal/container"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/handler"
)

func NewRouter(c *container.Container) http.Handler {
	var (
		r     = chi.NewRouter()
		authH = handler.NewAuthHandler(c.AuthSvc)
		userH = handler.NewUserHandler(c.UserSvc)
	)

	r.Use(correlation, recoverer, accessLog, noStore)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		handler.WriteError(w, r, apperrors.RouteNotFound("route not found"))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		handler.WriteError(w, r, apperrors.MethodNotAllowed("method not allowed"))
	})

	r.Post("/api/auth/login", authH.Login)
	r.Post("/api/auth/reset-password", authH.ResetPassword)
	r.Post("/api/auth/reset-password/check", authH.CheckResetToken)

	r.Group(func(r chi.Router) {
		r.Use(session(c.AuthSvc))
		r.Post("/api/auth/logout", authH.Logout)
		r.Get("/api/auth/me", authH.Me)
		r.Post("/api/auth/change-password", authH.ChangePassword)

		r.Group(func(r chi.Router) {
			r.Use(requireAdmin)
			r.Get("/api/users", userH.List)
			r.Post("/api/users", userH.Create)
			r.Patch("/api/users/{id}", userH.Update)
			r.Delete("/api/users/{id}", userH.Delete)
			r.Post("/api/users/{id}/reset-token", userH.IssueResetToken)
		})
	})

	return r
}

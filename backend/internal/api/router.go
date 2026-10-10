package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"ebookshelf/internal/container"
	"ebookshelf/internal/domain"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/handler"
)

func NewRouter(c *container.Container) http.Handler {
	var (
		r             = chi.NewRouter()
		authHandler   = handler.NewAuthHandler(c.AuthSvc)
		userHandler   = handler.NewUserHandler(c.UserSvc)
		bookHandler   = handler.NewBookHandler(c.BookSvc)
		uploadHandler = handler.NewUploadHandler(c.BookSvc)
	)

	r.Use(correlation, recoverer, accessLog, noStore)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		handler.WriteError(w, r, apperrors.RouteNotFound("route not found"))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		handler.WriteError(w, r, apperrors.MethodNotAllowed("method not allowed"))
	})

	r.Post("/api/auth/login", authHandler.Login)
	r.Post("/api/auth/reset-password", authHandler.ResetPassword)
	r.Post("/api/auth/reset-password/check", authHandler.CheckResetToken)

	r.Group(func(r chi.Router) {
		r.Use(requireSession(c.AuthSvc))
		r.Post("/api/auth/logout", authHandler.Logout)
		r.Get("/api/auth/me", authHandler.Me)
		r.Post("/api/auth/change-password", authHandler.ChangePassword)

		r.Get("/api/books", bookHandler.List)
		r.Get("/api/books/{id}", bookHandler.Get)
		r.Get("/api/books/{id}/file", bookHandler.File)
		r.Get("/api/books/{id}/cover", bookHandler.Cover)
		r.Put("/api/books/{id}/read", bookHandler.MarkRead)
		r.Delete("/api/books/{id}/read", bookHandler.MarkUnread)

		r.Group(func(r chi.Router) {
			r.Use(requirePermission(domain.PermUpload))
			r.Get("/api/uploads", uploadHandler.List)
			r.Post("/api/uploads", uploadHandler.Upload)
			r.Get("/api/uploads/{id}", uploadHandler.Get)
			r.Delete("/api/uploads/{id}", uploadHandler.Discard)
			r.Post("/api/uploads/{id}/commit", uploadHandler.Commit)
		})

		r.Group(func(r chi.Router) {
			r.Use(requireAdmin)
			r.Get("/api/users", userHandler.List)
			r.Post("/api/users", userHandler.Create)
			r.Patch("/api/users/{id}", userHandler.Update)
			r.Delete("/api/users/{id}", userHandler.Delete)
			r.Post("/api/users/{id}/reset-token", userHandler.IssueResetToken)
		})
	})

	return r
}

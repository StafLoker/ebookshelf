package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/handler"
	"ebookshelf/internal/logging"
	"ebookshelf/internal/service"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

func correlation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := logging.NewID()
		if err != nil {
			id = "-"
		}
		next.ServeHTTP(w, r.WithContext(logging.WithCorrelation(r.Context(), "req", id)))
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				slog.ErrorContext(r.Context(), "panic recovered", "panic", p)
				handler.WriteError(w, r, apperrors.Internal("internal error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)
		slog.DebugContext(r.Context(), "request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration_ms", time.Since(start).Milliseconds())
	})
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		next.ServeHTTP(w, r)
	})
}

func session(authSvc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var user dto.UserResponse

			cookie, err := r.Cookie(handler.SessionCookie)
			if err != nil {
				err = apperrors.Unauthenticated("not logged in")
			} else {
				user, err = authSvc.Authenticate(r.Context(), cookie.Value)
			}
			if err != nil {
				handler.WriteError(w, r, err)
			} else {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), handler.UserKey{}, user)))
			}
		})
	}
}

func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Context().Value(handler.UserKey{}).(dto.UserResponse)

		if user.Role == domain.RoleAdmin || user.Role == domain.RoleRoot {
			next.ServeHTTP(w, r)
		} else {
			handler.WriteError(w, r, apperrors.Forbidden("admin access required"))
		}
	})
}

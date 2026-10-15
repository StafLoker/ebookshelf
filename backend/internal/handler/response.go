package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"

	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
)

const (
	maxBodyBytes = 1 << 20 // 1 MB
	uploadField  = "file"
)

func writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.WarnContext(r.Context(), "write response failed", "err", err)
	}
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	appErr, ok := errors.AsType[*apperrors.AppError](err)
	if !ok {
		slog.ErrorContext(r.Context(), "unhandled error", "err", err)
		appErr = apperrors.Internal("internal error")
	}
	writeJSON(w, r, statusOf(appErr.Code), dto.ErrorCode(appErr.Message, appErr.Code, appErr.Details))
}

func statusOf(code apperrors.ErrorCode) int {
	switch code {
	case apperrors.AUTH_INVALID_CREDENTIALS, apperrors.AUTH_UNAUTHENTICATED:
		return http.StatusUnauthorized
	case apperrors.AUTH_FORBIDDEN:
		return http.StatusForbidden
	case apperrors.AUTH_INVALID_TOKEN, apperrors.AUTH_WEAK_PASSWORD, apperrors.AUTH_WRONG_PASSWORD, apperrors.REQ_INVALID:
		return http.StatusBadRequest
	case apperrors.AUTH_USER_NOT_FOUND, apperrors.AUTH_NO_ROOT, apperrors.BOOK_NOT_FOUND, apperrors.REQ_NOT_FOUND:
		return http.StatusNotFound
	case apperrors.AUTH_USERNAME_TAKEN, apperrors.AUTH_ROOT_EXISTS, apperrors.BOOK_DUPLICATE:
		return http.StatusConflict
	case apperrors.BOOK_UNSUPPORTED_FORMAT:
		return http.StatusUnsupportedMediaType
	case apperrors.BOOK_UNREADABLE_FILE:
		return http.StatusUnprocessableEntity
	case apperrors.BOOK_FILE_TOO_LARGE:
		return http.StatusRequestEntityTooLarge
	case apperrors.META_PROVIDER_DOWN:
		return http.StatusBadGateway
	case apperrors.REQ_METHOD_NOT_ALLOWED:
		return http.StatusMethodNotAllowed
	}
	return http.StatusInternalServerError
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return apperrors.InvalidRequest("invalid request body")
	}
	return nil
}

func currentUser(r *http.Request) dto.UserResponse {
	return r.Context().Value(UserKey{}).(dto.UserResponse)
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, apperrors.InvalidRequest("invalid id")
	}
	return id, nil
}

// an absent key is not an error: it returns 0, meaning no filter
func queryID(r *http.Request, key string) (int64, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, apperrors.InvalidRequest("invalid " + key + " filter")
	}
	return id, nil
}

// the first part of a multipart body, which must be the file field
func filePart(r *http.Request) (*multipart.Part, error) {
	missing := apperrors.InvalidRequest("multipart field \"" + uploadField + "\" with a file is required")

	reader, err := r.MultipartReader()
	if errors.Is(err, http.ErrNotMultipart) || errors.Is(err, http.ErrMissingBoundary) {
		return nil, missing
	}
	if err != nil {
		return nil, err
	}
	part, err := reader.NextPart()
	if errors.Is(err, io.EOF) {
		return nil, missing
	}
	if err != nil {
		return nil, err
	}
	if part.FormName() != uploadField || part.FileName() == "" {
		return nil, missing
	}
	return part, nil
}

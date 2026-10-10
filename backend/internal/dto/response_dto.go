package dto

import (
	"net/http"
	"strconv"

	"ebookshelf/internal/domain"
	apperrors "ebookshelf/internal/errors"
)

const (
	defaultLimit = 24
	maxLimit     = 100
)

type PaginationMetadata struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

type ErrorData struct {
	Code    apperrors.ErrorCode `json:"code"`
	Details any                 `json:"details,omitempty"`
}

type ApiResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Data     any    `json:"data,omitempty"`
	Metadata any    `json:"metadata,omitempty"`
}

func Success(message string, data any) ApiResponse {
	return ApiResponse{Status: "success", Message: message, Data: data}
}

func SuccessMetadata(message string, data any, metadata any) ApiResponse {
	return ApiResponse{Status: "success", Message: message, Data: data, Metadata: metadata}
}

func ErrorCode(message string, code apperrors.ErrorCode, details any) ApiResponse {
	return ApiResponse{Status: "error", Message: message, Data: ErrorData{Code: code, Details: details}}
}

func Pagination[T any](p domain.Page[T]) PaginationMetadata {
	return PaginationMetadata{Page: p.Page, Limit: p.PageSize, Total: p.Total, TotalPages: p.TotalPages}
}

func ParsePage(r *http.Request) (page, limit int) {
	q := r.URL.Query()
	page, _ = strconv.Atoi(q.Get("page"))
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit < 1 {
		limit = defaultLimit
	}
	return max(page, 1), min(limit, maxLimit)
}

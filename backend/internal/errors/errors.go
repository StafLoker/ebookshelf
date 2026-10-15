package errors

import (
	"fmt"
)

type ErrorCode string

const (
	// Auth
	AUTH_INVALID_CREDENTIALS ErrorCode = "AUTH_001"
	AUTH_UNAUTHENTICATED     ErrorCode = "AUTH_002"
	AUTH_FORBIDDEN           ErrorCode = "AUTH_010"
	AUTH_INVALID_TOKEN       ErrorCode = "AUTH_020"
	AUTH_WEAK_PASSWORD       ErrorCode = "AUTH_021"
	AUTH_WRONG_PASSWORD      ErrorCode = "AUTH_022"
	AUTH_USER_NOT_FOUND      ErrorCode = "AUTH_030"
	AUTH_USERNAME_TAKEN      ErrorCode = "AUTH_031"
	AUTH_ROOT_EXISTS         ErrorCode = "AUTH_032"
	AUTH_NO_ROOT             ErrorCode = "AUTH_033"

	// Book
	BOOK_DUPLICATE          ErrorCode = "BOOK_001"
	BOOK_UNSUPPORTED_FORMAT ErrorCode = "BOOK_002"
	BOOK_UNREADABLE_FILE    ErrorCode = "BOOK_003"
	BOOK_FILE_TOO_LARGE     ErrorCode = "BOOK_004"
	BOOK_NOT_FOUND          ErrorCode = "BOOK_005"

	// Metadata
	META_FILE_WRITE_FAILED ErrorCode = "META_001"
	META_PROVIDER_DOWN     ErrorCode = "META_002"

	// Request
	REQ_INVALID            ErrorCode = "REQ_001"
	REQ_NOT_FOUND          ErrorCode = "REQ_002"
	REQ_METHOD_NOT_ALLOWED ErrorCode = "REQ_003"

	// Internal
	INT_ERROR ErrorCode = "INT_001"
)

type AppError struct {
	Code    ErrorCode
	Message string
	Details any
}

func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	return ok && e.Code == t.Code
}

func InvalidCredentials(msg string) *AppError {
	return &AppError{Code: AUTH_INVALID_CREDENTIALS, Message: msg}
}

func Unauthenticated(msg string) *AppError {
	return &AppError{Code: AUTH_UNAUTHENTICATED, Message: msg}
}

func Forbidden(msg string) *AppError {
	return &AppError{Code: AUTH_FORBIDDEN, Message: msg}
}

func InvalidToken(msg string) *AppError {
	return &AppError{Code: AUTH_INVALID_TOKEN, Message: msg}
}

func WeakPassword(msg string, missing []string) *AppError {
	return &AppError{Code: AUTH_WEAK_PASSWORD, Message: msg, Details: missing}
}

func WrongPassword(msg string) *AppError {
	return &AppError{Code: AUTH_WRONG_PASSWORD, Message: msg}
}

func UserNotFound(msg string) *AppError {
	return &AppError{Code: AUTH_USER_NOT_FOUND, Message: msg}
}

func UsernameTaken(msg string) *AppError {
	return &AppError{Code: AUTH_USERNAME_TAKEN, Message: msg}
}

func RootExists(msg string) *AppError {
	return &AppError{Code: AUTH_ROOT_EXISTS, Message: msg}
}

func NoRoot(msg string) *AppError {
	return &AppError{Code: AUTH_NO_ROOT, Message: msg}
}

func BookDuplicate(msg string, bookID int64) *AppError {
	return &AppError{Code: BOOK_DUPLICATE, Message: msg, Details: map[string]int64{"book_id": bookID}}
}

func UnsupportedFormat(msg string) *AppError {
	return &AppError{Code: BOOK_UNSUPPORTED_FORMAT, Message: msg}
}

func UnreadableFile(msg string) *AppError {
	return &AppError{Code: BOOK_UNREADABLE_FILE, Message: msg}
}

func FileTooLarge(msg string) *AppError {
	return &AppError{Code: BOOK_FILE_TOO_LARGE, Message: msg}
}

func BookNotFound(msg string) *AppError {
	return &AppError{Code: BOOK_NOT_FOUND, Message: msg}
}

func FileWriteFailed(msg string) *AppError {
	return &AppError{Code: META_FILE_WRITE_FAILED, Message: msg}
}

func ProviderDown(msg string) *AppError {
	return &AppError{Code: META_PROVIDER_DOWN, Message: msg}
}

func InvalidRequest(msg string) *AppError {
	return &AppError{Code: REQ_INVALID, Message: msg}
}

func RouteNotFound(msg string) *AppError {
	return &AppError{Code: REQ_NOT_FOUND, Message: msg}
}

func MethodNotAllowed(msg string) *AppError {
	return &AppError{Code: REQ_METHOD_NOT_ALLOWED, Message: msg}
}

func Internal(msg string) *AppError {
	return &AppError{Code: INT_ERROR, Message: msg}
}

var (
	ErrInvalidCredentials = InvalidCredentials("")
	ErrUnauthenticated    = Unauthenticated("")
	ErrForbidden          = Forbidden("")
	ErrInvalidToken       = InvalidToken("")
	ErrWeakPassword       = WeakPassword("", nil)
	ErrWrongPassword      = WrongPassword("")
	ErrUserNotFound       = UserNotFound("")
	ErrUsernameTaken      = UsernameTaken("")
	ErrRootExists         = RootExists("")
	ErrNoRoot             = NoRoot("")
	ErrBookDuplicate      = BookDuplicate("", 0)
	ErrUnsupportedFormat  = UnsupportedFormat("")
	ErrUnreadableFile     = UnreadableFile("")
	ErrFileTooLarge       = FileTooLarge("")
	ErrBookNotFound       = BookNotFound("")
	ErrFileWriteFailed    = FileWriteFailed("")
	ErrProviderDown       = ProviderDown("")
	ErrInvalidRequest     = InvalidRequest("")
	ErrRouteNotFound      = RouteNotFound("")
	ErrMethodNotAllowed   = MethodNotAllowed("")
	ErrInternal           = Internal("")
)

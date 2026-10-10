package handler

import (
	"log/slog"
	"mime"
	"net/http"
	"os"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/service"
)

type BookHandler struct {
	bookSvc *service.BookService
}

func NewBookHandler(bookSvc *service.BookService) *BookHandler {
	return &BookHandler{bookSvc: bookSvc}
}

func (h *BookHandler) List(w http.ResponseWriter, r *http.Request) {
	var (
		books domain.Page[dto.BookSummaryResponse]
		q     = r.URL.Query()
		req   = dto.BookFilterRequest{Query: q.Get("q"), Language: q.Get("language"), Read: q.Get("read"), Sort: q.Get("sort")}
		err   error
	)

	page, limit := dto.ParsePage(r)
	if req.AuthorID, err = queryID(r, "author"); err == nil {
		req.TagID, err = queryID(r, "tag")
	}
	if err == nil {
		books, err = h.bookSvc.List(r.Context(), currentUser(r).ID, req, page, limit)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.SuccessMetadata("Books retrieved successfully", books.Items, dto.Pagination(books)))
	}
}

func (h *BookHandler) Get(w http.ResponseWriter, r *http.Request) {
	var book dto.BookResponse

	id, err := pathID(r)
	if err == nil {
		book, err = h.bookSvc.Get(r.Context(), currentUser(r).ID, id)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("Book retrieved successfully", book))
	}
}

func (h *BookHandler) File(w http.ResponseWriter, r *http.Request) {
	var file dto.FileResponse

	id, err := pathID(r)
	if err == nil {
		file, err = h.bookSvc.File(r.Context(), id)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		serveFile(w, r, file.Path, file.Name, file.Hash)
	}
}

func (h *BookHandler) Cover(w http.ResponseWriter, r *http.Request) {
	var path string

	id, err := pathID(r)
	if err == nil {
		path, err = h.bookSvc.Cover(r.Context(), id)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		serveFile(w, r, path, "", "")
	}
}

func (h *BookHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = h.bookSvc.MarkRead(r.Context(), currentUser(r).ID, id)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("Book marked as read", nil))
	}
}

func (h *BookHandler) MarkUnread(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = h.bookSvc.MarkUnread(r.Context(), currentUser(r).ID, id)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("Book marked as unread", nil))
	}
}

func serveFile(w http.ResponseWriter, r *http.Request, path, name, etag string) {
	var info os.FileInfo

	f, err := os.Open(path)
	if err == nil {
		defer func() { _ = f.Close() }()
		info, err = f.Stat()
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "read file failed", "err", err)
		WriteError(w, r, apperrors.Internal("failed to read file"))
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
		if etag != "" {
			w.Header().Set("ETag", `"`+etag+`"`)
		}
		if name != "" {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		}
		http.ServeContent(w, r, path, info.ModTime(), f)
	}
}

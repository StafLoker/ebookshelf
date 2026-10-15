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

func (h *BookHandler) Add(w http.ResponseWriter, r *http.Request) {
	part, err := filePart(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	book, err := h.bookSvc.Add(r.Context(), currentUser(r).ID, part.FileName(), part)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, dto.Success("File uploaded successfully", book))
}

func (h *BookHandler) List(w http.ResponseWriter, r *http.Request) {
	var (
		q   = r.URL.Query()
		req = dto.BookFilterRequest{Query: q.Get("q"), Language: q.Get("language"), Read: q.Get("read"), Sort: q.Get("sort")}
		err error
	)

	switch domain.BookStatus(q.Get("status")) {
	case "", domain.BookAdded:
	case domain.BookPending:
		pending, err := h.bookSvc.ListPending(r.Context(), currentUser(r))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, r, http.StatusOK, dto.Success("Pending books retrieved successfully", pending))
		return
	default:
		WriteError(w, r, apperrors.InvalidRequest("status must be added or pending"))
		return
	}
	page, limit := dto.ParsePage(r)
	req.AuthorID, err = queryID(r, "author")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	req.TagID, err = queryID(r, "tag")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	books, err := h.bookSvc.List(r.Context(), currentUser(r).ID, req, page, limit)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.SuccessMetadata("Books retrieved successfully", books.Items, dto.Pagination(books)))
}

func (h *BookHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	book, err := h.bookSvc.Get(r.Context(), currentUser(r), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.Success("Book retrieved successfully", book))
}

func (h *BookHandler) File(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	file, err := h.bookSvc.File(r.Context(), currentUser(r), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	serveFile(w, r, file.Path, file.Name, file.Hash)
}

func (h *BookHandler) Cover(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	path, err := h.bookSvc.Cover(r.Context(), currentUser(r), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	serveFile(w, r, path, "", "")
}

func (h *BookHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req dto.UpdateBookRequest

	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	err = decodeJSON(w, r, &req)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	book, err := h.bookSvc.Update(r.Context(), currentUser(r), id, req)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.Success("Book updated successfully", book))
}

func (h *BookHandler) SetCover(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	part, err := filePart(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	book, err := h.bookSvc.SetCover(r.Context(), currentUser(r), id, part)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.Success("Cover updated successfully", book))
}

func (h *BookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	err = h.bookSvc.Delete(r.Context(), currentUser(r), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.Success("Book deleted", nil))
}

func (h *BookHandler) Commit(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	book, err := h.bookSvc.Commit(r.Context(), currentUser(r), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.Success("Book added successfully", book))
}

func (h *BookHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	err = h.bookSvc.MarkRead(r.Context(), currentUser(r), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.Success("Book marked as read", nil))
}

func (h *BookHandler) MarkUnread(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	err = h.bookSvc.MarkUnread(r.Context(), currentUser(r), id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.Success("Book marked as unread", nil))
}

func serveFile(w http.ResponseWriter, r *http.Request, path, name, etag string) {
	f, err := os.Open(path)
	if err != nil {
		slog.ErrorContext(r.Context(), "read file failed", "err", err)
		WriteError(w, r, apperrors.Internal("failed to read file"))
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		slog.ErrorContext(r.Context(), "read file failed", "err", err)
		WriteError(w, r, apperrors.Internal("failed to read file"))
		return
	}
	w.Header().Set("Cache-Control", "private, no-cache")
	if etag != "" {
		w.Header().Set("ETag", `"`+etag+`"`)
	}
	if name != "" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
	http.ServeContent(w, r, path, info.ModTime(), f)
}

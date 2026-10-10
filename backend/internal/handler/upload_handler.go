package handler

import (
	"mime/multipart"
	"net/http"

	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/service"
)

const uploadField = "file"

type UploadHandler struct {
	bookSvc *service.BookService
}

func NewUploadHandler(bookSvc *service.BookService) *UploadHandler {
	return &UploadHandler{bookSvc: bookSvc}
}

func (h *UploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	var (
		book dto.BookResponse
		part *multipart.Part
	)

	reader, err := r.MultipartReader()
	if err == nil {
		part, err = reader.NextPart()
	}
	if err == nil && (part.FormName() != uploadField || part.FileName() == "") {
		err = apperrors.InvalidRequest("multipart field \"" + uploadField + "\" with a file is required")
	}
	if err == nil {
		book, err = h.bookSvc.Upload(r.Context(), currentUser(r).ID, part.FileName(), part)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusCreated, dto.Success("File uploaded successfully", book))
	}
}

func (h *UploadHandler) List(w http.ResponseWriter, r *http.Request) {
	books, err := h.bookSvc.ListPending(r.Context(), currentUser(r))
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("Uploads retrieved successfully", books))
	}
}

func (h *UploadHandler) Get(w http.ResponseWriter, r *http.Request) {
	var book dto.BookResponse

	id, err := pathID(r)
	if err == nil {
		book, err = h.bookSvc.GetPending(r.Context(), currentUser(r), id)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("Upload retrieved successfully", book))
	}
}

func (h *UploadHandler) Discard(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err == nil {
		err = h.bookSvc.DiscardPending(r.Context(), currentUser(r), id)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("Upload discarded", nil))
	}
}

func (h *UploadHandler) Commit(w http.ResponseWriter, r *http.Request) {
	var book dto.BookResponse

	id, err := pathID(r)
	if err == nil {
		book, err = h.bookSvc.CommitPending(r.Context(), currentUser(r), id)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("Book added successfully", book))
	}
}

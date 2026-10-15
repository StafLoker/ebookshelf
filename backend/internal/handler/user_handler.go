package handler

import (
	"net/http"

	"ebookshelf/internal/dto"
	"ebookshelf/internal/service"
)

type UserHandler struct {
	userSvc *service.UserService
}

func NewUserHandler(userSvc *service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateUserRequest

	err := decodeJSON(w, r, &req)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	created, err := h.userSvc.Create(r.Context(), currentUser(r).ID, req)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, dto.Success("User created successfully", created))
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	page, limit := dto.ParsePage(r)

	users, err := h.userSvc.List(r.Context(), page, limit)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.SuccessMetadata("Users retrieved successfully", users.Items, dto.Pagination(users)))
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req dto.UpdateUserRequest

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
	user, err := h.userSvc.Update(r.Context(), currentUser(r).ID, id, req)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.Success("User updated successfully", user))
}

func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	err = h.userSvc.Delete(r.Context(), currentUser(r).ID, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.Success("User deleted successfully", nil))
}

func (h *UserHandler) IssueResetToken(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	token, err := h.userSvc.IssueResetToken(r.Context(), currentUser(r).ID, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dto.Success("Reset token issued", map[string]string{"token": token}))
}

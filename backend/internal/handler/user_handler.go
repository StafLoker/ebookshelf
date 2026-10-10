package handler

import (
	"net/http"
	"strconv"

	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/service"
)

type UserHandler struct {
	userSvc *service.UserService
}

func NewUserHandler(userSvc *service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	page, limit := dto.ParsePage(r)

	users, err := h.userSvc.List(r.Context(), page, limit)
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.SuccessMetadata("Users retrieved successfully", users.Items, dto.PaginationMetadata{Page: users.Page, Limit: users.PageSize, Total: users.Total, TotalPages: (users.Total + users.PageSize - 1) / users.PageSize}))
	}
}

func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var (
		req     dto.CreateUserRequest
		created dto.CreateUserResponse
	)

	actor := r.Context().Value(UserKey{}).(dto.UserResponse)
	err := decodeJSON(w, r, &req)
	if err == nil {
		created, err = h.userSvc.Create(r.Context(), actor.ID, req)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusCreated, dto.Success("User created successfully", created))
	}
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	var (
		req  dto.UpdateUserRequest
		user dto.UserResponse
	)

	actor := r.Context().Value(UserKey{}).(dto.UserResponse)
	id, err := userID(r)
	if err == nil {
		err = decodeJSON(w, r, &req)
	}
	if err == nil {
		user, err = h.userSvc.Update(r.Context(), actor.ID, id, req)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("User updated successfully", user))
	}
}

func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	actor := r.Context().Value(UserKey{}).(dto.UserResponse)
	id, err := userID(r)
	if err == nil {
		err = h.userSvc.Delete(r.Context(), actor.ID, id)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("User deleted successfully", nil))
	}
}

func (h *UserHandler) IssueResetToken(w http.ResponseWriter, r *http.Request) {
	var token string

	actor := r.Context().Value(UserKey{}).(dto.UserResponse)
	id, err := userID(r)
	if err == nil {
		token, err = h.userSvc.IssueResetToken(r.Context(), actor.ID, id)
	}
	if err != nil {
		WriteError(w, r, err)
	} else {
		writeJSON(w, r, http.StatusOK, dto.Success("Reset token issued", map[string]string{"token": token}))
	}
}

func userID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, apperrors.InvalidRequest("invalid user id")
	}
	return id, nil
}

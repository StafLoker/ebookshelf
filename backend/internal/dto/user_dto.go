package dto

import "ebookshelf/internal/domain"

type CreateUserRequest struct {
	Username  string      `json:"username"`
	Role      domain.Role `json:"role"`
	CanUpload bool        `json:"can_upload"`
	CanEdit   bool        `json:"can_edit"`
	CanDelete bool        `json:"can_delete"`
}

type UpdateUserRequest struct {
	Role      *domain.Role `json:"role"`
	CanUpload *bool        `json:"can_upload"`
	CanEdit   *bool        `json:"can_edit"`
	CanDelete *bool        `json:"can_delete"`
}

type UserResponse struct {
	ID        int64       `json:"id"`
	Username  string      `json:"username"`
	Role      domain.Role `json:"role"`
	CanUpload bool        `json:"can_upload"`
	CanEdit   bool        `json:"can_edit"`
	CanDelete bool        `json:"can_delete"`
}

type CreateUserResponse struct {
	User  UserResponse `json:"user"`
	Token string       `json:"token"`
}

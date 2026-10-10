package service

import (
	"ebookshelf/internal/domain"
	"ebookshelf/internal/dto"
)

type Permission string

const (
	PermUpload Permission = "upload"
	PermEdit   Permission = "edit"
	PermDelete Permission = "delete"
)

func Allowed(user dto.UserResponse, perm Permission) bool {
	return user.Role != domain.RoleUser ||
		perm == PermUpload && user.CanUpload ||
		perm == PermEdit && user.CanEdit ||
		perm == PermDelete && user.CanDelete
}

func CanManage(actor, target *domain.User) bool {
	return actor.Role == domain.RoleRoot && target.Role != domain.RoleRoot ||
		actor.Role == domain.RoleAdmin && target.Role == domain.RoleUser
}

package domain

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type Role string

const (
	RoleRoot  Role = "root"
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"

	UsernameMinLen = 3
	UsernameMaxLen = 64
)

type User struct {
	ID             int64  `db:"id"`
	Username       string `db:"username"`
	PasswordHash   string `db:"password_hash"`
	Role           Role   `db:"role"`
	CanUpload      bool   `db:"can_upload"`
	CanEdit        bool   `db:"can_edit"`
	CanDelete      bool   `db:"can_delete"`
	SessionVersion int64  `db:"session_version"`
}

func (u *User) ValidUsername() bool {
	n := utf8.RuneCountInString(u.Username)

	return n >= UsernameMinLen && n <= UsernameMaxLen &&
		!strings.ContainsFunc(u.Username, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

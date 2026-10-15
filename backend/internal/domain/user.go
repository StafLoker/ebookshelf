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
)

type Permission string

const (
	PermAdd    Permission = "add"
	PermEdit   Permission = "edit"
	PermDelete Permission = "delete"
)

const (
	UsernameMinLen = 3
	UsernameMaxLen = 64
	PasswordMinLen = 12
)

type User struct {
	ID             int64  `db:"id"`
	Username       string `db:"username"`
	PasswordHash   string `db:"password_hash"`
	Role           Role   `db:"role"`
	CanAdd         bool   `db:"can_add"`
	CanEdit        bool   `db:"can_edit"`
	CanDelete      bool   `db:"can_delete"`
	SessionVersion int64  `db:"session_version"`
}

// staff sees every pending book and holds every permission; a plain user only its own
func (r Role) IsStaff() bool {
	return r != RoleUser
}

// roles that root may hand out; root itself is created only from the CLI
func (r Role) IsAssignable() bool {
	return r == RoleAdmin || r == RoleUser
}

// admins and root hold every permission; a plain user only the flags set on it
func (u *User) Allowed(perm Permission) bool {
	return u.Role.IsStaff() ||
		perm == PermAdd && u.CanAdd ||
		perm == PermEdit && u.CanEdit ||
		perm == PermDelete && u.CanDelete
}

// root manages everyone but itself, an admin manages plain users only
func (u *User) CanManage(target *User) bool {
	return u.Role == RoleRoot && target.Role != RoleRoot ||
		u.Role == RoleAdmin && target.Role == RoleUser
}

func (u *User) ValidUsername() bool {
	n := utf8.RuneCountInString(u.Username)

	return n >= UsernameMinLen && n <= UsernameMaxLen &&
		!strings.ContainsFunc(u.Username, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

// returns the rules the password breaks, empty when it meets the policy
func MissingPasswordRules(plain string) []string {
	var (
		missing                                  []string
		hasUpper, hasLower, hasDigit, hasSpecial bool
	)

	for _, r := range plain {
		hasUpper = hasUpper || unicode.IsUpper(r)
		hasLower = hasLower || unicode.IsLower(r)
		hasDigit = hasDigit || unicode.IsDigit(r)
		hasSpecial = hasSpecial || !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}
	if utf8.RuneCountInString(plain) < PasswordMinLen {
		missing = append(missing, "min_length")
	}
	if !hasUpper {
		missing = append(missing, "uppercase")
	}
	if !hasLower {
		missing = append(missing, "lowercase")
	}
	if !hasDigit {
		missing = append(missing, "digit")
	}
	if !hasSpecial {
		missing = append(missing, "special")
	}
	return missing
}

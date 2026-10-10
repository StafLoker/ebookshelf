package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"ebookshelf/internal/domain"
)

func TestMissingPasswordRules_Weak(t *testing.T) {
	assert.Equal(t, []string{"min_length", "uppercase", "digit", "special"}, domain.MissingPasswordRules("short"))
}

func TestUser_ValidUsername_OK(t *testing.T) {
	assert.True(t, (&domain.User{Username: "alice"}).ValidUsername())
	assert.True(t, (&domain.User{Username: strings.Repeat("я", domain.UsernameMaxLen)}).ValidUsername())
}

func TestUser_ValidUsername_Invalid(t *testing.T) {
	assert.False(t, (&domain.User{Username: "ab"}).ValidUsername())
	assert.False(t, (&domain.User{Username: strings.Repeat("a", domain.UsernameMaxLen+1)}).ValidUsername())
	assert.False(t, (&domain.User{Username: "al ice"}).ValidUsername())
	assert.False(t, (&domain.User{Username: "al\nice"}).ValidUsername())
}

func TestUser_Allowed(t *testing.T) {
	admin := &domain.User{Role: domain.RoleAdmin}
	flagged := &domain.User{Role: domain.RoleUser, CanEdit: true}
	plain := &domain.User{Role: domain.RoleUser}

	assert.True(t, admin.Allowed(domain.PermDelete))
	assert.True(t, flagged.Allowed(domain.PermEdit))
	assert.False(t, flagged.Allowed(domain.PermUpload))
	assert.False(t, plain.Allowed(domain.PermEdit))
}

func TestUser_CanManage(t *testing.T) {
	root := &domain.User{Role: domain.RoleRoot}
	admin := &domain.User{Role: domain.RoleAdmin}
	user := &domain.User{Role: domain.RoleUser}

	assert.True(t, admin.CanManage(user))
	assert.False(t, admin.CanManage(admin))
	assert.True(t, root.CanManage(admin))
	assert.False(t, root.CanManage(root))
	assert.False(t, admin.CanManage(root))
	assert.False(t, user.CanManage(user))
}

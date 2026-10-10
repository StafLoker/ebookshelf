package domain_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"ebookshelf/internal/domain"
)

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

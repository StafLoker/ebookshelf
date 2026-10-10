package security_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"ebookshelf/internal/security"
)

func TestValidatePassword_Weak(t *testing.T) {
	missing := security.ValidatePassword("short")

	assert.Equal(t, []string{"min_length", "uppercase", "digit", "special"}, missing)
}

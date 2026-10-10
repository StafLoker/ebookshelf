package security_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"ebookshelf/internal/security"
)

const sessionKey = "0123456789abcdef0123456789abcdef"

var sessionNow = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func TestVerifySession_Tampered(t *testing.T) {
	mine := security.SignSession([]byte(sessionKey), security.Session{UserID: 7, Version: 3, ExpiresAt: sessionNow.Add(time.Hour)})
	other := security.SignSession([]byte(sessionKey), security.Session{UserID: 1, Version: 3, ExpiresAt: sessionNow.Add(time.Hour)})
	otherPayload, _, _ := strings.Cut(other, ".")
	_, mySig, _ := strings.Cut(mine, ".")

	_, err := security.VerifySession([]byte(sessionKey), otherPayload+"."+mySig, sessionNow)

	assert.ErrorIs(t, err, security.ErrInvalidSession)
}

func TestVerifySession_Expired(t *testing.T) {
	cookie := security.SignSession([]byte(sessionKey), security.Session{UserID: 7, Version: 3, ExpiresAt: sessionNow})

	_, err := security.VerifySession([]byte(sessionKey), cookie, sessionNow)

	assert.ErrorIs(t, err, security.ErrSessionExpired)
}

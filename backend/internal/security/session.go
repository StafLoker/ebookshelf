package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"
)

const payloadLen = 24

var (
	ErrInvalidSession = errors.New("invalid session")
	ErrSessionExpired = errors.New("session expired")
)

type Session struct {
	UserID    int64
	Version   int64
	ExpiresAt time.Time
}

func SignSession(key []byte, s Session) string {
	var payload [payloadLen]byte

	binary.BigEndian.PutUint64(payload[0:8], uint64(s.UserID))
	binary.BigEndian.PutUint64(payload[8:16], uint64(s.Version))
	binary.BigEndian.PutUint64(payload[16:24], uint64(s.ExpiresAt.Unix()))
	return base64.RawURLEncoding.EncodeToString(payload[:]) + "." + base64.RawURLEncoding.EncodeToString(mac(key, payload[:]))
}

func VerifySession(key []byte, value string, now time.Time) (Session, error) {
	encPayload, encSig, found := strings.Cut(value, ".")
	payload, errPayload := base64.RawURLEncoding.DecodeString(encPayload)
	sig, errSig := base64.RawURLEncoding.DecodeString(encSig)
	if !found || errPayload != nil || errSig != nil || len(payload) != payloadLen || !hmac.Equal(mac(key, payload), sig) {
		return Session{}, ErrInvalidSession
	}
	s := Session{
		UserID:    int64(binary.BigEndian.Uint64(payload[0:8])),
		Version:   int64(binary.BigEndian.Uint64(payload[8:16])),
		ExpiresAt: time.Unix(int64(binary.BigEndian.Uint64(payload[16:24])), 0),
	}
	if !now.Before(s.ExpiresAt) {
		return Session{}, ErrSessionExpired
	}
	return s, nil
}

func mac(key, payload []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(payload)
	return h.Sum(nil)
}

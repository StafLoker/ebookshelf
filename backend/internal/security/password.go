package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

// OWASP argon2id minimum: 64 MiB would let a login flood exhaust ram on a small box
const (
	argonMemory  uint32 = 19 * 1024
	argonTime    uint32 = 2
	argonThreads uint8  = 1
	argonKeyLen  uint32 = 32
	saltLen             = 16
)

var ErrInvalidHash = errors.New("invalid password hash")

func HashPassword(plain string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(plain), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(plain, encoded string) (bool, error) {
	var memory, iterations uint32
	var threads uint8

	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrInvalidHash
	}
	_, errParams := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads)
	salt, errSalt := base64.RawStdEncoding.DecodeString(parts[4])
	want, errWant := base64.RawStdEncoding.DecodeString(parts[5])
	if errParams != nil || errSalt != nil || errWant != nil {
		return false, ErrInvalidHash
	}
	got := argon2.IDKey([]byte(plain), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

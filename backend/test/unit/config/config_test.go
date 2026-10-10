package config_test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/config"
)

func TestConfig_Load_Valid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.ini")
	secret := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 32)))
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	require.NoError(t, os.WriteFile(path, []byte("[security]\nsecret_key = "+secret+"\nencryption_key = "+key+"\n[log]\nlevel = DEBUG\n"), 0o600))

	cfg, err := config.Load(path)

	require.NoError(t, err)
	assert.Equal(t, 7717, cfg.Port)
	assert.Equal(t, 100, cfg.MaxUploadMB)
	assert.Equal(t, "DEBUG", cfg.LogLevel)
	assert.Equal(t, []byte(strings.Repeat("s", 32)), cfg.SecretKey)
	assert.Equal(t, []byte(strings.Repeat("k", 32)), cfg.EncryptionKey)
}

func TestConfig_Load_MissingSecretKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.ini")
	require.NoError(t, os.WriteFile(path, []byte("[security]\nencryption_key = "+base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))+"\n"), 0o600))

	_, err := config.Load(path)

	assert.ErrorContains(t, err, "secret_key")
}

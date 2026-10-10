package config

import (
	"encoding/base64"
	"fmt"

	"gopkg.in/ini.v1"
)

const (
	defaultPort        = 7717
	defaultMaxUploadMB = 100
	defaultLogLevel    = "INFO"
	defaultDataDir     = "/var/lib/ebookshelf"

	keyLen = 32
)

type Config struct {
	Port          int
	SecretKey     []byte
	EncryptionKey []byte
	MaxUploadMB   int
	LogLevel      string
	DataDir       string
}

func Load(path string) (*Config, error) {
	var (
		file *ini.File
		cfg  *Config
		err  error
	)
	server := struct {
		Port        int `ini:"port"`
		MaxUploadMB int `ini:"max_upload_mb"`
	}{Port: defaultPort, MaxUploadMB: defaultMaxUploadMB}
	security := struct {
		SecretKey     string `ini:"secret_key"`
		EncryptionKey string `ini:"encryption_key"`
	}{}

	file, err = ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	if err = file.Section("server").MapTo(&server); err != nil {
		return nil, fmt.Errorf("config: invalid [server]: %w", err)
	}
	if err = file.Section("security").MapTo(&security); err != nil {
		return nil, fmt.Errorf("config: invalid [security]: %w", err)
	}

	cfg = &Config{
		Port:        server.Port,
		MaxUploadMB: server.MaxUploadMB,
		LogLevel:    file.Section("log").Key("level").MustString(defaultLogLevel),
		DataDir:     file.Section("paths").Key("data_dir").MustString(defaultDataDir),
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, fmt.Errorf("config: invalid port: must be between 1 and 65535")
	}
	if cfg.MaxUploadMB < 1 {
		return nil, fmt.Errorf("config: invalid max_upload_mb: must be greater than 0")
	}
	if cfg.SecretKey, err = decodeKey("secret_key", security.SecretKey); err != nil {
		return nil, err
	}
	if cfg.EncryptionKey, err = decodeKey("encryption_key", security.EncryptionKey); err != nil {
		return nil, err
	}

	return cfg, nil
}

func decodeKey(name, value string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(key) != keyLen {
		return nil, fmt.Errorf("config: invalid %s: required, base64 of exactly %d bytes (openssl rand -base64 %d)", name, keyLen, keyLen)
	}
	return key, nil
}

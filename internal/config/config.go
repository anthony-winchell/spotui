// Package config loads spotui's user configuration
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the user's spotui configuration
type Config struct {
	ClientID string `toml:"client_id"`
}

// Load reads the config file at path, then applies environment overrides.
// A missing file is not an error as long as the result is valid.
func Load(path string) (Config, error) {
	var cfg Config

	md, err := toml.DecodeFile(path, &cfg)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// No file;
	case err != nil:
		return Config{}, fmt.Errorf("read config: %s: %w", path, err)
	default:
		if keys := md.Undecoded(); len(keys) > 0 {
			return Config{}, fmt.Errorf("config: %s: unknown keys %v", path, keys)
		}
	}

	if v := os.Getenv("SPOTUI_CLIENT_ID"); v != "" {
		cfg.ClientID = v
	}

	if cfg.ClientID == "" {
		return Config{}, fmt.Errorf("no Spotify client ID: set client_id in %s or the SPOTUI_CLIENT_ID environment variable", path)
	}
	return cfg, nil
}

// DefaultPath returns the OS-appropriate config file location.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}

	return filepath.Join(dir, "spotui", "config.toml"), nil
}

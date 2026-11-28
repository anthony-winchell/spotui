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

	// CallbackPort is the port of the login redirect,
	// http://127.0.0.1:<port>/callback, which must also be registered in
	// Spotify's developer dashboard
	CallbackPort int `toml:"callback_port"`

	VolumeStep int `toml:"volume_step"` // percent per - or + in the TUI
	SeekStep   int `toml:"seek_step"`   // seconds per , or . in the TUI
}

// defaults is the configuration before the file is read; the file only
// changes the keys it sets
var defaults = Config{
	CallbackPort: 8888,
	VolumeStep:   5,
	SeekStep:     5,
}

// Load reads the config file at path, then applies environment overrides.
// A missing file is not an error as long as the result is valid.
func Load(path string) (Config, error) {
	cfg := defaults

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
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, nil
}

// validate checks that every setting is in range
func (c Config) validate() error {
	switch {
	case c.CallbackPort < 1 || c.CallbackPort > 65535:
		return fmt.Errorf("callback_port %d is not a port number (1-65535)", c.CallbackPort)
	case c.VolumeStep < 1 || c.VolumeStep > 100:
		return fmt.Errorf("volume_step %d must be 1-100", c.VolumeStep)
	case c.SeekStep < 1:
		return fmt.Errorf("seek_step %d must be at least 1 second", c.SeekStep)
	}
	return nil
}

// DefaultPath returns the OS-appropriate config file location.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}

	return filepath.Join(dir, "spotui", "config.toml"), nil
}

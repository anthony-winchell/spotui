package spotify

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "spotui"
	keyringUser    = "spotify-refresh-token"
)

// ErrNoToken means no refresh token has been saved yet.
var ErrNoToken = errors.New("no saved Spotify token")

// TokenStore persists the refresh token between runs
type TokenStore interface {
	Load() (string, error) // returns ErrNoToken if nothing is saved
	Save(refreshToken string) error
}

// KeyringStore stores the refresh token in the OS credential store
// (Windows Credential Manager, macOS Keychain, Linux Secret Service)
type KeyringStore struct{}

func (KeyringStore) Load() (string, error) {
	rt, err := keyring.Get(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNoToken
	}

	if err != nil {
		return "", fmt.Errorf("read credential store: %w", err)
	}

	return rt, nil
}

func (KeyringStore) Save(refreshToken string) error {
	if err := keyring.Set(keyringService, keyringUser, refreshToken); err != nil {
		return fmt.Errorf("save to credential store: %w", err)
	}

	return nil
}

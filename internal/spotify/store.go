package spotify

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "spotui"
	keyringUser    = "spotify-refresh-token"
)

// ErrNoToken means no refresh token has been saved yet.
var ErrNoToken = errors.New("no saved Spotify token")

// TokenStore persists the token between runs, so separate spotui processes
// (the TUI, the CLI commands the Neovim plugin runs) share one login
type TokenStore interface {
	Load() (Token, error) // returns ErrNoToken if nothing is saved
	Save(tok Token) error
}

// KeyringStore stores the token as JSON in the OS credential store
// (Windows Credential Manager, macOS Keychain, Linux Secret Service)
type KeyringStore struct{}

func (KeyringStore) Load() (Token, error) {
	s, err := keyring.Get(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return Token{}, ErrNoToken
	}

	if err != nil {
		return Token{}, fmt.Errorf("read credential store: %w", err)
	}

	// Older versions saved the refresh token, as plain text
	if !strings.HasPrefix(s, "{") {
		return Token{RefreshToken: s}, nil
	}

	var tok Token
	if err := json.Unmarshal([]byte(s), &tok); err != nil {
		return Token{}, fmt.Errorf("read credential store: %w", err)
	}

	return tok, nil
}

func (KeyringStore) Save(tok Token) error {
	b, err := json.Marshal(tok)
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}

	if err := keyring.Set(keyringService, keyringUser, string(b)); err != nil {
		return fmt.Errorf("save to credential store: %w", err)
	}

	return nil
}

// Clear deletes the saved login. It's not an error if there is none.
func (KeyringStore) Clear() error {
	err := keyring.Delete(keyringService, keyringUser)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("clear credential store: %w", err)
	}
	return nil
}

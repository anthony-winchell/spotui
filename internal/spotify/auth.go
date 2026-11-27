// Package spotify is a client for the Spotify Web API.
package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

// newCodeVerifier returns a random PKCE code verifier.
// 32 random bytes encode to 43 base64url characters, the minimum length allowed.
func newCodeVerifier() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// codeChallenge derives the S256 PKCE challenge from a verifier.
func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

const (
	authorizeEndpoint = "https://accounts.spotify.com/authorize"
	redirectURI       = "http://127.0.0.1:8888/callback"
)

// scopes are the permissions requested at login
var scopes = []string{
	"user-read-playback-state",
	"user-read-currently-playing",
	"user-library-read",
	"user-library-modify",
	"user-modify-playback-state",
	"playlist-read-private",
	"playlist-read-collaborative",
	"user-library-read",
	"user-read-recently-played",
}

// buildAuthorizeURL returns the Spotify login URL for the PKCE flow.
func buildAuthorizeURL(clientID, state, challenge string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge_method", "S256")
	q.Set("code_challenge", challenge)
	q.Set("state", state)
	q.Set("scope", strings.Join(scopes, " "))
	return authorizeEndpoint + "?" + q.Encode()
}

// Login runs the browser-based OAuth PKCE flow.
func Login(ctx context.Context, clientID string) (Token, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:8888")
	if err != nil {
		return Token{}, fmt.Errorf("start callback server (is port 8888 in use?): %w", err)
	}

	verifier := newCodeVerifier()
	state := rand.Text()
	authURL := buildAuthorizeURL(clientID, state, codeChallenge(verifier))

	fmt.Println("Opening your browser to log in to Spotify.")
	fmt.Println("If it doesn't open, visit this URL:")
	fmt.Println(authURL)

	if err := openBrowser(authURL); err != nil {
		fmt.Println("Could not open browser:", err)
	}

	code, err := waitForCode(ctx, ln, state)
	if err != nil {
		return Token{}, err
	}

	hc := &http.Client{Timeout: 15 * time.Second}
	return exchangeCode(ctx, hc, tokenEndpoint, clientID, code, verifier)
}

// Authenticate returns a valid token, reusing the saved login when possible
// and failing back to the browser login when there is none or it is revoked.
func Authenticate(ctx context.Context, clientID string, store TokenStore) (Token, error) {
	hc := &http.Client{Timeout: 15 * time.Second}

	tok, err := resumeLogin(ctx, hc, clientID, store)
	if errors.Is(err, ErrNoToken) || isInvalidGrant(err) || (err == nil && missingScopes(tok.Scope)) {
		tok, err = Login(ctx, clientID)
		if err == nil {
			err = store.Save(tok)
		}
	}
	if err != nil {
		return Token{}, err
	}

	return tok, nil
}

// ErrNeedsLogin means there is no usable saved login, and the caller can't open
// the browser to make one
var ErrNeedsLogin = errors.New("not logged in")

// Resume returns a token from the saved login, and never opens the browser.
// It's for commands run by scripts, where nobody is there to log in
func Resume(ctx context.Context, clientID string, store TokenStore) (Token, error) {
	hc := &http.Client{Timeout: 15 * time.Second}

	tok, err := resumeLogin(ctx, hc, clientID, store)
	switch {
	case errors.Is(err, ErrNoToken), isInvalidGrant(err), err == nil && missingScopes(tok.Scope):
		return Token{}, ErrNeedsLogin
	case err != nil:
		return Token{}, err
	}
	return tok, nil
}

// resumeLogin returns the saved token, refreshing it first if it has expired
func resumeLogin(ctx context.Context, hc *http.Client, clientID string, store TokenStore) (Token, error) {
	tok, err := store.Load()
	if err != nil {
		return Token{}, err
	}
	if valid(tok) {
		return tok, nil // still good: no request needed
	}

	tok, err = refreshAccessToken(ctx, hc, tokenEndpoint, clientID, tok.RefreshToken)
	if err != nil {
		return Token{}, err
	}
	if err := store.Save(tok); err != nil {
		return Token{}, err
	}
	return tok, nil
}

// ErrLoginExpired means Spotify no longer accepts the saved login, e.g. because
// the user revoked access; signing in again is the only fix
var ErrLoginExpired = errors.New("spotify login expired")

// isInvalidGrant reports whether Spotify rejected the grant, e.g. a revoked refresh token.
func isInvalidGrant(err error) bool {
	var te *TokenError
	return errors.As(err, &te) && te.Code == "invalid_grant"
}

// missingScopes reports whether a token lacks any scopes spotui needs.
// An empty scope means Spotify didn't say, so we keep the token
func missingScopes(granted string) bool {
	if granted == "" {
		return false
	}

	have := strings.Fields(granted)
	for _, s := range scopes {
		if !slices.Contains(have, s) {
			return true
		}
	}
	return false
}

// Package spotify is a client for the Spotify Web API.
package spotify

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
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
// For now it only opens the browser; will complete later.
func Login(clientID string) error {
	verifier := newCodeVerifier()
	state := rand.Text()
	authURL := buildAuthorizeURL(clientID, state, codeChallenge(verifier))

	fmt.Println("Opening your browser to log in to Spotify.")
	fmt.Println("If it doesn't open, visit this URL: ")
	fmt.Println(authURL)

	if err := openBrowser(authURL); err != nil {
		fmt.Println("Could not open browser:", err)
	}

	return nil
}

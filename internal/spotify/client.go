package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const apiBaseURL = "https://api.spotify.com/v1"

// expiryMargin is how long before the expiry the access token is refreshed
const expiryMargin = time.Minute

// Client is an authenticated Spotify Web API client. It is safe for concurrent use.
type Client struct {
	hc       *http.Client
	clientID string
	store    TokenStore
	apiURL   string // overridable in tests
	tokenURL string // overridable in tests

	mu  sync.Mutex // guards tok
	tok Token
}

// NewClient returns a client that starts with tok and refreshes it as needed
func NewClient(clientID string, tok Token, store TokenStore) *Client {
	return &Client{
		hc:       &http.Client{Timeout: 15 * time.Second},
		clientID: clientID,
		store:    store,
		apiURL:   apiBaseURL,
		tokenURL: tokenEndpoint,
		tok:      tok,
	}
}

// accessToken returns a valid access token, refreshing it first if it is about
// to expire. Other spotui processes share the store, so it checks there before
// refreshing: one of them may have refreshed already
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if valid(c.tok) {
		return c.tok.AccessToken, nil
	}

	c.adoptStored()
	if valid(c.tok) {
		return c.tok.AccessToken, nil // another process refreshed it
	}

	used := c.tok.RefreshToken
	tok, err := refreshAccessToken(ctx, c.hc, c.tokenURL, c.clientID, used)
	if isInvalidGrant(err) && c.awaitRotation(ctx, used) {
		return c.tok.AccessToken, nil // another process refreshed at the same moment
	}
	if err != nil {
		if isInvalidGrant(err) {
			return "", fmt.Errorf("%w: %w", ErrLoginExpired, err)
		}
		return "", fmt.Errorf("refresh access token: %w", err)
	}

	c.tok = tok
	if err := c.store.Save(tok); err != nil {
		return "", err
	}
	return c.tok.AccessToken, nil
}

// adoptStored replaces c.tok with the saved token. Errors are ignored: the
// token we already have is still worth trying.
func (c *Client) adoptStored() {
	if c.store == nil {
		return
	}
	if tok, err := c.store.Load(); err == nil {
		c.tok = tok
	}
}

// awaitRotation waits briefly for another process's token to appear in the
// store. If two processes refresh at once, Spotify rotates the refresh token
// for the first and rejects the second; the first saves its new token just
// after Spotify answers, so the second may have to wait a moment for it.
func (c *Client) awaitRotation(ctx context.Context, used string) bool {
	for range 10 {
		c.adoptStored()
		if c.tok.RefreshToken != used && valid(c.tok) {
			return true
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
	return false
}

// valid reports whether tok's access token will work for at least expiryMargin
func valid(tok Token) bool {
	return tok.AccessToken != "" && time.Until(tok.Expiry) > expiryMargin
}

// do sends an authenticated API request.
// If body is non-nil it is sent as JSON. if out is non-nil and the
// response has content, it is decoded into out a
// 204 No Content leaves out unchanged
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	at, err := c.accessToken(ctx)
	if err != nil {
		return err
	}

	u := c.apiURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		r = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+at)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseAPIError(resp)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}

	return nil
}

// APIError is an error response from the Spotify Web API
type APIError struct {
	StatusCode int
	Message    string
	Reason     string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("spotify API: HTTP %d", e.StatusCode)

	if e.Message != "" {
		msg += ": " + e.Message
	}

	if e.Reason != "" {
		msg += " (" + e.Reason + ")"
	}

	return msg
}

// parseAPIError builds an APIError from a non-2xx API response
func parseAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))

	var wrapper struct {
		Error struct {
			Message string `json:"message"`
			Reason  string `json:"reason"`
		} `json:"error"`
	}

	_ = json.Unmarshal(body, &wrapper) // non-JSON bodies leave the fields empty

	e := &APIError{
		StatusCode: resp.StatusCode,
		Message:    wrapper.Error.Message,
		Reason:     wrapper.Error.Reason,
	}

	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
		e.RetryAfter = time.Duration(s) * time.Second
	}

	return e
}

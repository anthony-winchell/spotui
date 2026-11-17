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

// accessToken returns a valid access token, refreshing it first if it is about to expire.
func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Until(c.tok.Expiry) > expiryMargin {
		return c.tok.AccessToken, nil
	}

	tok, err := refreshAccessToken(ctx, c.hc, c.tokenURL, c.clientID, c.tok.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("refresh access token: %w", err)
	}

	rotated := tok.RefreshToken != c.tok.RefreshToken
	c.tok = tok
	if rotated {
		if err := c.store.Save(tok.RefreshToken); err != nil {
			return "", err
		}
	}

	return c.tok.AccessToken, nil
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

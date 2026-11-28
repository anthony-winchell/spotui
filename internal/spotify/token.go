package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const tokenEndpoint = "https://accounts.spotify.com/api/token"

// Token is an OAuth token set for the Spotify Web API.
type Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	Scope        string
}

// tokenResponse is the JSON body returned by Spotify's token endpoint.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// TokenError is an error response from Spotify's token endpoint.
type TokenError struct {
	StatusCode  int
	Code        string // OAuth error code, e.g. "invalid_grant"; empty if the body wasn't OAuth JSON
	Description string
}

func (e *TokenError) Error() string {
	msg := fmt.Sprintf("spotify token endpoint: HTTP %d", e.StatusCode)
	if e.Code != "" {
		msg += ": " + e.Code
	}
	if e.Description != "" {
		msg += " (" + e.Description + ")"
	}
	return msg
}

func parseTokenError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))

	var e struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}

	_ = json.Unmarshal(body, &e)

	return &TokenError{
		StatusCode:  resp.StatusCode,
		Code:        e.Error,
		Description: e.Description,
	}
}

// exchangeCode trades an authorization code for tokens. PKCE: no client secret.
// redirect must be the redirect URI the login used.
func exchangeCode(ctx context.Context, hc *http.Client, tokenURL, clientID, redirect, code, verifier string) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirect)
	form.Set("client_id", clientID)
	form.Set("code_verifier", verifier)
	return requestToken(ctx, hc, tokenURL, form)
}

// requestToken POSTs form to the token endpoint and decodes the resulting token.
func requestToken(ctx context.Context, hc *http.Client, tokenURL string, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	start := time.Now()
	resp, err := hc.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Token{}, parseTokenError(resp)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return Token{}, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return Token{}, errors.New("token response is missing access_token")
	}

	return Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		Expiry:       start.Add(time.Duration(tr.ExpiresIn) * time.Second),
		Scope:        tr.Scope,
	}, nil
}

// Spotify may rotate the refresh token; if it doesn't send a new one, the old one stays valid
func refreshAccessToken(ctx context.Context, hc *http.Client, tokenURL, clientID, refreshToken string) (Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", clientID)

	tok, err := requestToken(ctx, hc, tokenURL, form)
	if err != nil {
		return Token{}, err
	}

	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}

	return tok, nil
}

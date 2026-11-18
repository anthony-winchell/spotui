package spotify

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

type callbackResult struct {
	code string
	err  error
}

var ErrStateMismatch = errors.New("state mismatch: ignoring callback")

// waitForCode serves the OAuth redirect on ln until one callback arrives or ctx ends.
func waitForCode(ctx context.Context, ln net.Listener, state string) (string, error) {
	results := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /callback", func(w http.ResponseWriter, r *http.Request) {
		code, err := parseCallback(r.URL.Query(), state)
		if errors.Is(err, ErrStateMismatch) {
			http.Error(w, "Invalid state", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "Login failed: "+err.Error(), http.StatusBadRequest)
		} else {
			fmt.Fprintln(w, "Logged in to Spotify. You can close this tab.")
		}

		select {
		case results <- callbackResult{code, err}:
		default: // already have a result (e.g. user refreshed the tab); drop it
		}
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())

	select {
	case res := <-results:
		return res.code, res.err
	case <-ctx.Done():
		return "", fmt.Errorf("waiting for Spotify login: %w", ctx.Err())
	}
}

// parseCallback validates the OAuth redirect query and returns the authorization code.
func parseCallback(q url.Values, wantState string) (string, error) {
	if q.Get("state") != wantState {
		return "", ErrStateMismatch
	}

	if e := q.Get("error"); e != "" {
		return "", fmt.Errorf("spotify authorization failed: %s", e)
	}

	code := q.Get("code")
	if code == "" {
		return "", errors.New("callback is missing the authorization code")
	}

	return code, nil
}

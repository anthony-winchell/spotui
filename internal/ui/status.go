package ui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// statusDuration is how long the status message stays up
const statusDuration = 4 * time.Second

// status is a short-lived message shown in place of hints
type status struct {
	text  string
	until time.Time
}

// set shows text for statusDuration
func (s *status) set(text string) {
	s.text = text
	s.until = time.Now().Add(statusDuration)
}

// visible reports whether the message should still be shown
func (s status) visible(now time.Time) bool {
	return s.text != "" && now.Before(s.until)
}

// errorText turns an error into a short-lived message for the user
func errorText(err error) string {
	var ae *spotify.APIError
	switch {
	case errors.Is(err, spotify.ErrNoActiveDevice):
		return "No active device: open Spotify and start playing something"
	case errors.Is(err, spotify.ErrPremiumRequired):
		return "That needs Spotify premium"
	case errors.Is(err, context.DeadlineExceeded):
		return "Spotify didn't respond in time"
	case errors.As(err, &ae) && ae.StatusCode == http.StatusTooManyRequests:
		return fmt.Sprintf("Rate limited by Spotify, trying again in %s", ae.RetryAfter.Round(time.Second))
	case errors.As(err, &ae) && ae.StatusCode == http.StatusForbidden:
		return "Spotify won't allow that on this device"
	case errors.As(err, &ae) && ae.Message != "":
		return "Spotify: " + ae.Message
	}
	return err.Error()
}

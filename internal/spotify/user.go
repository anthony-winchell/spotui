package spotify

import (
	"context"
	"net/http"
)

// User is the logged-in Spotify user's profile
type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// CurrentUser returns the profile of the logged-in user
func (c *Client) CurrentUser(ctx context.Context) (User, error) {
	var u User
	err := c.do(ctx, http.MethodGet, "/me", nil, nil, &u)
	return u, err
}

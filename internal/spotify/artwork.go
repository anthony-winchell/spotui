package spotify

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg" // registers the JPEG decoder with image.Decode
	"io"
	"net/http"
)

// maxArtworkBytes limits how much of an artwork response is read
const maxArtworkBytes = 2 << 20

// Artwork downloads and decodes the image at url
func (c *Client) Artwork(ctx context.Context, url string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build artwork request: %w", err)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get artwork: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get artwork: HTTP %d", resp.StatusCode)
	}

	img, _, err := image.Decode(io.LimitReader(resp.Body, maxArtworkBytes))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	return img, nil
}

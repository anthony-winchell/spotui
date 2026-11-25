package ui

import (
	"context"
	"image"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// artwork is the album artwork of the current track
type artwork struct {
	url     string      // "" if the track has none
	img     image.Image // nil until downloaded
	loading bool
	retryAt time.Time // after a failure, the earliest time to try again
}

// artworkMsg carries the result of downloading the artwork at url
type artworkMsg struct {
	url string
	img image.Image
	err error
}

// artworkCmd downloads the artwork at url
func artworkCmd(c *spotify.Client, url string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()

		img, err := c.Artwork(ctx, url)
		return artworkMsg{url: url, img: img, err: err}
	}
}

// fetchArtwork starts downloading the current track's artwork, unless we
// already have it, are already downloading it, or recently failed to
func (m *Model) fetchArtwork(now time.Time) tea.Cmd {
	if m.playback.Track == nil {
		return nil
	}

	if url := m.playback.Track.ArtworkURL; url != m.art.url {
		m.art = artwork{url: url}
	}

	if m.art.url == "" || m.art.img != nil || m.art.loading || now.Before(m.art.retryAt) {
		return nil
	}

	m.art.loading = true
	return artworkCmd(m.client, m.art.url)
}

// renderArtwork draws img as width x height cells of half blocks, with two
// pixels per cell
func renderArtwork(img image.Image, width, height int) string {
	b := img.Bounds()
	rows := height * 2 // pixel rows

	// pixel returns the average color of the part of img that (x, y) covers
	pixel := func(x, y int) color.Color {
		return averageColor(img, image.Rect(
			b.Min.X+x*b.Dx()/width, b.Min.Y+y*b.Dy()/rows,
			b.Min.X+(x+1)*b.Dx()/width, b.Min.Y+(y+1)*b.Dy()/rows,
		))
	}

	lines := make([]string, height)
	for row := range height {
		var line strings.Builder
		for x := range width {
			line.WriteString(lipgloss.NewStyle().
				Foreground(pixel(x, row*2)).
				Background(pixel(x, row*2+1)).
				Render("▀"))
		}
		lines[row] = line.String()
	}
	return strings.Join(lines, "\n")
}

// averageColor returns the average color of img within r
func averageColor(img image.Image, r image.Rectangle) color.Color {
	var sumR, sumG, sumB, n uint32
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			sumR += cr >> 8
			sumG += cg >> 8
			sumB += cb >> 8
			n++
		}
	}
	if n == 0 {
		return color.Black
	}
	return color.RGBA{R: uint8(sumR / n), G: uint8(sumG / n), B: uint8(sumB / n), A: 255}
}

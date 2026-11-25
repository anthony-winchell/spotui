package spotify

import "time"

// minArtworkWidth is the narrowest artwork worth downloading, in px
const minArtworkWidth = 64

// Track is a Spotify track.
type Track struct {
	ID         string
	URI        string // "spotify:track:..."; used to start playback
	Name       string
	Artists    []string
	Album      string
	ArtworkURL string // album artwork, "" if there is none
	Duration   time.Duration
}

// imageObject is Spotify's JSON image object
type imageObject struct {
	URL   string `json:"url"`
	Width int    `json:"width"`
}

// artworkURL returns the URL of the smallest image at least minArtworkWidth
// wide, falling back to the first image, or "" if there are none
func artworkURL(images []imageObject) string {
	if len(images) == 0 {
		return ""
	}

	best := images[0]
	for _, img := range images[1:] {
		if img.Width >= minArtworkWidth && (best.Width < minArtworkWidth || img.Width < best.Width) {
			best = img
		}
	}
	return best.URL
}

// trackObject is Spotify's JSON track object, shared by many endpoints
type trackObject struct {
	ID         string         `json:"id"`
	URI        string         `json:"uri"`
	Name       string         `json:"name"`
	DurationMS int            `json:"duration_ms"`
	Artists    []artistObject `json:"artists"`
	Album      struct {
		Name   string        `json:"name"`
		Images []imageObject `json:"images"`
	} `json:"album"`
}

func (t trackObject) toTrack() Track {
	return Track{
		ID:         t.ID,
		URI:        t.URI,
		Name:       t.Name,
		Artists:    artistNames(t.Artists),
		Album:      t.Album.Name,
		ArtworkURL: artworkURL(t.Album.Images),
		Duration:   time.Duration(t.DurationMS) * time.Millisecond,
	}
}

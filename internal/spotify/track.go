package spotify

import "time"

// minArtworkWidth is the narrowest artwork worth downloading, in px
const minArtworkWidth = 64

// Track is a Spotify track, or a podcast episode when Episode is set
type Track struct {
	ID         string
	URI        string // "spotify:track:..." or "spotify:episode:..."; used to start playback
	Name       string
	Artists    []string // for an episode, its show
	Album      string   // for an episode, the show's publisher
	AlbumURI   string   // "spotify:album:..."; "" for episodes, or if Spotify left it out
	ArtworkURL string   // album artwork, "" if there is none
	Duration   time.Duration
	Episode    bool
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

// trackObject is Spotify's JSON track object, shared by many endpoints.
// Endpoints that can return podcast episodes put them in the same place, so
// it also reads the fields an episode has instead of artists and an album
type trackObject struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"` // "track" or "episode"
	URI        string         `json:"uri"`
	Name       string         `json:"name"`
	DurationMS int            `json:"duration_ms"`
	Artists    []artistObject `json:"artists"`
	Album      struct {
		URI    string        `json:"uri"`
		Name   string        `json:"name"`
		Images []imageObject `json:"images"`
	} `json:"album"`

	// Episodes only
	Images []imageObject `json:"images"`
	Show   struct {
		Name      string        `json:"name"`
		Publisher string        `json:"publisher"`
		Images    []imageObject `json:"images"`
	} `json:"show"`
}

func (t trackObject) toTrack() Track {
	duration := time.Duration(t.DurationMS) * time.Millisecond

	if t.Type == "episode" {
		images := t.Images
		if len(images) == 0 {
			images = t.Show.Images
		}
		return Track{
			ID:         t.ID,
			URI:        t.URI,
			Name:       t.Name,
			Artists:    []string{t.Show.Name},
			Album:      t.Show.Publisher,
			ArtworkURL: artworkURL(images),
			Duration:   duration,
			Episode:    true,
		}
	}

	return Track{
		ID:         t.ID,
		URI:        t.URI,
		Name:       t.Name,
		Artists:    artistNames(t.Artists),
		Album:      t.Album.Name,
		AlbumURI:   t.Album.URI,
		ArtworkURL: artworkURL(t.Album.Images),
		Duration:   duration,
	}
}

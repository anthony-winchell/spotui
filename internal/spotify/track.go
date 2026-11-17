package spotify

import "time"

// Track is a Spotify track.
type Track struct {
	ID       string
	URI      string // "spotify:track:..."; used to start playback
	Name     string
	Artists  []string
	Album    string
	Duration time.Duration
}

// trackObject is Spotify's JSON track object, shared by many endpoints
type trackObject struct {
	ID         string `json:"id"`
	URI        string `json:"uri"`
	Name       string `json:"name"`
	DurationMS int    `json:"duration_ms"`
	Artists    []struct {
		Name string `json:"name"`
	} `json:"artists"`
	Album struct {
		Name string `json:"name"`
	} `json:"album"`
}

func (t trackObject) toTrack() Track {
	artists := make([]string, len(t.Artists))
	for i, a := range t.Artists {
		artists[i] = a.Name
	}

	return Track{
		ID:       t.ID,
		URI:      t.URI,
		Name:     t.Name,
		Artists:  artists,
		Album:    t.Album.Name,
		Duration: time.Duration(t.DurationMS) * time.Millisecond,
	}
}

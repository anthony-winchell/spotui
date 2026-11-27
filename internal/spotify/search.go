package spotify

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// searchPageSize is the most results per type that Spotify returns to
// Development Mode apps (lowered from 50)
const searchPageSize = 10

// SearchType is a kind of search result
type SearchType string

const (
	SearchTracks  SearchType = "track"
	SearchArtists SearchType = "artist"
	SearchAlbums  SearchType = "album"
)

// Artist is a Spotify artist
type Artist struct {
	ID   string
	URI  string // "spotify:artist:..."; can be played as a context
	Name string
}

// Album is a Spotify Album
type Album struct {
	ID      string
	URI     string // "spotify:album:..."; can be played as context
	Name    string
	Artists []string
}

// artistObject is Spotify's JSON simplified artist object
type artistObject struct {
	ID   string `json:"id"`
	URI  string `json:"uri"`
	Name string `json:"name"`
}

func (a artistObject) toArtist() Artist {
	return Artist{
		ID:   a.ID,
		URI:  a.URI,
		Name: a.Name,
	}
}

// albumObject is Spotify's JSON simplified album object
type albumObject struct {
	ID      string         `json:"id"`
	URI     string         `json:"uri"`
	Name    string         `json:"name"`
	Artists []artistObject `json:"artists"`
}

func (a albumObject) toAlbum() Album {
	return Album{
		ID:      a.ID,
		URI:     a.URI,
		Name:    a.Name,
		Artists: artistNames(a.Artists),
	}
}

// artistNames returns just the names of a list of artists
func artistNames(as []artistObject) []string {
	names := make([]string, len(as))
	for i, a := range as {
		names[i] = a.Name
	}
	return names
}

// SearchResults holds one page of each result type
type SearchResults struct {
	Tracks  Page[Track]
	Artists Page[Artist]
	Albums  Page[Album]
}

// searchResponse is Spotify's JSON search response; one paging object per type
type searchResponse struct {
	Tracks  pagingResponse[trackObject]  `json:"tracks"`
	Artists pagingResponse[artistObject] `json:"artists"`
	Albums  pagingResponse[albumObject]  `json:"albums"`
}

// Search finds tracks, artists and albums matching query, starting at offset.
// The query may use Spotify's field filters, such as "artists:radiohead year:1997".
func (c *Client) Search(ctx context.Context, query string, offset int, types ...SearchType) (SearchResults, error) {
	if len(types) == 0 {
		types = []SearchType{SearchTracks, SearchArtists, SearchAlbums}
	}

	names := make([]string, len(types))
	for i, t := range types {
		names[i] = string(t)
	}

	q := url.Values{
		"q":      {query},
		"type":   {strings.Join(names, ",")},
		"limit":  {strconv.Itoa(searchPageSize)},
		"offset": {strconv.Itoa(offset)},
	}

	var sr searchResponse
	if err := c.do(ctx, http.MethodGet, "/search", q, nil, &sr); err != nil {
		return SearchResults{}, err
	}

	return SearchResults{
		Tracks:  mapPage(sr.Tracks, trackObject.toTrack),
		Artists: mapPage(sr.Artists, artistObject.toArtist),
		Albums:  mapPage(sr.Albums, albumObject.toAlbum),
	}, nil
}

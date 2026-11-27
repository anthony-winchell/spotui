package main

// Commands and JSON output for scripts and the Neovim plugin. Everything
// printed with --json is a single JSON value on stdout; errors go to stderr
// with exit code 1, like every other command.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

// likedListLimit is the most liked songs `liked --json` returns, to keep the
// picker quick to open (each page of 50 is one request)
const likedListLimit = 200

// cutFlag removes flag from args, reporting whether it was there
func cutFlag(args []string, flag string) ([]string, bool) {
	i := slices.Index(args, flag)
	if i < 0 {
		return args, false
	}
	return slices.Delete(slices.Clone(args), i, i+1), true
}

// printJSON writes v to stdout as JSON
func printJSON(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}

// statusJSON is the output of `status --json`
type statusJSON struct {
	Active     bool     `json:"active"` // false when no device is playing; the rest is then empty
	Playing    bool     `json:"playing"`
	Track      string   `json:"track,omitempty"`
	Artists    []string `json:"artists,omitempty"`
	Album      string   `json:"album,omitempty"`
	URI        string   `json:"uri,omitempty"`
	ProgressMS int64    `json:"progress_ms"`
	DurationMS int64    `json:"duration_ms"`
	Device     string   `json:"device,omitempty"`
	Volume     int      `json:"volume"`
	Shuffle    bool     `json:"shuffle"`
	Repeat     string   `json:"repeat"`
}

// printStatusJSON prints the playback state. No active device isn't an error
// here: it's an ordinary state for a statusline to show.
func printStatusJSON(ctx context.Context, c *spotify.Client) error {
	st, err := c.Playback(ctx)
	if errors.Is(err, spotify.ErrNoActiveDevice) {
		return printJSON(statusJSON{})
	}
	if err != nil {
		return err
	}

	out := statusJSON{
		Active:     true,
		Playing:    st.Playing,
		ProgressMS: st.ProgressAt(time.Now()).Milliseconds(),
		Device:     st.Device.Name,
		Volume:     st.Device.Volume,
		Shuffle:    st.Shuffle,
		Repeat:     string(st.Repeat),
	}
	if t := st.Track; t != nil {
		out.Track, out.Artists, out.Album, out.URI = t.Name, t.Artists, t.Album, t.URI
		out.DurationMS = t.Duration.Milliseconds()
	}
	return printJSON(out)
}

// itemJSON is one playable thing in a list: a track, artist, album or playlist
type itemJSON struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// label formats a track or album as "Name — Artist, Artist"
func label(name string, artists []string) string {
	return name + " — " + strings.Join(artists, ", ")
}

// printSearchJSON prints the first page of each result type, as one list
func printSearchJSON(ctx context.Context, c *spotify.Client, query string) error {
	res, err := c.Search(ctx, query, 0)
	if err != nil {
		return err
	}

	items := make([]itemJSON, 0, len(res.Tracks.Items)+len(res.Artists.Items)+len(res.Albums.Items))
	for _, t := range res.Tracks.Items {
		items = append(items, itemJSON{"track", label(t.Name, t.Artists), t.URI})
	}
	for _, a := range res.Artists.Items {
		items = append(items, itemJSON{"artist", a.Name, a.URI})
	}
	for _, a := range res.Albums.Items {
		items = append(items, itemJSON{"album", label(a.Name, a.Artists), a.URI})
	}
	return printJSON(items)
}

// printPlaylistsJSON prints every playlist the user has
func printPlaylistsJSON(ctx context.Context, c *spotify.Client) error {
	items := []itemJSON{}
	for offset := 0; ; {
		page, err := c.Playlists(ctx, offset)
		if err != nil {
			return err
		}
		for _, p := range page.Items {
			items = append(items, itemJSON{"playlist", p.Name, p.URI})
		}
		if !page.HasNext || len(page.Items) == 0 {
			return printJSON(items)
		}
		offset += len(page.Items)
	}
}

// printLikedJSON prints up to likedListLimit liked songs, newest first. A
// song's position in the list is its number for `liked N` (1-based).
func printLikedJSON(ctx context.Context, c *spotify.Client) error {
	items := []itemJSON{}
	for len(items) < likedListLimit {
		page, err := c.LikedSongs(ctx, len(items))
		if err != nil {
			return err
		}
		for _, t := range page.Items {
			items = append(items, itemJSON{"track", label(t.Name, t.Artists), t.URI})
		}
		if !page.HasNext || len(page.Items) == 0 {
			break
		}
	}
	return printJSON(items[:min(len(items), likedListLimit)])
}

// playURI plays a track on its own, or an album, artist or playlist from the start
func playURI(ctx context.Context, c *spotify.Client, uri string) error {
	if strings.HasPrefix(uri, "spotify:track:") {
		return c.PlayTracks(ctx, []string{uri}, 0)
	}
	return c.PlayContext(ctx, uri, 0)
}

// setLike likes or unlikes the current track; "togglelike" flips whatever it is now
func setLike(ctx context.Context, c *spotify.Client, mode string) error {
	st, err := c.Playback(ctx)
	if err != nil {
		return err
	}
	if st.Track == nil {
		return errors.New("nothing playing")
	}
	t := st.Track

	want := mode == "like"
	if mode == "togglelike" {
		liked, err := c.Liked(ctx, t.URI)
		if err != nil {
			return err
		}
		if len(liked) == 0 {
			return errors.New("Spotify didn't say whether the track is liked")
		}
		want = !liked[0]
	}

	if want {
		err = c.Like(ctx, t.URI)
	} else {
		err = c.Unlike(ctx, t.URI)
	}
	if err != nil {
		return err
	}

	if want {
		fmt.Printf("Liked %s\n", t.Name)
	} else {
		fmt.Printf("Removed %s from Liked Songs\n", t.Name)
	}
	return nil
}

// toggleShuffle flips shuffle and prints the new state
func toggleShuffle(ctx context.Context, c *spotify.Client) error {
	st, err := c.Playback(ctx)
	if err != nil {
		return err
	}
	if err := c.SetShuffle(ctx, !st.Shuffle); err != nil {
		return err
	}
	fmt.Println("shuffle", map[bool]string{true: "on", false: "off"}[!st.Shuffle])
	return nil
}

// cycleRepeat moves repeat to the next mode (off, context, track) and prints it
func cycleRepeat(ctx context.Context, c *spotify.Client) error {
	st, err := c.Playback(ctx)
	if err != nil {
		return err
	}

	next := spotify.RepeatContext
	switch st.Repeat {
	case spotify.RepeatContext:
		next = spotify.RepeatTrack
	case spotify.RepeatTrack:
		next = spotify.RepeatOff
	}

	if err := c.SetRepeat(ctx, next); err != nil {
		return err
	}
	fmt.Println("repeat", next)
	return nil
}

// amountArg parses args[1] as a number. A leading + or - makes it relative,
// e.g. "vol +10" or "seek -15".
func amountArg(args []string) (n int, relative bool, err error) {
	n, err = intArg(args)
	if err != nil {
		return 0, false, err
	}
	return n, strings.HasPrefix(args[1], "+") || strings.HasPrefix(args[1], "-"), nil
}

// setVolume sets the volume to a percentage, or changes it by one
func setVolume(ctx context.Context, c *spotify.Client, args []string) error {
	n, relative, err := amountArg(args)
	if err != nil {
		return err
	}
	if relative {
		st, err := c.Playback(ctx)
		if err != nil {
			return err
		}
		n += st.Device.Volume
	}
	return c.SetVolume(ctx, min(max(n, 0), 100))
}

// seek moves to a position in seconds, or by a number of seconds
func seek(ctx context.Context, c *spotify.Client, args []string) error {
	n, relative, err := amountArg(args)
	if err != nil {
		return err
	}
	pos := time.Duration(n) * time.Second
	if relative {
		st, err := c.Playback(ctx)
		if err != nil {
			return err
		}
		pos += st.ProgressAt(time.Now())
	}
	return c.Seek(ctx, max(pos, 0))
}

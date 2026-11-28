package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/anthony-winchell/spotui/internal/config"
	"github.com/anthony-winchell/spotui/internal/spotify"
	"github.com/anthony-winchell/spotui/internal/ui"
	"github.com/joho/godotenv"
)

func main() {
	// Help needs no config, so it works before anything is set up
	if len(os.Args) > 1 && isHelp(os.Args[1]) {
		fmt.Print(usage)
		return
	}

	// Dev convenience: load ./.env into the environment if present.
	// Real environment variables win; godotenv.Load never overrides them.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Fatal(err)
	}

	path, err := config.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		log.Fatal(err)
	}

	if len(os.Args) == 1 {
		if err := runTUI(cfg); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := runCLI(cfg, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "spotui:", cliErrorText(err))
		os.Exit(1)
	}
}

// runTUI logs in, through the browser if needed, and runs interactions
func runTUI(cfg config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute) // time to log in
	defer cancel()

	tok, err := spotify.Authenticate(ctx, cfg.ClientID, cfg.CallbackPort, spotify.KeyringStore{})
	if err != nil {
		return err
	}

	client := spotify.NewClient(cfg.ClientID, tok, spotify.KeyringStore{})
	me, err := client.CurrentUser(ctx)
	if err != nil {
		return err
	}

	_, err = tea.NewProgram(ui.New(client, me.ID)).Run()
	return err
}

// runCLI runs one command with the saved login. It prints nothing on success
// unless the command has output, so scripts and the Neovim plugin can use it
func runCLI(cfg config.Config, args []string) error {
	// These manage the saved login, so they run without one
	switch args[0] {
	case "login":
		return login(cfg)
	case "logout":
		return logout()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tok, err := spotify.Resume(ctx, cfg.ClientID, spotify.KeyringStore{})
	if err != nil {
		return err
	}

	client := spotify.NewClient(cfg.ClientID, tok, spotify.KeyringStore{})
	return runCommand(ctx, client, args)
}

// cliErrorText turns an error into a short one-line message
func cliErrorText(err error) string {
	switch {
	case errors.Is(err, spotify.ErrNeedsLogin), errors.Is(err, spotify.ErrLoginExpired):
		return "not logged in: run spotui to sign in"
	case errors.Is(err, spotify.ErrNoActiveDevice):
		return "no active device: open Spotify and start playback"
	case errors.Is(err, spotify.ErrPremiumRequired):
		return "playback control requires Spotify Premium"
	}
	return err.Error()
}

// runCommand runs one playback command from the command line.
func runCommand(ctx context.Context, c *spotify.Client, args []string) error {
	args, asJSON := cutFlag(args, "--json")
	if len(args) == 0 {
		return errors.New("no command given")
	}

	switch args[0] {
	case "status":
		if asJSON {
			return printStatusJSON(ctx, c)
		}
		return printStatus(ctx, c)
	case "play":
		return c.Play(ctx)
	case "pause":
		return c.Pause(ctx)
	case "toggle":
		st, err := c.Playback(ctx)
		if err != nil {
			return err
		}
		if st.Playing {
			return c.Pause(ctx)
		}
		return c.Play(ctx)
	case "next":
		return c.Next(ctx)
	case "prev":
		return c.Previous(ctx)
	case "seek":
		return seek(ctx, c, args)
	case "vol":
		return setVolume(ctx, c, args)
	case "shuffle":
		return toggleShuffle(ctx, c)
	case "repeat":
		return cycleRepeat(ctx, c)
	case "play-uri":
		if len(args) < 2 {
			return errors.New("play-uri needs a Spotify URI")
		}
		return playURI(ctx, c, args[1])
	case "playlists":
		if asJSON {
			return printPlaylistsJSON(ctx, c)
		}
		return printPlaylists(ctx, c)
	case "playlist":
		n, err := intArg(args)
		if err != nil {
			return err
		}
		return playPlaylist(ctx, c, n)
	case "liked":
		if asJSON {
			return printLikedJSON(ctx, c)
		}
		if len(args) == 1 {
			return printLikedSongs(ctx, c)
		}
		n, err := intArg(args)
		if err != nil {
			return err
		}
		return playLikedSongs(ctx, c, n)
	case "recent":
		return printRecent(ctx, c)
	case "search":
		if len(args) < 2 {
			return fmt.Errorf("search needs a query")
		}
		if asJSON {
			return printSearchJSON(ctx, c, strings.Join(args[1:], " "))
		}
		return printSearch(ctx, c, strings.Join(args[1:], " "))
	case "like", "unlike", "togglelike":
		return setLike(ctx, c, args[0])
	default:
		return fmt.Errorf("unknown command %q (see spotui help)", args[0])
	}
}

// intArg parses args[1] as an integer.
func intArg(args []string) (int, error) {
	if len(args) < 2 {
		return 0, fmt.Errorf("%s needs a number", args[0])
	}
	n, err := strconv.Atoi(args[1])
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a number", args[0], args[1])
	}
	return n, nil
}

// printStatus prints the current playback state.
func printStatus(ctx context.Context, c *spotify.Client) error {
	st, err := c.Playback(ctx)
	if err != nil {
		return err
	}
	if st.Track == nil {
		fmt.Println("Something is playing, but it isn't a track.")
		return nil
	}

	state := "paused"
	if st.Playing {
		state = "playing"
	}
	fmt.Printf("%s: %s by %s (%s / %s) on %s, volume %d%%\n",
		state, st.Track.Name, strings.Join(st.Track.Artists, ", "),
		mmss(st.ProgressAt(time.Now())), mmss(st.Track.Duration),
		st.Device.Name, st.Device.Volume)
	return nil
}

// printPlaylists prints all of the user's playlists, fetching every page
func printPlaylists(ctx context.Context, c *spotify.Client) error {
	offset := 0
	for {
		page, err := c.Playlists(ctx, offset)
		if err != nil {
			return err
		}

		for i, p := range page.Items {
			count := "?"
			if p.TrackCount >= 0 {
				count = strconv.Itoa(p.TrackCount)
			}
			fmt.Printf("%3d. %s (%s tracks, by %s)\n", page.Offset+i+1, p.Name, count, p.OwnerName)
		}

		if !page.HasNext || len(page.Items) == 0 {
			return nil
		}
		offset += len(page.Items)
	}
}

// playPlaylist plays the nth playlist (1-based), numbered as printPlaylists shows them
func playPlaylist(ctx context.Context, c *spotify.Client, n int) error {
	if n < 1 {
		return fmt.Errorf("playlist numbers start at 1")
	}

	page, err := c.Playlists(ctx, n-1)
	if err != nil {
		return err
	}

	if len(page.Items) == 0 {
		return fmt.Errorf("no playlist %d: you have %d", n, page.Total)
	}

	p := page.Items[0]
	fmt.Printf("playing %s\n", p.Name)
	return c.PlayContext(ctx, p.URI, 0)
}

// printLikedSongs prints the first page of the user's LikedSongs
func printLikedSongs(ctx context.Context, c *spotify.Client) error {
	page, err := c.LikedSongs(ctx, 0)
	if err != nil {
		return err
	}

	for i, t := range page.Items {
		fmt.Printf("%3d. %s by %s (%s)\n", i+1, t.Name, strings.Join(t.Artists, ", "), mmss(t.Duration))
	}

	fmt.Printf("showing %d of %d liked songs\n", len(page.Items), page.Total)
	return nil
}

// playLikedSongs plays Liked Songs from song n (1-based), continuing through
// the whole library like the TUI does
func playLikedSongs(ctx context.Context, c *spotify.Client, n int) error {
	if n < 1 {
		return fmt.Errorf("song numbers start at 1")
	}

	me, err := c.CurrentUser(ctx)
	if err != nil {
		return err
	}
	return c.PlayContext(ctx, spotify.LikedSongsURI(me.ID), n-1)
}

// printRecent prints the user's recently played tracks, newest first
func printRecent(ctx context.Context, c *spotify.Client) error {
	plays, err := c.RecentlyPlayed(ctx)
	if err != nil {
		return err
	}
	if len(plays) == 0 {
		fmt.Println("no recently played tracks")
		return nil
	}

	for i, p := range plays {
		fmt.Printf("%3d. %s %s by %s\n", i+1, p.PlayedAt.Local().Format("Mon 15:04"), p.Track.Name, strings.Join(p.Track.Artists, ", "))
	}

	return nil
}

// printSearch prints the first page of each search result type
func printSearch(ctx context.Context, c *spotify.Client, query string) error {
	res, err := c.Search(ctx, query, 0)
	if err != nil {
		return err
	}

	fmt.Printf("Tracks (%d):\n", res.Tracks.Total)
	for i, t := range res.Tracks.Items {
		fmt.Printf("%3d. %s by %s\n", i+1, t.Name, strings.Join(t.Artists, ", "))
	}

	fmt.Printf("\nArtists (%d):\n", res.Artists.Total)
	for i, a := range res.Artists.Items {
		fmt.Printf("%3d. %s\n", i+1, a.Name)
	}

	fmt.Printf("\nAlbums (%d):\n", res.Albums.Total)
	for i, a := range res.Albums.Items {
		fmt.Printf("%3d. %s by %s\n", i+1, a.Name, strings.Join(a.Artists, ", "))
	}

	return nil
}

// mmss formats d as m:ss
func mmss(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

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

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	tok, err := spotify.Authenticate(ctx, cfg.ClientID, spotify.KeyringStore{})
	if err != nil {
		log.Fatal(err)
	}

	client := spotify.NewClient(cfg.ClientID, tok, spotify.KeyringStore{})
	me, err := client.CurrentUser(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("logged in as %s\n", me.DisplayName)

	if len(os.Args) == 1 {
		if _, err := tea.NewProgram(ui.New(client)).Run(); err != nil {
			log.Fatal(err)
		}
		return
	}

	cmdErr := runCommand(ctx, client, os.Args[1:])

	switch {
	case errors.Is(cmdErr, spotify.ErrNoActiveDevice):
		fmt.Println("No active device: open Spotify and start playback")
	case errors.Is(cmdErr, spotify.ErrPremiumRequired):
		fmt.Println("Playback control requires Spotify Premium")
	case cmdErr != nil:
		log.Fatal(cmdErr)
	}
}

// runCommand runs one playback command from the command line.
func runCommand(ctx context.Context, c *spotify.Client, args []string) error {
	switch args[0] {
	case "status":
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
		secs, err := intArg(args)
		if err != nil {
			return err
		}
		return c.Seek(ctx, time.Duration(secs)*time.Second)
	case "vol":
		pct, err := intArg(args)
		if err != nil {
			return err
		}
		return c.SetVolume(ctx, pct)
	case "playlists":
		return printPlaylists(ctx, c)
	case "liked":
		return printLikedSongs(ctx, c)
	default:
		return fmt.Errorf("unknown command %q", args[0])
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

// mmss formats d as m:ss
func mmss(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

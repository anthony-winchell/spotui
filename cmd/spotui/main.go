package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"strings"
	"time"

	"github.com/anthony-winchell/spotui/internal/config"
	"github.com/anthony-winchell/spotui/internal/spotify"
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

	st, err := client.Playback(ctx)
	switch {
	case errors.Is(err, spotify.ErrNoActiveDevice):
		fmt.Println("Nothing playing: open spotify and start playback")
		return
	case err != nil:
		log.Fatal(err)
	case st.Track == nil:
		fmt.Println("Something is playing, but it isn't a track")
		return
	}

	state := "paused"

	if st.Playing {
		state = "playing"
	}

	fmt.Printf("%s: %s by %s (%s / %s) on %s, volume %d%%\n",
		state, st.Track.Name, strings.Join(st.Track.Artists, ", "),
		mmss(st.ProgressAt(time.Now())), mmss(st.Track.Duration),
		st.Device.Name, st.Device.Volume)
}

// mmss formats d as m:ss
func mmss(d time.Duration) string {
	d = d.Round(time.Second)
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

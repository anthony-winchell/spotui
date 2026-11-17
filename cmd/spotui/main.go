package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
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
}

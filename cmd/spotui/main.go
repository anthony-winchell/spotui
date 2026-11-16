package main

import (
	"context"
	"errors"
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

	if err := spotify.Login(ctx, cfg.ClientID); err != nil {
		log.Fatal(err)
	}
}

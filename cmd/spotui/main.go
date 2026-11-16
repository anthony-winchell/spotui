package main

import (
	"log"
	"os"

	"github.com/anthony-winchell/spotui/internal/spotify"
)

func main() {
	clientID := os.Getenv("SPOTUI_CLIENT_ID")
	if clientID == "" {
		log.Fatal("Client ID could not be loaded")
	}

	if err := spotify.Login(clientID); err != nil {
		log.Fatal(err)
	}
}

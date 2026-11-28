package main

// Help, and the commands that manage the saved login

import (
	"context"
	"fmt"
	"time"

	"github.com/anthony-winchell/spotui/internal/config"
	"github.com/anthony-winchell/spotui/internal/spotify"
)

// loginTimeout is how long `spotui login` waits for the browser login
const loginTimeout = 3 * time.Minute

const usage = `usage: spotui [command] [arguments] [--json]

With no command, spotui opens the terminal UI, signing in through the
browser first if there's no saved login. Commands use the saved login.

Playback
  status               what's playing
  play, pause, toggle
  next, prev
  seek N | +N | -N     go to N seconds in, or move by N seconds
  vol N | +N | -N      set the volume to N%, or change it by N
  shuffle              turn shuffle on or off
  repeat               cycle repeat: off, all, one
  play-uri URI [CTX]   play a track, album, artist or playlist URI; a track
                       plays within CTX (an album or playlist URI) if given
  queue                list what's up next
  queue URI            add a track or episode to the queue

Library
  playlists            list your playlists
  playlist N           play playlist N, numbered as playlists lists them
  liked                list the first page of Liked Songs
  liked N              play Liked Songs from song N
  recent               list recently played tracks
  search QUERY         search tracks, artists and albums
  like, unlike         add the playing song to Liked Songs, or remove it
  togglelike           like the playing song, or unlike it if it's liked

Account
  login                sign in through the browser, replacing any saved login
  logout               forget the saved login
  help                 show this

--json makes status, queue, playlists, liked, recent and search print JSON.
Settings go in config.toml in the spotui folder of your config directory:
client_id, callback_port, volume_step and seek_step.
`

// isHelp reports whether arg asks for help
func isHelp(arg string) bool {
	switch arg {
	case "help", "-h", "-help", "--help":
		return true
	}
	return false
}

// login signs in through the browser, replacing any saved login
func login(cfg config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), loginTimeout)
	defer cancel()

	tok, err := spotify.Login(ctx, cfg.ClientID, cfg.CallbackPort)
	if err != nil {
		return err
	}
	store := spotify.KeyringStore{}
	if err := store.Save(tok); err != nil {
		return err
	}

	me, err := spotify.NewClient(cfg.ClientID, tok, store).CurrentUser(ctx)
	if err != nil {
		return err
	}
	name := me.DisplayName
	if name == "" {
		name = me.ID
	}
	fmt.Println("logged in as", name)
	return nil
}

// logout deletes the saved login. Spotify can't be asked to revoke a token,
// so the app keeps its access until the user removes it from their account.
func logout() error {
	if err := (spotify.KeyringStore{}).Clear(); err != nil {
		return err
	}
	fmt.Println("logged out. To revoke spotui's access too, remove it at https://www.spotify.com/account/apps/")
	return nil
}

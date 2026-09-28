// Command spotify-tui is a terminal remote control for Spotify. It talks to
// the Web API, so it needs a Spotify Premium account and a device already
// running Spotify to play on.
//
// Subcommands:
//
//	spotify-tui            run the interface
//	spotify-tui setup      store the client ID from the Spotify dashboard
//	spotify-tui logout     forget the cached token
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zmb3/spotify/v2"

	"github.com/Sousf/spotify-tui/internal/auth"
	"github.com/Sousf/spotify-tui/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "spotify-tui:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "setup":
			return setup()
		case "logout":
			if err := auth.Logout(); err != nil {
				return err
			}
			fmt.Println("Token removed. The next run will open the browser to log in.")
			return nil
		case "-h", "--help", "help":
			fmt.Print(usage)
			return nil
		default:
			fmt.Print(usage)
			return fmt.Errorf("unknown command %q", os.Args[1])
		}
	}

	cfg, err := auth.LoadConfig()
	if err != nil {
		return err
	}
	if cfg.ClientID == "" {
		fmt.Print(setupHint)
		return setup()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpClient, err := auth.Login(ctx, cfg, os.Stderr)
	if err != nil {
		return err
	}
	client := spotify.New(httpClient, spotify.WithRetry(true))

	p := tea.NewProgram(ui.New(client), tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func setup() error {
	cfg, _ := auth.LoadConfig()
	dir, _ := auth.Dir()
	fmt.Println()
	fmt.Println("1. Go to https://developer.spotify.com/dashboard and create an app.")
	fmt.Printf("2. Add this Redirect URI exactly:  %s\n", auth.RedirectURL(cfg.Port))
	fmt.Println("3. Tick \"Web API\", save, and copy the Client ID.")
	fmt.Println()
	fmt.Print("Client ID: ")
	rd := bufio.NewReader(os.Stdin)
	line, err := rd.ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	id := strings.TrimSpace(line)
	if id == "" {
		return fmt.Errorf("no client ID entered")
	}
	cfg.ClientID = id
	if err := auth.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("Saved to %s/config.json. Run spotify-tui to log in.\n", dir)
	return nil
}

const usage = `spotify-tui: a terminal remote for Spotify

  spotify-tui          run the interface
  spotify-tui setup    store the client ID from developer.spotify.com
  spotify-tui logout   forget the cached login

Environment:
  SPOTIFY_TUI_CLIENT_ID   overrides the client ID in config.json
  XDG_CONFIG_HOME         config lives in $XDG_CONFIG_HOME/spotify-tui
`

const setupHint = `No client ID configured yet. Spotify needs one per app, so let's set it up.
`

// Package auth handles the Spotify OAuth PKCE flow and token persistence.
//
// Spotify's PKCE flow needs only a client ID, so no secret ever touches disk.
// The token, including the refresh token Spotify rotates on every refresh, is
// stored under the user's config directory and rewritten whenever it changes.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

// Config is what the user has to supply once. It lives in config.json next to
// the token cache.
type Config struct {
	ClientID string `json:"client_id"`
	Port     int    `json:"port,omitempty"`
}

const defaultPort = 8888

var scopes = []string{
	spotifyauth.ScopeUserReadPlaybackState,
	spotifyauth.ScopeUserModifyPlaybackState,
	spotifyauth.ScopeUserReadCurrentlyPlaying,
	spotifyauth.ScopeUserReadPrivate,
	spotifyauth.ScopePlaylistReadPrivate,
	spotifyauth.ScopePlaylistReadCollaborative,
	spotifyauth.ScopeUserLibraryRead,
	spotifyauth.ScopeUserLibraryModify,
	spotifyauth.ScopeUserFollowRead,
	spotifyauth.ScopeUserReadRecentlyPlayed,
}

// Dir returns the directory holding config.json and token.json.
func Dir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "spotify-tui"), nil
}

// LoadConfig reads config.json. The SPOTIFY_TUI_CLIENT_ID environment
// variable overrides the file so the app can run without one.
func LoadConfig() (Config, error) {
	var cfg Config
	dir, err := Dir()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config.json: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}
	if v := os.Getenv("SPOTIFY_TUI_CLIENT_ID"); v != "" {
		cfg.ClientID = v
	}
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	return cfg, nil
}

// SaveConfig writes config.json, creating the directory if needed.
func SaveConfig(cfg Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600)
}

// RedirectURL is the loopback address the user must register in the Spotify
// developer dashboard. Spotify rejects "localhost", it has to be 127.0.0.1.
func RedirectURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", port)
}

func tokenPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "token.json"), nil
}

func loadToken() (*oauth2.Token, error) {
	p, err := tokenPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}

func saveToken(tok *oauth2.Token) error {
	p, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// Logout deletes the cached token so the next run goes through the browser.
func Logout() error {
	p, err := tokenPath()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// persistingSource refreshes the token through the authenticator when it
// expires and writes every fresh token to disk. Spotify rotates the refresh
// token on each use, so losing one means logging in again.
type persistingSource struct {
	mu    sync.Mutex
	ctx   context.Context
	authr *spotifyauth.Authenticator
	tok   *oauth2.Token
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tok.Valid() {
		return p.tok, nil
	}
	tok, err := p.authr.RefreshToken(p.ctx, p.tok)
	if err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = p.tok.RefreshToken
	}
	p.tok = tok
	if err := saveToken(tok); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}
	return tok, nil
}

// Login returns an HTTP client that authenticates as the user. It reuses the
// cached token when there is one and otherwise runs the browser flow, writing
// progress to out.
func Login(ctx context.Context, cfg Config, out io.Writer) (*http.Client, error) {
	if cfg.ClientID == "" {
		return nil, errors.New("no client ID configured")
	}
	authr := spotifyauth.New(
		spotifyauth.WithClientID(cfg.ClientID),
		spotifyauth.WithRedirectURL(RedirectURL(cfg.Port)),
		spotifyauth.WithScopes(scopes...),
	)

	tok, err := loadToken()
	if err != nil {
		tok, err = browserFlow(ctx, authr, cfg.Port, out)
		if err != nil {
			return nil, err
		}
		if err := saveToken(tok); err != nil {
			return nil, err
		}
	}

	src := &persistingSource{ctx: ctx, authr: authr, tok: tok}
	return oauth2.NewClient(ctx, src), nil
}

func browserFlow(ctx context.Context, authr *spotifyauth.Authenticator, port int, out io.Writer) (*oauth2.Token, error) {
	verifier := oauth2.GenerateVerifier()
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, err
	}
	state := hex.EncodeToString(stateBytes)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("listen on port %d for the OAuth callback: %w", port, err)
	}
	defer ln.Close()

	type result struct {
		tok *oauth2.Token
		err error
	}
	done := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		tok, err := authr.Token(r.Context(), state, r, oauth2.VerifierOption(verifier))
		if err != nil {
			http.Error(w, "Login failed: "+err.Error(), http.StatusBadRequest)
		} else {
			fmt.Fprint(w, "<html><body style='font-family:sans-serif'><h2>Logged in.</h2><p>You can close this tab and go back to the terminal.</p></body></html>")
		}
		select {
		case done <- result{tok, err}:
		default:
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer srv.Close()

	url := authr.AuthURL(state, oauth2.S256ChallengeOption(verifier))
	fmt.Fprintln(out, "Opening Spotify login in your browser. If nothing happens, open this URL:")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  "+url)
	fmt.Fprintln(out)
	openBrowser(url)

	select {
	case r := <-done:
		return r.tok, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Minute):
		return nil, errors.New("timed out waiting for the login callback")
	}
}

func openBrowser(url string) {
	for _, bin := range []string{"xdg-open", "open"} {
		if p, err := exec.LookPath(bin); err == nil {
			_ = exec.Command(p, url).Start()
			return
		}
	}
}

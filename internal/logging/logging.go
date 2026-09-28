// Package logging sets up the log file every run writes to. The file is
// truncated on start so it only ever holds the most recent session, which is
// what you want when something just went wrong.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Path returns the log file location: $XDG_STATE_HOME/spotify-tui/spotify-tui.log,
// defaulting to ~/.local/state.
func Path() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "spotify-tui", "spotify-tui.log"), nil
}

// Setup opens the log file and installs it as the slog default. Debug level
// is enabled by SPOTIFY_TUI_DEBUG=1. The returned closer flushes the file.
func Setup() (io.Closer, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	level := slog.LevelInfo
	if os.Getenv("SPOTIFY_TUI_DEBUG") != "" {
		level = slog.LevelDebug
	}
	h := slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
	slog.Info("start", "args", os.Args[1:], "pid", os.Getpid(), "debug", level == slog.LevelDebug)
	return f, nil
}

// Transport logs every HTTP request the app makes: method, path, status and
// duration, plus the response body of failures. It sits outside the OAuth
// transport so the Authorization header never reaches the log.
type Transport struct {
	Base http.RoundTripper
}

func (t Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	start := time.Now()
	resp, err := base.RoundTrip(req)
	ms := time.Since(start).Milliseconds()
	target := req.URL.Path
	if q := req.URL.RawQuery; q != "" {
		target += "?" + q
	}
	if err != nil {
		slog.Error("http", "method", req.Method, "url", target, "ms", ms, "err", err)
		return nil, err
	}
	if resp.StatusCode/100 == 2 {
		slog.Debug("http", "method", req.Method, "url", target, "status", resp.StatusCode, "ms", ms)
		return resp, nil
	}
	// Read the error body so it can be logged, then hand it back untouched.
	body, rerr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if rerr != nil {
		body = []byte(fmt.Sprintf("<unreadable: %v>", rerr))
	}
	resp.Body = io.NopCloser(strings.NewReader(string(body)))
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 300 {
		snippet = snippet[:300] + "…"
	}
	slog.Warn("http", "method", req.Method, "url", target, "status", resp.StatusCode, "ms", ms, "body", snippet)
	return resp, nil
}

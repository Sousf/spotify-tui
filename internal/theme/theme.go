// Package theme reads the colour palette spotify-tui paints with.
//
// The palette lives in $XDG_CONFIG_HOME/spotify-tui/theme.toml as flat
// `key = "#rrggbb"` lines. Desktop theme switchers can regenerate that file
// and the running UI picks the change up within a couple of seconds. Without
// the file, or for any key it omits, the built-in Spotify-green look applies.
package theme

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Palette is every colour the UI uses, named after the role it plays. The
// names line up with the Catppuccin-style ramps most desktop themes expose
// so a template can map them one to one.
type Palette struct {
	Text     string // primary text
	Subtext  string // secondary text
	Overlay  string // dimmed text, column headers, hints
	Surface0 string // cursor row background
	Surface1 string // unfocused borders
	Surface2 string // progress track, empty meters
	Accent   string // focused border, playing track, progress fill
	Accent2  string // visualiser mid band
	Red      string // errors, visualiser top band
	Yellow   string // status notices
	Green    string // reserved for meters
	Peach    string // reserved
}

// Default is the look with no theme file: Spotify green on neutral greys.
func Default() Palette {
	return Palette{
		Text:     "#f0f0f0",
		Subtext:  "#c8c8c8",
		Overlay:  "#6b6b6b",
		Surface0: "#2a2a2a",
		Surface1: "#3a3a3a",
		Surface2: "#3a3a3a",
		Accent:   "#1DB954",
		Accent2:  "#e5c07b",
		Red:      "#e06c75",
		Yellow:   "#e5c07b",
		Green:    "#1DB954",
		Peach:    "#ff9e6b",
	}
}

// Path is where the palette file is expected.
func Path() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "spotify-tui", "theme.toml"), nil
}

// ModTime returns the palette file's modification time, or zero if there is
// no file. Callers poll it to notice a theme switch.
func ModTime() time.Time {
	p, err := Path()
	if err != nil {
		return time.Time{}
	}
	st, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return st.ModTime()
}

// Load reads the palette file over the defaults. A missing file is not an
// error; a malformed line is.
func Load() (Palette, error) {
	p, err := Path()
	if err != nil {
		return Default(), err
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return Default(), err
	}
	defer f.Close()
	return Parse(f)
}

var (
	lineRe  = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*=\s*"(#[0-9A-Fa-f]{6})"$`)
	colorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

// Parse reads `key = "#rrggbb"` lines. Comments, blank lines and [section]
// headers are ignored, and unknown keys are skipped so a template can carry
// extra values for other consumers.
func Parse(r interface{ Read([]byte) (int, error) }) (Palette, error) {
	pal := Default()
	sc := bufio.NewScanner(r)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimSpace(line[:i]) // trailing comment
		}
		m := lineRe.FindStringSubmatch(line)
		if m == nil {
			return Default(), fmt.Errorf("theme.toml line %d: want key = \"#rrggbb\", got %q", n, line)
		}
		pal.set(strings.ToLower(m[1]), m[2])
	}
	return pal, sc.Err()
}

func (p *Palette) set(key, val string) {
	if !colorRe.MatchString(val) {
		return
	}
	switch key {
	case "text":
		p.Text = val
	case "subtext":
		p.Subtext = val
	case "overlay", "overlay0":
		p.Overlay = val
	case "surface0":
		p.Surface0 = val
	case "surface1":
		p.Surface1 = val
	case "surface2":
		p.Surface2 = val
	case "accent":
		p.Accent = val
	case "accent2":
		p.Accent2 = val
	case "red":
		p.Red = val
	case "yellow":
		p.Yellow = val
	case "green":
		p.Green = val
	case "peach":
		p.Peach = val
	}
}

package ui

import (
	"fmt"
	"os"
	"testing"
)

// TestDump prints a frame for eyeballing: SPOTIFY_TUI_DUMP=1 go test -run TestDump -v ./internal/ui
func TestDump(t *testing.T) {
	if os.Getenv("SPOTIFY_TUI_DUMP") == "" {
		t.Skip("set SPOTIFY_TUI_DUMP=1 to print frames")
	}
	m := fakeModel(110, 28)
	m.focus = focusContent
	m.top().cursor = 3
	fmt.Println(m.View())
	m.showHelp = true
	fmt.Println(m.View())
}

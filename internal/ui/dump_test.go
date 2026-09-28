package ui

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/Sousf/spotify-tui/internal/viz"
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
	m.showHelp = false
	m.showViz = true
	n := m.vizBarCount()
	m.vizLevels = make([]float64, n)
	m.vizPeaks = make([]float64, n)
	for i := range m.vizLevels {
		m.vizLevels[i] = 0.5 + 0.5*math.Sin(float64(i)/3)
		m.vizPeaks[i] = m.vizLevels[i] + 0.15
	}
	m.viz = &viz.Capture{}
	fmt.Println(m.View())
}

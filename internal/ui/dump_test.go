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
	m.vizMode = vizBars
	n := m.vizBarCount()
	m.vizLevels = make([]float64, n)
	m.vizPeaks = make([]float64, n)
	for i := range m.vizLevels {
		m.vizLevels[i] = 0.5 + 0.5*math.Sin(float64(i)/3)
		m.vizPeaks[i] = m.vizLevels[i] + 0.15
	}
	m.viz = &viz.Capture{}
	fmt.Println(m.View())
	m.vizMode = vizKeys
	m.vizSource = "spotifyd"
	m.vizKeys = make([]float64, viz.Keys)
	for _, k := range []int{27, 39, 43, 46, 51, 60} { // C3 and a C major with extras
		m.vizKeys[k] = 1
	}
	m.vizKeys[40], m.vizKeys[49] = 0.5, 0.25 // C#4 and A#4 fading
	fmt.Println(m.View())
	m.vizMode = vizRoll
	for i := 0; i < 30; i++ { // a rising scale with a held bass note
		snap := make([]float64, viz.Keys)
		snap[27] = 1
		snap[39+i%12] = 1
		snap[(39+i%12)-1] = 0.3
		m.vizRoll = append(m.vizRoll, snap)
	}
	fmt.Println(m.View())
}

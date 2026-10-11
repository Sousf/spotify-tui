package viz

import (
	"math/rand"
	"testing"
)

// TestSynthLightsTheChord runs the demo tune through the key detector: in
// the middle of the first bar the C major triad has to be among the lit keys.
func TestSynthLightsTheChord(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	x := make([]float64, ring)
	start := sampleRate // one second in, inside the first bar
	for i := range x {
		x[i] = synthSample(start+i, rng)
	}
	sal := detectKeys(keyMagnitudes(x))
	lit := 0
	for _, k := range synthChords[0] {
		if sal[k] > 0 {
			lit++
		}
	}
	if lit < 2 {
		t.Errorf("only %d of the chord's keys lit; saliences %v", lit, sal)
	}
}

package viz

import (
	"math"
	"math/rand"
	"testing"
)

// tone fills a ring-sized buffer with the given partials: pairs of
// (frequency, amplitude).
func tone(partials ...float64) []float64 {
	x := make([]float64, ring)
	for i := range x {
		t := float64(i) / sampleRate
		for p := 0; p+1 < len(partials); p += 2 {
			x[i] += partials[p+1] * math.Sin(2*math.Pi*partials[p]*t)
		}
	}
	return x
}

func loudest(v []float64) int {
	best := 0
	for i := range v {
		if v[i] > v[best] {
			best = i
		}
	}
	return best
}

func TestKeyFreq(t *testing.T) {
	if f := KeyFreq(48); math.Abs(f-440) > 1e-9 {
		t.Errorf("A4 = %f", f)
	}
	if f := KeyFreq(0); math.Abs(f-27.5) > 1e-6 {
		t.Errorf("A0 = %f", f)
	}
	if k := keyFor(261.63); k != 39 {
		t.Errorf("C4 -> key %d, want 39", k)
	}
}

func TestMagnitudesResolveNeighbours(t *testing.T) {
	for _, k := range []int{7, 20, 39, 48, 70, 84} { // E1, F2, C4, A4, G6, A7
		mag := keyMagnitudes(tone(KeyFreq(k), 0.5))
		if got := loudest(mag); got != k {
			t.Errorf("key %d: loudest %d", k, got)
			continue
		}
		// Below about C2 the window cannot be long enough to separate
		// neighbours; the peak pick in detectKeys still finds the right
		// key, so only the louder half of the keyboard is held to this.
		for _, j := range []int{k - 1, k + 1} {
			if k >= 15 && j >= 0 && j < Keys && mag[j] > 0.5*mag[k] {
				t.Errorf("key %d: neighbour %d at %.2f of peak", k, j, mag[j]/mag[k])
			}
		}
		if sal := detectKeys(mag); sal == nil || countLit(sal) != 1 || loudest(sal) != k {
			t.Errorf("key %d: detector lit %v", k, sal)
		}
		if mag[k] < 0.35 || mag[k] > 0.65 {
			t.Errorf("key %d: magnitude %.2f for a 0.5 sine", k, mag[k])
		}
	}
}

func TestDetectChord(t *testing.T) {
	c4, e4, g4 := 39, 43, 46
	sal := detectKeys(keyMagnitudes(tone(KeyFreq(c4), 0.3, KeyFreq(e4), 0.3, KeyFreq(g4), 0.3)))
	if sal == nil {
		t.Fatal("chord gated out as noise")
	}
	for k, v := range sal {
		switch k {
		case c4, e4, g4:
			if v <= 0 {
				t.Errorf("chord note %d not lit", k)
			}
		default:
			if v > 0 {
				t.Errorf("key %d lit at %.3f", k, v)
			}
		}
	}
}

func TestDetectSuppressesHarmonics(t *testing.T) {
	c3 := 27
	f := KeyFreq(c3)
	// A plucked string: fundamental with 1/h harmonics up to the 6th.
	sal := detectKeys(keyMagnitudes(tone(f, 0.5, 2*f, 0.25, 3*f, 0.17, 4*f, 0.125, 5*f, 0.1, 6*f, 0.08)))
	if sal == nil {
		t.Fatal("gated out")
	}
	if loudest(sal) != c3 {
		t.Fatalf("loudest key %d, want C3 (%d)", loudest(sal), c3)
	}
	for h, name := range map[int]string{2: "octave", 3: "twelfth", 4: "double octave"} {
		k := keyFor(f * float64(h))
		if sal[k] > 0.25*sal[c3] {
			t.Errorf("%s (key %d) at %.2f of the fundamental, want it discounted", name, k, sal[k]/sal[c3])
		}
	}
}

func TestDetectRejectsNoise(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	x := make([]float64, ring)
	for i := range x {
		x[i] = r.Float64()*0.6 - 0.3
	}
	if sal := detectKeys(keyMagnitudes(x)); sal != nil {
		t.Errorf("noise lit %d keys", countLit(sal))
	}
	if sal := detectKeys(keyMagnitudes(make([]float64, ring))); sal != nil {
		t.Error("silence lit keys")
	}
}

func TestDetectNoteOverNoise(t *testing.T) {
	// A note over quieter drums should still come through.
	r := rand.New(rand.NewSource(2))
	x := tone(KeyFreq(48), 0.4)
	for i := range x {
		x[i] += r.Float64()*0.1 - 0.05
	}
	sal := detectKeys(keyMagnitudes(x))
	if sal == nil || loudest(sal) != 48 {
		t.Fatalf("A4 over noise: %v", sal)
	}
}

func TestKeysLightAndFade(t *testing.T) {
	c := &Capture{}
	x := tone(KeyFreq(48), 0.5)
	copy(c.samples[:], x)
	c.pos = 0 // buffer is exactly full; recent(ring) wraps to the start
	first := c.Keys()
	if first[48] < 0.99 {
		t.Fatalf("A4 level %.2f after gain, want 1", first[48])
	}
	if n := countLit(first); n != 1 {
		t.Errorf("%d keys lit, want 1", n)
	}
	c.clear()
	second := c.Keys()
	if second[48] >= first[48] || second[48] < 0.5 {
		t.Errorf("after one silent frame A4 = %.2f, want a partial fade", second[48])
	}
}

func countLit(v []float64) int {
	n := 0
	for _, x := range v {
		if x > 0 {
			n++
		}
	}
	return n
}

func BenchmarkKeys(b *testing.B) {
	c := &Capture{}
	copy(c.samples[:], tone(KeyFreq(39), 0.3, KeyFreq(43), 0.3, KeyFreq(46), 0.3))
	for i := 0; i < b.N; i++ {
		c.Keys()
	}
}

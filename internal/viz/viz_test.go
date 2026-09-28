package viz

import (
	"math"
	"os"
	"testing"
	"time"
)

func TestFFTMatchesDFT(t *testing.T) {
	n := 64
	re := make([]float64, n)
	im := make([]float64, n)
	for i := range re {
		re[i] = math.Sin(float64(i)*0.3) + 0.5*math.Cos(float64(i)*1.1)
	}
	want := make([]complex128, n)
	for k := 0; k < n; k++ {
		for i := 0; i < n; i++ {
			ang := -2 * math.Pi * float64(k*i) / float64(n)
			want[k] += complex(re[i]*math.Cos(ang), re[i]*math.Sin(ang))
		}
	}
	fft(re, im)
	for k := 0; k < n; k++ {
		if math.Abs(re[k]-real(want[k])) > 1e-9 || math.Abs(im[k]-imag(want[k])) > 1e-9 {
			t.Fatalf("bin %d: got (%f,%f) want (%f,%f)", k, re[k], im[k], real(want[k]), imag(want[k]))
		}
	}
}

func TestSpectrumPutsToneInRightBand(t *testing.T) {
	const bars = 32
	for _, hz := range []float64{100, 1000, 8000} {
		samples := make([]float64, window)
		for i := range samples {
			samples[i] = math.Sin(2 * math.Pi * hz * float64(i) / sampleRate)
		}
		out := spectrum(samples, bars)
		best := 0
		for i, v := range out {
			if v > out[best] {
				best = i
			}
		}
		// Expected bar from the log spacing used in spectrum.
		// A tone right on a band edge can land in either neighbour.
		want := int(math.Log(hz/minHz) / math.Log(maxHz/minHz) * bars)
		if best < want-1 || best > want+1 {
			t.Errorf("%g Hz: loudest bar %d, want %d", hz, best, want)
		}
	}
}

func TestBarsSmoothAndDecay(t *testing.T) {
	c := &Capture{}
	for i := range c.samples {
		c.samples[i] = math.Sin(2 * math.Pi * 440 * float64(i) / sampleRate)
	}
	first, _ := c.Bars(16)
	loud := 0
	for i, v := range first {
		if v > first[loud] {
			loud = i
		}
	}
	if first[loud] < 0.9 {
		t.Fatalf("loudest bar %f, want near 1 after gain", first[loud])
	}
	// Silence: the bar should fall but not vanish on the next frame.
	for i := range c.samples {
		c.samples[i] = 0
	}
	second, peaks := c.Bars(16)
	if second[loud] >= first[loud] || second[loud] < 0.5 {
		t.Errorf("bar after one silent frame = %f, want a partial decay from %f", second[loud], first[loud])
	}
	if peaks[loud] < first[loud]-0.03 {
		t.Errorf("peak fell too fast: %f", peaks[loud])
	}
}

// TestLiveCapture starts parec against the real sound server. Gated because
// CI boxes have no PipeWire.
func TestLiveCapture(t *testing.T) {
	if os.Getenv("SPOTIFY_TUI_LIVE") == "" {
		t.Skip("set SPOTIFY_TUI_LIVE=1")
	}
	c, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	time.Sleep(400 * time.Millisecond)
	if err := c.Err(); err != nil {
		t.Fatal(err)
	}
	levels, _ := c.Bars(16)
	t.Logf("levels after 400ms: %.2f", levels)
}

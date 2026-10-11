package viz

import (
	"math"
	"slices"
)

// Piano key detection. Each of the 88 keys gets a Hann-windowed Goertzel
// filter at its exact pitch, run over a window long enough to separate it
// from its neighbours: low notes need a long one, high notes a short one, so
// the display follows a melody quickly while still resolving a bassline.
// What stands out from the noise floor, after the harmonics of louder notes
// below are discounted, is lit.

const (
	// Keys covers MIDI notes 21 (A0) to 108 (C8).
	Keys     = 88
	lowMidi  = 21
	minKeyN  = 1024
	semitone = 1.0594630943592953 // 2^(1/12)
)

// KeyFreq is the pitch of key k (0 = A0) in hertz, A4 = 440.
func KeyFreq(k int) float64 {
	return 440 * math.Pow(2, float64(k+lowMidi-69)/12)
}

// keyWindow is how many samples the filter for key k looks at: enough that
// the Hann main lobe (two bins either side) is narrower than a semitone,
// within the ring buffer and never shorter than minKeyN.
func keyWindow(k int) int {
	f := KeyFreq(k)
	n := int(2 * sampleRate / (f * (semitone - 1)))
	if n > ring {
		n = ring
	}
	if n < minKeyN {
		n = minKeyN
	}
	return n
}

// goertzel returns the normalised magnitude of frequency f in the most
// recent n samples of x, Hann windowed.
func goertzel(x []float64, n int, f float64) float64 {
	x = x[len(x)-n:]
	w := 2 * math.Pi * f / sampleRate
	coeff := 2 * math.Cos(w)
	// The Hann window is 0.5*(1-cos(θ)) with θ stepping by d each sample;
	// cos(θ) comes from the recurrence cos(θ+d) = 2cos(d)cos(θ) - cos(θ-d)
	// rather than a cosine per sample.
	d := 2 * math.Pi / float64(n-1)
	cd := 2 * math.Cos(d)
	c1, c0 := math.Cos(-d), 1.0 // cos(θ-d), cos(θ) at θ=0
	var s0, s1, s2 float64
	for _, v := range x {
		s0 = v*0.5*(1-c0) + coeff*s1 - s2
		s2 = s1
		s1 = s0
		c0, c1 = cd*c0-c1, c0
	}
	re := s1 - s2*math.Cos(w)
	im := s2 * math.Sin(w)
	// A Hann window halves the coherent gain, so 4/n restores a full-scale
	// sine to a magnitude near 1.
	return math.Hypot(re, im) * 4 / float64(n)
}

// keyMagnitudes measures every key in the sample buffer.
func keyMagnitudes(x []float64) []float64 {
	out := make([]float64, Keys)
	for k := range out {
		out[k] = goertzel(x, keyWindow(k), KeyFreq(k))
	}
	return out
}

// detectKeys turns raw per-key magnitudes into a sparse salience: 0 for keys
// not sounding, otherwise how far they stand above the floor. It returns nil
// when the frame has no tonal content worth showing.
func detectKeys(mag []float64) []float64 {
	n := len(mag)
	// Noise floor: the median magnitude. Real notes are a few keys out of
	// 88, so the median is noise or reverb, not signal.
	sorted := slices.Clone(mag)
	slices.Sort(sorted)
	floor := sorted[n/2]
	peak := sorted[n-1]
	if peak < 1e-4 {
		return nil // silence
	}
	// Tonality gate: drums and noise raise the floor towards the peak.
	// Pitched material has peaks many times the median.
	if peak < 6*floor {
		return nil
	}
	// Candidates: local maxima clearly above the floor and not mere
	// sidelobes of the loudest note.
	sal := make([]float64, n)
	for k := range mag {
		v := mag[k] - 3*floor
		if v <= 0 || mag[k] < 0.05*peak {
			continue
		}
		if k > 0 && mag[k-1] > mag[k] || k+1 < n && mag[k+1] >= mag[k] {
			continue
		}
		sal[k] = v
	}
	// Harmonic suppression, lowest note first: a note at f also puts energy
	// at 2f, 3f, ... which would light the octave, the twelfth and so on.
	// Discount those by what a typical string spectrum would contribute.
	for k := 0; k < n; k++ {
		if sal[k] <= 0 {
			continue
		}
		f := KeyFreq(k)
		for h := 2; h <= 8; h++ {
			hk := keyFor(f * float64(h))
			if hk < 0 || hk >= n {
				break
			}
			expect := sal[k] * harmonicWeight(h)
			for _, j := range []int{hk - 1, hk, hk + 1} {
				if j >= 0 && j < n && j != k {
					sal[j] -= expect
					if sal[j] < 0 {
						sal[j] = 0
					}
				}
			}
		}
	}
	return sal
}

// harmonicWeight is the expected level of harmonic h relative to the
// fundamental: roughly 1/h with the first few a little stronger.
func harmonicWeight(h int) float64 {
	return 1.2 / float64(h)
}

// keyFor is the key nearest frequency f, or -1 if it is outside the range.
func keyFor(f float64) int {
	if f <= 0 {
		return -1
	}
	k := int(math.Round(12*math.Log2(f/440))) + 69 - lowMidi
	if k < 0 || k >= Keys {
		return -1
	}
	return k
}

// Keys returns the level of each of the 88 piano keys in [0,1], A0 first.
// Notes light at once and fade over a few frames so sustained notes stay
// steady and short ones still register.
func (c *Capture) Keys() []float64 {
	c.mu.Lock()
	x := c.recent(ring)
	c.mu.Unlock()

	sal := detectKeys(keyMagnitudes(x))

	if len(c.keyLevels) != Keys {
		c.keyLevels = make([]float64, Keys)
	}
	maxSal := 0.0
	for _, v := range sal {
		if v > maxSal {
			maxSal = v
		}
	}
	// Gain follows the loudest note with a slow release, as for the bars.
	if maxSal > c.keyGain {
		c.keyGain = maxSal
	} else {
		c.keyGain = c.keyGain*0.99 + maxSal*0.01
	}
	if c.keyGain < 1e-4 {
		c.keyGain = 1e-4
	}
	for k := range c.keyLevels {
		v := 0.0
		if sal != nil {
			v = sal[k] / c.keyGain
			if v > 1 {
				v = 1
			}
			// Faint remnants after suppression are not worth lighting.
			if v < 0.12 {
				v = 0
			}
		}
		if v >= c.keyLevels[k] {
			c.keyLevels[k] = v
		} else {
			c.keyLevels[k] *= 0.8
			if c.keyLevels[k] < 0.05 {
				c.keyLevels[k] = 0
			}
		}
	}
	return append([]float64(nil), c.keyLevels...)
}

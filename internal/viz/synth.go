package viz

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// Synth returns a Capture fed by a built-in tune instead of a recorder, for
// the demo mode: chords, a bassline, an arpeggio and a drum kit, generated
// in real time so the bars, keys and roll views move as they would on a
// real stream.
func Synth() *Capture {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Capture{cancel: cancel, done: make(chan struct{}), source: "demo tune"}
	go c.runSynth(ctx)
	return c
}

const (
	synthBPM  = 112
	synthStep = 20 * time.Millisecond
)

// The progression, one chord per bar, as piano key indexes (0 = A0).
// C, A minor, F, G: root in the bass, triad in the middle, arpeggio above.
var synthChords = [][3]int{
	{39, 43, 46}, // C4 E4 G4
	{36, 39, 43}, // A3 C4 E4
	{32, 36, 39}, // F3 A3 C4
	{34, 38, 41}, // G3 B3 D4
}

func (c *Capture) runSynth(ctx context.Context) {
	defer close(c.done)
	rng := rand.New(rand.NewSource(1))
	start := time.Now()
	var n int // samples written so far
	t := time.NewTicker(synthStep)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		target := int(time.Since(start).Seconds() * sampleRate)
		c.mu.Lock()
		for ; n < target; n++ {
			c.samples[c.pos] = synthSample(n, rng)
			c.pos = (c.pos + 1) % ring
		}
		c.mu.Unlock()
	}
}

// synthSample is sample n of the tune.
func synthSample(n int, rng *rand.Rand) float64 {
	t := float64(n) / sampleRate
	beat := 60.0 / synthBPM
	b := t / beat                      // position in beats
	bar := int(b/4) % len(synthChords) // chord index
	inBeat := (b - math.Floor(b)) * beat
	chord := synthChords[bar]

	var v float64
	// Pad: the triad, held for the bar.
	for _, k := range chord {
		v += 0.10 * math.Sin(2*math.Pi*KeyFreq(k)*t)
	}
	// Bass: the root two octaves down, struck on every beat.
	v += 0.22 * math.Exp(-inBeat*3) * math.Sin(2*math.Pi*KeyFreq(chord[0]-24)*t)
	// Arpeggio: up and down the chord an octave up, in eighth notes.
	eighth := int(b * 2)
	pattern := [...]int{0, 1, 2, 1}
	k := chord[pattern[eighth%len(pattern)]] + 12
	inEighth := (b*2 - math.Floor(b*2)) * beat / 2
	v += 0.18 * math.Exp(-inEighth*3) * math.Sin(2*math.Pi*KeyFreq(k)*t)
	// Kick on one and three: a short low thump, kept at a fixed pitch so it
	// lights one bass key rather than smearing across the bottom octave.
	if beatN := int(b) % 4; beatN == 0 || beatN == 2 {
		v += 0.25 * math.Exp(-inBeat*18) * math.Sin(2*math.Pi*50*inBeat)
	}
	// Hi-hat on the off beats, short noise.
	if inOff := inBeat - beat/2; inOff >= 0 {
		v += 0.05 * math.Exp(-inOff*60) * (rng.Float64()*2 - 1)
	}
	return v * 0.8
}

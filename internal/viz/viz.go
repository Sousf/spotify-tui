// Package viz captures what the sound server is playing and turns it into
// spectrum bars. It reads the default sink's monitor through parec, which
// PipeWire provides via pipewire-pulse, so it sees spotifyd's output (or
// anything else) without touching the player.
package viz

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
)

const (
	sampleRate = 44100
	window     = 2048 // samples per FFT, ~46 ms at 44.1 kHz
	minHz      = 40.0
	maxHz      = 14000.0
)

// Capture runs the recorder and keeps the most recent window of samples.
type Capture struct {
	mu      sync.Mutex
	samples [window]float64 // ring buffer of the last window samples
	pos     int
	cmd     *exec.Cmd
	cancel  context.CancelFunc
	err     error
	done    chan struct{}

	// smoothing state, one entry per bar for the last requested bar count
	levels []float64
	peaks  []float64
	gain   float64 // running maximum for automatic level scaling
}

// Start launches parec on the default monitor source. It returns an error
// only if the recorder cannot be started at all; later failures are exposed
// through Err.
func Start() (*Capture, error) {
	if _, err := exec.LookPath("parec"); err != nil {
		return nil, errors.New("parec not found (install pulseaudio-utils / libpulse)")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "parec",
		"--device=@DEFAULT_MONITOR@",
		"--format=s16le",
		fmt.Sprintf("--rate=%d", sampleRate),
		"--channels=1",
		"--latency-msec=40",
		"--raw",
	)
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	c := &Capture{cmd: cmd, cancel: cancel, done: make(chan struct{})}
	go c.read(out, stderr)
	return c, nil
}

func (c *Capture) read(out, stderr io.Reader) {
	defer close(c.done)
	var errText []byte
	if stderr != nil {
		go func() { errText, _ = io.ReadAll(stderr) }()
	}
	buf := make([]byte, 4096)
	for {
		n, err := out.Read(buf)
		if n > 0 {
			c.mu.Lock()
			for i := 0; i+1 < n; i += 2 {
				v := int16(binary.LittleEndian.Uint16(buf[i:]))
				c.samples[c.pos] = float64(v) / 32768
				c.pos = (c.pos + 1) % window
			}
			c.mu.Unlock()
		}
		if err != nil {
			werr := c.cmd.Wait()
			c.mu.Lock()
			switch {
			case len(errText) > 0:
				c.err = fmt.Errorf("parec: %s", string(errText))
			case werr != nil && !errors.Is(err, io.EOF):
				c.err = fmt.Errorf("parec: %w", werr)
			default:
				c.err = errors.New("parec stopped")
			}
			c.mu.Unlock()
			return
		}
	}
}

// Stop kills the recorder.
func (c *Capture) Stop() {
	if c == nil {
		return
	}
	c.cancel()
	<-c.done
}

// Err reports why capture stopped, or nil while it is running.
func (c *Capture) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Bars returns n spectrum levels in [0,1], low frequencies first. Levels
// decay smoothly between calls, and Peaks (same length) hold slower-falling
// peak markers for each bar.
func (c *Capture) Bars(n int) (levels, peaks []float64) {
	c.mu.Lock()
	var re [window]float64
	for i := 0; i < window; i++ {
		re[i] = c.samples[(c.pos+i)%window]
	}
	c.mu.Unlock()

	raw := spectrum(re[:], n)

	if len(c.levels) != n {
		c.levels = make([]float64, n)
		c.peaks = make([]float64, n)
	}
	// Automatic gain: track the loudest band with a slow release so quiet
	// and loud tracks both fill the display.
	maxRaw := 0.0
	for _, v := range raw {
		if v > maxRaw {
			maxRaw = v
		}
	}
	if maxRaw > c.gain {
		c.gain = maxRaw
	} else {
		c.gain = c.gain*0.995 + maxRaw*0.005
	}
	if c.gain < 1e-4 {
		c.gain = 1e-4
	}
	for i, v := range raw {
		v /= c.gain
		if v > 1 {
			v = 1
		}
		if v > c.levels[i] {
			c.levels[i] = v
		} else {
			c.levels[i] = c.levels[i]*0.75 + v*0.25
		}
		if c.levels[i] > c.peaks[i] {
			c.peaks[i] = c.levels[i]
		} else {
			c.peaks[i] -= 0.02
			if c.peaks[i] < 0 {
				c.peaks[i] = 0
			}
		}
	}
	levels = append([]float64(nil), c.levels...)
	peaks = append([]float64(nil), c.peaks...)
	return levels, peaks
}

// spectrum computes n log-spaced band magnitudes from a window of samples.
func spectrum(samples []float64, n int) []float64 {
	size := len(samples)
	re := make([]float64, size)
	im := make([]float64, size)
	for i, v := range samples {
		// Hann window keeps energy from smearing across bins.
		w := 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(size-1)))
		re[i] = v * w
	}
	fft(re, im)

	binHz := float64(sampleRate) / float64(size)
	out := make([]float64, n)
	ratio := maxHz / minHz
	for b := 0; b < n; b++ {
		lo := minHz * math.Pow(ratio, float64(b)/float64(n))
		hi := minHz * math.Pow(ratio, float64(b+1)/float64(n))
		from := int(lo / binHz)
		to := int(hi / binHz)
		if to <= from {
			to = from + 1
		}
		if to > size/2 {
			to = size / 2
		}
		peak := 0.0
		for k := from; k < to; k++ {
			mag := math.Hypot(re[k], im[k]) / float64(size)
			if mag > peak {
				peak = mag
			}
		}
		// Perceptual-ish compression so mids and highs are visible next to bass.
		out[b] = math.Sqrt(peak)
	}
	return out
}

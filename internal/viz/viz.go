// Package viz captures what Spotify is playing and turns it into spectrum
// bars. On PipeWire it records the player's own playback stream, found with
// pw-dump and read with pw-record, so other applications' audio never shows
// up. Where the PipeWire tools are missing it falls back to parec on the
// default sink's monitor, which hears everything.
package viz

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

const (
	sampleRate = 44100
	window     = 2048  // samples per FFT for the bars, ~46 ms at 44.1 kHz
	ring       = 16384 // samples kept; the lowest keys need ~370 ms to resolve
	minHz      = 40.0
	maxHz      = 14000.0

	scanEvery  = time.Second     // how often to look for the player while idle
	watchEvery = 2 * time.Second // how often to confirm the stream still exists
)

// Capture runs the recorder and keeps the most recent window of samples.
type Capture struct {
	mu      sync.Mutex
	samples [ring]float64 // ring buffer of the most recent samples
	pos     int
	cancel  context.CancelFunc
	err     error  // fatal: capture has given up
	status  string // why no audio is flowing right now, "" while recording
	source  string // label of the stream being recorded
	done    chan struct{}

	// smoothing state, one entry per bar for the last requested bar count
	levels []float64
	peaks  []float64
	gain   float64 // running maximum for automatic level scaling

	// the same for the piano keys view
	keyLevels []float64
	keyGain   float64
}

// Start begins capturing in the background. It returns an error only if no
// recorder is installed at all; later failures are exposed through Err, and
// the wait for Spotify to start playing through Status.
func Start() (*Capture, error) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Capture{cancel: cancel, done: make(chan struct{})}
	switch {
	case have("pw-record") && have("pw-dump"):
		match := matchWords(os.Getenv("SPOTIFY_TUI_VIZ_SOURCE"))
		c.status = "waiting for Spotify to play…"
		go c.runPipeWire(ctx, match)
	case have("parec"):
		slog.Warn("visualiser: pw-record not found, falling back to parec on the default monitor (all applications)")
		c.source = "all audio"
		go c.runParec(ctx)
	default:
		cancel()
		return nil, errors.New("no recorder: install pipewire (pw-record) or pulseaudio-utils (parec)")
	}
	return c, nil
}

func have(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// runPipeWire follows the player's stream: wait for it to appear, record it
// until it goes away, repeat.
func (c *Capture) runPipeWire(ctx context.Context, match []string) {
	defer close(c.done)
	for ctx.Err() == nil {
		objs, err := listStreams(ctx)
		if err != nil {
			if ctx.Err() == nil {
				c.fail(err)
			}
			return
		}
		s := findStream(objs, match)
		if s == nil {
			c.setStatus("waiting for Spotify to play…", "")
			c.clear()
			sleep(ctx, scanEvery)
			continue
		}
		slog.Info("visualiser: recording stream", "serial", s.serial, "app", s.label)
		c.setStatus("", s.label)
		started := time.Now()
		if err := c.record(ctx, s); err != nil && ctx.Err() == nil {
			slog.Warn("visualiser: recorder stopped", "err", err)
		}
		c.clear()
		if time.Since(started) < time.Second {
			// Immediate failures would otherwise spin.
			sleep(ctx, scanEvery)
		}
	}
}

// record runs pw-record on one stream until it exits, the stream leaves
// the graph, or ctx is cancelled.
func (c *Capture) record(ctx context.Context, s *stream) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, "pw-record",
		"--target", strconv.Itoa(s.serial),
		"--format", "s16",
		"--rate", strconv.Itoa(sampleRate),
		"--channels", "1",
		"--latency", "40ms",
		"--raw",
		"-",
	)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	var errText []byte
	if stderr != nil {
		go func() { errText, _ = io.ReadAll(stderr) }()
	}
	// pw-record keeps running silently when its target disappears, so check
	// the graph and stop it ourselves once the player's stream is gone.
	go func() {
		for {
			if !sleep(ctx, watchEvery) {
				return
			}
			objs, err := listStreams(ctx)
			if err == nil && !hasStream(objs, s.serial) {
				slog.Info("visualiser: stream went away", "serial", s.serial)
				cancel()
				return
			}
		}
	}()
	c.pump(out)
	werr := cmd.Wait()
	switch {
	case ctx.Err() != nil:
		return nil
	case len(errText) > 0:
		return fmt.Errorf("pw-record: %s", string(errText))
	case werr != nil:
		return fmt.Errorf("pw-record: %w", werr)
	default:
		return errors.New("pw-record stopped")
	}
}

// runParec is the fallback for systems without PipeWire's tools: the default
// sink's monitor, which mixes every application.
func (c *Capture) runParec(ctx context.Context) {
	defer close(c.done)
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
		c.fail(err)
		return
	}
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		c.fail(err)
		return
	}
	var errText []byte
	if stderr != nil {
		go func() { errText, _ = io.ReadAll(stderr) }()
	}
	c.pump(out)
	werr := cmd.Wait()
	if ctx.Err() != nil {
		return
	}
	switch {
	case len(errText) > 0:
		c.fail(fmt.Errorf("parec: %s", string(errText)))
	case werr != nil:
		c.fail(fmt.Errorf("parec: %w", werr))
	default:
		c.fail(errors.New("parec stopped"))
	}
}

// pump copies raw s16le mono samples into the ring buffer until r ends.
func (c *Capture) pump(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			c.mu.Lock()
			for i := 0; i+1 < n; i += 2 {
				v := int16(binary.LittleEndian.Uint16(buf[i:]))
				c.samples[c.pos] = float64(v) / 32768
				c.pos = (c.pos + 1) % ring
			}
			c.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// clear zeroes the buffer so the bars fall to silence instead of freezing on
// the last frame when the stream stops.
func (c *Capture) clear() {
	c.mu.Lock()
	c.samples = [ring]float64{}
	c.mu.Unlock()
}

func (c *Capture) setStatus(status, source string) {
	c.mu.Lock()
	c.status, c.source = status, source
	c.mu.Unlock()
}

func (c *Capture) fail(err error) {
	c.mu.Lock()
	c.err = err
	c.mu.Unlock()
}

// sleep waits for d or until ctx is cancelled, reporting which.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Stop kills the recorder.
func (c *Capture) Stop() {
	if c == nil || c.cancel == nil {
		return
	}
	c.cancel()
	<-c.done
}

// Err reports why capture gave up, or nil while it is running.
func (c *Capture) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Status explains why no audio is flowing, such as the player being idle,
// or "" while a stream is being recorded.
func (c *Capture) Status() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Source names the application whose stream is being recorded, or "" while
// none is.
func (c *Capture) Source() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.source
}

// Bars returns n spectrum levels in [0,1], low frequencies first. Levels
// decay smoothly between calls, and Peaks (same length) hold slower-falling
// peak markers for each bar.
func (c *Capture) Bars(n int) (levels, peaks []float64) {
	c.mu.Lock()
	x := c.recent(window)
	c.mu.Unlock()

	raw := spectrum(x, n)

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

// recent copies the last n samples (n <= ring), oldest first. The caller
// holds c.mu.
func (c *Capture) recent(n int) []float64 {
	out := make([]float64, n)
	start := (c.pos - n + ring) % ring
	k := copy(out, c.samples[start:])
	copy(out[k:], c.samples[:])
	return out
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

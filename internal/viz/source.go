package viz

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// defaultMatch is what a playback stream must mention, in any of the
// properties checked by matches, to be taken as the Spotify player. spotifyd
// through pipewire-pulse leaves application.name empty and is only
// identifiable by its process binary; librespot is the engine behind most
// third-party players.
var defaultMatch = []string{"spotify", "librespot"}

// stream is a playback node in the PipeWire graph.
type stream struct {
	serial int    // object.serial, what pw-record's --target takes
	label  string // for display: the app name or process binary
}

// pwObject is the part of a pw-dump entry we read.
type pwObject struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Info struct {
		Props map[string]any `json:"props"`
	} `json:"info"`
}

// listStreams runs pw-dump and returns the audio playback streams it holds.
func listStreams(ctx context.Context) ([]pwObject, error) {
	out, err := exec.CommandContext(ctx, "pw-dump").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("pw-dump: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("pw-dump: %w", err)
	}
	var objs []pwObject
	if err := json.Unmarshal(out, &objs); err != nil {
		return nil, fmt.Errorf("pw-dump: %w", err)
	}
	return objs, nil
}

// findStream picks the first playback stream whose identifying properties
// contain one of the match words (case-insensitive). It returns nil when
// none is present, which is the normal state while the player is idle.
func findStream(objs []pwObject, match []string) *stream {
	for _, o := range objs {
		if o.Type != "PipeWire:Interface:Node" {
			continue
		}
		p := o.Info.Props
		if prop(p, "media.class") != "Stream/Output/Audio" {
			continue
		}
		if !matches(p, match) {
			continue
		}
		serial, ok := p["object.serial"].(float64)
		if !ok {
			continue
		}
		label := prop(p, "application.name")
		if label == "" {
			label = prop(p, "application.process.binary")
		}
		if label == "" {
			label = prop(p, "node.name")
		}
		return &stream{serial: int(serial), label: label}
	}
	return nil
}

// identifying lists the properties a player might be recognised by.
var identifying = []string{
	"application.name",
	"application.process.binary",
	"application.id",
	"node.name",
	"media.name",
}

func matches(p map[string]any, match []string) bool {
	for _, key := range identifying {
		v := strings.ToLower(prop(p, key))
		if v == "" {
			continue
		}
		for _, m := range match {
			if m != "" && strings.Contains(v, m) {
				return true
			}
		}
	}
	return false
}

func prop(p map[string]any, key string) string {
	switch v := p[key].(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%v", v)
	default:
		return ""
	}
}

// hasStream reports whether a node with this serial is still in the graph.
func hasStream(objs []pwObject, serial int) bool {
	for _, o := range objs {
		if o.Type != "PipeWire:Interface:Node" {
			continue
		}
		if s, ok := o.Info.Props["object.serial"].(float64); ok && int(s) == serial {
			return true
		}
	}
	return false
}

// matchWords parses the SPOTIFY_TUI_VIZ_SOURCE override: comma-separated
// substrings, matched case-insensitively against the stream's properties.
func matchWords(env string) []string {
	if strings.TrimSpace(env) == "" {
		return defaultMatch
	}
	var words []string
	for _, w := range strings.Split(env, ",") {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" {
			words = append(words, w)
		}
	}
	if len(words) == 0 {
		return defaultMatch
	}
	return words
}

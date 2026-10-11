package viz

import (
	"encoding/json"
	"testing"
)

// dump is a trimmed pw-dump of a desktop with Chrome playing, spotifyd
// playing through pipewire-pulse (which leaves its names empty), and a
// recorder of our own that must never be picked.
const dump = `[
 {"id": 34, "type": "PipeWire:Interface:Node", "info": {"props": {
   "media.class": "Audio/Sink", "node.name": "alsa_output.usb-headset", "object.serial": 58}}},
 {"id": 83, "type": "PipeWire:Interface:Node", "info": {"props": {
   "media.class": "Stream/Output/Audio", "node.name": "Google Chrome",
   "application.name": "Google Chrome", "application.process.binary": "chrome", "object.serial": 108}}},
 {"id": 94, "type": "PipeWire:Interface:Node", "info": {"props": {
   "media.class": "Stream/Input/Audio", "node.name": "spotify-tui-viz",
   "application.process.binary": "pw-cat", "object.serial": 344}}},
 {"id": 93, "type": "PipeWire:Interface:Node", "info": {"props": {
   "media.class": "Stream/Output/Audio", "node.name": "", "application.name": "",
   "application.process.binary": "spotifyd", "object.serial": 366}}},
 {"id": 120, "type": "PipeWire:Interface:Port", "info": {"props": {
   "object.serial": 367, "port.name": "output_FL"}}}
]`

func parseDump(t *testing.T) []pwObject {
	var objs []pwObject
	if err := json.Unmarshal([]byte(dump), &objs); err != nil {
		t.Fatal(err)
	}
	return objs
}

func TestFindStreamPicksSpotifydByBinary(t *testing.T) {
	s := findStream(parseDump(t), defaultMatch)
	if s == nil {
		t.Fatal("no stream found")
	}
	if s.serial != 366 || s.label != "spotifyd" {
		t.Errorf("got serial %d label %q, want 366 spotifyd", s.serial, s.label)
	}
}

func TestFindStreamIgnoresOtherApps(t *testing.T) {
	if s := findStream(parseDump(t), []string{"firefox"}); s != nil {
		t.Errorf("matched %+v, want nothing", *s)
	}
	// Overriding the match can target another player.
	s := findStream(parseDump(t), matchWords(" Chrome , "))
	if s == nil || s.serial != 108 || s.label != "Google Chrome" {
		t.Errorf("got %+v, want Chrome's stream", s)
	}
}

func TestHasStream(t *testing.T) {
	objs := parseDump(t)
	if !hasStream(objs, 366) {
		t.Error("serial 366 should be present")
	}
	if hasStream(objs, 367) {
		t.Error("serial 367 is a port, not a node")
	}
	if hasStream(objs, 999) {
		t.Error("serial 999 should be absent")
	}
}

func TestMatchWords(t *testing.T) {
	if got := matchWords(""); len(got) != len(defaultMatch) {
		t.Errorf("empty override gave %v", got)
	}
	if got := matchWords(" , "); len(got) != len(defaultMatch) {
		t.Errorf("blank override gave %v", got)
	}
	got := matchWords("Spotify, ncspot")
	if len(got) != 2 || got[0] != "spotify" || got[1] != "ncspot" {
		t.Errorf("got %v", got)
	}
}

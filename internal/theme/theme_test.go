package theme

import (
	"strings"
	"testing"
)

func TestParseOverlaysDefaults(t *testing.T) {
	src := `# generated
[colors]
accent = "#112233"   # trailing comment
Overlay0 = "#445566"
unknown = "#000000"
`
	p, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if p.Accent != "#112233" || p.Overlay != "#445566" {
		t.Errorf("got %+v", p)
	}
	if p.Text != Default().Text {
		t.Errorf("text should keep its default, got %s", p.Text)
	}
}

func TestParseRejectsBadLine(t *testing.T) {
	if _, err := Parse(strings.NewReader(`accent = "not a colour"`)); err == nil {
		t.Fatal("expected an error")
	}
}

package share

import (
	"encoding/xml"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestBadgeGolden(t *testing.T) {
	cases := []struct{ name, label, value, color string }{
		{"badge_week", "focus this week", "12h", BadgeColorActive},
		{"badge_es_escape", "foco este mes", "3h 5m <&>", BadgeColorIdle},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Badge(c.label, c.value, c.color)
			path := filepath.Join("testdata", c.name+".svg")
			if *update {
				if err := os.WriteFile(path, got, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("badge differs from %s (run go test ./internal/share -update)\n got: %s\nwant: %s", path, got, want)
			}
			// Must be well-formed XML.
			dec := xml.NewDecoder(strings.NewReader(string(got)))
			for {
				if _, err := dec.Token(); err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					t.Fatalf("invalid XML: %v", err)
				}
			}
		})
	}
}

func TestBadgeWidthGrowsWithText(t *testing.T) {
	short := Badge("focus", "1h", BadgeColorActive)
	long := Badge("focus this year", "1234h 59m", BadgeColorActive)
	if len(long) <= len(short) || !strings.Contains(string(long), `aria-label="focus this year: 1234h 59m"`) {
		t.Errorf("unexpected badge output:\n%s", long)
	}
}

package share

import (
	"bytes"
	"image/png"
	"os"
	"testing"
)

func sampleCard() Card {
	return Card{
		Title: "flow wrapped", Subtitle: "Week of Jun 9", HeroValue: "12h 30m", HeroLabel: "of deep focus",
		Stats: []Stat{
			{"Sessions", "18"}, {"Streak", "5 days"}, {"Best day", "Wed Jun 11"},
			{"Best hour", "10:00"}, {"Top tag", "#backend"},
			{"Top task", "A very long task title that certainly does not fit in one tile"},
		},
		FunFact: "Night owl: 34% of your focus started after 8pm. Ñandú, café y años bisiestos.",
		Footer:  "github.com/xvierd/flow-cli",
	}
}

func TestRenderPNG(t *testing.T) {
	for _, name := range []string{"dark", "light"} {
		th, err := ThemeByName(name)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := RenderPNG(&buf, sampleCard(), th); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		raw := append([]byte(nil), buf.Bytes()...)
		img, err := png.Decode(&buf)
		if err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if b := img.Bounds(); b.Dx() != CardWidth || b.Dy() != CardHeight {
			t.Errorf("%s: size %v", name, b)
		}
		// Background corner is the theme background; a tile pixel is the panel.
		if got := img.At(CardWidth-4, CardHeight-4); got != th.Background {
			t.Errorf("%s: corner = %v, want background %v", name, got, th.Background)
		}
		if got := img.At(612+120, 64+20); got != th.Panel {
			t.Errorf("%s: tile = %v, want panel %v", name, got, th.Panel)
		}
		if out := os.Getenv("FLOW_CARD_PREVIEW_DIR"); out != "" {
			_ = os.WriteFile(out+"/card-"+name+".png", raw, 0o644)
		}
	}
	if _, err := ThemeByName("neon"); err == nil {
		t.Error("expected error for unknown theme")
	}
}

func TestFitTruncates(t *testing.T) {
	if err := loadFonts(); err != nil {
		t.Fatal(err)
	}
	f, err := face(regularFont, 20)
	if err != nil {
		t.Fatal(err)
	}
	got := fit(f, "a very long string that will not fit", 80)
	if got == "" || got[len(got)-len("…"):] != "…" {
		t.Errorf("fit = %q, want ellipsis-terminated", got)
	}
	if fit(f, "ok", 80) != "ok" {
		t.Error("short strings must not change")
	}
}

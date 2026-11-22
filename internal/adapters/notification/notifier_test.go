package notification

import (
	"testing"

	"github.com/xvierd/flow-cli/internal/domain"
)

func TestMethodologyEmoji(t *testing.T) {
	cases := map[domain.Methodology]string{
		domain.MethodologyPomodoro:  "🍅",
		domain.MethodologyDeepWork:  "🧠",
		domain.MethodologyMakeTime:  "🎯",
		domain.Methodology("other"): "✅",
	}
	for m, want := range cases {
		if got := methodologyEmoji(m.Label()); got != want {
			t.Errorf("methodologyEmoji(%q) = %q, want %q", m.Label(), got, want)
		}
	}
}

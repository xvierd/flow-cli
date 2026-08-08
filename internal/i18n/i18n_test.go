package i18n

import (
	"reflect"
	"testing"
	"time"
)

func TestSetLanguage(t *testing.T) {
	tests := []struct{ in, want string }{
		{"es", "es"},
		{"en", "en"},
		{"", "en"},
		{"EN", "en"},
		{"fr", "en"},
		{"spanish", "en"},
	}
	for _, tt := range tests {
		SetLanguage(tt.in)
		if got := Language(); got != tt.want {
			t.Errorf("SetLanguage(%q) → Language() = %q, want %q", tt.in, got, tt.want)
		}
	}
	SetLanguage("en")
}

func TestT_Fallback(t *testing.T) {
	SetLanguage("en")
	if got := T("plain text"); got != "plain text" {
		t.Errorf("T() = %q, want key back", got)
	}

	SetLanguage("es")
	defer SetLanguage("en")
	missing := "this key is not in the catalog"
	if got := T(missing); got != missing {
		t.Errorf("T() missing key = %q, want English key %q", got, missing)
	}
}

func TestT_Spanish(t *testing.T) {
	SetLanguage("es")
	defer SetLanguage("en")
	if got := T("Yes"); got != "Sí" {
		t.Errorf("T(Yes) = %q, want %q", got, "Sí")
	}
}

func TestT_Sprintf(t *testing.T) {
	SetLanguage("en")
	if got := T("%d days", 5); got != "5 days" {
		t.Errorf("T() = %q, want %q", got, "5 days")
	}

	SetLanguage("es")
	defer SetLanguage("en")
	if got := T("%d days", 5); got != "5 días" {
		t.Errorf("T() = %q, want %q", got, "5 días")
	}
	// No args: percent escapes in the key must survive untouched.
	if got := T("(%d%% complete)"); got != "(%d%% completado)" {
		t.Errorf("T() without args = %q, want raw catalog value", got)
	}
}

func TestDayMonthNames(t *testing.T) {
	SetLanguage("en")
	if got := DayName(time.Monday); got != "Mon" {
		t.Errorf("DayName(Monday) = %q, want Mon", got)
	}
	if got := MonthName(time.January); got != "Jan" {
		t.Errorf("MonthName(January) = %q, want Jan", got)
	}
	if got := FullMonthName(time.August); got != "August" {
		t.Errorf("FullMonthName(August) = %q, want August", got)
	}

	SetLanguage("es")
	defer SetLanguage("en")
	if got := DayName(time.Monday); got != "lun" {
		t.Errorf("DayName(Monday) = %q, want lun", got)
	}
	if got := MonthName(time.January); got != "ene" {
		t.Errorf("MonthName(January) = %q, want ene", got)
	}
	if got := FullMonthName(time.August); got != "agosto" {
		t.Errorf("FullMonthName(August) = %q, want agosto", got)
	}
}

// printfVerbs extracts the verb letters of a format string in order,
// skipping %% escapes, flags, widths, and precisions (e.g. "%-3d" → "d").
func printfVerbs(s string) []string {
	const verbLetters = "vtbcdoOqxXUeEfFgGsp"
	var verbs []string
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		i++
		if i >= len(s) {
			break
		}
		if s[i] == '%' {
			continue
		}
		for i < len(s) {
			c := s[i]
			isVerb := false
			for j := 0; j < len(verbLetters); j++ {
				if verbLetters[j] == c {
					isVerb = true
					break
				}
			}
			if isVerb {
				verbs = append(verbs, string(c))
				break
			}
			i++
		}
	}
	return verbs
}

// TestEsCatalogDrift guards the catalog: no empty translations and every
// entry keeps the same printf verbs as its English key, in the same order.
func TestEsCatalogDrift(t *testing.T) {
	if len(esCatalog) == 0 {
		t.Fatal("esCatalog is empty")
	}
	for key, val := range esCatalog {
		if val == "" {
			t.Errorf("esCatalog[%q] is empty", key)
			continue
		}
		want := printfVerbs(key)
		got := printfVerbs(val)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("esCatalog[%q] = %q: verbs %v, want %v", key, val, got, want)
		}
	}
}

// Package i18n provides minimal runtime internationalization for Flow.
//
// English source text is used directly as the lookup key, so English needs
// no catalog; other languages live in per-language catalog files (es.go).
// The language is set once at startup from FLOW_LANG or the config file and
// read on every T() call; a RWMutex keeps it safe under -race.
package i18n

import (
	"fmt"
	"sync"
	"time"
)

var (
	mu   sync.RWMutex
	lang = "en"
)

// SetLanguage sets the active language ("en" | "es"). Unknown/empty → "en".
func SetLanguage(l string) {
	if l != "es" {
		l = "en"
	}
	mu.Lock()
	lang = l
	mu.Unlock()
}

// Language returns the active language.
func Language() string {
	mu.RLock()
	defer mu.RUnlock()
	return lang
}

// T translates key (English source text) into the active language,
// applying fmt.Sprintf when args are given. Missing key → English key itself.
func T(key string, args ...interface{}) string {
	text := key
	if Language() == "es" {
		if translated, ok := esCatalog[key]; ok {
			text = translated
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}

// Short day names indexed by time.Weekday (0 = Sunday).
var dayNames = map[string][7]string{
	"en": {"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"},
	"es": {"dom", "lun", "mar", "mié", "jue", "vie", "sáb"},
}

// Short month names indexed by time.Month - 1.
var monthNames = map[string][12]string{
	"en": {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"},
	"es": {"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"},
}

// Full month names indexed by time.Month - 1.
var fullMonthNames = map[string][12]string{
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	"es": {"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"},
}

// DayName returns the localized short name for a weekday (e.g. "Mon" / "lun").
func DayName(d time.Weekday) string {
	return dayNames[Language()][int(d)%7]
}

// MonthName returns the localized short name for a month (e.g. "Jan" / "ene").
func MonthName(m time.Month) string {
	return monthNames[Language()][(int(m)-1)%12]
}

// FullMonthName returns the localized full name for a month (e.g. "January" / "enero").
func FullMonthName(m time.Month) string {
	return fullMonthNames[Language()][(int(m)-1)%12]
}

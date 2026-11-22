package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
	"github.com/xvierd/flow-cli/internal/services"
	"github.com/xvierd/flow-cli/internal/share"
)

const shareFooter = "github.com/xvierd/flow-cli"

var (
	wrappedWeek  bool
	wrappedMonth bool
	wrappedYear  bool
	wrappedPNG   string
	wrappedTheme string
)

var wrappedCmd = &cobra.Command{
	Use:   "wrapped",
	Short: "Show a shareable summary card of your focus",
	Long: `Render a "wrapped" card for the current week, month, or year: focus hours,
sessions, streak, best day and hour, top tag, top task, and a fun fact.

Use --png to also save the card as an image you can share.`,
	Example: `  flow wrapped
  flow wrapped --year
  flow wrapped --month --png wrapped.png --theme light`,
	RunE: runWrapped,
}

func init() {
	wrappedCmd.Flags().BoolVar(&wrappedWeek, "week", false, "Summarize the current week (default)")
	wrappedCmd.Flags().BoolVar(&wrappedMonth, "month", false, "Summarize the current month")
	wrappedCmd.Flags().BoolVar(&wrappedYear, "year", false, "Summarize the current year")
	wrappedCmd.Flags().StringVar(&wrappedPNG, "png", "", "Also write the card as a PNG image to this path")
	wrappedCmd.Flags().StringVar(&wrappedTheme, "theme", "dark", "PNG theme: dark or light")
	wrappedCmd.MarkFlagsMutuallyExclusive("week", "month", "year")
}

// periodFromFlags maps the --week/--month/--year booleans to a period
// (week when none is set).
func periodFromFlags(month, year bool) services.ReportPeriod {
	switch {
	case year:
		return services.ReportPeriodYear
	case month:
		return services.ReportPeriodMonth
	default:
		return services.ReportPeriodWeek
	}
}

func runWrapped(cmd *cobra.Command, args []string) error {
	theme, err := share.ThemeByName(wrappedTheme)
	if err != nil {
		return err
	}
	period := periodFromFlags(wrappedMonth, wrappedYear)
	now := time.Now()
	w, err := services.NewWrappedService(app.storage).GetWrapped(context.Background(), period, now)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to build wrapped"), err)
	}
	card := buildWrappedCard(w)

	if wrappedPNG != "" {
		if err := writeCardPNG(wrappedPNG, card, theme); err != nil {
			return err
		}
	}

	if jsonOutput {
		out := wrappedJSON(w)
		out["fun_fact_text"] = card.FunFact
		if wrappedPNG != "" {
			out["png"] = wrappedPNG
		}
		return jsonOut(out)
	}

	fmt.Println()
	fmt.Println(renderWrappedCard(card))
	if wrappedPNG != "" {
		fmt.Printf("  %s\n\n", i18n.T("Saved card to %s", wrappedPNG))
	}
	return nil
}

// wrappedJSON flattens a Wrapped summary for --json, following the report
// convention: each duration as a Go duration string plus whole minutes.
func wrappedJSON(w *domain.Wrapped) map[string]interface{} {
	dur := func(out map[string]interface{}, key string, d time.Duration) {
		out[key] = d.String()
		out[key+"_minutes"] = int(d.Minutes())
	}
	out := map[string]interface{}{
		"period":         w.Period,
		"label":          wrappedLabel(w),
		"start":          w.Start.Format(time.RFC3339),
		"end":            w.End.Format(time.RFC3339),
		"sessions":       w.Sessions,
		"active_days":    w.ActiveDays,
		"period_days":    w.PeriodDays,
		"current_streak": w.CurrentStreak,
		"longest_streak": w.LongestStreak,
		"best_hour":      w.BestHour,
		"top_tag":        w.TopTag,
		"top_task":       w.TopTask,
		"fun_fact":       w.FunFact,
		"best_day":       nil,
	}
	if !w.BestDay.IsZero() {
		out["best_day"] = w.BestDay.Format("2006-01-02")
	}
	dur(out, "total_focus", w.TotalFocus)
	dur(out, "best_day_focus", w.BestDayFocus)
	dur(out, "best_hour_focus", w.BestHourFocus)
	dur(out, "top_tag_focus", w.TopTagFocus)
	dur(out, "top_task_focus", w.TopTaskFocus)
	dur(out, "longest_session", w.LongestSession)
	return out
}

func writeCardPNG(path string, card share.Card, theme share.Theme) error {
	f, err := os.Create(path) // #nosec G304 -- user-specified output path
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to create output file"), err)
	}
	if err := share.RenderPNG(f, card, theme); err != nil {
		_ = f.Close()
		return fmt.Errorf("%s: %w", i18n.T("failed to render image"), err)
	}
	return f.Close()
}

// wrappedLabel names the period of a Wrapped summary.
func wrappedLabel(w *domain.Wrapped) string {
	switch services.ReportPeriod(w.Period) {
	case services.ReportPeriodYear:
		return fmt.Sprintf("%d", w.Start.Year())
	case services.ReportPeriodMonth:
		return fmt.Sprintf("%s %d", i18n.FullMonthName(w.Start.Month()), w.Start.Year())
	default:
		return i18n.T("Week of %s", fmt.Sprintf("%s %d", i18n.MonthName(w.Start.Month()), w.Start.Day()))
	}
}

// buildWrappedCard turns a Wrapped summary into localized card text.
func buildWrappedCard(w *domain.Wrapped) share.Card {
	dash := "—"
	bestDay, bestHour, topTag, topTask := dash, dash, dash, dash
	bestDayLabel := i18n.T("Best day")
	if !w.BestDay.IsZero() {
		bestDay = fmt.Sprintf("%s %s %d", i18n.DayName(w.BestDay.Weekday()), i18n.MonthName(w.BestDay.Month()), w.BestDay.Day())
		bestDayLabel += " · " + formatHours(w.BestDayFocus.Hours())
	}
	if w.BestHour >= 0 {
		bestHour = fmt.Sprintf("%02d:00", w.BestHour)
	}
	if w.TopTag != "" {
		topTag = "#" + w.TopTag
	}
	if w.TopTask != "" {
		topTask = w.TopTask
	}
	return share.Card{
		Title:     "flow wrapped",
		Subtitle:  wrappedLabel(w),
		HeroValue: formatHours(w.TotalFocus.Hours()),
		HeroLabel: i18n.T("of focus"),
		Stats: []share.Stat{
			{Label: i18n.T("Sessions"), Value: fmt.Sprintf("%d", w.Sessions)},
			{Label: i18n.T("Streak · best %d", w.LongestStreak), Value: i18n.T("%d days", w.CurrentStreak)},
			{Label: bestDayLabel, Value: bestDay},
			{Label: i18n.T("Best hour"), Value: bestHour},
			{Label: i18n.T("Top tag"), Value: topTag},
			{Label: i18n.T("Top task"), Value: topTask},
		},
		FunFact: funFactText(w),
		Footer:  shareFooter,
	}
}

// funFactText localizes the Wrapped fun fact.
func funFactText(w *domain.Wrapped) string {
	v := w.FunFact.Value
	switch w.FunFact.Kind {
	case domain.FunFactNightOwl:
		return i18n.T("Night owl: %d%% of your focus started after 8pm.", v)
	case domain.FunFactEarlyBird:
		return i18n.T("Early bird: %d%% of your focus started before 9am.", v)
	case domain.FunFactWeekend:
		return i18n.T("Weekend warrior: %d%% of your focus happened on weekends.", v)
	case domain.FunFactMarathon:
		// Non-breaking space keeps "1h 30m" together when the line wraps.
		return i18n.T("Marathon mode: your longest session lasted %s.", strings.ReplaceAll(formatHours(float64(v)/60), " ", "\u00a0"))
	case domain.FunFactMovies:
		return i18n.T("That's %d back-to-back movies worth of focus.", v)
	case domain.FunFactFirstSteps:
		return i18n.T("First steps: %d minutes of focus. Every streak starts somewhere.", v)
	case domain.FunFactConsistency:
		return i18n.T("You showed up on %d%% of the days.", v)
	default:
		return i18n.T("No focus yet. Start a session with `flow start`.")
	}
}

// Adaptive palette: readable on both light and dark terminal backgrounds.
var (
	wrapAccent = lipgloss.AdaptiveColor{Light: "#5B4BD6", Dark: "#A79BFF"}
	wrapText   = lipgloss.AdaptiveColor{Light: "#1E1B2E", Dark: "#F3F1FF"}
	wrapMuted  = lipgloss.AdaptiveColor{Light: "#6B6780", Dark: "#9C97B8"}
	wrapBorder = lipgloss.AdaptiveColor{Light: "#7C6FE0", Dark: "#7C6FE0"}
)

const wrappedColWidth = 26

// renderWrappedCard draws the card for the terminal with lipgloss.
func renderWrappedCard(c share.Card) string {
	accent := lipgloss.NewStyle().Foreground(wrapAccent).Bold(true)
	muted := lipgloss.NewStyle().Foreground(wrapMuted)
	hero := lipgloss.NewStyle().Foreground(wrapText).Bold(true)
	text := lipgloss.NewStyle().Foreground(wrapText)
	inner := 2 * wrappedColWidth

	var b strings.Builder
	title := accent.Render(c.Title)
	sub := muted.Render(c.Subtitle)
	pad := inner - lipgloss.Width(title) - lipgloss.Width(sub)
	b.WriteString(title + strings.Repeat(" ", max(pad, 1)) + sub + "\n\n")
	b.WriteString(hero.Render(bigText(c.HeroValue)) + "\n")
	b.WriteString(muted.Render(c.HeroLabel) + "\n\n")

	cell := lipgloss.NewStyle().Width(wrappedColWidth)
	for i := 0; i < len(c.Stats); i += 2 {
		left := cell.Render(muted.Render(truncate(c.Stats[i].Label, wrappedColWidth-2)) + "\n" +
			accent.Render(truncate(c.Stats[i].Value, wrappedColWidth-2)))
		right := ""
		if i+1 < len(c.Stats) {
			right = cell.Render(muted.Render(truncate(c.Stats[i+1].Label, wrappedColWidth-2)) + "\n" +
				accent.Render(truncate(c.Stats[i+1].Value, wrappedColWidth-2)))
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n\n")
	}
	if c.FunFact != "" {
		fact := lipgloss.NewStyle().Width(inner - 2).Render(text.Render(c.FunFact))
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, accent.Render("✦ "), fact) + "\n\n")
	}
	b.WriteString(muted.Render(c.Footer))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(wrapBorder).
		Padding(1, 3).
		MarginLeft(2)
	return box.Render(b.String())
}

// truncate shortens s to at most n display cells, adding an ellipsis.
func truncate(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > n {
		r = r[:len(r)-1]
	}
	return strings.TrimRight(string(r), " ") + "…"
}

// bigGlyphs is a 3-row half-block font for the hero figure ("12h 30m"); unit
// letters stay small on the baseline.
var bigGlyphs = map[rune][3]string{
	'0': {"█▀█", "█ █", "▀▀▀"}, '1': {"▀█ ", " █ ", "▀▀▀"}, '2': {"▀▀█", "█▀▀", "▀▀▀"},
	'3': {"▀▀█", " ▀█", "▀▀▀"}, '4': {"█ █", "▀▀█", "  ▀"}, '5': {"█▀▀", "▀▀█", "▀▀▀"},
	'6': {"█▀▀", "█▀█", "▀▀▀"}, '7': {"▀▀█", "  █", "  ▀"}, '8': {"█▀█", "█▀█", "▀▀▀"},
	'9': {"█▀█", "▀▀█", "▀▀▀"}, 'h': {" ", " ", "h"}, 'm': {" ", " ", "m"},
	' ': {" ", " ", " "},
}

// bigText renders s with bigGlyphs, falling back to plain text if any rune is
// not in the font.
func bigText(s string) string {
	var rows [3]strings.Builder
	for i, r := range s {
		g, ok := bigGlyphs[r]
		if !ok {
			return s
		}
		for k := range rows {
			if i > 0 {
				rows[k].WriteString(" ")
			}
			rows[k].WriteString(g[k])
		}
	}
	return rows[0].String() + "\n" + rows[1].String() + "\n" + rows[2].String()
}

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and edit session presets and break durations",
	Long:  `Interactively configure the three session presets, short break, long break, and sessions before long break.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)

		methodology := app.config.Methodology
		if methodology == "" {
			methodology = "pomodoro"
		}
		meth := domain.Methodology(methodology)

		// Show presets for the active methodology
		var presets []config.SessionPreset
		switch meth {
		case domain.MethodologyDeepWork:
			presets = app.config.DeepWork.GetPresets()
		case domain.MethodologyMakeTime:
			presets = app.config.MakeTime.GetPresets()
		default:
			presets = app.config.Pomodoro.GetPresets()
		}

		fmt.Println()
		fmt.Println("  " + i18n.T("Current configuration:"))
		fmt.Println()
		fmt.Printf("  %s\n", i18n.T("Methodology:  %s", meth.Label()))
		fmt.Println()
		fmt.Println("  " + i18n.T("Session presets:"))
		for i, p := range presets {
			fmt.Printf("    [%d] %-8s  %s\n", i+1, p.Name, formatMinutes(p.Duration))
		}
		fmt.Println()
		switch meth {
		case domain.MethodologyPomodoro:
			fmt.Printf("    %s\n", i18n.T("Short break:          %s", formatMinutes(time.Duration(app.config.Pomodoro.ShortBreak))))
			fmt.Printf("    %s\n", i18n.T("Long break:           %s", formatMinutes(time.Duration(app.config.Pomodoro.LongBreak))))
			fmt.Printf("    %s\n", i18n.T("Sessions before long:  %d", app.config.Pomodoro.SessionsBeforeLong))
			fmt.Printf("    %s\n", i18n.T("Auto-break:            %v", app.config.Pomodoro.AutoBreak))
		case domain.MethodologyDeepWork:
			fmt.Printf("    %s\n", i18n.T("Break duration:        %s", formatMinutes(time.Duration(app.config.DeepWork.BreakDuration))))
		case domain.MethodologyMakeTime:
			fmt.Printf("    %s\n", i18n.T("Break duration:        %s", formatMinutes(time.Duration(app.config.MakeTime.BreakDuration))))
		}
		notifStatus := i18n.T("off")
		if app.config.Notifications.Enabled {
			notifStatus = i18n.T("on")
			if app.config.Notifications.Sound {
				notifStatus = i18n.T("on (with sound)")
			}
		}
		fmt.Printf("    %s\n", i18n.T("Notifications:         %s", notifStatus))
		fmt.Printf("    %s\n", i18n.T("Language:              %s", app.config.Language))
		fmt.Println()
		fmt.Println("  " + i18n.T("What would you like to change?"))
		fmt.Println("    [1] " + i18n.T("Edit preset 1"))
		fmt.Println("    [2] " + i18n.T("Edit preset 2"))
		fmt.Println("    [3] " + i18n.T("Edit preset 3"))
		fmt.Println("    [b] " + i18n.T("Edit break durations"))
		fmt.Println("    [m] " + i18n.T("Change methodology"))
		fmt.Println("    [p] " + i18n.T("Change Deep Work philosophy"))
		fmt.Println("    [n] " + i18n.T("Toggle notifications"))
		fmt.Println("    [l] " + i18n.T("Change language"))
		fmt.Println("    [q] " + i18n.T("Quit without saving"))
		fmt.Print("  " + i18n.T("Choose: "))

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToLower(choice))

		switch choice {
		case "1":
			return editPreset(reader, app.config, 1)
		case "2":
			return editPreset(reader, app.config, 2)
		case "3":
			return editPreset(reader, app.config, 3)
		case "b":
			return editBreaks(reader, app.config)
		case "m":
			return editMethodology(reader, app.config)
		case "p":
			return editDeepWorkPhilosophy(reader, app.config)
		case "n":
			return editNotifications(reader, app.config)
		case "l":
			return editLanguage(reader, app.config)
		case "q", "":
			fmt.Println("  " + i18n.T("No changes made."))
			return nil
		default:
			return fmt.Errorf("%s", i18n.T("invalid choice %q", choice))
		}
	},
}

func editPreset(reader *bufio.Reader, cfg *config.Config, num int) error {
	methodology := cfg.Methodology
	if methodology == "" {
		methodology = "pomodoro"
	}
	meth := domain.Methodology(methodology)

	var presets []config.SessionPreset
	switch meth {
	case domain.MethodologyDeepWork:
		presets = cfg.DeepWork.GetPresets()
	case domain.MethodologyMakeTime:
		presets = cfg.MakeTime.GetPresets()
	default:
		presets = cfg.Pomodoro.GetPresets()
	}

	p := presets[num-1]

	fmt.Printf("\n  %s\n", i18n.T("Editing preset %d (currently: %s — %s)", num, p.Name, formatMinutes(p.Duration)))

	fmt.Printf("  %s", i18n.T("Name [%s]: ", p.Name))
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		name = p.Name
	}

	fmt.Printf("  %s", i18n.T("Duration [%s]: ", formatMinutes(p.Duration)))
	durInput, _ := reader.ReadString('\n')
	durInput = strings.TrimSpace(durInput)

	dur := p.Duration
	if durInput != "" {
		parsed, err := time.ParseDuration(durInput)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("invalid duration %q", durInput), err)
		}
		dur = parsed
	}

	switch meth {
	case domain.MethodologyDeepWork:
		switch num {
		case 1:
			cfg.DeepWork.Preset1Name = name
			cfg.DeepWork.Preset1Duration = config.Duration(dur)
		case 2:
			cfg.DeepWork.Preset2Name = name
			cfg.DeepWork.Preset2Duration = config.Duration(dur)
		case 3:
			cfg.DeepWork.Preset3Name = name
			cfg.DeepWork.Preset3Duration = config.Duration(dur)
		}
	case domain.MethodologyMakeTime:
		switch num {
		case 1:
			cfg.MakeTime.Preset1Name = name
			cfg.MakeTime.Preset1Duration = config.Duration(dur)
		case 2:
			cfg.MakeTime.Preset2Name = name
			cfg.MakeTime.Preset2Duration = config.Duration(dur)
		case 3:
			cfg.MakeTime.Preset3Name = name
			cfg.MakeTime.Preset3Duration = config.Duration(dur)
		}
	default: // Pomodoro
		switch num {
		case 1:
			cfg.Pomodoro.Preset1Name = name
			cfg.Pomodoro.Preset1Duration = config.Duration(dur)
			cfg.Pomodoro.WorkDuration = config.Duration(dur)
		case 2:
			cfg.Pomodoro.Preset2Name = name
			cfg.Pomodoro.Preset2Duration = config.Duration(dur)
		case 3:
			cfg.Pomodoro.Preset3Name = name
			cfg.Pomodoro.Preset3Duration = config.Duration(dur)
		}
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to save config"), err)
	}

	fmt.Printf("\n  %s\n", i18n.T("Saved: [%d] %s — %s", num, name, formatMinutes(dur)))
	return nil
}

func editBreaks(reader *bufio.Reader, cfg *config.Config) error {
	methodology := cfg.Methodology
	if methodology == "" {
		methodology = "pomodoro"
	}
	meth := domain.Methodology(methodology)

	if meth == domain.MethodologyPomodoro {
		return editPomodoroBreaks(reader, cfg)
	}
	return editMethodologyBreak(reader, cfg, meth)
}

func editPomodoroBreaks(reader *bufio.Reader, cfg *config.Config) error {
	shortBreak := time.Duration(cfg.Pomodoro.ShortBreak)
	longBreak := time.Duration(cfg.Pomodoro.LongBreak)
	sessionsBeforeLong := cfg.Pomodoro.SessionsBeforeLong

	fmt.Println("\n  " + i18n.T("Editing break settings"))

	fmt.Printf("  %s", i18n.T("Short break [%s]: ", formatMinutes(shortBreak)))
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		parsed, err := time.ParseDuration(input)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("invalid duration %q", input), err)
		}
		shortBreak = parsed
	}

	fmt.Printf("  %s", i18n.T("Long break [%s]: ", formatMinutes(longBreak)))
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		parsed, err := time.ParseDuration(input)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("invalid duration %q", input), err)
		}
		longBreak = parsed
	}

	fmt.Printf("  %s", i18n.T("Sessions before long break [%d]: ", sessionsBeforeLong))
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		var n int
		if _, err := fmt.Sscanf(input, "%d", &n); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("invalid number %q", input), err)
		}
		if n < 1 {
			return fmt.Errorf("%s", i18n.T("sessions before long break must be at least 1"))
		}
		sessionsBeforeLong = n
	}

	cfg.Pomodoro.ShortBreak = config.Duration(shortBreak)
	cfg.Pomodoro.LongBreak = config.Duration(longBreak)
	cfg.Pomodoro.SessionsBeforeLong = sessionsBeforeLong

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to save config"), err)
	}

	fmt.Println()
	fmt.Printf("  %s\n", i18n.T("Saved: short break %s, long break %s, long every %d sessions",
		formatMinutes(shortBreak), formatMinutes(longBreak), sessionsBeforeLong))
	return nil
}

func editMethodologyBreak(reader *bufio.Reader, cfg *config.Config, meth domain.Methodology) error {
	var current config.Duration
	switch meth {
	case domain.MethodologyDeepWork:
		current = cfg.DeepWork.BreakDuration
	case domain.MethodologyMakeTime:
		current = cfg.MakeTime.BreakDuration
	}

	fmt.Println("\n  " + i18n.T("Editing break duration"))
	fmt.Printf("  %s", i18n.T("Break duration [%s]: ", formatMinutes(time.Duration(current))))
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	dur := time.Duration(current)
	if input != "" {
		parsed, err := time.ParseDuration(input)
		if err != nil {
			return fmt.Errorf("%s: %w", i18n.T("invalid duration %q", input), err)
		}
		dur = parsed
	}

	switch meth {
	case domain.MethodologyDeepWork:
		cfg.DeepWork.BreakDuration = config.Duration(dur)
	case domain.MethodologyMakeTime:
		cfg.MakeTime.BreakDuration = config.Duration(dur)
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to save config"), err)
	}

	fmt.Printf("\n  %s\n", i18n.T("Saved: break duration %s", formatMinutes(dur)))
	return nil
}

func editMethodology(reader *bufio.Reader, cfg *config.Config) error {
	current := cfg.Methodology
	if current == "" {
		current = "pomodoro"
	}

	fmt.Printf("\n  %s\n\n", i18n.T("Current methodology: %s", domain.Methodology(current).Label()))
	fmt.Println("    [1] " + i18n.T("Simple Pomodoro — classic 25/5 timer, quick and frictionless"))
	fmt.Println("    [2] " + i18n.T("Deep Work       — longer sessions, distraction tracking, shutdown ritual"))
	fmt.Println("    [3] " + i18n.T("Make Time       — daily Highlight, focus scoring, energize reminders"))
	fmt.Print("  " + i18n.T("Choose: "))

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	var m string
	switch choice {
	case "1":
		m = "pomodoro"
	case "2":
		m = "deepwork"
	case "3":
		m = "maketime"
	default:
		fmt.Println("  " + i18n.T("No changes made."))
		return nil
	}

	cfg.Methodology = m
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to save config"), err)
	}

	fmt.Printf("\n  %s\n", i18n.T("Saved: methodology set to %s", domain.Methodology(m).Label()))
	return nil
}

func editNotifications(reader *bufio.Reader, cfg *config.Config) error {
	current := i18n.T("off")
	if cfg.Notifications.Enabled {
		current = i18n.T("on")
		if cfg.Notifications.Sound {
			current = i18n.T("on (with sound)")
		}
	}

	fmt.Printf("\n  %s\n\n", i18n.T("Current notifications: %s", current))
	fmt.Println("    [1] " + i18n.T("Off"))
	fmt.Println("    [2] " + i18n.T("On (visual only)"))
	fmt.Println("    [3] " + i18n.T("On (with sound)"))
	fmt.Print("  " + i18n.T("Choose: "))

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		cfg.Notifications.Enabled = false
		cfg.Notifications.Sound = false
	case "2":
		cfg.Notifications.Enabled = true
		cfg.Notifications.Sound = false
	case "3":
		cfg.Notifications.Enabled = true
		cfg.Notifications.Sound = true
	default:
		fmt.Println("  " + i18n.T("No changes made."))
		return nil
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to save config"), err)
	}

	status := i18n.T("off")
	if cfg.Notifications.Enabled {
		status = i18n.T("on")
		if cfg.Notifications.Sound {
			status = i18n.T("on (with sound)")
		}
	}
	fmt.Printf("\n  %s\n", i18n.T("Saved: notifications %s", status))
	return nil
}

func editDeepWorkPhilosophy(reader *bufio.Reader, cfg *config.Config) error {
	current := cfg.DeepWork.Philosophy
	if current == "" {
		current = "rhythmic"
	}

	fmt.Printf("\n  %s\n\n", i18n.T("Current Deep Work philosophy: %s", current))
	fmt.Println("    [1] " + i18n.T("Rhythmic    — Daily habit, same time each day"))
	fmt.Println("    [2] " + i18n.T("Bimodal     — Alternate deep/shallow periods"))
	fmt.Println("    [3] " + i18n.T("Journalistic — Grab depth whenever possible"))
	fmt.Println("    [4] " + i18n.T("Monastic    — Deep work is your primary work"))
	fmt.Print("  " + i18n.T("Choose: "))

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	var p string
	switch choice {
	case "1":
		p = "rhythmic"
	case "2":
		p = "bimodal"
	case "3":
		p = "journalistic"
	case "4":
		p = "monastic"
	default:
		fmt.Println("  " + i18n.T("No changes made."))
		return nil
	}

	cfg.DeepWork.Philosophy = p
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to save config"), err)
	}

	fmt.Printf("\n  %s\n", i18n.T("Saved: Deep Work philosophy set to %s", p))
	return nil
}

// editLanguage switches the display language and applies it immediately.
func editLanguage(reader *bufio.Reader, cfg *config.Config) error {
	fmt.Printf("\n  %s\n\n", i18n.T("Current language: %s", cfg.Language))
	fmt.Println("    [1] English")
	fmt.Println("    [2] Español")
	fmt.Print("  " + i18n.T("Choose: "))

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		cfg.Language = "en"
	case "2":
		cfg.Language = "es"
	default:
		fmt.Println("  " + i18n.T("No changes made."))
		return nil
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("failed to save config"), err)
	}

	i18n.SetLanguage(cfg.Language)
	fmt.Printf("\n  %s\n", i18n.T("Saved: language set to %s", cfg.Language))
	return nil
}

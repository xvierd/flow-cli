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
		fmt.Println("  Current configuration:")
		fmt.Println()
		fmt.Printf("  Methodology:  %s\n", meth.Label())
		fmt.Println()
		fmt.Println("  Session presets:")
		for i, p := range presets {
			fmt.Printf("    [%d] %-8s  %s\n", i+1, p.Name, formatMinutes(p.Duration))
		}
		fmt.Println()
		switch meth {
		case domain.MethodologyPomodoro:
			fmt.Printf("    Short break:          %s\n", formatMinutes(time.Duration(app.config.Pomodoro.ShortBreak)))
			fmt.Printf("    Long break:           %s\n", formatMinutes(time.Duration(app.config.Pomodoro.LongBreak)))
			fmt.Printf("    Sessions before long:  %d\n", app.config.Pomodoro.SessionsBeforeLong)
			fmt.Printf("    Auto-break:            %v\n", app.config.Pomodoro.AutoBreak)
		case domain.MethodologyDeepWork:
			fmt.Printf("    Break duration:        %s\n", formatMinutes(time.Duration(app.config.DeepWork.BreakDuration)))
		case domain.MethodologyMakeTime:
			fmt.Printf("    Break duration:        %s\n", formatMinutes(time.Duration(app.config.MakeTime.BreakDuration)))
		}
		notifStatus := "off"
		if app.config.Notifications.Enabled {
			notifStatus = "on"
			if app.config.Notifications.Sound {
				notifStatus = "on (with sound)"
			}
		}
		fmt.Printf("    Notifications:         %s\n", notifStatus)
		fmt.Println()
		fmt.Println("  What would you like to change?")
		fmt.Println("    [1] Edit preset 1")
		fmt.Println("    [2] Edit preset 2")
		fmt.Println("    [3] Edit preset 3")
		fmt.Println("    [b] Edit break durations")
		fmt.Println("    [m] Change methodology")
		fmt.Println("    [p] Change Deep Work philosophy")
		fmt.Println("    [n] Toggle notifications")
		fmt.Println("    [q] Quit without saving")
		fmt.Print("  Choose: ")

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
		case "q", "":
			fmt.Println("  No changes made.")
			return nil
		default:
			return fmt.Errorf("invalid choice %q", choice)
		}
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
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

	fmt.Printf("\n  Editing preset %d (currently: %s — %s)\n", num, p.Name, formatMinutes(p.Duration))

	fmt.Printf("  Name [%s]: ", p.Name)
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)
	if name == "" {
		name = p.Name
	}

	fmt.Printf("  Duration [%s]: ", formatMinutes(p.Duration))
	durInput, _ := reader.ReadString('\n')
	durInput = strings.TrimSpace(durInput)

	dur := p.Duration
	if durInput != "" {
		parsed, err := time.ParseDuration(durInput)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", durInput, err)
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
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\n  Saved: [%d] %s — %s\n", num, name, formatMinutes(dur))
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

	fmt.Println("\n  Editing break settings")

	fmt.Printf("  Short break [%s]: ", formatMinutes(shortBreak))
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		parsed, err := time.ParseDuration(input)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", input, err)
		}
		shortBreak = parsed
	}

	fmt.Printf("  Long break [%s]: ", formatMinutes(longBreak))
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		parsed, err := time.ParseDuration(input)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", input, err)
		}
		longBreak = parsed
	}

	fmt.Printf("  Sessions before long break [%d]: ", sessionsBeforeLong)
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		var n int
		if _, err := fmt.Sscanf(input, "%d", &n); err != nil {
			return fmt.Errorf("invalid number %q: %w", input, err)
		}
		if n < 1 {
			return fmt.Errorf("sessions before long break must be at least 1")
		}
		sessionsBeforeLong = n
	}

	cfg.Pomodoro.ShortBreak = config.Duration(shortBreak)
	cfg.Pomodoro.LongBreak = config.Duration(longBreak)
	cfg.Pomodoro.SessionsBeforeLong = sessionsBeforeLong

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Println()
	fmt.Printf("  Saved: short break %s, long break %s, long every %d sessions\n",
		formatMinutes(shortBreak), formatMinutes(longBreak), sessionsBeforeLong)
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

	fmt.Println("\n  Editing break duration")
	fmt.Printf("  Break duration [%s]: ", formatMinutes(time.Duration(current)))
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	dur := time.Duration(current)
	if input != "" {
		parsed, err := time.ParseDuration(input)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", input, err)
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
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\n  Saved: break duration %s\n", formatMinutes(dur))
	return nil
}

func editMethodology(reader *bufio.Reader, cfg *config.Config) error {
	current := cfg.Methodology
	if current == "" {
		current = "pomodoro"
	}

	fmt.Printf("\n  Current methodology: %s\n\n", domain.Methodology(current).Label())
	fmt.Println("    [1] Simple Pomodoro — classic 25/5 timer, quick and frictionless")
	fmt.Println("    [2] Deep Work       — longer sessions, distraction tracking, shutdown ritual")
	fmt.Println("    [3] Make Time       — daily Highlight, focus scoring, energize reminders")
	fmt.Print("  Choose: ")

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
		fmt.Println("  No changes made.")
		return nil
	}

	cfg.Methodology = m
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\n  Saved: methodology set to %s\n", domain.Methodology(m).Label())
	return nil
}

func editNotifications(reader *bufio.Reader, cfg *config.Config) error {
	current := "off"
	if cfg.Notifications.Enabled {
		current = "on"
		if cfg.Notifications.Sound {
			current = "on (with sound)"
		}
	}

	fmt.Printf("\n  Current notifications: %s\n\n", current)
	fmt.Println("    [1] Off")
	fmt.Println("    [2] On (visual only)")
	fmt.Println("    [3] On (with sound)")
	fmt.Print("  Choose: ")

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
		fmt.Println("  No changes made.")
		return nil
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	status := "off"
	if cfg.Notifications.Enabled {
		status = "on"
		if cfg.Notifications.Sound {
			status = "on (with sound)"
		}
	}
	fmt.Printf("\n  Saved: notifications %s\n", status)
	return nil
}

func editDeepWorkPhilosophy(reader *bufio.Reader, cfg *config.Config) error {
	current := cfg.DeepWork.Philosophy
	if current == "" {
		current = "rhythmic"
	}

	fmt.Printf("\n  Current Deep Work philosophy: %s\n\n", current)
	fmt.Println("    [1] Rhythmic    — Daily habit, same time each day")
	fmt.Println("    [2] Bimodal     — Alternate deep/shallow periods")
	fmt.Println("    [3] Journalistic — Grab depth whenever possible")
	fmt.Println("    [4] Monastic    — Deep work is your primary work")
	fmt.Print("  Choose: ")

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
		fmt.Println("  No changes made.")
		return nil
	}

	cfg.DeepWork.Philosophy = p
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\n  Saved: Deep Work philosophy set to %s\n", p)
	return nil
}

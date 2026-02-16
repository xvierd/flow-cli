package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View and edit session presets and break durations",
	Long:  `Interactively configure the three session presets, short break, long break, and sessions before long break.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reader := bufio.NewReader(os.Stdin)
		presets := appConfig.Pomodoro.GetPresets()

		fmt.Println()
		fmt.Println("  Current configuration:")
		fmt.Println()
		fmt.Println("  Session presets:")
		for i, p := range presets {
			fmt.Printf("    [%d] %-8s  %s\n", i+1, p.Name, formatMinutes(p.Duration))
		}
		fmt.Println()
		fmt.Printf("    Short break:          %s\n", formatMinutes(time.Duration(appConfig.Pomodoro.ShortBreak)))
		fmt.Printf("    Long break:           %s\n", formatMinutes(time.Duration(appConfig.Pomodoro.LongBreak)))
		fmt.Printf("    Sessions before long:  %d\n", appConfig.Pomodoro.SessionsBeforeLong)
		fmt.Println()
		fmt.Println("  What would you like to change?")
		fmt.Println("    [1] Edit preset 1")
		fmt.Println("    [2] Edit preset 2")
		fmt.Println("    [3] Edit preset 3")
		fmt.Println("    [b] Edit break durations")
		fmt.Println("    [q] Quit without saving")
		fmt.Print("  Choose: ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(strings.ToLower(choice))

		switch choice {
		case "1":
			return editPreset(reader, appConfig, 1)
		case "2":
			return editPreset(reader, appConfig, 2)
		case "3":
			return editPreset(reader, appConfig, 3)
		case "b":
			return editBreaks(reader, appConfig)
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
	presets := cfg.Pomodoro.GetPresets()
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

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\n  Saved: [%d] %s — %s\n", num, name, formatMinutes(dur))
	return nil
}

func editBreaks(reader *bufio.Reader, cfg *config.Config) error {
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

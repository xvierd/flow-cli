// Package config provides configuration management for Flow.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"github.com/xvierd/flow-cli/internal/domain"
)

// Config holds all configuration for the Flow application.
type Config struct {
	Methodology   string             `mapstructure:"methodology"`
	FirstRun      bool               `mapstructure:"first_run"`
	Pomodoro      PomodoroConfig     `mapstructure:"pomodoro"`
	DeepWork      DeepWorkConfig     `mapstructure:"deepwork"`
	MakeTime      MakeTimeConfig     `mapstructure:"maketime"`
	Notifications NotificationConfig `mapstructure:"notifications"`
	MCP           MCPConfig          `mapstructure:"mcp"`
	Storage       StorageConfig      `mapstructure:"storage"`
	Theme         ThemeConfig        `mapstructure:"theme"`
}

// ThemeConfig holds theme customization settings (colors and icons).
type ThemeConfig struct {
	ColorWork           string `mapstructure:"color_work"`
	ColorBreak          string `mapstructure:"color_break"`
	ColorPaused         string `mapstructure:"color_paused"`
	ColorTitle          string `mapstructure:"color_title"`
	ColorTask           string `mapstructure:"color_task"`
	ColorHelp           string `mapstructure:"color_help"`
	WorkGradientStart   string `mapstructure:"work_gradient_start"`
	WorkGradientEnd     string `mapstructure:"work_gradient_end"`
	BreakGradientStart  string `mapstructure:"break_gradient_start"`
	BreakGradientEnd    string `mapstructure:"break_gradient_end"`
	PausedGradientStart string `mapstructure:"paused_gradient_start"`
	PausedGradientEnd   string `mapstructure:"paused_gradient_end"`
	IconApp             string `mapstructure:"icon_app"`
	IconTask            string `mapstructure:"icon_task"`
	IconStats           string `mapstructure:"icon_stats"`
	IconGit             string `mapstructure:"icon_git"`
	IconPaused          string `mapstructure:"icon_paused"`
}

// DefaultThemeConfig returns the default theme configuration.
func DefaultThemeConfig() ThemeConfig {
	return ThemeConfig{
		ColorWork:           "#7C6FE0",
		ColorBreak:          "#4ECDC4",
		ColorPaused:         "#6B7280",
		ColorTitle:          "#6B7280",
		ColorTask:           "#A0AEC0",
		ColorHelp:           "#95A5A6",
		WorkGradientStart:   "#7C6FE0",
		WorkGradientEnd:     "#A78BFA",
		BreakGradientStart:  "#4ECDC4",
		BreakGradientEnd:    "#2ECC71",
		PausedGradientStart: "#6B7280",
		PausedGradientEnd:   "#4B5563",
		IconApp:             "🍅",
		IconTask:            "📋",
		IconStats:           "📊",
		IconGit:             "🌿",
		IconPaused:          "⏸",
	}
}

// PomodoroConfig holds pomodoro timer settings.
type PomodoroConfig struct {
	WorkDuration       Duration `mapstructure:"work_duration"`
	ShortBreak         Duration `mapstructure:"short_break"`
	LongBreak          Duration `mapstructure:"long_break"`
	SessionsBeforeLong int      `mapstructure:"sessions_before_long"`
	AutoBreak          bool     `mapstructure:"auto_break"`
	Preset1Name        string   `mapstructure:"preset1_name"`
	Preset1Duration    Duration `mapstructure:"preset1_duration"`
	Preset2Name        string   `mapstructure:"preset2_name"`
	Preset2Duration    Duration `mapstructure:"preset2_duration"`
	Preset3Name        string   `mapstructure:"preset3_name"`
	Preset3Duration    Duration `mapstructure:"preset3_duration"`
}

// SessionPreset represents a named session duration preset.
type SessionPreset struct {
	Name     string
	Duration time.Duration
}

// GetPresets returns the three session presets.
func (c *PomodoroConfig) GetPresets() []SessionPreset {
	return []SessionPreset{
		{Name: c.Preset1Name, Duration: time.Duration(c.Preset1Duration)},
		{Name: c.Preset2Name, Duration: time.Duration(c.Preset2Duration)},
		{Name: c.Preset3Name, Duration: time.Duration(c.Preset3Duration)},
	}
}

// DeepWorkConfig holds deep work timer settings.
type DeepWorkConfig struct {
	DeepWorkGoalHours float64  `mapstructure:"deep_work_goal_hours"`
	BreakDuration     Duration `mapstructure:"break_duration"`
	Preset1Name       string   `mapstructure:"preset1_name"`
	Preset1Duration   Duration `mapstructure:"preset1_duration"`
	Preset2Name       string   `mapstructure:"preset2_name"`
	Preset2Duration   Duration `mapstructure:"preset2_duration"`
	Preset3Name       string   `mapstructure:"preset3_name"`
	Preset3Duration   Duration `mapstructure:"preset3_duration"`
}

// GetPresets returns the three session presets for deep work.
func (c *DeepWorkConfig) GetPresets() []SessionPreset {
	return []SessionPreset{
		{Name: c.Preset1Name, Duration: time.Duration(c.Preset1Duration)},
		{Name: c.Preset2Name, Duration: time.Duration(c.Preset2Duration)},
		{Name: c.Preset3Name, Duration: time.Duration(c.Preset3Duration)},
	}
}

// MakeTimeConfig holds make time timer settings.
type MakeTimeConfig struct {
	BreakDuration   Duration `mapstructure:"break_duration"`
	Preset1Name     string   `mapstructure:"preset1_name"`
	Preset1Duration Duration `mapstructure:"preset1_duration"`
	Preset2Name     string   `mapstructure:"preset2_name"`
	Preset2Duration Duration `mapstructure:"preset2_duration"`
	Preset3Name     string   `mapstructure:"preset3_name"`
	Preset3Duration Duration `mapstructure:"preset3_duration"`
}

// GetPresets returns the three session presets for make time.
func (c *MakeTimeConfig) GetPresets() []SessionPreset {
	return []SessionPreset{
		{Name: c.Preset1Name, Duration: time.Duration(c.Preset1Duration)},
		{Name: c.Preset2Name, Duration: time.Duration(c.Preset2Duration)},
		{Name: c.Preset3Name, Duration: time.Duration(c.Preset3Duration)},
	}
}

// NotificationConfig holds notification settings.
type NotificationConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Sound   bool `mapstructure:"sound"`
}

// MCPConfig holds MCP server settings.
type MCPConfig struct {
	Enabled   bool `mapstructure:"enabled"`
	AutoStart bool `mapstructure:"auto_start"`
}

// StorageConfig holds storage settings.
type StorageConfig struct {
	DataDir string `mapstructure:"data_dir"`
}

// Duration is a wrapper around time.Duration for TOML parsing.
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(text []byte) error {
	duration, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(duration)
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// String returns the string representation of the duration.
func (d Duration) String() string {
	return time.Duration(d).String()
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Methodology: "pomodoro",
		FirstRun:    true,
		Pomodoro: PomodoroConfig{
			WorkDuration:       Duration(25 * time.Minute),
			ShortBreak:         Duration(5 * time.Minute),
			LongBreak:          Duration(15 * time.Minute),
			SessionsBeforeLong: 4,
			Preset1Name:        "Focus",
			Preset1Duration:    Duration(25 * time.Minute),
			Preset2Name:        "Short",
			Preset2Duration:    Duration(15 * time.Minute),
			Preset3Name:        "Deep",
			Preset3Duration:    Duration(50 * time.Minute),
		},
		DeepWork: DeepWorkConfig{
			DeepWorkGoalHours: 4.0,
			BreakDuration:     Duration(20 * time.Minute),
			Preset1Name:       "Deep",
			Preset1Duration:   Duration(90 * time.Minute),
			Preset2Name:       "Focus",
			Preset2Duration:   Duration(50 * time.Minute),
			Preset3Name:       "Shallow",
			Preset3Duration:   Duration(25 * time.Minute),
		},
		MakeTime: MakeTimeConfig{
			BreakDuration:   Duration(15 * time.Minute),
			Preset1Name:     "Highlight",
			Preset1Duration: Duration(60 * time.Minute),
			Preset2Name:     "Sprint",
			Preset2Duration: Duration(25 * time.Minute),
			Preset3Name:     "Quick",
			Preset3Duration: Duration(15 * time.Minute),
		},
		Notifications: NotificationConfig{
			Enabled: true,
			Sound:   true,
		},
		MCP: MCPConfig{
			Enabled:   true,
			AutoStart: false,
		},
		Storage: StorageConfig{
			DataDir: "~/.flow",
		},
		Theme: DefaultThemeConfig(),
	}
}

// Load loads the configuration from the config file.
func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get config path: %w", err)
	}

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")

	// Set defaults
	setDefaults()

	// If config file doesn't exist, create it with defaults
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := Save(DefaultConfig()); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
	}

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Expand ~ in data directory
	if cfg.Storage.DataDir == "~/.flow" || cfg.Storage.DataDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cfg.Storage.DataDir = filepath.Join(homeDir, ".flow")
	}

	return &cfg, nil
}

// Save saves the configuration to the config file.
func Save(cfg *Config) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	// Ensure config directory exists
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	viper.SetConfigFile(configPath)
	viper.SetConfigType("toml")

	// Set all values
	viper.Set("methodology", cfg.Methodology)
	viper.Set("first_run", cfg.FirstRun)
	viper.Set("pomodoro.work_duration", cfg.Pomodoro.WorkDuration.String())
	viper.Set("pomodoro.short_break", cfg.Pomodoro.ShortBreak.String())
	viper.Set("pomodoro.long_break", cfg.Pomodoro.LongBreak.String())
	viper.Set("pomodoro.sessions_before_long", cfg.Pomodoro.SessionsBeforeLong)
	viper.Set("pomodoro.auto_break", cfg.Pomodoro.AutoBreak)
	viper.Set("pomodoro.preset1_name", cfg.Pomodoro.Preset1Name)
	viper.Set("pomodoro.preset1_duration", cfg.Pomodoro.Preset1Duration.String())
	viper.Set("pomodoro.preset2_name", cfg.Pomodoro.Preset2Name)
	viper.Set("pomodoro.preset2_duration", cfg.Pomodoro.Preset2Duration.String())
	viper.Set("pomodoro.preset3_name", cfg.Pomodoro.Preset3Name)
	viper.Set("pomodoro.preset3_duration", cfg.Pomodoro.Preset3Duration.String())
	viper.Set("deepwork.deep_work_goal_hours", cfg.DeepWork.DeepWorkGoalHours)
	viper.Set("deepwork.break_duration", cfg.DeepWork.BreakDuration.String())
	viper.Set("deepwork.preset1_name", cfg.DeepWork.Preset1Name)
	viper.Set("deepwork.preset1_duration", cfg.DeepWork.Preset1Duration.String())
	viper.Set("deepwork.preset2_name", cfg.DeepWork.Preset2Name)
	viper.Set("deepwork.preset2_duration", cfg.DeepWork.Preset2Duration.String())
	viper.Set("deepwork.preset3_name", cfg.DeepWork.Preset3Name)
	viper.Set("deepwork.preset3_duration", cfg.DeepWork.Preset3Duration.String())
	viper.Set("maketime.break_duration", cfg.MakeTime.BreakDuration.String())
	viper.Set("maketime.preset1_name", cfg.MakeTime.Preset1Name)
	viper.Set("maketime.preset1_duration", cfg.MakeTime.Preset1Duration.String())
	viper.Set("maketime.preset2_name", cfg.MakeTime.Preset2Name)
	viper.Set("maketime.preset2_duration", cfg.MakeTime.Preset2Duration.String())
	viper.Set("maketime.preset3_name", cfg.MakeTime.Preset3Name)
	viper.Set("maketime.preset3_duration", cfg.MakeTime.Preset3Duration.String())
	viper.Set("notifications.enabled", cfg.Notifications.Enabled)
	viper.Set("notifications.sound", cfg.Notifications.Sound)
	viper.Set("mcp.enabled", cfg.MCP.Enabled)
	viper.Set("mcp.auto_start", cfg.MCP.AutoStart)
	viper.Set("storage.data_dir", cfg.Storage.DataDir)

	return viper.WriteConfig()
}

// GetConfigPath returns the path to the config file.
func GetConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".flow", "config.toml"), nil
}

// GetDBPath returns the path to the database file.
func GetDBPath(cfg *Config) string {
	return filepath.Join(cfg.Storage.DataDir, "flow.db")
}

// setDefaults sets default values for viper.
func setDefaults() {
	viper.SetDefault("methodology", "pomodoro")
	viper.SetDefault("first_run", true)
	viper.SetDefault("pomodoro.work_duration", "25m")
	viper.SetDefault("pomodoro.short_break", "5m")
	viper.SetDefault("pomodoro.long_break", "15m")
	viper.SetDefault("pomodoro.sessions_before_long", 4)
	viper.SetDefault("pomodoro.auto_break", false)
	viper.SetDefault("pomodoro.preset1_name", "Focus")
	viper.SetDefault("pomodoro.preset1_duration", "25m0s")
	viper.SetDefault("pomodoro.preset2_name", "Short")
	viper.SetDefault("pomodoro.preset2_duration", "15m0s")
	viper.SetDefault("pomodoro.preset3_name", "Deep")
	viper.SetDefault("pomodoro.preset3_duration", "50m0s")
	viper.SetDefault("deepwork.deep_work_goal_hours", 4.0)
	viper.SetDefault("deepwork.break_duration", "20m0s")
	viper.SetDefault("deepwork.preset1_name", "Deep")
	viper.SetDefault("deepwork.preset1_duration", "1h30m0s")
	viper.SetDefault("deepwork.preset2_name", "Focus")
	viper.SetDefault("deepwork.preset2_duration", "50m0s")
	viper.SetDefault("deepwork.preset3_name", "Shallow")
	viper.SetDefault("deepwork.preset3_duration", "25m0s")
	viper.SetDefault("maketime.break_duration", "15m0s")
	viper.SetDefault("maketime.preset1_name", "Highlight")
	viper.SetDefault("maketime.preset1_duration", "1h0m0s")
	viper.SetDefault("maketime.preset2_name", "Sprint")
	viper.SetDefault("maketime.preset2_duration", "25m0s")
	viper.SetDefault("maketime.preset3_name", "Quick")
	viper.SetDefault("maketime.preset3_duration", "15m0s")
	viper.SetDefault("notifications.enabled", true)
	viper.SetDefault("notifications.sound", true)
	viper.SetDefault("mcp.enabled", true)
	viper.SetDefault("mcp.auto_start", false)
	viper.SetDefault("storage.data_dir", "~/.flow")

	// Theme defaults
	defaults := DefaultThemeConfig()
	viper.SetDefault("theme.color_work", defaults.ColorWork)
	viper.SetDefault("theme.color_break", defaults.ColorBreak)
	viper.SetDefault("theme.color_paused", defaults.ColorPaused)
	viper.SetDefault("theme.color_title", defaults.ColorTitle)
	viper.SetDefault("theme.color_task", defaults.ColorTask)
	viper.SetDefault("theme.color_help", defaults.ColorHelp)
	viper.SetDefault("theme.work_gradient_start", defaults.WorkGradientStart)
	viper.SetDefault("theme.work_gradient_end", defaults.WorkGradientEnd)
	viper.SetDefault("theme.break_gradient_start", defaults.BreakGradientStart)
	viper.SetDefault("theme.break_gradient_end", defaults.BreakGradientEnd)
	viper.SetDefault("theme.paused_gradient_start", defaults.PausedGradientStart)
	viper.SetDefault("theme.paused_gradient_end", defaults.PausedGradientEnd)
	viper.SetDefault("theme.icon_app", defaults.IconApp)
	viper.SetDefault("theme.icon_task", defaults.IconTask)
	viper.SetDefault("theme.icon_stats", defaults.IconStats)
	viper.SetDefault("theme.icon_git", defaults.IconGit)
	viper.SetDefault("theme.icon_paused", defaults.IconPaused)
}

// ToPomodoroDomainConfig converts the config to the domain PomodoroConfig.
func (c *Config) ToPomodoroDomainConfig() (work, shortBreak, longBreak time.Duration, sessionsBeforeLong int) {
	return time.Duration(c.Pomodoro.WorkDuration),
		time.Duration(c.Pomodoro.ShortBreak),
		time.Duration(c.Pomodoro.LongBreak),
		c.Pomodoro.SessionsBeforeLong
}

// GetBreakDurations returns the short and long break durations for the given methodology.
// Deep Work and Make Time use a single break duration; Pomodoro uses short/long.
func (c *Config) GetBreakDurations(m domain.Methodology) (short, long time.Duration) {
	switch m {
	case domain.MethodologyDeepWork:
		bd := time.Duration(c.DeepWork.BreakDuration)
		if bd == 0 {
			bd = 20 * time.Minute
		}
		return bd, bd
	case domain.MethodologyMakeTime:
		bd := time.Duration(c.MakeTime.BreakDuration)
		if bd == 0 {
			bd = 15 * time.Minute
		}
		return bd, bd
	default:
		return time.Duration(c.Pomodoro.ShortBreak), time.Duration(c.Pomodoro.LongBreak)
	}
}

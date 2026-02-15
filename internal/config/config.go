// Package config provides configuration management for Flow.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the Flow application.
type Config struct {
	Pomodoro      PomodoroConfig     `mapstructure:"pomodoro"`
	Notifications NotificationConfig `mapstructure:"notifications"`
	MCP           MCPConfig          `mapstructure:"mcp"`
	Storage       StorageConfig      `mapstructure:"storage"`
}

// PomodoroConfig holds pomodoro timer settings.
type PomodoroConfig struct {
	WorkDuration       Duration `mapstructure:"work_duration"`
	ShortBreak         Duration `mapstructure:"short_break"`
	LongBreak          Duration `mapstructure:"long_break"`
	SessionsBeforeLong int      `mapstructure:"sessions_before_long"`
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
		Pomodoro: PomodoroConfig{
			WorkDuration:       Duration(25 * time.Minute),
			ShortBreak:         Duration(5 * time.Minute),
			LongBreak:          Duration(15 * time.Minute),
			SessionsBeforeLong: 4,
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
	viper.Set("pomodoro.work_duration", cfg.Pomodoro.WorkDuration.String())
	viper.Set("pomodoro.short_break", cfg.Pomodoro.ShortBreak.String())
	viper.Set("pomodoro.long_break", cfg.Pomodoro.LongBreak.String())
	viper.Set("pomodoro.sessions_before_long", cfg.Pomodoro.SessionsBeforeLong)
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
	viper.SetDefault("pomodoro.work_duration", "25m")
	viper.SetDefault("pomodoro.short_break", "5m")
	viper.SetDefault("pomodoro.long_break", "15m")
	viper.SetDefault("pomodoro.sessions_before_long", 4)
	viper.SetDefault("notifications.enabled", true)
	viper.SetDefault("notifications.sound", true)
	viper.SetDefault("mcp.enabled", true)
	viper.SetDefault("mcp.auto_start", false)
	viper.SetDefault("storage.data_dir", "~/.flow")
}

// ToPomodoroDomainConfig converts the config to the domain PomodoroConfig.
func (c *Config) ToPomodoroDomainConfig() (work, shortBreak, longBreak time.Duration, sessionsBeforeLong int) {
	return time.Duration(c.Pomodoro.WorkDuration),
		time.Duration(c.Pomodoro.ShortBreak),
		time.Duration(c.Pomodoro.LongBreak),
		c.Pomodoro.SessionsBeforeLong
}

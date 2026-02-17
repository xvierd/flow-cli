package cmd

import (
	"bytes"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// executeCmd is a helper to execute a cobra command in tests
func executeCmd(cmd *cobra.Command, args ...string) (stdout string, stderr string, err error) {
	bufOut := new(bytes.Buffer)
	bufErr := new(bytes.Buffer)

	cmd.SetOut(bufOut)
	cmd.SetErr(bufErr)
	cmd.SetArgs(args)

	err = cmd.Execute()
	return bufOut.String(), bufErr.String(), err
}

// TestRootCmd_BareExecution tests the root command with no args (wizard mode would run)
func TestRootCmd_BareExecution(t *testing.T) {
	// We can't easily test the interactive wizard, but we can verify the command exists
	if rootCmd == nil {
		t.Fatal("rootCmd should not be nil")
	}

	if rootCmd.Use != "flow" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "flow")
	}
}

// TestRootCmd_Help tests the --help flag
func TestRootCmd_Help(t *testing.T) {
	stdout, _, err := executeCmd(rootCmd, "--help")
	if err != nil {
		t.Fatalf("help command failed: %v", err)
	}

	// Check for flow or Flow in output
	if !bytes.Contains([]byte(stdout), []byte("flow")) && !bytes.Contains([]byte(stdout), []byte("Flow")) {
		t.Error("help output should contain 'flow' or 'Flow'")
	}
}

// TestRootCmd_Flags tests that global flags are registered
func TestRootCmd_Flags(t *testing.T) {
	// Test db flag
	dbFlag := rootCmd.PersistentFlags().Lookup("db")
	if dbFlag == nil {
		t.Error("--db flag should be registered")
	}

	// Test json flag
	jsonFlag := rootCmd.PersistentFlags().Lookup("json")
	if jsonFlag == nil {
		t.Error("--json flag should be registered")
	}
}

// TestFormatMinutes tests the formatMinutes helper function
func TestFormatMinutes(t *testing.T) {
	tests := []struct {
		name     string
		duration int64 // minutes
		want     string
	}{
		{"25 minutes", 25, "25m"},
		{"60 minutes", 60, "1h"},
		{"90 minutes", 90, "1h30m"},
		{"120 minutes", 120, "2h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := time.Duration(tt.duration) * time.Minute
			got := formatMinutes(d)
			if got != tt.want {
				t.Errorf("formatMinutes(%d min) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

// TestFormatWizardDuration tests the formatWizardDuration helper
func TestFormatWizardDuration(t *testing.T) {
	tests := []struct {
		minutes  int
		seconds  int
		expected string
	}{
		{25, 0, "25:00"},
		{5, 30, "05:30"},
		{0, 45, "00:45"},
		{100, 5, "100:05"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			d := time.Duration(tt.minutes)*time.Minute + time.Duration(tt.seconds)*time.Second
			got := formatWizardDuration(d)
			if got != tt.expected {
				t.Errorf("formatWizardDuration() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestGetDir tests the getDir helper function
func TestGetDir(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/home/user/file.txt", "/home/user"},
		{"/home/user/", "/home/user"},
		{"file.txt", "."},
		{"/file.txt", "."}, // Root case returns "."
		{"C:\\Users\\file.txt", "C:\\Users"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := getDir(tt.path)
			if got != tt.expected {
				t.Errorf("getDir(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

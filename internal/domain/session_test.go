package domain

import (
	"testing"
	"time"
)

func TestDefaultPomodoroConfig(t *testing.T) {
	config := DefaultPomodoroConfig()

	if config.WorkDuration != 25*time.Minute {
		t.Errorf("WorkDuration = %v, want %v", config.WorkDuration, 25*time.Minute)
	}

	if config.ShortBreakDuration != 5*time.Minute {
		t.Errorf("ShortBreakDuration = %v, want %v", config.ShortBreakDuration, 5*time.Minute)
	}

	if config.LongBreakDuration != 15*time.Minute {
		t.Errorf("LongBreakDuration = %v, want %v", config.LongBreakDuration, 15*time.Minute)
	}

	if config.SessionsBeforeLong != 4 {
		t.Errorf("SessionsBeforeLong = %v, want %v", config.SessionsBeforeLong, 4)
	}
}

func TestNewPomodoroSession(t *testing.T) {
	config := DefaultPomodoroConfig()
	taskID := "task-123"

	session := NewPomodoroSession(config, &taskID)

	if session.ID == "" {
		t.Error("NewPomodoroSession() ID is empty")
	}

	if session.Type != SessionTypeWork {
		t.Errorf("Type = %v, want %v", session.Type, SessionTypeWork)
	}

	if session.Status != SessionStatusRunning {
		t.Errorf("Status = %v, want %v", session.Status, SessionStatusRunning)
	}

	if session.Duration != config.WorkDuration {
		t.Errorf("Duration = %v, want %v", session.Duration, config.WorkDuration)
	}

	if session.TaskID == nil || *session.TaskID != taskID {
		t.Errorf("TaskID = %v, want %v", session.TaskID, taskID)
	}

	if session.StartedAt.IsZero() {
		t.Error("StartedAt is zero")
	}
}

func TestNewBreakSession_ShortBreak(t *testing.T) {
	config := DefaultPomodoroConfig()

	session := NewBreakSession(config, 1)

	if session.Type != SessionTypeShortBreak {
		t.Errorf("Type = %v, want %v", session.Type, SessionTypeShortBreak)
	}

	if session.Duration != config.ShortBreakDuration {
		t.Errorf("Duration = %v, want %v", session.Duration, config.ShortBreakDuration)
	}
}

func TestNewBreakSession_LongBreak(t *testing.T) {
	config := DefaultPomodoroConfig()

	session := NewBreakSession(config, 4)

	if session.Type != SessionTypeLongBreak {
		t.Errorf("Type = %v, want %v", session.Type, SessionTypeLongBreak)
	}

	if session.Duration != config.LongBreakDuration {
		t.Errorf("Duration = %v, want %v", session.Duration, config.LongBreakDuration)
	}
}

func TestPomodoroSession_Pause(t *testing.T) {
	config := DefaultPomodoroConfig()
	session := NewPomodoroSession(config, nil)

	session.Pause()

	if session.Status != SessionStatusPaused {
		t.Errorf("Status = %v, want %v", session.Status, SessionStatusPaused)
	}

	if session.PausedAt == nil {
		t.Error("PausedAt should not be nil")
	}
}

func TestPomodoroSession_Resume(t *testing.T) {
	config := DefaultPomodoroConfig()
	session := NewPomodoroSession(config, nil)

	originalStart := session.StartedAt
	time.Sleep(50 * time.Millisecond)

	session.Pause()
	time.Sleep(50 * time.Millisecond)

	session.Resume()

	if session.Status != SessionStatusRunning {
		t.Errorf("Status = %v, want %v", session.Status, SessionStatusRunning)
	}

	if session.PausedAt != nil {
		t.Error("PausedAt should be nil after resume")
	}

	if !session.StartedAt.After(originalStart) {
		t.Error("StartedAt should be adjusted after resume")
	}
}

func TestPomodoroSession_Complete(t *testing.T) {
	config := DefaultPomodoroConfig()
	session := NewPomodoroSession(config, nil)

	session.Complete()

	if session.Status != SessionStatusCompleted {
		t.Errorf("Status = %v, want %v", session.Status, SessionStatusCompleted)
	}

	if session.CompletedAt == nil {
		t.Error("CompletedAt should not be nil")
	}
}

func TestPomodoroSession_Cancel(t *testing.T) {
	config := DefaultPomodoroConfig()
	session := NewPomodoroSession(config, nil)

	session.Cancel()

	if session.Status != SessionStatusCancelled {
		t.Errorf("Status = %v, want %v", session.Status, SessionStatusCancelled)
	}
}

func TestPomodoroSession_RemainingTime(t *testing.T) {
	config := PomodoroConfig{WorkDuration: 100 * time.Millisecond}
	session := NewPomodoroSession(config, nil)

	remaining := session.RemainingTime()
	if remaining <= 0 || remaining > config.WorkDuration {
		t.Errorf("RemainingTime = %v, should be between 0 and %v", remaining, config.WorkDuration)
	}

	time.Sleep(150 * time.Millisecond)

	remaining = session.RemainingTime()
	if remaining != 0 {
		t.Errorf("RemainingTime after completion = %v, want 0", remaining)
	}
}

func TestPomodoroSession_Progress(t *testing.T) {
	config := PomodoroConfig{WorkDuration: 100 * time.Millisecond}
	session := NewPomodoroSession(config, nil)

	progress := session.Progress()
	if progress < 0 || progress > 0.01 {
		t.Errorf("Progress at start = %v, want ~0", progress)
	}

	time.Sleep(50 * time.Millisecond)

	progress = session.Progress()
	if progress <= 0 || progress >= 1 {
		t.Errorf("Progress in middle = %v, should be between 0 and 1", progress)
	}

	time.Sleep(100 * time.Millisecond)

	progress = session.Progress()
	if progress != 1 {
		t.Errorf("Progress after completion = %v, want 1", progress)
	}
}

func TestPomodoroSession_IsWorkSession(t *testing.T) {
	config := DefaultPomodoroConfig()

	workSession := NewPomodoroSession(config, nil)
	if !workSession.IsWorkSession() {
		t.Error("IsWorkSession() should return true for work session")
	}

	breakSession := NewBreakSession(config, 1)
	if breakSession.IsWorkSession() {
		t.Error("IsWorkSession() should return false for break session")
	}
}

func TestPomodoroSession_IsBreakSession(t *testing.T) {
	config := DefaultPomodoroConfig()

	workSession := NewPomodoroSession(config, nil)
	if workSession.IsBreakSession() {
		t.Error("IsBreakSession() should return false for work session")
	}

	breakSession := NewBreakSession(config, 1)
	if !breakSession.IsBreakSession() {
		t.Error("IsBreakSession() should return true for break session")
	}
}

func TestPomodoroSession_SetGitContext(t *testing.T) {
	config := DefaultPomodoroConfig()
	session := NewPomodoroSession(config, nil)

	branch := "feature/test"
	commit := "abc123"
	modified := []string{"file1.go", "file2.go"}

	session.SetGitContext(branch, commit, modified)

	if session.GitBranch != branch {
		t.Errorf("GitBranch = %v, want %v", session.GitBranch, branch)
	}

	if session.GitCommit != commit {
		t.Errorf("GitCommit = %v, want %v", session.GitCommit, commit)
	}

	if len(session.GitModified) != len(modified) {
		t.Errorf("GitModified length = %v, want %v", len(session.GitModified), len(modified))
	}
}

func TestParseTagsFromInput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantText string
		wantTags []string
	}{
		{
			name:     "no tags",
			input:    "Build the API",
			wantText: "Build the API",
			wantTags: nil,
		},
		{
			name:     "with tags",
			input:    "Build API #coding #backend",
			wantText: "Build API",
			wantTags: []string{"coding", "backend"},
		},
		{
			name:     "only tags",
			input:    "#coding #backend",
			wantText: "",
			wantTags: []string{"coding", "backend"},
		},
		{
			name:     "hash alone is not a tag",
			input:    "Build # something",
			wantText: "Build # something",
			wantTags: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, tags := ParseTagsFromInput(tt.input)
			if text != tt.wantText {
				t.Errorf("text = %q, want %q", text, tt.wantText)
			}
			if len(tags) != len(tt.wantTags) {
				t.Errorf("tags = %v, want %v", tags, tt.wantTags)
				return
			}
			for i, tag := range tags {
				if tag != tt.wantTags[i] {
					t.Errorf("tag[%d] = %q, want %q", i, tag, tt.wantTags[i])
				}
			}
		})
	}
}

func TestValidateMethodology(t *testing.T) {
	tests := []struct {
		input   string
		want    Methodology
		wantErr bool
	}{
		{"pomodoro", MethodologyPomodoro, false},
		{"deepwork", MethodologyDeepWork, false},
		{"maketime", MethodologyMakeTime, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ValidateMethodology(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMethodology(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateMethodology(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMethodology_Label(t *testing.T) {
	tests := []struct {
		m    Methodology
		want string
	}{
		{MethodologyPomodoro, "Pomodoro"},
		{MethodologyDeepWork, "Deep Work"},
		{MethodologyMakeTime, "Make Time"},
		{Methodology("unknown"), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.m), func(t *testing.T) {
			if got := tt.m.Label(); got != tt.want {
				t.Errorf("Label() = %v, want %v", got, tt.want)
			}
		})
	}
}

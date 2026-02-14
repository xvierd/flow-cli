package domain

import (
	"testing"
)

func TestCurrentState_IsSessionActive(t *testing.T) {
	tests := []struct {
		name    string
		session *PomodoroSession
		want    bool
	}{
		{
			name:    "no session",
			session: nil,
			want:    false,
		},
		{
			name: "running session",
			session: &PomodoroSession{
				Status: SessionStatusRunning,
			},
			want: true,
		},
		{
			name: "paused session",
			session: &PomodoroSession{
				Status: SessionStatusPaused,
			},
			want: true,
		},
		{
			name: "completed session",
			session: &PomodoroSession{
				Status: SessionStatusCompleted,
			},
			want: false,
		},
		{
			name: "cancelled session",
			session: &PomodoroSession{
				Status: SessionStatusCancelled,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := CurrentState{
				ActiveSession: tt.session,
			}

			if got := cs.IsSessionActive(); got != tt.want {
				t.Errorf("IsSessionActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCurrentState_CanStartSession(t *testing.T) {
	tests := []struct {
		name    string
		session *PomodoroSession
		want    bool
	}{
		{
			name:    "no session",
			session: nil,
			want:    true,
		},
		{
			name: "running session",
			session: &PomodoroSession{
				Status: SessionStatusRunning,
			},
			want: false,
		},
		{
			name: "paused session",
			session: &PomodoroSession{
				Status: SessionStatusPaused,
			},
			want: false,
		},
		{
			name: "completed session",
			session: &PomodoroSession{
				Status: SessionStatusCompleted,
			},
			want: true,
		},
		{
			name: "cancelled session",
			session: &PomodoroSession{
				Status: SessionStatusCancelled,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := CurrentState{
				ActiveSession: tt.session,
			}

			if got := cs.CanStartSession(); got != tt.want {
				t.Errorf("CanStartSession() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSessionTypeLabel(t *testing.T) {
	tests := []struct {
		sessionType SessionType
		want        string
	}{
		{SessionTypeWork, "Work"},
		{SessionTypeShortBreak, "Short Break"},
		{SessionTypeLongBreak, "Long Break"},
		{"unknown", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.sessionType), func(t *testing.T) {
			if got := GetSessionTypeLabel(tt.sessionType); got != tt.want {
				t.Errorf("GetSessionTypeLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetStatusLabel(t *testing.T) {
	tests := []struct {
		status SessionStatus
		want   string
	}{
		{SessionStatusRunning, "Running"},
		{SessionStatusPaused, "Paused"},
		{SessionStatusCompleted, "Completed"},
		{SessionStatusCancelled, "Cancelled"},
		{"unknown", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := GetStatusLabel(tt.status); got != tt.want {
				t.Errorf("GetStatusLabel() = %v, want %v", got, tt.want)
			}
		})
	}
}

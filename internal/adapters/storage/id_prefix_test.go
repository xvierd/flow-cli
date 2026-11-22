package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

func saveTaskWithID(t *testing.T, s ports.Storage, id, title string) {
	t.Helper()
	task, _ := domain.NewTask(title)
	task.ID = id
	if err := s.Tasks().Save(context.Background(), task); err != nil {
		t.Fatalf("Save(%s) error = %v", id, err)
	}
}

func TestTaskFindByID_Prefix(t *testing.T) {
	s, err := NewMemory()
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	saveTaskWithID(t, s, "321e0b0a-46e0-4f09-9c92-1b082ef10b82", "Fix auth bug")
	saveTaskWithID(t, s, "4f894d3e-077c-4a1d-93d3-b30276151653", "Write docs")
	saveTaskWithID(t, s, "4f8a0000-0000-4000-8000-000000000000", "Other")

	tests := []struct {
		name    string
		id      string
		want    string
		wantErr error
	}{
		{"full id", "321e0b0a-46e0-4f09-9c92-1b082ef10b82", "Fix auth bug", nil},
		{"8-char prefix from flow list", "321e0b0a", "Fix auth bug", nil},
		{"unique short prefix", "4f894", "Write docs", nil},
		{"below minimum length", "4f8", "", domain.ErrTaskNotFound},
		{"4-char prefix", "4f89", "Write docs", nil},
		{"sibling 4-char prefix", "4f8a", "Other", nil},
		{"no match", "deadbeef", "", domain.ErrTaskNotFound},
		{"wildcards are not prefixes", "4f8%", "", domain.ErrTaskNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := s.Tasks().FindByID(ctx, tt.id)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("FindByID(%q) error = %v, want %v", tt.id, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("FindByID(%q) error = %v", tt.id, err)
			}
			if task.Title != tt.want {
				t.Errorf("FindByID(%q) = %q, want %q", tt.id, task.Title, tt.want)
			}
		})
	}

	saveTaskWithID(t, s, "4f894d3e-ffff-4000-8000-000000000000", "Twin")
	if _, err := s.Tasks().FindByID(ctx, "4f894d3e"); !errors.Is(err, domain.ErrAmbiguousID) {
		t.Errorf("FindByID(ambiguous) error = %v, want ErrAmbiguousID", err)
	}
	if err := s.Tasks().Delete(ctx, "4f894d3e"); !errors.Is(err, domain.ErrAmbiguousID) {
		t.Errorf("Delete(ambiguous) error = %v, want ErrAmbiguousID", err)
	}
	if err := s.Tasks().Delete(ctx, "321e0b0a"); err != nil {
		t.Errorf("Delete(prefix) error = %v", err)
	}
	if _, err := s.Tasks().FindByID(ctx, "321e0b0a-46e0-4f09-9c92-1b082ef10b82"); !errors.Is(err, domain.ErrTaskNotFound) {
		t.Errorf("task still present after Delete(prefix): %v", err)
	}
}

func TestSessionFindByID_Prefix(t *testing.T) {
	s, err := NewMemory()
	if err != nil {
		t.Fatalf("NewMemory() error = %v", err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	session := domain.NewPomodoroSession(domain.DefaultPomodoroConfig(), nil)
	session.Complete()
	if err := s.Sessions().Save(ctx, session); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := s.Sessions().FindByID(ctx, session.ID[:8])
	if err != nil {
		t.Fatalf("FindByID(prefix) error = %v", err)
	}
	if got.ID != session.ID {
		t.Errorf("FindByID(prefix) = %s, want %s", got.ID, session.ID)
	}
}

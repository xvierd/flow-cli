package cmd

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/xvierd/flow-cli/internal/adapters/integrations"
	"github.com/xvierd/flow-cli/internal/domain"
)

// integrationsEnv builds the environment the built-in integrations run in.
// Their state lives next to the database (like the heartbeat files), and
// generated scripts only pass --db when it was given explicitly.
func integrationsEnv() integrations.Env {
	stateDir := ""
	if dbPath != "" {
		stateDir = filepath.Dir(dbPath)
	}
	explicitDB := ""
	if f := rootCmd.PersistentFlags().Lookup("db"); f != nil && f.Changed {
		if abs, err := filepath.Abs(dbPath); err == nil {
			explicitDB = abs
		}
	}
	return integrations.DefaultEnv(currentIntegrationsConfig(), stateDir, explicitDB)
}

// dispatchIntegrations runs the enabled integrations for ev. It is a
// variable so tests can observe calls without running anything; it is set in
// init because integrationsEnv reads rootCmd's flags (an initialization
// cycle otherwise).
var dispatchIntegrations func(ctx context.Context, ev integrations.Event)

func init() {
	dispatchIntegrations = func(ctx context.Context, ev integrations.Event) {
		_ = integrations.Dispatch(ctx, integrationsEnv(), ev)
	}
}

// integrationsEvent describes s for the integrations.
func integrationsEvent(ctx context.Context, event string, s *domain.PomodoroSession) integrations.Event {
	ev := integrations.Event{Kind: event}
	if s == nil {
		return ev
	}
	ev.SessionType = string(s.Type)
	ev.StartedAt = s.StartedAt
	ev.Duration = s.Duration
	if s.TaskID != nil && app.tasks != nil {
		if task, err := app.tasks.GetTask(ctx, *s.TaskID); err == nil {
			ev.Task = task.Title
		}
	}
	return ev
}

// startIntegrations runs the integrations for a session event in the
// background and returns a function that waits for them. Each integration has
// its own short timeout, so waiting never holds a session up for long.
func startIntegrations(ctx context.Context, event string, s *domain.PomodoroSession) (wait func()) {
	if !anyIntegrationEnabled() {
		return func() {}
	}
	ev := integrationsEvent(ctx, event, s)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		dispatchIntegrations(ctx, ev)
	}()
	return wg.Wait
}

// endIntegrations undoes what the integrations did at start for a session
// that ended without an on_end hook (voided or cancelled).
func endIntegrations(ctx context.Context) {
	startIntegrations(ctx, "end", nil)()
}

func anyIntegrationEnabled() bool {
	cfg := currentIntegrationsConfig()
	for _, in := range integrations.All() {
		if in.Enabled(cfg) {
			return true
		}
	}
	return false
}

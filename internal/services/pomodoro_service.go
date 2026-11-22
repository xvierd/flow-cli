package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/ports"
)

// PomodoroService handles pomodoro session use cases.
type PomodoroService struct {
	storage     ports.Storage
	gitDetector ports.GitDetector
	config      domain.PomodoroConfig
	strictMode  bool
	// methodology is the configured methodology: it is recorded on breaks
	// and on StateService sessions whose request omits one.
	methodology domain.Methodology
	// defaultDurations holds each methodology's default work duration for
	// StateService sessions whose request omits one.
	defaultDurations map[domain.Methodology]time.Duration
}

// NewPomodoroService creates a new pomodoro service.
func NewPomodoroService(storage ports.Storage, gitDetector ports.GitDetector) *PomodoroService {
	return &PomodoroService{
		storage:     storage,
		gitDetector: gitDetector,
		config:      domain.DefaultPomodoroConfig(),
	}
}

// SetConfig updates the pomodoro configuration. A non-positive
// SessionsBeforeLong is replaced by the default cadence.
func (s *PomodoroService) SetConfig(config domain.PomodoroConfig) {
	if config.SessionsBeforeLong < 1 {
		config.SessionsBeforeLong = domain.DefaultPomodoroConfig().SessionsBeforeLong
	}
	s.config = config
}

// SetMethodologyDefaults sets the configured methodology and each
// methodology's default work duration. Breaks record m; StateService.StartSession
// uses m when the request omits a methodology and durations[methodology] when
// it omits a duration (falling back to the configured work duration).
func (s *PomodoroService) SetMethodologyDefaults(m domain.Methodology, durations map[domain.Methodology]time.Duration) {
	s.methodology = m
	s.defaultDurations = durations
}

// defaultDuration returns the default work duration of methodology m, or 0
// when none is configured (startPomodoro then uses the configured work
// duration).
func (s *PomodoroService) defaultDuration(m domain.Methodology) time.Duration {
	if m == "" {
		m = domain.MethodologyPomodoro
	}
	if d := s.defaultDurations[m]; d > 0 {
		return d
	}
	return 0
}

// isExpired reports whether session is running with its timer fully elapsed.
func isExpired(session *domain.PomodoroSession) bool {
	return session != nil && session.Status == domain.SessionStatusRunning && session.RemainingTime() == 0
}

// findActiveSessionIn returns the active (running or paused) session from the
// given storage, which may be transaction-scoped. A running session whose
// timer has already expired is completed (and its task released) first, and
// nil is returned for it: an expired session must not block starting new
// work. Callers holding a non-transactional storage use findActiveSession.
func findActiveSessionIn(ctx context.Context, storage ports.Storage) (*domain.PomodoroSession, error) {
	session, err := storage.Sessions().FindActive(ctx)
	if err != nil {
		return nil, err
	}
	if isExpired(session) {
		session.Complete()
		if err := endSession(ctx, storage, session); err != nil {
			return nil, fmt.Errorf("failed to complete expired session: %w", err)
		}
		return nil, nil
	}
	return session, nil
}

// findActiveSession is findActiveSessionIn for a storage that is not
// transaction-scoped. The usual read runs without a transaction (the TUI
// polls GetCurrentState every second and must not take the write lock each
// time); only completing an expired session opens one, re-reading inside it.
func findActiveSession(ctx context.Context, storage ports.Storage) (*domain.PomodoroSession, error) {
	session, err := storage.Sessions().FindActive(ctx)
	if err != nil || !isExpired(session) {
		return session, err
	}
	var active *domain.PomodoroSession
	err = storage.WithTx(ctx, func(tx ports.Storage) error {
		var err error
		active, err = findActiveSessionIn(ctx, tx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return active, nil
}

// CompleteIfExpired completes the active session when it is running with its
// timer fully elapsed, and returns that session. It returns nil when there is
// no such session (none active, paused, or time left). The check and the
// write run in one transaction, so across processes (TUI, MCP server,
// background watcher) exactly one caller gets the completed session and can
// announce it.
func (s *PomodoroService) CompleteIfExpired(ctx context.Context) (*domain.PomodoroSession, error) {
	var done *domain.PomodoroSession
	err := s.storage.WithTx(ctx, func(tx ports.Storage) error {
		session, err := tx.Sessions().FindActive(ctx)
		if err != nil || !isExpired(session) {
			return err
		}
		session.Complete()
		if err := endSession(ctx, tx, session); err != nil {
			return fmt.Errorf("failed to complete expired session: %w", err)
		}
		done = session
		return nil
	})
	if err != nil {
		return nil, err
	}
	return done, nil
}

// endSession persists an ended session and releases its task: starting a
// session marks its task in_progress, so a task still in_progress when its
// session ends goes back to pending. A task in any other state (for example
// completed by the user meanwhile) is left alone, as is a deleted task.
// storage should be transaction-scoped so both writes commit together.
func endSession(ctx context.Context, storage ports.Storage, session *domain.PomodoroSession) error {
	if err := storage.Sessions().Update(ctx, session); err != nil {
		return err
	}
	if session.TaskID == nil {
		return nil
	}
	task, err := storage.Tasks().FindByID(ctx, *session.TaskID)
	if errors.Is(err, domain.ErrTaskNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to find session task: %w", err)
	}
	if task.Status != domain.StatusInProgress {
		return nil
	}
	task.Status = domain.StatusPending
	task.UpdatedAt = time.Now()
	if err := storage.Tasks().Update(ctx, task); err != nil {
		return fmt.Errorf("failed to release session task: %w", err)
	}
	return nil
}

// SetStrictMode toggles focus strict mode. When enabled, active work sessions
// cannot be paused, stopped, voided, or cancelled — they must run to completion.
func (s *PomodoroService) SetStrictMode(enabled bool) {
	s.strictMode = enabled
}

// enforceStrict returns ErrStrictFocusBlocked when strict mode is active and the
// given session is a work session with time still on the clock. A work session
// whose timer has fully elapsed is exempt: ending it is completion, not early
// disengagement.
func (s *PomodoroService) enforceStrict(session *domain.PomodoroSession) error {
	if s.strictMode && session.IsWorkSession() && session.RemainingTime() > 0 {
		return domain.ErrStrictFocusBlocked
	}
	return nil
}

// StartPomodoroRequest contains data to start a work session.
type StartPomodoroRequest struct {
	TaskID          *string
	WorkingDir      string
	Duration        time.Duration
	Methodology     domain.Methodology
	IntendedOutcome string
	Tags            []string
	// Git is the git context recorded on the session, gathered with
	// DetectGit before any transaction begins. StartPomodoro fills it in
	// when nil; StartPomodoroWith never runs git itself.
	Git *ports.GitInfo
}

// DetectGit returns the git context of workingDir, or nil when git is not
// available or workingDir is not a repository. It runs git, so callers must
// call it before opening a transaction: transactions take the database write
// lock at BEGIN and would make other flow processes wait on git.
func (s *PomodoroService) DetectGit(ctx context.Context, workingDir string) *ports.GitInfo {
	if s.gitDetector == nil || !s.gitDetector.IsAvailable() {
		return nil
	}
	gitInfo, err := s.gitDetector.Detect(ctx, workingDir)
	if err != nil {
		return nil
	}
	return gitInfo
}

// StartPomodoro begins a new pomodoro work session.
// The task update and session insert run in a single transaction; the git
// context is gathered before it.
func (s *PomodoroService) StartPomodoro(ctx context.Context, req StartPomodoroRequest) (*domain.PomodoroSession, error) {
	if req.Git == nil {
		req.Git = s.DetectGit(ctx, req.WorkingDir)
	}
	var session *domain.PomodoroSession
	err := s.storage.WithTx(ctx, func(tx ports.Storage) error {
		var err error
		session, err = s.startPomodoro(ctx, tx, req)
		return err
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

// StartPomodoroWith is StartPomodoro without its own transaction: it runs
// against the given storage, which may be transaction-scoped, so callers can
// compose it into a larger WithTx transaction. It does not detect git: set
// req.Git with DetectGit before the transaction begins.
func (s *PomodoroService) StartPomodoroWith(ctx context.Context, storage ports.Storage, req StartPomodoroRequest) (*domain.PomodoroSession, error) {
	return s.startPomodoro(ctx, storage, req)
}

// startPomodoro implements StartPomodoro against the given storage.
func (s *PomodoroService) startPomodoro(ctx context.Context, storage ports.Storage, req StartPomodoroRequest) (*domain.PomodoroSession, error) {
	// Check if there's already an active session
	active, err := findActiveSessionIn(ctx, storage)
	if err != nil {
		return nil, fmt.Errorf("failed to check active sessions: %w", err)
	}
	if active != nil {
		return nil, domain.ErrSessionAlreadyActive
	}

	// If task ID provided, verify it exists and mark it as active
	if req.TaskID != nil {
		task, err := storage.Tasks().FindByID(ctx, *req.TaskID)
		if err != nil {
			return nil, err
		}
		// A short ID prefix resolves to the full one; the session stores that.
		fullID := task.ID
		req.TaskID = &fullID
		task.Start()
		if err := storage.Tasks().Update(ctx, task); err != nil {
			return nil, fmt.Errorf("failed to update task: %w", err)
		}
	}

	// Create and save the session
	session := domain.NewPomodoroSession(s.config, req.TaskID)

	// Apply custom duration if provided
	if req.Duration > 0 {
		session.Duration = req.Duration
	}

	// Set methodology fields
	if req.Methodology != "" {
		session.Methodology = req.Methodology
	}
	session.IntendedOutcome = req.IntendedOutcome
	session.Tags = req.Tags

	// Git context, gathered by the caller before the transaction.
	if req.Git != nil {
		session.SetGitContext(req.Git.Branch, req.Git.Commit, req.Git.Modified)
	}

	if err := storage.Sessions().Save(ctx, session); err != nil {
		// The partial unique index idx_sessions_one_active closes the race
		// between the FindActive check above and this insert.
		if isUniqueConstraintError(err) {
			return nil, domain.ErrSessionAlreadyActive
		}
		return nil, fmt.Errorf("failed to save session: %w", err)
	}

	return session, nil
}

// isUniqueConstraintError reports whether err wraps a SQLite UNIQUE
// constraint violation.
func isUniqueConstraintError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// StartBreak begins a new break session.
func (s *PomodoroService) StartBreak(ctx context.Context, workingDir string) (*domain.PomodoroSession, error) {
	// Check if there's already an active session
	active, err := findActiveSession(ctx, s.storage)
	if err != nil {
		return nil, fmt.Errorf("failed to check active sessions: %w", err)
	}
	if active != nil {
		return nil, domain.ErrSessionAlreadyActive
	}

	// Get today's session count to determine short vs long break
	stats, err := s.storage.Sessions().GetDailyStats(ctx, time.Now())
	if err != nil {
		stats = &domain.DailyStats{}
	}

	session := domain.NewBreakSession(s.config, stats.WorkSessions)
	// Record the methodology the break belongs to; without it storage reads
	// the break back as Pomodoro.
	if s.methodology != "" {
		session.Methodology = s.methodology
	}

	if err := s.storage.Sessions().Save(ctx, session); err != nil {
		// Another process started a session between the check and the insert.
		if isUniqueConstraintError(err) {
			return nil, domain.ErrSessionAlreadyActive
		}
		return nil, fmt.Errorf("failed to save break session: %w", err)
	}

	return session, nil
}

// PauseSession pauses the active session.
func (s *PomodoroService) PauseSession(ctx context.Context) (*domain.PomodoroSession, error) {
	session, err := findActiveSession(ctx, s.storage)
	if err != nil {
		return nil, fmt.Errorf("failed to find active session: %w", err)
	}
	if session == nil {
		return nil, domain.ErrNoActiveSession
	}

	// Strict focus: running work sessions cannot be paused.
	if session.Status == domain.SessionStatusRunning {
		if err := s.enforceStrict(session); err != nil {
			return nil, err
		}
	}

	session.Pause()
	if err := s.storage.Sessions().Update(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return session, nil
}

// ResumeSession resumes a paused session.
func (s *PomodoroService) ResumeSession(ctx context.Context) (*domain.PomodoroSession, error) {
	session, err := s.storage.Sessions().FindActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to find active session: %w", err)
	}
	if session == nil {
		return nil, domain.ErrNoActiveSession
	}

	session.Resume()
	if err := s.storage.Sessions().Update(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return session, nil
}

// StopSession completes the active session.
// In strict focus mode a work session can only be stopped once its timer has
// fully elapsed; stopping it earlier returns ErrStrictFocusBlocked.
// The session update and the release of its task run in one transaction.
func (s *PomodoroService) StopSession(ctx context.Context) (*domain.PomodoroSession, error) {
	var session *domain.PomodoroSession
	err := s.storage.WithTx(ctx, func(tx ports.Storage) error {
		var err error
		session, err = tx.Sessions().FindActive(ctx)
		if err != nil {
			return fmt.Errorf("failed to find active session: %w", err)
		}
		if session == nil {
			return domain.ErrNoActiveSession
		}
		if err := s.enforceStrict(session); err != nil {
			return err
		}

		// Record the actual worked time if the session was stopped early. The
		// elapsed time is pause-aware: wall time spent paused is not work. It is
		// read before Complete(), which clears PausedAt.
		elapsed := session.ElapsedTime()
		if elapsed < 0 {
			elapsed = 0
		}
		if elapsed < session.Duration {
			session.Duration = elapsed
		}
		session.Complete()
		if err := endSession(ctx, tx, session); err != nil {
			return fmt.Errorf("failed to update session: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

// CancelSession aborts the active session and releases its task, in one
// transaction.
func (s *PomodoroService) CancelSession(ctx context.Context) error {
	return s.storage.WithTx(ctx, func(tx ports.Storage) error {
		session, err := tx.Sessions().FindActive(ctx)
		if err != nil {
			return fmt.Errorf("failed to find active session: %w", err)
		}
		if session == nil {
			return domain.ErrNoActiveSession
		}
		if err := s.enforceStrict(session); err != nil {
			return err
		}

		session.Cancel()
		return endSession(ctx, tx, session)
	})
}

// VoidSession marks the active session as interrupted (voided).
// Interrupted sessions are excluded from productivity stats. The session
// update and the release of its task run in one transaction.
func (s *PomodoroService) VoidSession(ctx context.Context) (*domain.PomodoroSession, error) {
	var session *domain.PomodoroSession
	err := s.storage.WithTx(ctx, func(tx ports.Storage) error {
		var err error
		session, err = tx.Sessions().FindActive(ctx)
		if err != nil {
			return fmt.Errorf("failed to find active session: %w", err)
		}
		if session == nil {
			return domain.ErrNoActiveSession
		}
		if err := s.enforceStrict(session); err != nil {
			return err
		}

		session.Interrupt()
		if err := endSession(ctx, tx, session); err != nil {
			return fmt.Errorf("failed to void session: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return session, nil
}

// LogDistraction appends a distraction entry to the active session.
func (s *PomodoroService) LogDistraction(ctx context.Context, sessionID string, text string, category string) error {
	session, err := s.storage.Sessions().FindByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return domain.ErrSessionNotFound
	}
	session.Distractions = append(session.Distractions, domain.Distraction{Text: text, Category: category})
	return s.storage.Sessions().Update(ctx, session)
}

// SetAccomplishment records the accomplishment text on a session (Deep Work shutdown ritual).
func (s *PomodoroService) SetAccomplishment(ctx context.Context, sessionID string, text string) error {
	session, err := s.storage.Sessions().FindByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return domain.ErrSessionNotFound
	}
	session.Accomplishment = text
	return s.storage.Sessions().Update(ctx, session)
}

// SetFocusScore records the focus score (1–5) on a session (Make Time).
func (s *PomodoroService) SetFocusScore(ctx context.Context, sessionID string, score int) error {
	if score < 1 || score > 5 {
		return fmt.Errorf("focus score must be between 1 and 5, got %d", score)
	}
	session, err := s.storage.Sessions().FindByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return domain.ErrSessionNotFound
	}
	session.FocusScore = &score
	return s.storage.Sessions().Update(ctx, session)
}

// SetEnergizeActivity records the energize activity on a session (Make Time).
func (s *PomodoroService) SetEnergizeActivity(ctx context.Context, sessionID string, activity string) error {
	session, err := s.storage.Sessions().FindByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return domain.ErrSessionNotFound
	}
	session.EnergizeActivity = activity
	return s.storage.Sessions().Update(ctx, session)
}

// SetShutdownRitual records the shutdown ritual on a session (Deep Work).
func (s *PomodoroService) SetShutdownRitual(ctx context.Context, sessionID string, ritual domain.ShutdownRitual) error {
	session, err := s.storage.Sessions().FindByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return domain.ErrSessionNotFound
	}
	session.ShutdownRitual = &ritual
	return s.storage.Sessions().Update(ctx, session)
}

// SetOutcomeAchieved records whether the intended outcome was achieved (y/p/n) on a session (Deep Work).
func (s *PomodoroService) SetOutcomeAchieved(ctx context.Context, sessionID string, achieved string) error {
	if achieved != "y" && achieved != "p" && achieved != "n" {
		return fmt.Errorf("outcome achieved must be 'y', 'p', or 'n', got %s", achieved)
	}
	session, err := s.storage.Sessions().FindByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return domain.ErrSessionNotFound
	}
	session.OutcomeAchieved = achieved
	return s.storage.Sessions().Update(ctx, session)
}

// AddSessionNotes adds notes to a pomodoro session.
func (s *PomodoroService) AddSessionNotes(ctx context.Context, sessionID string, notes string) (*domain.PomodoroSession, error) {
	session, err := s.storage.Sessions().FindByID(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to find session: %w", err)
	}
	if session == nil {
		return nil, domain.ErrSessionNotFound
	}

	session.AddNotes(notes)
	if err := s.storage.Sessions().Update(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return session, nil
}

// GetDeepWorkStreak returns the number of consecutive days with >= threshold of deep work.
// If threshold is 0, uses the default of 4 hours.
func (s *PomodoroService) GetDeepWorkStreak(ctx context.Context, threshold time.Duration) (int, error) {
	if threshold == 0 {
		threshold = 4 * time.Hour
	}
	return s.storage.Sessions().GetDeepWorkStreak(ctx, threshold)
}

// GetTaskHistory retrieves session history for a specific task.
// taskID may be a unique ID prefix; a task that no longer exists is looked
// up by the given ID as is (its sessions keep the ID).
func (s *PomodoroService) GetTaskHistory(ctx context.Context, taskID string) ([]*domain.PomodoroSession, error) {
	if task, err := s.storage.Tasks().FindByID(ctx, taskID); err == nil {
		taskID = task.ID
	} else if !errors.Is(err, domain.ErrTaskNotFound) {
		return nil, err
	}
	return s.storage.Sessions().FindByTask(ctx, taskID)
}

// GetRecentSessions retrieves recent pomodoro sessions.
func (s *PomodoroService) GetRecentSessions(ctx context.Context, limit int) ([]*domain.PomodoroSession, error) {
	// Get sessions from the last 7 days
	since := time.Now().AddDate(0, 0, -7)
	sessions, err := s.storage.Sessions().FindRecent(ctx, since)
	if err != nil {
		return nil, err
	}

	if len(sessions) > limit {
		return sessions[:limit], nil
	}
	return sessions, nil
}

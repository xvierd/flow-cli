package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/domain"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// fakeRunner records commands and answers from a script; nothing is executed.
type fakeRunner struct {
	mu    sync.Mutex
	calls [][]string
	// reply returns the output for a command (nil → empty, no error).
	reply func(name string, args []string) ([]byte, error)
	// block makes every call wait for its context to end.
	block bool
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	f.calls = append(f.calls, append([]string{name}, args...))
	f.mu.Unlock()
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if f.reply != nil {
		return f.reply(name, args)
	}
	return nil, nil
}

func (f *fakeRunner) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func lookPathFor(tools ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, t := range tools {
			if t == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", exec.ErrNotFound
	}
}

func testEnv(t *testing.T, goos string, r *fakeRunner, tools ...string) Env {
	t.Helper()
	return Env{
		Runner:   r,
		LookPath: lookPathFor(tools...),
		Getenv:   func(string) string { return "" },
		GOOS:     goos,
		Home:     t.TempDir(),
		StateDir: t.TempDir(),
		FlowBin:  "/opt/flow/bin/flow",
		Now:      func() time.Time { return time.Date(2026, 11, 22, 10, 0, 0, 0, time.Local) },
		Config:   config.DefaultIntegrationsConfig(),
	}
}

func workStart() Event {
	return Event{Kind: "start", SessionType: "work", StartedAt: time.Date(2026, 11, 22, 10, 0, 0, 0, time.Local), Duration: 25 * time.Minute}
}

func shortcutsList(names ...string) func(string, []string) ([]byte, error) {
	return func(name string, args []string) ([]byte, error) {
		if name == "shortcuts" && len(args) > 0 && args[0] == "list" {
			return []byte(strings.Join(names, "\n") + "\n"), nil
		}
		return nil, nil
	}
}

func TestDispatch_NothingEnabledRunsNothing(t *testing.T) {
	r := &fakeRunner{}
	env := testEnv(t, "darwin", r, "shortcuts", "osascript")
	if errs := Dispatch(context.Background(), env, workStart()); len(errs) != 0 {
		t.Fatalf("errs = %v", errs)
	}
	if got := r.commands(); len(got) != 0 {
		t.Errorf("ran %v with every integration disabled", got)
	}
}

func TestDispatch_BreakStartIsIgnored(t *testing.T) {
	r := &fakeRunner{}
	env := testEnv(t, "darwin", r, "shortcuts", "osascript")
	env.Config.DND, env.Config.Music = true, true
	ev := workStart()
	ev.SessionType = "short_break"
	Dispatch(context.Background(), env, ev)
	if got := r.commands(); len(got) != 0 {
		t.Errorf("a break start ran %v", got)
	}
}

func TestDND_MacOSShortcutsStartEnd(t *testing.T) {
	r := &fakeRunner{}
	env := testEnv(t, "darwin", r, "shortcuts")
	env.Config.DND = true

	if errs := Dispatch(context.Background(), env, workStart()); len(errs) != 0 {
		t.Fatalf("start errs = %v", errs)
	}
	if got, want := r.commands(), []string{"shortcuts run Flow Focus On"}; !equal(got, want) {
		t.Errorf("start ran %v, want %v", got, want)
	}
	if !LoadState(env.StateDir).get().DND {
		t.Fatal("state should record that flow turned DND on")
	}

	// The end usually runs in another process: it only has the state file.
	Dispatch(context.Background(), env, Event{Kind: "end", SessionType: "work"})
	if got, want := r.commands(), []string{"shortcuts run Flow Focus On", "shortcuts run Flow Focus Off"}; !equal(got, want) {
		t.Errorf("start+end ran %v, want %v", got, want)
	}
	if _, err := os.Stat(filepath.Join(env.StateDir, stateFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state file should be removed when nothing is pending, stat err = %v", err)
	}

	// A second end (e.g. void after stop) must not touch the user's Focus.
	Dispatch(context.Background(), env, Event{Kind: "end"})
	if n := len(r.commands()); n != 2 {
		t.Errorf("end without a recorded start ran commands: %v", r.commands())
	}
}

func TestDND_StaleStateStillTurnsOn(t *testing.T) {
	r := &fakeRunner{}
	env := testEnv(t, "darwin", r, "shortcuts")
	env.Config.DND = true
	st := LoadState(env.StateDir)
	st.update(func(d *stateData) { d.DND = true })
	_ = st.Save()
	Dispatch(context.Background(), env, workStart())
	if got := r.commands(); len(got) != 1 || got[0] != "shortcuts run Flow Focus On" {
		t.Errorf("ran %v, want the on shortcut despite stale state", got)
	}
}

func TestTmux_UsesXDGConfigWhenOnlyOne(t *testing.T) {
	env := testEnv(t, "darwin", &fakeRunner{})
	xdg := filepath.Join(env.Home, ".config", "tmux", "tmux.conf")
	_ = os.MkdirAll(filepath.Dir(xdg), 0750)
	_ = os.WriteFile(xdg, []byte("set -g mouse on\n"), 0600)
	if got := tmuxConfPath(env); got != xdg {
		t.Errorf("path = %s, want %s", got, xdg)
	}
	classic := filepath.Join(env.Home, ".tmux.conf")
	_ = os.WriteFile(classic, nil, 0600)
	if got := tmuxConfPath(env); got != classic {
		t.Errorf("with both files path = %s, want %s", got, classic)
	}
}

func TestDND_CustomShortcutNames(t *testing.T) {
	r := &fakeRunner{}
	env := testEnv(t, "darwin", r, "shortcuts")
	env.Config.DND = true
	env.Config.DNDOnShortcut, env.Config.DNDOffShortcut = "Work On", "Work Off"
	Dispatch(context.Background(), env, workStart())
	Dispatch(context.Background(), env, Event{Kind: "end"})
	if got, want := r.commands(), []string{"shortcuts run Work On", "shortcuts run Work Off"}; !equal(got, want) {
		t.Errorf("ran %v, want %v", got, want)
	}
}

func TestDND_Linux(t *testing.T) {
	tests := []struct {
		name  string
		tools []string
		want  []string
	}{
		{"dunst", []string{"dunstctl", "gsettings"}, []string{"dunstctl set-paused true", "dunstctl set-paused false"}},
		{"gnome", []string{"gsettings"}, []string{
			"gsettings set org.gnome.desktop.notifications show-banners false",
			"gsettings set org.gnome.desktop.notifications show-banners true",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{}
			env := testEnv(t, "linux", r, tt.tools...)
			env.Config.DND = true
			Dispatch(context.Background(), env, workStart())
			Dispatch(context.Background(), env, Event{Kind: "end"})
			if got := r.commands(); !equal(got, tt.want) {
				t.Errorf("ran %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDND_Check(t *testing.T) {
	ctx := context.Background()

	r := &fakeRunner{reply: shortcutsList("Flow Focus On")}
	st := dnd{}.Check(ctx, testEnv(t, "darwin", r, "shortcuts"))
	if !st.Supported || st.Ready || !strings.Contains(st.Detail, "missing") || st.Args[0] != "Flow Focus Off" {
		t.Errorf("one shortcut missing: %+v", st)
	}

	r = &fakeRunner{reply: shortcutsList("Other", "Flow Focus On", "Flow Focus Off")}
	if st := (dnd{}).Check(ctx, testEnv(t, "darwin", r, "shortcuts")); !st.Ready {
		t.Errorf("both shortcuts present should be ready: %+v", st)
	}

	if st := (dnd{}).Check(ctx, testEnv(t, "linux", &fakeRunner{})); st.Supported {
		t.Errorf("linux without dunstctl/gsettings should be unsupported: %+v", st)
	}
	if st := (dnd{}).Check(ctx, testEnv(t, "windows", &fakeRunner{})); st.Supported {
		t.Errorf("windows should be unsupported: %+v", st)
	}

	// enable explains how to create the missing Shortcuts.
	r = &fakeRunner{reply: shortcutsList()}
	p, err := dnd{}.Plan(ctx, testEnv(t, "darwin", r, "shortcuts"), true)
	if err != nil || len(p.Notes) == 0 || len(p.Changes) != 0 {
		t.Fatalf("plan = %+v, %v; want notes only", p, err)
	}
	if !strings.Contains(p.Notes[0].Text, "Shortcuts") {
		t.Errorf("first note = %q", p.Notes[0].Text)
	}
}

func TestDispatch_HangingRunnerDoesNotBlock(t *testing.T) {
	r := &fakeRunner{block: true}
	env := testEnv(t, "darwin", r, "shortcuts", "osascript")
	env.Config.DND, env.Config.Music = true, true
	env.Timeout = 50 * time.Millisecond

	start := time.Now()
	errs := Dispatch(context.Background(), env, workStart())
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Dispatch took %v with hanging commands", elapsed)
	}
	if !errors.Is(errs["dnd"], context.DeadlineExceeded) || errs["music"] == nil {
		t.Errorf("errs = %v, want timeouts for dnd and music", errs)
	}
	if LoadState(env.StateDir).get().DND {
		t.Error("a failed start must not record DND as on")
	}
}

// ignoringRunner never returns, even after its context ends.
type ignoringRunner struct{ release chan struct{} }

func (r ignoringRunner) Run(context.Context, string, ...string) ([]byte, error) {
	<-r.release
	return nil, nil
}

func TestDispatch_HandlerIgnoringContextDoesNotBlock(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	env := testEnv(t, "darwin", nil, "shortcuts")
	env.Runner = ignoringRunner{release: release}
	env.Config.DND = true
	env.Timeout = 50 * time.Millisecond

	done := make(chan struct{})
	go func() {
		Dispatch(context.Background(), env, workStart())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Dispatch waited for a handler that ignores its context")
	}
}

// slackServer is a stand-in for the Slack Web API.
type slackServer struct {
	mu       sync.Mutex
	auth     []string
	profiles []slackProfile
	reply    string
}

func newSlackServer(t *testing.T, reply string) (*slackServer, *httptest.Server) {
	s := &slackServer{reply: reply}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users.profile.set" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Profile slackProfile `json:"profile"`
		}
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &body); err != nil {
			t.Errorf("bad body %q: %v", data, err)
		}
		s.mu.Lock()
		s.auth = append(s.auth, r.Header.Get("Authorization"))
		s.profiles = append(s.profiles, body.Profile)
		s.mu.Unlock()
		_, _ = io.WriteString(w, s.reply)
	}))
	t.Cleanup(srv.Close)
	return s, srv
}

func TestSlack_SetsAndClearsStatus(t *testing.T) {
	s, srv := newSlackServer(t, `{"ok":true}`)
	r := &fakeRunner{}
	env := testEnv(t, "darwin", r, "security")
	env.Config.Slack = true
	env.HTTP = srv.Client()
	env.SlackAPI = srv.URL
	env.Getenv = func(k string) string {
		if k == SlackTokenEnv {
			return "xoxp-env-token"
		}
		return ""
	}

	ev := workStart()
	if errs := Dispatch(context.Background(), env, ev); len(errs) != 0 {
		t.Fatalf("start errs = %v", errs)
	}
	Dispatch(context.Background(), env, Event{Kind: "end"})

	if len(s.profiles) != 2 {
		t.Fatalf("got %d Slack calls, want 2", len(s.profiles))
	}
	set := s.profiles[0]
	if set.StatusText != "Focusing until 10:25" || set.StatusEmoji != ":tomato:" || set.StatusExpiration != ev.Ends().Unix() {
		t.Errorf("start profile = %+v", set)
	}
	if s.profiles[1] != (slackProfile{}) {
		t.Errorf("end profile = %+v, want cleared", s.profiles[1])
	}
	for _, a := range s.auth {
		if a != "Bearer xoxp-env-token" {
			t.Errorf("Authorization = %q", a)
		}
	}
	if len(r.commands()) != 0 {
		t.Errorf("an env token should not touch the keychain, ran %v", r.commands())
	}
}

func TestSlack_KeychainTokenAndAPIError(t *testing.T) {
	s, srv := newSlackServer(t, `{"ok":false,"error":"missing_scope"}`)
	r := &fakeRunner{reply: func(name string, args []string) ([]byte, error) {
		if name == "security" {
			return []byte("xoxp-keychain\n"), nil
		}
		return nil, nil
	}}
	env := testEnv(t, "darwin", r, "security")
	env.Config.Slack = true
	env.HTTP = srv.Client()
	env.SlackAPI = srv.URL

	errs := Dispatch(context.Background(), env, workStart())
	if errs["slack"] == nil || !strings.Contains(errs["slack"].Error(), "missing_scope") {
		t.Errorf("errs = %v, want the Slack error", errs)
	}
	if len(s.auth) != 1 || s.auth[0] != "Bearer xoxp-keychain" {
		t.Errorf("auth = %v", s.auth)
	}
	want := "security find-generic-password -s " + SlackKeychainItem + " -w"
	if got := r.commands(); len(got) != 1 || got[0] != want {
		t.Errorf("ran %v, want %q", got, want)
	}
	if LoadState(env.StateDir).get().Slack {
		t.Error("a failed status update must not be recorded")
	}
	// Nothing recorded → the end does not clear a status flow never set.
	Dispatch(context.Background(), env, Event{Kind: "end"})
	if len(s.profiles) != 1 {
		t.Errorf("end called Slack %d times without a recorded start", len(s.profiles)-1)
	}
}

func TestSlack_NoTokenCheckAndPlan(t *testing.T) {
	r := &fakeRunner{reply: func(string, []string) ([]byte, error) { return nil, errors.New("not found") }}
	env := testEnv(t, "darwin", r, "security")
	if st := (slack{}).Check(context.Background(), env); st.Ready || !st.Supported {
		t.Errorf("no token: %+v", st)
	}
	p, _ := slack{}.Plan(context.Background(), env, true)
	joined := ""
	for _, n := range p.Notes {
		joined += n.Text + "\n"
	}
	if !strings.Contains(joined, "users.profile:write") || !strings.Contains(joined, "add-generic-password") {
		t.Errorf("plan notes do not explain the token setup:\n%s", joined)
	}
	if len(p.Changes) != 0 {
		t.Errorf("slack must never write the token to a file: %+v", p.Changes)
	}
}

func TestMusic_PausesAndResumesOnlyWhatItPaused(t *testing.T) {
	r := &fakeRunner{reply: func(name string, args []string) ([]byte, error) {
		if name == "osascript" && strings.Contains(args[1], `"Spotify"`) && strings.Contains(args[1], "pause") {
			return []byte("paused\n"), nil
		}
		return []byte("\n"), nil
	}}
	env := testEnv(t, "darwin", r, "osascript")
	env.Config.Music = true

	Dispatch(context.Background(), env, workStart())
	if got := LoadState(env.StateDir).get().Music; !equal(got, []string{"Spotify"}) {
		t.Fatalf("paused = %v, want [Spotify]", got)
	}
	for _, c := range r.calls {
		if !strings.Contains(c[2], "is running") {
			t.Errorf("script without the is-running guard would launch the app: %q", c[2])
		}
	}

	before := len(r.commands())
	Dispatch(context.Background(), env, Event{Kind: "end"})
	calls := r.calls[before:]
	if len(calls) != 1 || !strings.Contains(calls[0][2], `tell application "Spotify" to play`) {
		t.Errorf("end ran %v, want only Spotify play", calls)
	}
}

func TestMusic_NotMacOS(t *testing.T) {
	r := &fakeRunner{}
	env := testEnv(t, "linux", r, "osascript")
	env.Config.Music = true
	Dispatch(context.Background(), env, workStart())
	if len(r.commands()) != 0 {
		t.Errorf("ran %v on linux", r.commands())
	}
	if st := (music{}).Check(context.Background(), env); st.Supported {
		t.Errorf("music should be unsupported on linux: %+v", st)
	}
}

func TestUndo_RunsEndEvenWhenDisabled(t *testing.T) {
	r := &fakeRunner{}
	env := testEnv(t, "darwin", r, "shortcuts")
	env.Config.DND = true
	Dispatch(context.Background(), env, workStart())
	env.Config.DND = false
	if err := Undo(context.Background(), env, dnd{}); err != nil {
		t.Fatal(err)
	}
	if got := r.commands(); len(got) != 2 || got[1] != "shortcuts run Flow Focus Off" {
		t.Errorf("ran %v", got)
	}
	if err := Undo(context.Background(), env, tmux{}); err != nil {
		t.Errorf("Undo of an install-only integration = %v", err)
	}
}

func TestStatusLine(t *testing.T) {
	now := time.Now()
	work := &domain.PomodoroSession{Type: domain.SessionTypeWork, Status: domain.SessionStatusRunning, StartedAt: now.Add(-5 * time.Minute), Duration: 25 * time.Minute}
	if got := StatusLine(work, ""); got != "🍅 20:00" && got != "🍅 19:59" {
		t.Errorf("work = %q", got)
	}
	long := &domain.PomodoroSession{Type: domain.SessionTypeWork, Status: domain.SessionStatusRunning, StartedAt: now, Duration: 90 * time.Minute}
	if got := StatusLine(long, "🔥"); !strings.HasPrefix(got, "🔥 1:") {
		t.Errorf("long = %q", got)
	}
	brk := &domain.PomodoroSession{Type: domain.SessionTypeShortBreak, Status: domain.SessionStatusRunning, StartedAt: now, Duration: 5 * time.Minute}
	if got := StatusLine(brk, ""); !strings.HasPrefix(got, "☕ ") {
		t.Errorf("break = %q", got)
	}
	paused := now.Add(-time.Minute)
	p := &domain.PomodoroSession{Type: domain.SessionTypeWork, Status: domain.SessionStatusPaused, StartedAt: now.Add(-2 * time.Minute), PausedAt: &paused, Duration: 25 * time.Minute}
	if got := StatusLine(p, ""); !strings.HasPrefix(got, "⏸ ") {
		t.Errorf("paused = %q", got)
	}
	if got := StatusLine(nil, ""); got != "" {
		t.Errorf("idle = %q, want empty", got)
	}
}

func TestBlocks_UpsertIsIdempotentAndRemoveRestores(t *testing.T) {
	user := "set -g mouse on\nset -g status-right '%H:%M'\n"
	once := upsertBlock(user, "tmux", "line 1\nline 2")
	twice := upsertBlock(once, "tmux", "line 1\nline 2")
	if once != twice {
		t.Errorf("upsert not idempotent:\n%s\n---\n%s", once, twice)
	}
	if !strings.HasPrefix(once, user) || strings.Count(once, "line 1") != 1 {
		t.Errorf("upsert lost user content:\n%s", once)
	}
	updated := upsertBlock(once+"bind r source-file ~/.tmux.conf\n", "tmux", "new")
	if strings.Contains(updated, "line 1") || !strings.Contains(updated, "new") || !strings.HasSuffix(updated, "bind r source-file ~/.tmux.conf\n") {
		t.Errorf("upsert did not replace in place:\n%s", updated)
	}
	if got := removeBlock(once, "tmux"); got != user {
		t.Errorf("remove = %q, want %q", got, user)
	}
	if got := removeBlock(user, "tmux"); got != user {
		t.Errorf("remove without a block changed the text: %q", got)
	}
	if got := removeBlock(upsertBlock("", "tmux", "x"), "tmux"); got != "" {
		t.Errorf("remove from a flow-only file = %q", got)
	}
}

func TestTmux_PlanEditsConfWithConfirmation(t *testing.T) {
	env := testEnv(t, "darwin", &fakeRunner{}, "tmux")
	env.DBPath = "/tmp/my db/flow.db"
	conf := filepath.Join(env.Home, ".tmux.conf")
	if err := os.WriteFile(conf, []byte("set -g mouse on\n"), 0600); err != nil {
		t.Fatal(err)
	}

	p, err := tmux{}.Plan(context.Background(), env, true)
	if err != nil || len(p.Changes) != 1 {
		t.Fatalf("plan = %+v, %v", p, err)
	}
	c := p.Changes[0]
	if !c.Confirm || c.Kind != UpsertBlock || c.Path != conf {
		t.Errorf("change = %+v, want a confirmed block edit of %s", c, conf)
	}
	if err := c.Apply(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(conf)
	want := `set -ga status-right " #(/opt/flow/bin/flow --db '/tmp/my db/flow.db' tmux-status)"`
	if !strings.Contains(string(data), want) || !strings.HasPrefix(string(data), "set -g mouse on\n") {
		t.Errorf(".tmux.conf =\n%s\nwant it to contain %s", data, want)
	}
	if info, _ := os.Stat(conf); info.Mode().Perm() != 0600 {
		t.Errorf("mode changed to %v", info.Mode().Perm())
	}

	p, _ = tmux{}.Plan(context.Background(), env, false)
	if err := p.Changes[0].Apply(); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(conf); string(data) != "set -g mouse on\n" {
		t.Errorf("after disable .tmux.conf = %q", data)
	}
}

func TestBars_WriteOwnedAndRemoveOnlyOwned(t *testing.T) {
	for _, b := range []bar{sketchybar(), waybar(), xbar()} {
		t.Run(b.name, func(t *testing.T) {
			env := testEnv(t, "darwin", &fakeRunner{})
			p, err := b.Plan(context.Background(), env, true)
			if err != nil || len(p.Changes) != 1 || p.Changes[0].Confirm {
				t.Fatalf("plan = %+v, %v", p, err)
			}
			if err := p.Changes[0].Apply(); err != nil {
				t.Fatal(err)
			}
			path := b.path(env)
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm()&0100 == 0 {
				t.Fatalf("script %s not executable: %v %v", path, info, err)
			}
			data, _ := os.ReadFile(path)
			if !strings.Contains(string(data), Marker) || !strings.Contains(string(data), "status --json") || !strings.Contains(string(data), "/opt/flow/bin/flow") {
				t.Errorf("script content:\n%s", data)
			}
			if st := b.Check(context.Background(), env); !st.Ready {
				t.Errorf("after install: %+v", st)
			}

			p, _ = b.Plan(context.Background(), env, false)
			if len(p.Changes) != 1 || p.Changes[0].Kind != RemoveFile {
				t.Fatalf("disable plan = %+v", p)
			}
			_ = p.Changes[0].Apply()
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("script not removed: %v", err)
			}

			// A file flow did not write: enable asks, disable leaves it.
			_ = os.WriteFile(path, []byte("#!/bin/sh\necho mine\n"), 0700)
			p, _ = b.Plan(context.Background(), env, true)
			if !p.Changes[0].Confirm {
				t.Error("replacing a foreign file should need confirmation")
			}
			p, _ = b.Plan(context.Background(), env, false)
			if len(p.Changes) != 0 {
				t.Errorf("disable would touch a foreign file: %+v", p.Changes)
			}
		})
	}
}

// TestBarScripts_RenderStatusJSON runs the generated scripts against a fake
// flow binary that prints a fixed `status --json`.
func TestBarScripts_RenderStatusJSON(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("no jq")
	}
	dir := t.TempDir()
	fake := filepath.Join(dir, "flow")
	status := `{"active_session":{"type":"work","status":"running","remaining_time":"12m34.56s"},"active_task":{"title":"Write \"docs\""}}`
	script := "#!/bin/sh\nif [ \"$1\" = status ]; then echo '" + status + "'; else echo 'fallback'; fi\n"
	if err := os.WriteFile(fake, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	env := testEnv(t, "darwin", &fakeRunner{})
	env.FlowBin = fake

	run := func(b bar) string {
		path := filepath.Join(dir, b.name+".sh")
		if err := os.WriteFile(path, []byte(b.body(env)), 0700); err != nil {
			t.Fatal(err)
		}
		// sketchybar is replaced by echo so the script's call is visible.
		shim := filepath.Join(dir, "bin")
		_ = os.MkdirAll(shim, 0750)
		_ = os.WriteFile(filepath.Join(shim, "sketchybar"), []byte("#!/bin/sh\necho \"$@\"\n"), 0700)
		cmd := exec.Command(path)
		cmd.Env = append(os.Environ(), "PATH="+shim+":"+os.Getenv("PATH"), "NAME=flow")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %v", b.name, err)
		}
		return string(out)
	}

	if got := run(xbar()); !strings.HasPrefix(got, "🍅 12:34\n---\nWrite \"docs\"\n") {
		t.Errorf("xbar output:\n%s", got)
	}
	var w struct{ Text, Tooltip, Class string }
	if err := json.Unmarshal([]byte(run(waybar())), &w); err != nil {
		t.Fatalf("waybar output is not JSON: %v", err)
	}
	if w.Text != "🍅 12:34" || w.Tooltip != `Write "docs"` || w.Class != "work" {
		t.Errorf("waybar = %+v", w)
	}
	if got := run(sketchybar()); !strings.Contains(got, "--set flow drawing=on label=🍅 12:34") {
		t.Errorf("sketchybar call: %q", got)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/local/bin/flow": "/usr/local/bin/flow",
		"/a b/flow":           "'/a b/flow'",
		"it's":                `'it'\''s'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRegistry(t *testing.T) {
	names := Names()
	for _, want := range []string{"dnd", "slack", "tmux", "sketchybar", "waybar", "xbar", "music"} {
		if _, ok := Find(want); !ok {
			t.Errorf("integration %q missing (have %v)", want, names)
		}
	}
	// Enabled/SetEnabled must address the same flag for every integration.
	for _, in := range All() {
		var c config.IntegrationsConfig
		in.SetEnabled(&c, true)
		if !in.Enabled(c) {
			t.Errorf("%s: SetEnabled(true) not seen by Enabled", in.Name())
		}
		on := 0
		for _, other := range All() {
			if other.Enabled(c) {
				on++
			}
		}
		if on != 1 {
			t.Errorf("%s: SetEnabled turned on %d integrations", in.Name(), on)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSpanishCatalogCoversIntegrations(t *testing.T) {
	i18n.SetLanguage("es")
	defer i18n.SetLanguage("en")
	keys := []string{"Focusing until %s"}
	for _, in := range All() {
		keys = append(keys, in.Description())
	}
	ctx := context.Background()
	for _, goos := range []string{"darwin", "linux", "windows"} {
		env := testEnv(t, goos, &fakeRunner{reply: shortcutsList()}, "shortcuts", "osascript")
		for _, in := range All() {
			if st := in.Check(ctx, env); st.Detail != "%s" {
				keys = append(keys, st.Detail)
			}
			for _, on := range []bool{true, false} {
				p, _ := in.Plan(ctx, env, on)
				for _, c := range p.Changes {
					text, _ := c.Describe()
					keys = append(keys, text)
				}
				for _, n := range p.Notes {
					if n.Text != "  %s" && !strings.HasPrefix(n.Text, "  sketchybar ") && !strings.HasPrefix(n.Text, "  \"custom/flow\"") && !strings.HasPrefix(n.Text, "  secret-tool") {
						keys = append(keys, n.Text)
					}
				}
			}
		}
	}
	for _, k := range keys {
		if i18n.T(k) == k {
			t.Errorf("no Spanish translation for %q", k)
		}
	}
}

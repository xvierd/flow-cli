package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/i18n"
)

// SlackTimeout bounds one Slack API call.
const SlackTimeout = 4 * time.Second

// Where the Slack token is looked up. It is never stored in config.toml.
const (
	SlackTokenEnv     = "FLOW_SLACK_TOKEN"
	SlackKeychainItem = "flow-cli-slack"
)

// slack sets "🍅 Focusing until HH:MM" as the Slack status during work
// sessions, with an expiration at the session end so it clears itself even
// if the end is never seen.
type slack struct{}

func (slack) Name() string { return "slack" }
func (slack) Description() string {
	return "Slack status \"Focusing until HH:MM\" during work sessions"
}
func (slack) Enabled(c config.IntegrationsConfig) bool { return c.Slack }
func (slack) SetEnabled(c *config.IntegrationsConfig, on bool) {
	c.Slack = on
	if c.SlackEmoji == "" {
		c.SlackEmoji = config.DefaultSlackEmoji
	}
}
func (slack) Timeout() time.Duration { return SlackTimeout + time.Second }

var errNoSlackToken = errors.New("no Slack token found")

// token returns the user token from FLOW_SLACK_TOKEN, else the OS keychain
// (macOS Keychain via `security`, Linux Secret Service via `secret-tool`).
// The token is read through the runner's stdout, never passed as an argv.
func slackToken(ctx context.Context, env Env) (string, error) {
	if env.Getenv != nil {
		if t := strings.TrimSpace(env.Getenv(SlackTokenEnv)); t != "" {
			return t, nil
		}
	}
	var out []byte
	var err error
	switch {
	case env.GOOS == "darwin" && env.has("security"):
		out, err = env.Runner.Run(ctx, "security", "find-generic-password", "-s", SlackKeychainItem, "-w")
	case env.GOOS != "darwin" && env.GOOS != "windows" && env.has("secret-tool"):
		out, err = env.Runner.Run(ctx, "secret-tool", "lookup", "service", SlackKeychainItem)
	default:
		return "", errNoSlackToken
	}
	if err != nil {
		return "", errNoSlackToken
	}
	t := strings.TrimSpace(string(out))
	if t == "" {
		return "", errNoSlackToken
	}
	return t, nil
}

func (s slack) Check(ctx context.Context, env Env) Status {
	if _, err := slackToken(ctx, env); err != nil {
		return Status{Supported: true, Detail: "no token: set %s or store it in the keychain (see flow integrate enable slack)", Args: []any{SlackTokenEnv}}
	}
	return Status{Supported: true, Ready: true, Detail: "token found"}
}

func (s slack) Plan(ctx context.Context, env Env, on bool) (Plan, error) {
	var p Plan
	if !on {
		return p, nil
	}
	if _, err := slackToken(ctx, env); err == nil {
		return p, nil
	}
	p.note("Slack needs a user token (xoxp-…) with the users.profile:write scope: create a Slack app at https://api.slack.com/apps, add that user scope and install it to your workspace.")
	p.note("Then give flow the token in one of these ways (it is never written to config.toml):")
	switch env.GOOS {
	case "darwin":
		p.note("  security add-generic-password -a \"$USER\" -s %s -w   # prompts for the token", SlackKeychainItem)
	case "windows":
	default:
		p.note("  secret-tool store --label=\"flow Slack token\" service %s", SlackKeychainItem)
	}
	p.note("  export %s=xoxp-…   # in your shell profile", SlackTokenEnv)
	return p, nil
}

type slackProfile struct {
	StatusText       string `json:"status_text"`
	StatusEmoji      string `json:"status_emoji"`
	StatusExpiration int64  `json:"status_expiration"`
}

// setStatus calls users.profile.set. Slack answers HTTP 200 with
// {"ok":false,"error":...} on failure, so ok is checked too.
func setStatus(ctx context.Context, env Env, p slackProfile) error {
	token, err := slackToken(ctx, env)
	if err != nil {
		return err
	}
	if env.HTTP == nil {
		return errors.New("no HTTP client")
	}
	body, err := json.Marshal(map[string]any{"profile": p})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(env.SlackAPI, "/")+"/users.profile.set", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := env.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack: HTTP %d", resp.StatusCode)
	}
	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("slack: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("slack: %s", result.Error)
	}
	return nil
}

// FocusStatusText is the Slack status shown until ends.
func FocusStatusText(ends time.Time) string {
	return i18n.T("Focusing until %s", ends.Local().Format("15:04"))
}

func (s slack) Start(ctx context.Context, env Env, st *State, ev Event) error {
	ends := ev.Ends()
	if ev.StartedAt.IsZero() || ev.Duration <= 0 {
		ends = env.now().Add(ev.Duration)
	}
	emoji := env.Config.SlackEmoji
	if emoji == "" {
		emoji = config.DefaultSlackEmoji
	}
	err := setStatus(ctx, env, slackProfile{
		StatusText:       FocusStatusText(ends),
		StatusEmoji:      emoji,
		StatusExpiration: ends.Unix(),
	})
	if err != nil {
		return err
	}
	st.update(func(d *stateData) { d.Slack = true })
	return nil
}

func (s slack) End(ctx context.Context, env Env, st *State, _ Event) error {
	if !st.get().Slack {
		return nil
	}
	err := setStatus(ctx, env, slackProfile{})
	st.update(func(d *stateData) { d.Slack = false })
	return err
}

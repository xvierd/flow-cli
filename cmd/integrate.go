package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xvierd/flow-cli/internal/adapters/integrations"
	"github.com/xvierd/flow-cli/internal/config"
	"github.com/xvierd/flow-cli/internal/i18n"
)

var (
	integrateDryRun bool
	integrateYes    bool
)

// confirmChange asks the user before an edit of a file they own. It is a
// variable so tests can answer.
var confirmChange = func(question string) bool {
	if !interactiveTerminal() {
		return false
	}
	fmt.Print(question + " ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes" || answer == "s" || answer == "si" || answer == "sí"
}

func currentIntegrationsConfig() config.IntegrationsConfig {
	if app.config == nil {
		return config.DefaultIntegrationsConfig()
	}
	return app.config.Integrations
}

var integrateCmd = &cobra.Command{
	Use:     "integrate",
	Aliases: []string{"integrations"},
	Short:   "Built-in integrations: Do Not Disturb, Slack status, music, tmux and status bars",
	Long: `Turn on ready-made recipes that run when sessions start and end, next to
your own [hooks]: Do Not Disturb / Focus, a Slack status, pausing music, and
the timer in tmux, SketchyBar, Waybar or xbar.

  flow integrate list
  flow integrate enable dnd --dry-run
  flow integrate disable slack`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIntegrateList(cmd.Context())
	},
}

var integrateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the integrations and whether they are enabled and ready",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIntegrateList(cmd.Context())
	},
}

var integrateEnableCmd = &cobra.Command{
	Use:       "enable <name>",
	Short:     "Enable an integration (shows what it changes; --dry-run changes nothing)",
	Args:      cobra.ExactArgs(1),
	ValidArgs: integrations.Names(),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIntegrateToggle(cmd.Context(), args[0], true)
	},
}

var integrateDisableCmd = &cobra.Command{
	Use:       "disable <name>",
	Short:     "Disable an integration and remove what flow installed for it",
	Args:      cobra.ExactArgs(1),
	ValidArgs: integrations.Names(),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIntegrateToggle(cmd.Context(), args[0], false)
	},
}

var tmuxStatusCmd = &cobra.Command{
	Use:   "tmux-status",
	Short: "Print a short timer for the tmux status bar (empty when idle)",
	Long: `Print the active session as "🍅 12:34" (work), "☕ 04:10" (break) or
"⏸ 12:34" (paused), and nothing when no session is active. Meant for
tmux's status-right: set -ga status-right " #(flow tmux-status)".`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := app.state.GetCurrentState(context.Background())
		if err != nil || state == nil {
			// A status bar must never show an error message.
			return nil
		}
		icon := ""
		if app.config != nil {
			icon = app.config.Theme.IconApp
		}
		if line := integrations.StatusLine(state.ActiveSession, icon); line != "" {
			fmt.Println(line)
		}
		return nil
	},
}

func init() {
	for _, c := range []*cobra.Command{integrateEnableCmd, integrateDisableCmd} {
		c.Flags().BoolVar(&integrateDryRun, "dry-run", false, "Show what would change without changing anything")
		c.Flags().BoolVarP(&integrateYes, "yes", "y", false, "Apply edits to your own files (e.g. ~/.tmux.conf) without asking")
	}
	integrateCmd.AddCommand(integrateListCmd, integrateEnableCmd, integrateDisableCmd)
	rootCmd.AddCommand(integrateCmd, tmuxStatusCmd)
}

func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// statusText translates an integration status.
func statusText(st integrations.Status) (state, detail string) {
	switch {
	case !st.Supported:
		state = i18n.T("unsupported")
	case st.Ready:
		state = i18n.T("ready")
	default:
		state = i18n.T("needs setup")
	}
	return state, i18n.T(st.Detail, st.Args...)
}

func runIntegrateList(ctx context.Context) error {
	ctx = ctxOrBackground(ctx)
	env := integrationsEnv()
	cfg := currentIntegrationsConfig()

	type row struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
		Supported   bool   `json:"supported"`
		Ready       bool   `json:"ready"`
		Detail      string `json:"detail"`
	}
	var rows []row
	for _, in := range integrations.All() {
		st := in.Check(ctx, env)
		_, detail := statusText(st)
		rows = append(rows, row{
			Name:        in.Name(),
			Description: i18n.T(in.Description()),
			Enabled:     in.Enabled(cfg),
			Supported:   st.Supported,
			Ready:       st.Ready,
			Detail:      detail,
		})
	}
	if jsonOutput {
		return jsonOut(rows)
	}

	fmt.Println()
	fmt.Println("  " + i18n.T("Integrations"))
	fmt.Println()
	for i, in := range integrations.All() {
		r := rows[i]
		mark := "○"
		onOff := i18n.T("disabled")
		if r.Enabled {
			mark, onOff = "●", i18n.T("enabled")
		}
		state, _ := statusText(in.Check(ctx, env))
		fmt.Printf("  %s %-11s %s — %s\n", mark, r.Name, onOff, r.Description)
		fmt.Printf("    %-11s %s: %s\n", "", state, r.Detail)
	}
	fmt.Println()
	fmt.Println("  " + i18n.T("Enable one with: flow integrate enable <name>  (add --dry-run to preview)"))
	return nil
}

func runIntegrateToggle(ctx context.Context, name string, on bool) error {
	ctx = ctxOrBackground(ctx)
	in, ok := integrations.Find(name)
	if !ok {
		return errors.New(i18n.T("unknown integration %q (available: %s)", name, strings.Join(integrations.Names(), ", ")))
	}
	env := integrationsEnv()

	if on {
		if st := in.Check(ctx, env); !st.Supported {
			_, detail := statusText(st)
			return errors.New(i18n.T("%s cannot be enabled here: %s", name, detail))
		}
	}

	plan, err := in.Plan(ctx, env, on)
	if err != nil {
		return err
	}

	configPath, _ := config.GetConfigPath()
	header := "Enabling %s:"
	if !on {
		header = "Disabling %s:"
	}
	if integrateDryRun {
		header = "Would enable %s:"
		if !on {
			header = "Would disable %s:"
		}
	}
	fmt.Println(i18n.T(header, name))
	for _, c := range plan.Changes {
		text, args := c.Describe()
		fmt.Println("  • " + i18n.T(text, args...))
	}
	fmt.Println("  • " + i18n.T("set [integrations] %s = %v in %s", name, on, configPath))
	if !on {
		if _, isHandler := in.(integrations.SessionHandler); isHandler {
			fmt.Println("  • " + i18n.T("undo what it changed for the current session, if anything"))
		}
	}

	printNotes := func() {
		if len(plan.Notes) == 0 {
			return
		}
		fmt.Println()
		for _, n := range plan.Notes {
			fmt.Println("  " + i18n.T(n.Text, n.Args...))
		}
	}

	if integrateDryRun {
		printNotes()
		fmt.Println()
		fmt.Println(i18n.T("Dry run: nothing was changed."))
		return nil
	}

	for _, c := range plan.Changes {
		if c.Confirm && !integrateYes {
			if !confirmChange(i18n.T("Change %s? [y/N]", c.Path)) {
				printNotes()
				fmt.Println()
				fmt.Println(i18n.T("Nothing was changed. Re-run with --yes to apply it without asking, or make the change yourself."))
				return nil
			}
		}
	}
	for _, c := range plan.Changes {
		if err := c.Apply(); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("failed to update %s", c.Path), err)
		}
	}

	if !on {
		_ = integrations.Undo(ctx, env, in)
	}

	in.SetEnabled(&app.config.Integrations, on)
	if err := saveConfig(app.config); err != nil {
		return err
	}

	printNotes()
	fmt.Println()
	if on {
		fmt.Println(i18n.T("✓ %s enabled.", name))
		env.Config = app.config.Integrations
		if st := in.Check(ctx, env); !st.Ready {
			_, detail := statusText(st)
			fmt.Println(i18n.T("  Not ready yet: %s", detail))
		}
	} else {
		fmt.Println(i18n.T("✓ %s disabled.", name))
	}
	return nil
}

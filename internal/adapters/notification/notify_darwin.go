//go:build darwin

package notification

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
)

// macOS shows the icon of the app that posts a notification, and
// `osascript` (what beeep falls back to) always posts as Script Editor. So
// Flow keeps a tiny AppleScript applet, Flow.app, with the Flow icon in
// ~/Library/Application Support/flow and posts through it. The applet is
// built at runtime with osacompile (present on every Mac), because release
// binaries are cross-compiled on Linux and cannot ship a prebuilt bundle.
//
// The first notification makes macOS ask whether Flow may send
// notifications. If anything in this path fails, Flow falls back to beeep,
// so the user still gets a notification, just without the icon.

//go:embed assets/flow.icns
var iconICNS []byte

const (
	bundleID   = "dev.flowcli.flow"
	bundleName = "Flow"

	// bundleRevision is part of the stamp; bump it when the bundle layout
	// changes in a way the script and icon hashes don't capture.
	bundleRevision = "1"

	titleEnv   = "FLOW_NOTIFY_TITLE"
	messageEnv = "FLOW_NOTIFY_MESSAGE"

	lsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"
)

// appletScript reads the notification from the environment that `open --env`
// passes in. It reads through printenv because `system attribute` decodes
// the value as Mac Roman and garbles anything non-ASCII (emoji, accents).
// The short delay lets the notification post before the applet quits.
const appletScript = `on run
	set t to do shell script "/usr/bin/printenv ` + titleEnv + ` || true"
	set m to do shell script "/usr/bin/printenv ` + messageEnv + ` || true"
	display notification m with title t
	delay 1
end run
`

const stampName = "flow-notifier.stamp"

var bundleMu sync.Mutex

// send posts a desktop notification with the Flow icon.
func send(title, message string) error {
	if err := bundleNotify(title, message); err == nil {
		return nil
	}
	return beeep.Notify(title, message, "")
}

func bundleNotify(title, message string) error {
	app, err := bundlePath()
	if err != nil {
		return err
	}
	created, err := ensureBundle(app, true)
	if err != nil {
		return err
	}
	// -n: a fresh instance per notification; -g: don't steal focus.
	if err := run("/usr/bin/open", "-n", "-g",
		"--env", titleEnv+"="+title,
		"--env", messageEnv+"="+message,
		app); err != nil {
		return err
	}
	// A brand-new app's first notification only triggers macOS's permission
	// prompt and is dropped, and open can't tell. Send the plain one too so
	// this session end isn't missed.
	if created {
		_ = beeep.Notify(title, message, "")
	}
	return nil
}

func bundlePath() (string, error) {
	dir, err := os.UserConfigDir() // ~/Library/Application Support
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "flow", bundleName+".app"), nil
}

func bundleStamp() string {
	h := sha256.New()
	h.Write([]byte(bundleRevision + bundleID + appletScript))
	h.Write(iconICNS)
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// ensureBundle builds the applet at app unless an up-to-date one is already
// there. It builds next to app and renames into place, so a half-built
// bundle is never left at app. register adds it to Launch Services, which
// macOS needs before it will ask the user for notification permission.
// created reports that no bundle existed at app before this call.
func ensureBundle(app string, register bool) (created bool, err error) {
	bundleMu.Lock()
	defer bundleMu.Unlock()

	stamp := bundleStamp()
	if b, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", stampName)); err == nil && string(b) == stamp { //nolint:gosec // path built from the user's config dir
		return false, nil
	}
	if _, err := os.Stat(app); os.IsNotExist(err) {
		created = true
	}

	parent := filepath.Dir(app)
	if err := os.MkdirAll(parent, 0o750); err != nil {
		return false, err
	}
	tmp, err := os.MkdirTemp(parent, ".build-")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	script := filepath.Join(tmp, "notify.applescript")
	if err := os.WriteFile(script, []byte(appletScript), 0o600); err != nil {
		return false, err
	}
	out := filepath.Join(tmp, bundleName+".app")
	if err := run("/usr/bin/osacompile", "-o", out, script); err != nil {
		return false, err
	}

	res := filepath.Join(out, "Contents", "Resources")
	if err := os.WriteFile(filepath.Join(res, "applet.icns"), iconICNS, 0o600); err != nil {
		return false, err
	}
	// The compiled asset catalog holds the default applet icon and would
	// win over applet.icns.
	if err := os.Remove(filepath.Join(res, "Assets.car")); err != nil && !os.IsNotExist(err) {
		return false, err
	}

	plist := filepath.Join(out, "Contents", "Info.plist")
	for _, kv := range [][2]string{{"CFBundleIdentifier", bundleID}, {"CFBundleName", bundleName}} {
		if err := run("/usr/bin/plutil", "-replace", kv[0], "-string", kv[1], plist); err != nil {
			return false, err
		}
	}
	_ = run("/usr/bin/plutil", "-remove", "CFBundleIconName", plist)

	if err := os.WriteFile(filepath.Join(res, stampName), []byte(stamp), 0o600); err != nil {
		return false, err
	}
	// Editing the bundle invalidated osacompile's signature; re-sign ad hoc.
	if err := run("/usr/bin/codesign", "--force", "--deep", "-s", "-", out); err != nil {
		return false, err
	}

	if err := os.RemoveAll(app); err != nil {
		return false, err
	}
	if err := os.Rename(out, app); err != nil {
		return false, err
	}
	if register {
		_ = run(lsregister, "-f", app)
	}
	return created, nil
}

func run(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // fixed system tools; args are ours
	if err != nil {
		return fmt.Errorf("%s: %w: %s", filepath.Base(name), err, out)
	}
	return nil
}

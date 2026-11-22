//go:build darwin

package notification

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureBundle(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an applet with osacompile")
	}
	if _, err := exec.LookPath("osacompile"); err != nil {
		t.Skip("osacompile not available")
	}

	app := filepath.Join(t.TempDir(), "Flow.app")
	created, err := ensureBundle(app, false)
	if err != nil || !created {
		t.Fatalf("ensureBundle: created=%v err=%v", created, err)
	}

	icns, err := os.ReadFile(filepath.Join(app, "Contents", "Resources", "applet.icns"))
	if err != nil || !bytes.Equal(icns, iconICNS) {
		t.Fatalf("applet.icns should be the Flow icon (err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(app, "Contents", "Resources", "Assets.car")); !os.IsNotExist(err) {
		t.Fatalf("Assets.car should be removed, stat err=%v", err)
	}
	id, err := exec.Command("/usr/bin/plutil", "-extract", "CFBundleIdentifier", "raw", filepath.Join(app, "Contents", "Info.plist")).Output()
	if err != nil || strings.TrimSpace(string(id)) != bundleID {
		t.Fatalf("CFBundleIdentifier = %q (err=%v), want %q", id, err, bundleID)
	}
	if err := exec.Command("/usr/bin/codesign", "--verify", app).Run(); err != nil {
		t.Fatalf("bundle signature invalid: %v", err)
	}

	// An up-to-date bundle is left alone.
	marker := filepath.Join(app, "Contents", "Resources", "marker")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if created, err := ensureBundle(app, false); err != nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("up-to-date bundle was rebuilt: %v", err)
	}

	// A stale stamp triggers a rebuild.
	if err := os.WriteFile(filepath.Join(app, "Contents", "Resources", stampName), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if created, err := ensureBundle(app, false); err != nil || created {
		t.Fatalf("rebuild of an existing bundle: created=%v err=%v", created, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("stale bundle was not rebuilt")
	}
}

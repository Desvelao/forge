package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNameInjection builds the binary under two names and checks that everything
// the user sees follows the injected name.
func TestNameInjection(t *testing.T) {
	if testing.Short() {
		t.Skip("builds binaries")
	}
	tmp := t.TempDir()
	cfg := filepath.Join(tmp, "cfg")

	build := func(name string) (help, version, storeDir string) {
		exe := filepath.Join(tmp, name)
		cmd := exec.Command("go", "build", "-ldflags", "-X app/internal/buildinfo.Name="+name, "-o", exe, ".")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", name, err, out)
		}
		run := func(args ...string) string {
			c := exec.Command(exe, args...)
			c.Env = append(os.Environ(), "XDG_CONFIG_HOME="+cfg, "HOME="+tmp)
			out, err := c.Output()
			if err != nil {
				t.Fatalf("%s %v: %v", name, args, err)
			}
			return string(out)
		}
		return run("help"), run("version"), strings.TrimSpace(run("template", "dir", "--print"))
	}

	helpA, verA, dirA := build("probe-a")
	helpB, verB, dirB := build("probe-b")

	if helpA == helpB || verA == verB || dirA == dirB {
		t.Fatal("output does not depend on the injected name")
	}
	if !strings.Contains(helpA, "probe-a") || strings.Contains(helpA, "probe-b") {
		t.Errorf("help A:\n%s", helpA)
	}
	if !strings.HasPrefix(verA, "probe-a ") || !strings.HasPrefix(verB, "probe-b ") {
		t.Errorf("versions: %q %q", verA, verB)
	}
	if filepath.Base(filepath.Dir(dirA)) != "probe-a" || filepath.Base(filepath.Dir(dirB)) != "probe-b" {
		t.Errorf("config paths: %s %s", dirA, dirB)
	}
}

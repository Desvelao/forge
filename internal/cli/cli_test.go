package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"app/internal/buildinfo"
)

const fixture = "../../testdata/templates/basic"

type harness struct {
	t     *testing.T
	home  string
	shell []string
}

func newHarness(t *testing.T) *harness { return &harness{t: t, home: t.TempDir()} }

func (h *harness) run(stdin string, args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = RunEnv(args, Env{
		Stdin:  strings.NewReader(stdin),
		Stdout: &out,
		Stderr: &errb,
		Getenv: func(k string) string {
			if k == buildinfo.EnvPrefix()+"_HOME" {
				return h.home
			}
			return ""
		},
		Shell: func(dir, line string, _, _ io.Writer) error {
			h.shell = append(h.shell, dir+"|"+line)
			return nil
		},
	})
	return code, out.String(), errb.String()
}

func TestVersionAndHelp(t *testing.T) {
	h := newHarness(t)
	if code, out, _ := h.run("", "version"); code != 0 || !strings.Contains(out, buildinfo.AppName()) {
		t.Errorf("version: %d %q", code, out)
	}
	if code, out, _ := h.run("", "help"); code != 0 || !strings.Contains(out, "template add") {
		t.Errorf("help: %d %q", code, out)
	}
}

func TestExitCodes(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{
		{},
		{"bogus"},
		{"new"},
		{"new", "only-one"},
		{"new", "--nope", "a", "b"},
		{"new", "x", "y", "--hooks", "--no-hooks"},
		{"new", "name", "dest", "--ref", "v1"},
		{"template"},
		{"template", "bogus"},
		{"template", "add", "plain-name"},
		{"template", "add", "./x", "--ref", "v1"},
		{"template", "remove"},
	} {
		if code, _, errs := h.run("", args...); code != 2 || errs == "" {
			t.Errorf("%v: code %d, stderr %q", args, code, errs)
		}
	}
	if code, _, errs := h.run("", "new", "missing-template", t.TempDir()+"/o"); code != 1 || !strings.Contains(errs, "not found") {
		t.Errorf("runtime error: %d %q", code, errs)
	}
}

func TestNewFromLocalDir(t *testing.T) {
	h := newHarness(t)
	dest := filepath.Join(t.TempDir(), "proj")
	code, out, errs := h.run("", "new", fixture, dest, "--defaults", "--var", "license=none")
	if code != 0 {
		t.Fatalf("code %d: %s", code, errs)
	}
	if !strings.Contains(out, "Created") {
		t.Errorf("out = %q", out)
	}
	b, err := os.ReadFile(filepath.Join(dest, "README.md"))
	if err != nil || !strings.Contains(string(b), "License: none") {
		t.Errorf("%q %v", b, err)
	}
	if len(h.shell) != 0 {
		t.Errorf("hooks ran without consent: %v", h.shell)
	}
}

func TestNewInteractiveAndHooks(t *testing.T) {
	h := newHarness(t)
	dest := filepath.Join(t.TempDir(), "proj")
	// answers: project name, module (default), license (2nd choice), docker (default), run hooks
	code, _, errs := h.run("shiny\n\n2\n\ny\n", "new", fixture, dest)
	if code != 0 {
		t.Fatalf("code %d: %s", code, errs)
	}
	b, _ := os.ReadFile(filepath.Join(dest, "README.md"))
	if !strings.Contains(string(b), "# shiny") || !strings.Contains(string(b), "example.com/shiny") || !strings.Contains(string(b), "Apache-2.0") {
		t.Errorf("README = %q", b)
	}
	if len(h.shell) != 1 || !strings.HasSuffix(h.shell[0], "|echo hello shiny") {
		t.Errorf("shell = %v", h.shell)
	}

	h.shell = nil
	dest2 := filepath.Join(t.TempDir(), "p2")
	h.run("", "new", fixture, dest2, "--defaults", "--hooks")
	if len(h.shell) != 1 {
		t.Errorf("--hooks: shell = %v", h.shell)
	}
	h.shell = nil
	h.run("", "new", fixture, filepath.Join(t.TempDir(), "p3"), "--defaults", "--no-hooks")
	if len(h.shell) != 0 {
		t.Errorf("--no-hooks: shell = %v", h.shell)
	}
}

func TestNewDryRun(t *testing.T) {
	h := newHarness(t)
	dest := filepath.Join(t.TempDir(), "proj")
	code, out, _ := h.run("", "new", fixture, dest, "--defaults", "--dry-run")
	if code != 0 || !strings.Contains(out, "README.md") {
		t.Errorf("%d %q", code, out)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("dry run wrote files")
	}
}

func TestTemplateLifecycleLocal(t *testing.T) {
	h := newHarness(t)

	if code, out, _ := h.run("", "template", "list"); code != 0 || !strings.Contains(out, "No templates") {
		t.Errorf("empty list: %d %q", code, out)
	}
	if code, _, errs := h.run("", "template", "add", fixture); code != 0 {
		t.Fatalf("add: %d %s", code, errs)
	}
	if code, _, _ := h.run("", "template", "add", fixture); code != 1 {
		t.Errorf("duplicate add: code %d", code)
	}
	if code, _, _ := h.run("", "template", "add", fixture, "--name", "second"); code != 0 {
		t.Errorf("add --name failed")
	}
	_, out, _ := h.run("", "template", "list", "-l")
	for _, want := range []string{"basic", "second", "0.1.0", "Fixture template", "local"} {
		if !strings.Contains(out, want) {
			t.Errorf("list -l missing %q:\n%s", want, out)
		}
	}
	if code, out, _ := h.run("", "template", "update"); code != 0 || !strings.Contains(out, "skipped") {
		t.Errorf("update: %d %q", code, out)
	}

	dest := filepath.Join(t.TempDir(), "p")
	if code, _, errs := h.run("", "new", "basic", dest, "--defaults"); code != 0 {
		t.Fatalf("new from stored: %d %s", code, errs)
	}

	code, out, _ := h.run("", "template", "dir", "basic", "--print")
	if code != 0 || strings.TrimSpace(out) != filepath.Join(h.home, "templates", "basic") {
		t.Errorf("dir --print: %d %q", code, out)
	}

	if code, out, _ := h.run("n\n", "template", "remove", "second"); code != 0 || !strings.Contains(out, "Aborted") {
		t.Errorf("remove declined: %d %q", code, out)
	}
	if code, _, _ := h.run("", "template", "remove", "second", "--yes"); code != 0 {
		t.Error("remove --yes failed")
	}
	if code, _, _ := h.run("y\n", "template", "remove", "basic"); code != 0 {
		t.Error("remove with confirmation failed")
	}
	if _, out, _ := h.run("", "template", "list"); !strings.Contains(out, "No templates") {
		t.Errorf("after removal: %q", out)
	}
}

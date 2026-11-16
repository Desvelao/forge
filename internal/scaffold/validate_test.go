package scaffold

import (
	"path/filepath"
	"strings"
	"testing"
)

const idVar = "variables:\n  - name: id\n    pattern: '^[a-z]+(\\.[a-z]+)+$'\n    message: must be a dotted lowercase id\n    default: \"a.b\"\n"

func TestVarPatternRejected(t *testing.T) {
	dir := tmpl(t, idVar, map[string]string{"x.tmpl": "{{.id}}"})
	_, err := Run(dir, load(t, dir), filepath.Join(t.TempDir(), "o"),
		Options{Vars: map[string]string{"id": "BAD"}, Defaults: true}, noPrompt{t})
	if err == nil || !strings.Contains(err.Error(), `"BAD" must be a dotted lowercase id`) {
		t.Errorf("err = %v", err)
	}
}

func TestVarPatternAccepted(t *testing.T) {
	dir := tmpl(t, idVar, map[string]string{"x.tmpl": "{{.id}}"})
	dest := filepath.Join(t.TempDir(), "o")
	if _, err := Run(dir, load(t, dir), dest, Options{Vars: map[string]string{"id": "io.me"}, Defaults: true}, noPrompt{t}); err != nil {
		t.Fatal(err)
	}
	if got := tree(t, dest)["x"]; got != "io.me" {
		t.Errorf("got %q", got)
	}
}

// A templated default is only known once rendered, so it is checked at run time.
func TestRenderedDefaultChecked(t *testing.T) {
	dir := tmpl(t, "variables:\n  - name: id\n    pattern: '^[a-z]+$'\n    default: \"{{.dest_name}}\"\n", map[string]string{"a": "x"})
	_, err := Run(dir, load(t, dir), filepath.Join(t.TempDir(), "Bad_Dest"), Options{Defaults: true}, noPrompt{t})
	if err == nil || !strings.Contains(err.Error(), "must match") {
		t.Errorf("err = %v", err)
	}
}

func TestEmptyOptionalSkipsPattern(t *testing.T) {
	dir := tmpl(t, "variables:\n  - name: url\n    pattern: '^https://'\n", map[string]string{"a": "x"})
	if _, err := Run(dir, load(t, dir), filepath.Join(t.TempDir(), "o"), Options{Defaults: true}, noPrompt{t}); err != nil {
		t.Fatal(err)
	}
}

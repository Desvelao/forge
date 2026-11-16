package scaffold

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"app/internal/manifest"
)

var update = flag.Bool("update", false, "regenerate golden files")

var fixedNow = time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

const fixture = "../../testdata/templates/basic"

type noPrompt struct{ t *testing.T }

func (n noPrompt) Text(string, string, bool, func(string) error) (string, error) {
	n.t.Fatal("unexpected prompt")
	return "", nil
}
func (n noPrompt) Choice(string, []string, string) (string, error) {
	n.t.Fatal("unexpected prompt")
	return "", nil
}
func (n noPrompt) Bool(string, bool) (bool, error) { n.t.Fatal("unexpected prompt"); return false, nil }

type scripted struct {
	text, choice string
	yes          bool
}

func (s scripted) Text(label, def string, req bool, v func(string) error) (string, error) {
	return s.text, nil
}
func (s scripted) Choice(label string, c []string, def string) (string, error) { return s.choice, nil }
func (s scripted) Bool(label string, def bool) (bool, error)                   { return s.yes, nil }

func load(t *testing.T, dir string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// tmpl builds a template directory with the given manifest body and content files.
func tmpl(t *testing.T, vars string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"template.yml": "schema: 1\nname: t\n" + vars})
	if err := os.MkdirAll(filepath.Join(dir, "template"), 0o755); err != nil {
		t.Fatal(err)
	}
	for k, v := range files {
		writeTree(t, filepath.Join(dir, "template"), map[string]string{k: v})
	}
	return dir
}

func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		out[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGolden(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "demo")
	_, err := Run(fixture, load(t, fixture), dest, Options{
		Defaults: true, Now: fixedNow,
	}, noPrompt{t})
	if err != nil {
		t.Fatal(err)
	}
	golden := "../../testdata/golden/basic"
	if *update {
		os.RemoveAll(golden)
		if err := os.MkdirAll(golden, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, content := range tree(t, dest) {
			writeTree(t, golden, map[string]string{name: content})
		}
	}
	got, want := tree(t, dest), tree(t, golden)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rendered tree differs from golden (run `make golden` to update)\ngot:  %v\nwant: %v", keys(got), keys(want))
		for k, v := range got {
			if want[k] != v {
				t.Errorf("%s:\n got: %q\nwant: %q", k, v, want[k])
			}
		}
	}
	if got["assets/logo.bin"] != "\x00\x01\xff{{not rendered}}\n" {
		t.Errorf("binary file was altered: %q", got["assets/logo.bin"])
	}
	if _, ok := got["README.md"]; !ok || strings.Contains(got["README.md"], "never copied") {
		t.Error("template repository README leaked into the output")
	}
}

func keys(m map[string]string) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}

func TestSkipAndVarOverride(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out")
	res, err := Run(fixture, load(t, fixture), dest, Options{
		Vars: map[string]string{"docker": "false", "project_name": "svc", "license": "none"}, Defaults: true, Now: fixedNow,
	}, noPrompt{t})
	if err != nil {
		t.Fatal(err)
	}
	got := tree(t, dest)
	if _, ok := got["Dockerfile"]; ok {
		t.Error("Dockerfile should be skipped")
	}
	if !strings.Contains(got["README.md"], "License: none") || !strings.Contains(got["cmd/svc/main.go"], "Svc is") {
		t.Errorf("vars not applied: %v", got)
	}
	if len(res.Hooks) != 1 || res.Hooks[0] != "echo hello svc" {
		t.Errorf("hooks = %v", res.Hooks)
	}
}

func TestPrompts(t *testing.T) {
	dir := tmpl(t, "variables:\n  - name: who\n  - name: kind\n    choices: [a, b]\n  - name: flag\n    type: bool\n",
		map[string]string{"x.txt.tmpl": "{{.who}} {{.kind}} {{.flag}}"})
	dest := filepath.Join(t.TempDir(), "o")
	if _, err := Run(dir, load(t, dir), dest, Options{}, scripted{text: "bob", choice: "b", yes: true}); err != nil {
		t.Fatal(err)
	}
	if got := tree(t, dest)["x.txt"]; got != "bob b true" {
		t.Errorf("got %q", got)
	}
}

func TestRequiredWithoutDefault(t *testing.T) {
	dir := tmpl(t, "variables:\n  - name: m\n    required: true\n", map[string]string{"a": "x"})
	_, err := Run(dir, load(t, dir), filepath.Join(t.TempDir(), "o"), Options{Defaults: true}, noPrompt{t})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Errorf("err = %v", err)
	}
}

func TestUnknownVar(t *testing.T) {
	_, err := Run(fixture, load(t, fixture), filepath.Join(t.TempDir(), "o"),
		Options{Vars: map[string]string{"nope": "1"}, Defaults: true}, noPrompt{t})
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %v", err)
	}
}

func TestUndefinedVariableInTemplate(t *testing.T) {
	dir := tmpl(t, "", map[string]string{"a.tmpl": "{{.missing}}"})
	dest := filepath.Join(t.TempDir(), "o")
	if _, err := Run(dir, load(t, dir), dest, Options{Defaults: true}, noPrompt{t}); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("destination should not exist after a failed render")
	}
	if es, _ := os.ReadDir(filepath.Dir(dest)); len(es) != 0 {
		t.Errorf("leftover files: %v", es)
	}
}

func TestPathEscape(t *testing.T) {
	dir := tmpl(t, "variables:\n  - name: d\n    default: ..\n", map[string]string{"{{.d}}/evil.txt": "x"})
	dest := filepath.Join(t.TempDir(), "o")
	if _, err := Run(dir, load(t, dir), dest, Options{Defaults: true}, noPrompt{t}); err == nil {
		t.Fatal("want error for .. segment")
	}
	dir = tmpl(t, "variables:\n  - name: d\n    default: a/b\n", map[string]string{"{{.d}}.txt": "x"})
	if _, err := Run(dir, load(t, dir), dest, Options{Defaults: true}, noPrompt{t}); err == nil {
		t.Fatal("want error for separator in segment")
	}
}

func TestSymlinkRejected(t *testing.T) {
	dir := tmpl(t, "", map[string]string{"a": "x"})
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "template", "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	_, err := Run(dir, load(t, dir), filepath.Join(t.TempDir(), "o"), Options{Defaults: true}, noPrompt{t})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("err = %v", err)
	}
}

func TestDuplicateOutput(t *testing.T) {
	dir := tmpl(t, "", map[string]string{"a.txt": "1", "a.txt.tmpl": "2"})
	if _, err := Run(dir, load(t, dir), filepath.Join(t.TempDir(), "o"), Options{Defaults: true}, noPrompt{t}); err == nil {
		t.Fatal("want collision error")
	}
}

func TestNonEmptyDestination(t *testing.T) {
	dir := tmpl(t, "", map[string]string{"new.txt": "n", "both.txt": "template"})
	dest := t.TempDir()
	writeTree(t, dest, map[string]string{"keep.txt": "k", "both.txt": "old"})
	m := load(t, dir)
	if _, err := Run(dir, m, dest, Options{Defaults: true}, noPrompt{t}); err == nil {
		t.Fatal("want non-empty error")
	}
	if tree(t, dest)["both.txt"] != "old" {
		t.Fatal("existing file was touched without --force")
	}
	if _, err := Run(dir, m, dest, Options{Defaults: true, Force: true}, noPrompt{t}); err != nil {
		t.Fatal(err)
	}
	got := tree(t, dest)
	if got["keep.txt"] != "k" || got["both.txt"] != "template" || got["new.txt"] != "n" || len(got) != 3 {
		t.Errorf("got %v", got)
	}
}

func TestDryRun(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "o")
	res, err := Run(fixture, load(t, fixture), dest, Options{Defaults: true, DryRun: true, Now: fixedNow}, noPrompt{t})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) == 0 {
		t.Error("no files listed")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("dry run wrote the destination")
	}
}

func TestFileModePreserved(t *testing.T) {
	dir := tmpl(t, "", nil)
	p := filepath.Join(dir, "template", "run.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "o")
	if _, err := Run(dir, load(t, dir), dest, Options{Defaults: true}, noPrompt{t}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(filepath.Join(dest, "run.sh"))
	if err != nil || fi.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, err = %v", fi, err)
	}
}

func TestCaseHelpers(t *testing.T) {
	cases := []struct{ in, snake, kebab, camel, pascal string }{
		{"hello world", "hello_world", "hello-world", "helloWorld", "HelloWorld"},
		{"MyHTTPServer", "my_http_server", "my-http-server", "myHttpServer", "MyHttpServer"},
		{"go-service_2", "go_service_2", "go-service-2", "goService2", "GoService2"},
		{"", "", "", "", ""},
	}
	for _, c := range cases {
		if got := snake(c.in); got != c.snake {
			t.Errorf("snake(%q) = %q", c.in, got)
		}
		if got := kebab(c.in); got != c.kebab {
			t.Errorf("kebab(%q) = %q", c.in, got)
		}
		if got := camel(c.in); got != c.camel {
			t.Errorf("camel(%q) = %q", c.in, got)
		}
		if got := pascal(c.in); got != c.pascal {
			t.Errorf("pascal(%q) = %q", c.in, got)
		}
	}
}

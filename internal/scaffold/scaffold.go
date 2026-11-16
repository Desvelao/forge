// Package scaffold resolves variables and renders a template into a destination.
package scaffold

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"app/internal/manifest"
)

// Prompter asks the user for variable values.
type Prompter interface {
	Text(label, def string, required bool, validate func(string) error) (string, error)
	Choice(label string, choices []string, def string) (string, error)
	Bool(label string, def bool) (bool, error)
}

type Options struct {
	Vars     map[string]string // preset variables, never asked
	Defaults bool              // ask nothing
	DryRun   bool              // write nothing
	Force    bool              // allow a non-empty destination
	Now      time.Time         // zero means time.Now()
}

type Result struct {
	Dest  string         // absolute destination
	Files []string       // slash-separated output paths of files
	Hooks []string       // rendered post-create commands
	Vars  map[string]any // resolved variables
}

type op struct {
	src    string
	rel    string // source path relative to the content dir, for messages
	out    string // slash-separated output path
	dir    bool
	render bool
	mode   fs.FileMode
}

// Run renders the template in tplDir (whose manifest is m) into dest.
func Run(tplDir string, m *manifest.Manifest, dest string, o Options, p Prompter) (*Result, error) {
	abs, err := filepath.Abs(dest)
	if err != nil {
		return nil, err
	}
	exists := false
	switch fi, err := os.Stat(abs); {
	case err == nil:
		if !fi.IsDir() {
			return nil, fmt.Errorf("destination %s exists and is not a directory", abs)
		}
		exists = true
		if !o.Force {
			entries, err := os.ReadDir(abs)
			if err != nil {
				return nil, err
			}
			if len(entries) > 0 {
				return nil, fmt.Errorf("destination %s is not empty (use --force to allow)", abs)
			}
		}
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	data, err := resolve(m, filepath.Base(abs), o, p)
	if err != nil {
		return nil, err
	}
	skips, err := activeSkips(m, data)
	if err != nil {
		return nil, err
	}
	ops, err := plan(filepath.Join(tplDir, manifest.ContentDir), data, skips)
	if err != nil {
		return nil, err
	}
	res := &Result{Dest: abs, Vars: data}
	for _, op := range ops {
		if !op.dir {
			res.Files = append(res.Files, op.out)
		}
	}
	for _, h := range m.Hooks.Post {
		r, err := render("hook", h, data)
		if err != nil {
			return nil, fmt.Errorf("hook %q: %w", h, err)
		}
		res.Hooks = append(res.Hooks, r)
	}
	if o.DryRun {
		return res, nil
	}

	// Render into a temporary directory first so a failure leaves nothing half-written.
	var stage string
	if exists {
		stage, err = os.MkdirTemp(abs, ".stage-*")
	} else {
		parent := filepath.Dir(abs)
		if err = os.MkdirAll(parent, 0o755); err != nil {
			return nil, err
		}
		stage, err = os.MkdirTemp(parent, "."+filepath.Base(abs)+".tmp-*")
	}
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	if err := write(stage, ops, data); err != nil {
		return nil, err
	}
	if exists {
		err = merge(stage, abs)
	} else if err = os.Chmod(stage, 0o755); err == nil {
		err = os.Rename(stage, abs)
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

// resolve computes the value of every variable, in manifest order.
func resolve(m *manifest.Manifest, destName string, o Options, p Prompter) (map[string]any, error) {
	now := o.Now
	if now.IsZero() {
		now = time.Now()
	}
	data := map[string]any{
		"dest_name": destName,
		"year":      now.Format("2006"),
		"date":      now.Format("2006-01-02"),
	}
	known := map[string]bool{}
	for _, v := range m.Variables {
		known[v.Name] = true
	}
	var unknown []string
	for k := range o.Vars {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("unknown variable(s) in --var: %s", strings.Join(unknown, ", "))
	}

	for _, v := range m.Variables {
		if raw, ok := o.Vars[v.Name]; ok {
			val, err := coerce(v, raw)
			if err != nil {
				return nil, err
			}
			data[v.Name] = val
			continue
		}
		def, hasDef := "", false
		if v.Default != nil {
			r, err := render("default of "+v.Name, *v.Default, data)
			if err != nil {
				return nil, fmt.Errorf("variable %q: default: %w", v.Name, err)
			}
			def, hasDef = r, true
		}
		if o.Defaults {
			if !hasDef {
				if v.Required {
					return nil, fmt.Errorf("variable %q is required (pass --var %s=...)", v.Name, v.Name)
				}
				data[v.Name] = zero(v)
				continue
			}
			val, err := coerce(v, def)
			if err != nil {
				return nil, fmt.Errorf("%w (default)", err)
			}
			data[v.Name] = val
			continue
		}
		val, err := ask(v, def, hasDef, p)
		if err != nil {
			return nil, err
		}
		data[v.Name] = val
	}
	return data, nil
}

func zero(v manifest.Variable) any {
	if v.IsBool() {
		return false
	}
	return ""
}

func ask(v manifest.Variable, def string, hasDef bool, p Prompter) (any, error) {
	label := v.Prompt
	if label == "" {
		label = v.Name
	}
	switch {
	case v.IsBool():
		d := false
		if hasDef {
			b, err := parseBool(def)
			if err != nil {
				return nil, fmt.Errorf("variable %q: %w", v.Name, err)
			}
			d = b
		}
		return p.Bool(label, d)
	case len(v.Choices) > 0:
		return p.Choice(label, v.Choices, def)
	}
	return p.Text(label, def, v.Required, v.Check)
}

func coerce(v manifest.Variable, raw string) (any, error) {
	if v.IsBool() {
		b, err := parseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("variable %q: %w", v.Name, err)
		}
		return b, nil
	}
	if len(v.Choices) > 0 {
		for _, c := range v.Choices {
			if c == raw {
				return raw, nil
			}
		}
		return nil, fmt.Errorf("variable %q: %q is not one of %s", v.Name, raw, strings.Join(v.Choices, ", "))
	}
	if v.Required && raw == "" {
		return nil, fmt.Errorf("variable %q is required", v.Name)
	}
	if err := v.Check(raw); err != nil {
		return nil, fmt.Errorf("variable %q: %q %w", v.Name, raw, err)
	}
	return raw, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	}
	b, err := strconv.ParseBool(strings.TrimSpace(s))
	if err != nil {
		return false, fmt.Errorf("%q is not a boolean", s)
	}
	return b, nil
}

func render(name, text string, data map[string]any) (string, error) {
	t, err := template.New(name).Funcs(funcs).Option("missingkey=error").Parse(text)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

func activeSkips(m *manifest.Manifest, data map[string]any) ([]string, error) {
	var out []string
	for _, s := range m.Skip {
		if s.If == "" {
			out = append(out, s.Path)
			continue
		}
		r, err := render("skip "+s.Path, s.If, data)
		if err != nil {
			return nil, fmt.Errorf("skip %q: %w", s.Path, err)
		}
		switch strings.TrimSpace(r) {
		case "true":
			out = append(out, s.Path)
		case "false":
		default:
			return nil, fmt.Errorf("skip %q: condition rendered %q, want true or false", s.Path, strings.TrimSpace(r))
		}
	}
	return out, nil
}

func skipped(pats []string, out string) bool {
	for _, p := range pats {
		p = path.Clean(p)
		if ok, _ := path.Match(p, out); ok || strings.HasPrefix(out, p+"/") {
			return true
		}
	}
	return false
}

// plan walks the content directory and decides every output path before anything is written.
func plan(contentDir string, data map[string]any, skips []string) ([]op, error) {
	var ops []op
	seen := map[string]string{}
	err := filepath.WalkDir(contentDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relOS, err := filepath.Rel(contentDir, p)
		if err != nil {
			return err
		}
		if relOS == "." {
			return nil
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel := filepath.ToSlash(relOS)
		if d.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s: symlinks are not allowed in templates", rel)
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return fmt.Errorf("%s: unsupported file type", rel)
		}
		segs := strings.Split(rel, "/")
		for i, seg := range segs {
			if strings.Contains(seg, "{{") {
				r, err := render(rel, seg, data)
				if err != nil {
					return fmt.Errorf("%s: %w", rel, err)
				}
				seg = r
			}
			if seg == "" || seg == "." || seg == ".." || strings.ContainsAny(seg, "/\\\x00") {
				return fmt.Errorf("%s: rendered path segment %q is not allowed", rel, seg)
			}
			segs[i] = seg
		}
		isTmpl := !d.IsDir() && strings.HasSuffix(segs[len(segs)-1], ".tmpl")
		if isTmpl {
			segs[len(segs)-1] = strings.TrimSuffix(segs[len(segs)-1], ".tmpl")
			if segs[len(segs)-1] == "" {
				return fmt.Errorf("%s: empty file name after removing .tmpl", rel)
			}
		}
		out := path.Join(segs...)
		if !filepath.IsLocal(filepath.FromSlash(out)) {
			return fmt.Errorf("%s: rendered path %q escapes the destination", rel, out)
		}
		if skipped(skips, out) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if prev, dup := seen[out]; dup {
			return fmt.Errorf("%s and %s both render to %s", prev, rel, out)
		}
		seen[out] = rel
		info, err := d.Info()
		if err != nil {
			return err
		}
		ops = append(ops, op{src: p, rel: rel, out: out, dir: d.IsDir(), render: isTmpl, mode: info.Mode()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ops, nil
}

func write(stage string, ops []op, data map[string]any) error {
	for _, o := range ops {
		target := filepath.Join(stage, filepath.FromSlash(o.out))
		if o.dir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if o.render {
			b, err := os.ReadFile(o.src)
			if err != nil {
				return err
			}
			s, err := render(o.rel, string(b), data)
			if err != nil {
				return fmt.Errorf("%s: %w", o.rel, err)
			}
			if err := os.WriteFile(target, []byte(s), o.mode.Perm()); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(o.src, target, o.mode.Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// merge moves everything rendered in stage into an existing dest, replacing files of the same path.
func merge(stage, dest string) error {
	return filepath.WalkDir(stage, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(stage, p)
		if err != nil || rel == "." {
			return err
		}
		t := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(t, 0o755)
		}
		return os.Rename(p, t)
	})
}

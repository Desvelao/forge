// Package store manages the local template store: one git clone (or copy) per template.
package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"app/internal/buildinfo"
	"app/internal/gitcmd"
	"app/internal/manifest"
)

type Store struct {
	Root string
	Git  *gitcmd.Git
}

type Kind int

const (
	KindName Kind = iota // name of a stored template
	KindURL              // git URL
	KindPath             // local directory
)

// Classify decides what a source argument refers to.
func Classify(s string) Kind {
	switch {
	case strings.Contains(s, "://"), strings.HasPrefix(s, "git@"), strings.HasSuffix(s, ".git"):
		return KindURL
	case strings.HasPrefix(s, "."), strings.HasPrefix(s, "/"), strings.HasPrefix(s, "~"), filepath.IsAbs(s):
		return KindPath
	}
	return KindName
}

// ExpandPath expands a leading ~ and makes p absolute.
func ExpandPath(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[1:])
	}
	return filepath.Abs(p)
}

// DefaultRoot is <config dir>/<app>/templates, or $<APP>_HOME/templates when that is set.
func DefaultRoot(getenv func(string) string) (string, error) {
	if h := getenv(buildinfo.EnvPrefix() + "_HOME"); h != "" {
		return filepath.Join(h, "templates"), nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate config directory: %w", err)
	}
	return filepath.Join(base, buildinfo.AppName(), "templates"), nil
}

// Path returns the directory of a stored template.
func (s *Store) Path(name string) (string, error) {
	if !manifest.ValidName(name) {
		return "", fmt.Errorf("invalid template name %q", name)
	}
	p := filepath.Join(s.Root, name)
	if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("template %q not found (see `template list`)", name)
	}
	return p, nil
}

// Fetched is a template cloned into a temporary directory.
type Fetched struct {
	Dir         string // clone root
	TemplateDir string // directory holding template.yml (Dir, or a subdirectory)
	Manifest    *manifest.Manifest
}

// Discard removes the temporary clone; it is safe to call after Install.
func (f *Fetched) Discard() { os.RemoveAll(f.Dir) }

// Fetch clones url (at ref) into a temporary directory and loads its manifest.
func (s *Store) Fetch(url, ref, subdir string) (*Fetched, error) {
	if subdir != "" && !filepath.IsLocal(filepath.FromSlash(subdir)) {
		return nil, fmt.Errorf("invalid --subdir %q", subdir)
	}
	tmp, err := os.MkdirTemp("", "tpl-*")
	if err != nil {
		return nil, err
	}
	if err := s.Git.Clone(url, tmp, ref); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	td := tmp
	if subdir != "" {
		td = filepath.Join(tmp, filepath.FromSlash(subdir))
	}
	m, err := manifest.Load(td)
	if err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	return &Fetched{Dir: tmp, TemplateDir: td, Manifest: m}, nil
}

type AddOptions struct {
	Name  string
	Ref   string
	Force bool
}

// Add stores a template from a git URL or a local directory and returns its store name.
func (s *Store) Add(source string, o AddOptions) (string, error) {
	switch Classify(source) {
	case KindURL:
		f, err := s.Fetch(source, o.Ref, "")
		if err != nil {
			return "", err
		}
		defer f.Discard()
		name := firstNonEmpty(o.Name, f.Manifest.Name)
		return name, s.Install(f.Dir, name, o.Force)
	case KindPath:
		if o.Ref != "" {
			return "", errors.New("--ref only applies to git URLs")
		}
		return s.addLocal(source, o)
	}
	return "", fmt.Errorf("%q is neither a git URL nor a local directory", source)
}

func (s *Store) addLocal(source string, o AddOptions) (string, error) {
	dir, err := ExpandPath(source)
	if err != nil {
		return "", err
	}
	m, err := manifest.Load(dir)
	if err != nil {
		return "", err
	}
	name := firstNonEmpty(o.Name, m.Name)
	if !manifest.ValidName(name) {
		return "", fmt.Errorf("invalid template name %q", name)
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(s.Root, ".add-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := copyTree(dir, stage, true); err != nil {
		return "", err
	}
	return name, s.Install(stage, name, o.Force)
}

// Install moves srcDir into the store as name. An existing template is replaced only with force.
func (s *Store) Install(srcDir, name string, force bool) error {
	if !manifest.ValidName(name) {
		return fmt.Errorf("invalid template name %q", name)
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	target := filepath.Join(s.Root, name)
	backup := ""
	if _, err := os.Lstat(target); err == nil {
		if !force {
			return fmt.Errorf("template %q already exists (use --force to replace it)", name)
		}
		backup = filepath.Join(s.Root, ".old-"+name)
		os.RemoveAll(backup)
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	}
	if err := moveDir(srcDir, target); err != nil {
		if backup != "" {
			os.RemoveAll(target)
			os.Rename(backup, target)
		}
		return err
	}
	os.Chmod(target, 0o755)
	if backup != "" {
		os.RemoveAll(backup)
	}
	return nil
}

type Entry struct {
	Name     string
	Manifest *manifest.Manifest
	Err      error
	Source   string // remote URL, or "local" for a copied directory (detail only)
	Rev      string // short revision (detail only)
}

// List returns the stored templates sorted by name.
func (s *Store) List(detail bool) ([]Entry, error) {
	des, err := os.ReadDir(s.Root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, de := range des {
		if !de.IsDir() || strings.HasPrefix(de.Name(), ".") {
			continue
		}
		dir := filepath.Join(s.Root, de.Name())
		e := Entry{Name: de.Name()}
		e.Manifest, e.Err = manifest.Load(dir)
		if detail {
			e.Source, e.Rev = "local", "-"
			if isGit(dir) {
				e.Source, e.Rev = "?", "?"
				if u, err := s.Git.RemoteURL(dir); err == nil {
					e.Source = u
				}
				if r, err := s.Git.ShortRev(dir); err == nil {
					e.Rev = r
				}
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type UpdateResult struct {
	Name   string
	Status string // updated, unchanged or skipped
	Note   string
	Err    error
}

// Update pulls the named templates, or all of them when names is empty.
func (s *Store) Update(names []string) ([]UpdateResult, error) {
	if len(names) == 0 {
		es, err := s.List(false)
		if err != nil {
			return nil, err
		}
		for _, e := range es {
			names = append(names, e.Name)
		}
	}
	var out []UpdateResult
	for _, n := range names {
		dir, err := s.Path(n)
		if err != nil {
			return nil, err
		}
		out = append(out, s.update(n, dir))
	}
	return out, nil
}

func (s *Store) update(name, dir string) UpdateResult {
	r := UpdateResult{Name: name}
	switch {
	case !isGit(dir):
		r.Status, r.Note = "skipped", "local copy, not a git clone (re-add with --force to refresh)"
	case s.Git.Detached(dir):
		r.Status, r.Note = "skipped", "pinned to a tag or commit"
	default:
		before, _ := s.Git.ShortRev(dir)
		if err := s.Git.Pull(dir); err != nil {
			r.Err = err
			return r
		}
		after, _ := s.Git.ShortRev(dir)
		if before == after {
			r.Status = "unchanged"
		} else {
			r.Status, r.Note = "updated", before+" -> "+after
		}
	}
	return r
}

// Remove deletes a stored template.
func (s *Store) Remove(name string) error {
	dir, err := s.Path(name)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(s.Root, dir)
	if err != nil || !filepath.IsLocal(rel) {
		return fmt.Errorf("refusing to delete %s: outside the store", dir)
	}
	return os.RemoveAll(dir)
}

func isGit(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

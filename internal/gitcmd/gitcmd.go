// Package gitcmd is a thin wrapper over the system git binary.
package gitcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// ErrNotInstalled is returned when git cannot be found.
var ErrNotInstalled = errors.New("git is required for this operation but was not found in PATH")

var commitRE = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// Runner runs git with args in dir ("" for the current directory) and returns trimmed stdout.
type Runner interface {
	Run(dir string, args ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrNotInstalled
		}
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(out.String()), nil
}

type Git struct{ R Runner }

func New(r Runner) *Git { return &Git{R: r} }

// Clone does a shallow clone of url into the existing empty directory dir.
// ref may be empty (default branch), a branch or tag, or a commit hash.
func (g *Git) Clone(url, dir, ref string) error {
	if strings.HasPrefix(url, "-") {
		return fmt.Errorf("invalid git URL %q", url)
	}
	var err error
	switch {
	case ref == "":
		_, err = g.R.Run("", "clone", "--depth", "1", "--", url, dir)
	case commitRE.MatchString(ref):
		for _, args := range [][]string{
			{"init", "-q"},
			{"remote", "add", "origin", url},
			{"fetch", "-q", "--depth", "1", "origin", ref},
			{"checkout", "-q", "FETCH_HEAD"},
		} {
			if _, err = g.R.Run(dir, args...); err != nil {
				break
			}
		}
	default:
		_, err = g.R.Run("", "clone", "--depth", "1", "--branch", ref, "--", url, dir)
	}
	return err
}

func (g *Git) Pull(dir string) error {
	_, err := g.R.Run(dir, "pull", "--ff-only")
	return err
}

func (g *Git) RemoteURL(dir string) (string, error) {
	return g.R.Run(dir, "remote", "get-url", "origin")
}

func (g *Git) ShortRev(dir string) (string, error) {
	return g.R.Run(dir, "rev-parse", "--short", "HEAD")
}

// Detached reports whether HEAD is detached (a tag or commit checkout).
func (g *Git) Detached(dir string) bool {
	_, err := g.R.Run(dir, "symbolic-ref", "-q", "HEAD")
	return err != nil
}

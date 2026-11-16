// Package cli dispatches subcommands, parses flags and maps errors to exit codes.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"app/internal/buildinfo"
	"app/internal/gitcmd"
	"app/internal/opener"
	"app/internal/prompt"
	"app/internal/store"
)

// ShellFunc runs one shell command line in dir.
type ShellFunc func(dir, line string, out, errw io.Writer) error

// Env carries everything the commands touch outside the process, so tests can fake it.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Getenv         func(string) string
	Shell          ShellFunc
	Opener         *opener.Opener
	Git            *gitcmd.Git
	Now            time.Time // zero means time.Now()
}

type app struct {
	env  Env
	p    *prompt.Prompter
	name string
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return usageError{fmt.Sprintf(format, a...)} }

var (
	errHelp  = errors.New("help requested")
	errShown = errors.New("usage already shown")
)

// Run executes the command line and returns the process exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return RunEnv(args, Env{Stdin: stdin, Stdout: stdout, Stderr: stderr})
}

// RunEnv is Run with every external dependency injectable.
func RunEnv(args []string, env Env) int {
	if env.Stdin == nil {
		env.Stdin = strings.NewReader("")
	}
	if env.Getenv == nil {
		env.Getenv = os.Getenv
	}
	if env.Shell == nil {
		env.Shell = defaultShell
	}
	if env.Opener == nil {
		env.Opener = opener.Default()
	}
	if env.Git == nil {
		env.Git = gitcmd.New(gitcmd.ExecRunner{})
	}
	a := &app{env: env, p: prompt.New(env.Stdin, env.Stderr), name: buildinfo.AppName()}

	err := a.dispatch(args)
	var ue usageError
	switch {
	case err == nil, errors.Is(err, errHelp):
		return 0
	case errors.Is(err, errShown):
		return 2
	case errors.As(err, &ue):
		fmt.Fprintf(env.Stderr, "%s: %v\nRun '%s help' for usage.\n", a.name, ue, a.name)
		return 2
	}
	fmt.Fprintf(env.Stderr, "%s: error: %v\n", a.name, err)
	return 1
}

func defaultShell(dir, line string, out, errw io.Writer) error {
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("cmd", "/C", line)
	} else {
		c = exec.Command("sh", "-c", line)
	}
	c.Dir, c.Stdout, c.Stderr, c.Stdin = dir, out, errw, os.Stdin
	return c.Run()
}

func (a *app) dispatch(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(a.env.Stderr, a.usage())
		return errShown
	}
	switch args[0] {
	case "new":
		return a.cmdNew(args[1:])
	case "template":
		return a.cmdTemplate(args[1:])
	case "version", "--version":
		return a.cmdVersion()
	case "help", "-h", "--help":
		fmt.Fprint(a.env.Stdout, a.usage())
		return nil
	}
	return usagef("unknown command %q", args[0])
}

func (a *app) usage() string {
	return fmt.Sprintf(`Usage: %[1]s <command> [options]

Commands:
  new <source> <dest>       Create a project from a stored template name,
                            a git URL, or a local template directory
  template list             List stored templates (-l for source and revision)
  template add <source>     Store a template from a git URL or a local directory
  template update [name]    Pull the latest revision of one template, or all
  template remove <name>    Delete a stored template
  template manifest <name>  Open the template's manifest in the editor
  template dir [name]       Open the template's directory (--print to print it)
  version                   Print name, version and commit
  help                      Show this help

Run '%[1]s <command> -h' for the options of a command.
`, a.name)
}

func (a *app) cmdVersion() error {
	s := a.name + " " + buildinfo.Version
	if buildinfo.Commit != "" {
		s += " " + buildinfo.Commit
	}
	fmt.Fprintln(a.env.Stdout, s)
	return nil
}

// flags creates a flag set that reports problems on stderr.
func (a *app) flags(cmd, synopsis string) *flag.FlagSet {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(a.env.Stderr)
	fs.Usage = func() {
		fmt.Fprintf(a.env.Stderr, "Usage: %s %s %s\n\nOptions:\n", a.name, cmd, synopsis)
		fs.PrintDefaults()
	}
	return fs
}

// parse parses flags that may appear before, between or after positional arguments.
func (a *app) parse(fs *flag.FlagSet, args []string, min, max int, want string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, errHelp
			}
			return nil, errShown
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
	if len(pos) < min || len(pos) > max {
		return nil, usagef("%s: expected %s", fs.Name(), want)
	}
	return pos, nil
}

func (a *app) confirm(label string) bool {
	ok, err := a.p.Bool(label, false)
	return err == nil && ok
}

func (a *app) store() (*store.Store, error) {
	root, err := store.DefaultRoot(a.env.Getenv)
	if err != nil {
		return nil, err
	}
	return &store.Store{Root: root, Git: a.env.Git}, nil
}

// varFlag collects repeated --var key=value flags.
type varFlag map[string]string

func (v varFlag) String() string { return "" }

func (v varFlag) Set(s string) error {
	k, val, ok := strings.Cut(s, "=")
	if !ok || k == "" {
		return fmt.Errorf("want key=value, got %q", s)
	}
	v[k] = val
	return nil
}

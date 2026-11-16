package cli

import (
	"fmt"

	"app/internal/manifest"
	"app/internal/scaffold"
	"app/internal/store"
)

func (a *app) cmdNew(args []string) error {
	fs := a.flags("new", "[options] <source> <dest>")
	vars := varFlag{}
	fs.Var(vars, "var", "preset a variable as `key=value`; repeatable")
	defaults := fs.Bool("defaults", false, "ask nothing; undefined variables take their defaults")
	ref := fs.String("ref", "", "branch, tag or commit when the source is a git URL")
	subdir := fs.String("subdir", "", "directory inside the repository that holds the template")
	save := fs.Bool("save", false, "keep a URL template in the store after use")
	dry := fs.Bool("dry-run", false, "list the files that would be written, write nothing")
	force := fs.Bool("force", false, "allow a non-empty destination")
	hooks := fs.Bool("hooks", false, "run post-create hooks without asking")
	noHooks := fs.Bool("no-hooks", false, "skip post-create hooks")
	pos, err := a.parse(fs, args, 2, 2, "<source> <dest>")
	if err != nil {
		return err
	}
	source, dest := pos[0], pos[1]

	if *hooks && *noHooks {
		return usagef("--hooks and --no-hooks cannot be combined")
	}
	kind := store.Classify(source)
	if kind != store.KindURL && (*ref != "" || *subdir != "" || *save) {
		return usagef("--ref, --subdir and --save only apply to a git URL source")
	}
	if *save && *subdir != "" {
		return usagef("--save cannot be combined with --subdir")
	}

	var (
		tplDir  string
		m       *manifest.Manifest
		fetched *store.Fetched
		st      *store.Store
	)
	switch kind {
	case store.KindURL:
		st = &store.Store{Git: a.env.Git}
		if fetched, err = st.Fetch(source, *ref, *subdir); err != nil {
			return err
		}
		defer fetched.Discard()
		tplDir, m = fetched.TemplateDir, fetched.Manifest
	case store.KindPath:
		if tplDir, err = store.ExpandPath(source); err != nil {
			return err
		}
	default:
		if st, err = a.store(); err != nil {
			return err
		}
		if tplDir, err = st.Path(source); err != nil {
			return err
		}
	}
	if m == nil {
		if m, err = manifest.Load(tplDir); err != nil {
			return err
		}
	}

	res, err := scaffold.Run(tplDir, m, dest, scaffold.Options{
		Vars:     vars,
		Defaults: *defaults,
		DryRun:   *dry,
		Force:    *force,
		Now:      a.env.Now,
	}, a.p)
	if err != nil {
		return err
	}

	if *dry {
		fmt.Fprintf(a.env.Stdout, "%d file(s) would be written to %s:\n", len(res.Files), res.Dest)
		for _, f := range res.Files {
			fmt.Fprintln(a.env.Stdout, "  "+f)
		}
		return nil
	}
	fmt.Fprintf(a.env.Stdout, "Created %s (%d files)\n", res.Dest, len(res.Files))

	if *save && fetched != nil {
		sst, err := a.store()
		if err == nil {
			err = sst.Install(fetched.Dir, m.Name, false)
		}
		if err != nil {
			fmt.Fprintf(a.env.Stderr, "%s: warning: template not saved: %v\n", a.name, err)
		} else {
			fmt.Fprintf(a.env.Stdout, "Saved template as %s\n", m.Name)
		}
	}
	return a.runHooks(res.Dest, res.Hooks, *hooks, *noHooks, *defaults)
}

// runHooks runs post-create commands, asking first unless --hooks was given.
func (a *app) runHooks(dir string, cmds []string, run, skip, defaults bool) error {
	if len(cmds) == 0 || skip {
		return nil
	}
	if !run {
		if defaults {
			fmt.Fprintf(a.env.Stderr, "skipping %d post-create command(s); pass --hooks to run them\n", len(cmds))
			return nil
		}
		fmt.Fprintf(a.env.Stderr, "Post-create commands (run in %s):\n", dir)
		for _, c := range cmds {
			fmt.Fprintf(a.env.Stderr, "  $ %s\n", c)
		}
		if !a.confirm("Run them?") {
			return nil
		}
	}
	for _, c := range cmds {
		fmt.Fprintf(a.env.Stderr, "$ %s\n", c)
		if err := a.env.Shell(dir, c, a.env.Stdout, a.env.Stderr); err != nil {
			return fmt.Errorf("hook %q failed: %w", c, err)
		}
	}
	return nil
}

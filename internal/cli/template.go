package cli

import (
	"fmt"
	"path/filepath"
	"text/tabwriter"

	"app/internal/manifest"
	"app/internal/store"
)

func (a *app) cmdTemplate(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(a.env.Stderr, a.usage())
		return errShown
	}
	rest := args[1:]
	switch args[0] {
	case "list", "ls":
		return a.tplList(rest)
	case "add":
		return a.tplAdd(rest)
	case "update":
		return a.tplUpdate(rest)
	case "remove", "rm":
		return a.tplRemove(rest)
	case "manifest":
		return a.tplManifest(rest)
	case "dir":
		return a.tplDir(rest)
	case "help", "-h", "--help":
		fmt.Fprint(a.env.Stdout, a.usage())
		return nil
	}
	return usagef("unknown template command %q", args[0])
}

func (a *app) tplList(args []string) error {
	fs := a.flags("template list", "[-l]")
	long := fs.Bool("l", false, "also show source URL and revision")
	if _, err := a.parse(fs, args, 0, 0, "no arguments"); err != nil {
		return err
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	entries, err := st.List(*long)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(a.env.Stdout, "No templates stored.")
		return nil
	}
	tw := tabwriter.NewWriter(a.env.Stdout, 0, 4, 2, ' ', 0)
	head := "NAME\tVERSION\tDESCRIPTION"
	if *long {
		head += "\tSOURCE\tREV"
	}
	fmt.Fprintln(tw, head)
	for _, e := range entries {
		ver, desc := "", ""
		if e.Err != nil {
			ver, desc = "-", "invalid: "+e.Err.Error()
		} else {
			ver, desc = e.Manifest.Version, e.Manifest.Description
		}
		line := fmt.Sprintf("%s\t%s\t%s", e.Name, ver, desc)
		if *long {
			line += fmt.Sprintf("\t%s\t%s", e.Source, e.Rev)
		}
		fmt.Fprintln(tw, line)
	}
	return tw.Flush()
}

func (a *app) tplAdd(args []string) error {
	fs := a.flags("template add", "[options] <git-url|directory>")
	name := fs.String("name", "", "store the template under this name")
	ref := fs.String("ref", "", "branch, tag or commit (git URLs only)")
	force := fs.Bool("force", false, "replace an existing template of the same name")
	pos, err := a.parse(fs, args, 1, 1, "<git-url|directory>")
	if err != nil {
		return err
	}
	source := pos[0]
	switch store.Classify(source) {
	case store.KindName:
		return usagef("template add: %q is neither a git URL nor a local directory (local paths start with . / or ~)", source)
	case store.KindPath:
		if *ref != "" {
			return usagef("template add: --ref only applies to git URLs")
		}
	}
	if *name != "" && !manifest.ValidName(*name) {
		return usagef("template add: invalid --name %q", *name)
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	got, err := st.Add(source, store.AddOptions{Name: *name, Ref: *ref, Force: *force})
	if err != nil {
		return err
	}
	fmt.Fprintf(a.env.Stdout, "Added template %s\n", got)
	return nil
}

func (a *app) tplUpdate(args []string) error {
	fs := a.flags("template update", "[name...]")
	names, err := a.parse(fs, args, 0, 1<<10, "[name...]")
	if err != nil {
		return err
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	results, err := st.Update(names)
	if err != nil {
		return err
	}
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
			fmt.Fprintf(a.env.Stderr, "%s: %s: error: %v\n", a.name, r.Name, r.Err)
			continue
		}
		line := fmt.Sprintf("%s: %s", r.Name, r.Status)
		if r.Note != "" {
			line += " (" + r.Note + ")"
		}
		fmt.Fprintln(a.env.Stdout, line)
	}
	if failed > 0 {
		return fmt.Errorf("%d template(s) failed to update", failed)
	}
	return nil
}

func (a *app) tplRemove(args []string) error {
	fs := a.flags("template remove", "[--yes] <name>")
	yes := fs.Bool("yes", false, "do not ask for confirmation")
	pos, err := a.parse(fs, args, 1, 1, "<name>")
	if err != nil {
		return err
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	if _, err := st.Path(pos[0]); err != nil {
		return err
	}
	if !*yes && !a.confirm(fmt.Sprintf("Remove template %q?", pos[0])) {
		fmt.Fprintln(a.env.Stdout, "Aborted.")
		return nil
	}
	if err := st.Remove(pos[0]); err != nil {
		return err
	}
	fmt.Fprintf(a.env.Stdout, "Removed template %s\n", pos[0])
	return nil
}

func (a *app) tplManifest(args []string) error {
	fs := a.flags("template manifest", "<name>")
	pos, err := a.parse(fs, args, 1, 1, "<name>")
	if err != nil {
		return err
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	dir, err := st.Path(pos[0])
	if err != nil {
		return err
	}
	return a.env.Opener.Edit(filepath.Join(dir, manifest.FileName))
}

func (a *app) tplDir(args []string) error {
	fs := a.flags("template dir", "[--print] [name]")
	print := fs.Bool("print", false, "print the path instead of opening it")
	pos, err := a.parse(fs, args, 0, 1, "[name]")
	if err != nil {
		return err
	}
	st, err := a.store()
	if err != nil {
		return err
	}
	dir := st.Root // without a name: the store itself
	if len(pos) == 1 {
		if dir, err = st.Path(pos[0]); err != nil {
			return err
		}
	}
	if *print {
		fmt.Fprintln(a.env.Stdout, dir)
		return nil
	}
	return a.env.Opener.Open(dir)
}

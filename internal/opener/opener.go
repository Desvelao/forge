// Package opener launches the user's editor and file manager.
package opener

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type Opener struct {
	GOOS   string
	Getenv func(string) string
	Run    func(name string, args []string) error
}

func Default() *Opener {
	return &Opener{GOOS: runtime.GOOS, Getenv: os.Getenv, Run: runAttached}
}

func runAttached(name string, args []string) error {
	c := exec.Command(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// Edit opens a file in $VISUAL, then $EDITOR, then the OS opener.
func (o *Opener) Edit(path string) error {
	for _, v := range []string{"VISUAL", "EDITOR"} {
		if f := strings.Fields(o.Getenv(v)); len(f) > 0 {
			return o.Run(f[0], append(f[1:], path))
		}
	}
	return o.Open(path)
}

// Open hands a file or directory to the OS opener.
func (o *Opener) Open(path string) error {
	switch o.GOOS {
	case "darwin":
		return o.Run("open", []string{path})
	case "windows":
		err := o.Run("explorer", []string{path})
		var ee *exec.ExitError
		if errors.As(err, &ee) { // explorer reports exit status 1 on success
			return nil
		}
		return err
	}
	return o.Run("xdg-open", []string{path})
}

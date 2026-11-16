// Package buildinfo holds values injected at link time.
package buildinfo

import (
	"os"
	"path/filepath"
	"strings"
)

var (
	Name    = "" // set with -ldflags -X
	Version = "dev"
	Commit  = ""
)

// AppName is the injected name, or the executable's base name when none was injected.
func AppName() string {
	if Name != "" {
		return Name
	}
	if exe, err := os.Executable(); err == nil {
		return strings.TrimSuffix(filepath.Base(exe), ".exe")
	}
	return "tool"
}

// EnvPrefix is the upper-cased app name usable as an environment-variable prefix.
func EnvPrefix() string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		}
		return '_'
	}, AppName())
}

// Package manifest loads and validates template.yml.
package manifest

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

const (
	FileName      = "template.yml"
	ContentDir    = "template"
	SchemaVersion = 1
)

var (
	nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	varRE  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

	reserved = map[string]bool{"dest_name": true, "year": true, "date": true}
)

// ValidName reports whether s is a legal template name.
func ValidName(s string) bool { return nameRE.MatchString(s) }

type Variable struct {
	Name     string   `yaml:"name"`
	Prompt   string   `yaml:"prompt"`
	Default  *string  `yaml:"default"`
	Required bool     `yaml:"required"`
	Type     string   `yaml:"type"`
	Choices  []string `yaml:"choices"`
	Pattern  string   `yaml:"pattern"`    // regexp a string value must match
	Message  string   `yaml:"message"`    // shown instead of "must match <pattern>"
	MaxLen   int      `yaml:"max_length"` // longest accepted string, in characters; 0 means no limit
}

func (v Variable) IsBool() bool { return v.Type == "bool" }

// Check validates a string value against the variable's pattern and max_length.
// An empty value always passes: whether one is allowed is the job of "required".
// The returned error reads as a continuation of the value, e.g. `must match ...`.
func (v Variable) Check(s string) error {
	if s == "" {
		return nil
	}
	if v.MaxLen > 0 && utf8.RuneCountInString(s) > v.MaxLen {
		return fmt.Errorf("must be at most %d characters", v.MaxLen)
	}
	if v.Pattern != "" {
		re, err := regexp.Compile(v.Pattern)
		if err != nil {
			return fmt.Errorf("invalid pattern: %w", err)
		}
		if !re.MatchString(s) {
			if v.Message != "" {
				return errors.New(v.Message)
			}
			return fmt.Errorf("must match %s", v.Pattern)
		}
	}
	return nil
}

type Skip struct {
	Path string `yaml:"path"`
	If   string `yaml:"if"`
}

type Hooks struct {
	Post []string `yaml:"post"`
}

type Manifest struct {
	Schema      int        `yaml:"schema"`
	Name        string     `yaml:"name"`
	Description string     `yaml:"description"`
	Version     string     `yaml:"version"`
	Variables   []Variable `yaml:"variables"`
	Skip        []Skip     `yaml:"skip"`
	Hooks       Hooks      `yaml:"hooks"`
}

// Load reads and validates the manifest in dir and checks that dir/template exists.
func Load(dir string) (*Manifest, error) {
	f, err := os.Open(filepath.Join(dir, FileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s: no %s found (not a template)", dir, FileName)
		}
		return nil, err
	}
	defer f.Close()
	m, err := Parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Join(dir, FileName), err)
	}
	if fi, err := os.Stat(filepath.Join(dir, ContentDir)); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("%s: missing %s/ directory", dir, ContentDir)
	}
	return m, nil
}

// Parse decodes and validates a manifest. Unknown fields are an error.
func Parse(r io.Reader) (*Manifest, error) {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("manifest is empty")
		}
		return nil, err
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) Validate() error {
	if m.Schema != SchemaVersion {
		return fmt.Errorf("unsupported schema %d (want %d)", m.Schema, SchemaVersion)
	}
	if !ValidName(m.Name) {
		return fmt.Errorf("invalid name %q (must match %s)", m.Name, nameRE)
	}
	seen := map[string]bool{}
	for i, v := range m.Variables {
		if !varRE.MatchString(v.Name) {
			return fmt.Errorf("variables[%d]: invalid name %q", i, v.Name)
		}
		if reserved[v.Name] {
			return fmt.Errorf("variables[%d]: %q is a built-in name", i, v.Name)
		}
		if seen[v.Name] {
			return fmt.Errorf("variables[%d]: duplicate name %q", i, v.Name)
		}
		seen[v.Name] = true
		switch v.Type {
		case "", "string":
		case "bool":
			if len(v.Choices) > 0 {
				return fmt.Errorf("variable %q: choices cannot be combined with type bool", v.Name)
			}
		default:
			return fmt.Errorf("variable %q: unknown type %q", v.Name, v.Type)
		}
		if v.Default != nil && len(v.Choices) > 0 && !hasTemplate(*v.Default) && !contains(v.Choices, *v.Default) {
			return fmt.Errorf("variable %q: default %q is not one of the choices", v.Name, *v.Default)
		}
		if v.Pattern != "" || v.Message != "" || v.MaxLen != 0 {
			if v.IsBool() || len(v.Choices) > 0 {
				return fmt.Errorf("variable %q: pattern, message and max_length apply to free-text variables only", v.Name)
			}
			if v.Pattern == "" && v.Message != "" {
				return fmt.Errorf("variable %q: message needs a pattern", v.Name)
			}
			if v.MaxLen < 0 {
				return fmt.Errorf("variable %q: max_length cannot be negative", v.Name)
			}
			if v.Pattern != "" {
				if _, err := regexp.Compile(v.Pattern); err != nil {
					return fmt.Errorf("variable %q: invalid pattern: %w", v.Name, err)
				}
			}
			if v.Default != nil && !hasTemplate(*v.Default) {
				if err := v.Check(*v.Default); err != nil {
					return fmt.Errorf("variable %q: default %q %w", v.Name, *v.Default, err)
				}
			}
		}
	}
	for i, s := range m.Skip {
		if s.Path == "" {
			return fmt.Errorf("skip[%d]: path is required", i)
		}
	}
	return nil
}

func hasTemplate(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '{' && s[i+1] == '{' {
			return true
		}
	}
	return false
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

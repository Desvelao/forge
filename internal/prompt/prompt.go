// Package prompt provides small line-based prompts.
package prompt

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ErrNoInput is returned when the input ends before an answer is given.
var ErrNoInput = errors.New("no input available (use --var or --defaults)")

type Prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func New(in io.Reader, out io.Writer) *Prompter {
	return &Prompter{in: bufio.NewReader(in), out: out}
}

func (p *Prompter) line() (string, error) {
	s, err := p.in.ReadString('\n')
	if err != nil {
		if errors.Is(err, io.EOF) {
			if s == "" {
				return "", ErrNoInput
			}
		} else {
			return "", err
		}
	}
	return strings.TrimSpace(s), nil
}

// Text asks for free text. Enter accepts def; when required and def is empty it asks again.
// An answer (or accepted default) that validate rejects is reported and asked again; validate may be nil.
func (p *Prompter) Text(label, def string, required bool, validate func(string) error) (string, error) {
	for {
		if def != "" {
			fmt.Fprintf(p.out, "%s [%s]: ", label, def)
		} else {
			fmt.Fprintf(p.out, "%s: ", label)
		}
		s, err := p.line()
		if err != nil {
			return "", err
		}
		if s == "" {
			s = def
		}
		if s == "" {
			if !required {
				return "", nil
			}
			fmt.Fprintln(p.out, "a value is required")
			continue
		}
		if validate != nil {
			if err := validate(s); err != nil {
				fmt.Fprintf(p.out, "%q %v\n", s, err)
				continue
			}
		}
		return s, nil
	}
}

// Choice asks for one of choices, by name or 1-based number.
func (p *Prompter) Choice(label string, choices []string, def string) (string, error) {
	for {
		fmt.Fprintf(p.out, "%s (%s)", label, strings.Join(choices, "/"))
		if def != "" {
			fmt.Fprintf(p.out, " [%s]", def)
		}
		fmt.Fprint(p.out, ": ")
		s, err := p.line()
		if err != nil {
			return "", err
		}
		if s == "" && def != "" {
			return def, nil
		}
		for _, c := range choices {
			if s == c {
				return c, nil
			}
		}
		if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= len(choices) {
			return choices[n-1], nil
		}
		fmt.Fprintf(p.out, "choose one of: %s\n", strings.Join(choices, ", "))
	}
}

// Bool asks a yes/no question.
func (p *Prompter) Bool(label string, def bool) (bool, error) {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	for {
		fmt.Fprintf(p.out, "%s [%s]: ", label, hint)
		s, err := p.line()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(s) {
		case "":
			return def, nil
		case "y", "yes", "true":
			return true, nil
		case "n", "no", "false":
			return false, nil
		}
		fmt.Fprintln(p.out, "please answer y or n")
	}
}

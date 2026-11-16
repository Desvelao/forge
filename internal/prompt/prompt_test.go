package prompt

import (
	"errors"
	"strings"
	"testing"
)

func mustBeOK(s string) error {
	if s != "ok" {
		return errors.New("must be ok")
	}
	return nil
}

func TestTextReasksOnInvalid(t *testing.T) {
	var out strings.Builder
	p := New(strings.NewReader("BAD\n\nok\n"), &out)
	got, err := p.Text("Name", "", true, mustBeOK)
	if err != nil || got != "ok" {
		t.Fatalf("got %q, %v", got, err)
	}
	if !strings.Contains(out.String(), `"BAD" must be ok`) || !strings.Contains(out.String(), "a value is required") {
		t.Errorf("output = %q", out.String())
	}
}

func TestTextValidatesAcceptedDefault(t *testing.T) {
	var out strings.Builder
	p := New(strings.NewReader("\nok\n"), &out)
	got, err := p.Text("Name", "bad", false, mustBeOK)
	if err != nil || got != "ok" {
		t.Fatalf("got %q, %v", got, err)
	}
}

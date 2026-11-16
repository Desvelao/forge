package manifest

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name, in, wantErr string
	}{
		{"valid", "schema: 1\nname: a-b.c\nvariables:\n  - name: x\n    default: true\n    type: bool\n", ""},
		{"empty", "", "empty"},
		{"unknown field", "schema: 1\nname: a\nbogus: 1\n", "bogus"},
		{"bad schema", "schema: 2\nname: a\n", "schema"},
		{"missing schema", "name: a\n", "schema"},
		{"bad name", "schema: 1\nname: Bad_Name\n", "invalid name"},
		{"bad var name", "schema: 1\nname: a\nvariables:\n  - name: 1x\n", "invalid name"},
		{"reserved", "schema: 1\nname: a\nvariables:\n  - name: year\n", "built-in"},
		{"duplicate", "schema: 1\nname: a\nvariables:\n  - name: x\n  - name: x\n", "duplicate"},
		{"bad type", "schema: 1\nname: a\nvariables:\n  - name: x\n    type: int\n", "unknown type"},
		{"bool choices", "schema: 1\nname: a\nvariables:\n  - name: x\n    type: bool\n    choices: [a]\n", "choices"},
		{"default not in choices", "schema: 1\nname: a\nvariables:\n  - name: x\n    choices: [a, b]\n    default: c\n", "choices"},
		{"templated default ok", "schema: 1\nname: a\nvariables:\n  - name: x\n    choices: [a, b]\n    default: \"{{.dest_name}}\"\n", ""},
		{"valid pattern", "schema: 1\nname: a\nvariables:\n  - name: x\n    pattern: '^[a-z]+$'\n    message: m\n    max_length: 5\n    default: abc\n", ""},
		{"bad pattern", "schema: 1\nname: a\nvariables:\n  - name: x\n    pattern: '['\n", "invalid pattern"},
		{"pattern on bool", "schema: 1\nname: a\nvariables:\n  - name: x\n    type: bool\n    pattern: y\n", "free-text"},
		{"pattern on choices", "schema: 1\nname: a\nvariables:\n  - name: x\n    choices: [a]\n    max_length: 3\n", "free-text"},
		{"message without pattern", "schema: 1\nname: a\nvariables:\n  - name: x\n    message: m\n", "needs a pattern"},
		{"negative max_length", "schema: 1\nname: a\nvariables:\n  - name: x\n    max_length: -1\n", "negative"},
		{"default fails pattern", "schema: 1\nname: a\nvariables:\n  - name: x\n    pattern: '^[a-z]+$'\n    default: ABC\n", "must match"},
		{"default too long", "schema: 1\nname: a\nvariables:\n  - name: x\n    max_length: 2\n    default: abc\n", "at most"},
		{"templated default skips pattern", "schema: 1\nname: a\nvariables:\n  - name: x\n    pattern: '^[a-z]+$'\n    default: \"{{.dest_name}}\"\n", ""},
		{"skip without path", "schema: 1\nname: a\nskip:\n  - if: \"true\"\n", "path"},
		{"unquoted template", "schema: 1\nname: a\nvariables:\n  - name: x\n    default: {{.dest_name}}\n", "yaml"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := Parse(strings.NewReader(c.in))
			if c.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), c.wantErr) {
				t.Errorf("err = %v, want containing %q (m=%v)", err, c.wantErr, m)
			}
		})
	}
}

func TestBoolDefaultDecodesAsString(t *testing.T) {
	m, err := Parse(strings.NewReader("schema: 1\nname: a\nvariables:\n  - name: x\n    type: bool\n    default: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	if d := m.Variables[0].Default; d == nil || *d != "true" {
		t.Errorf("default = %v", d)
	}
}

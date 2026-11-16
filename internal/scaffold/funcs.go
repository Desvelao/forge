package scaffold

import (
	"strings"
	"text/template"
	"unicode"
)

var funcs = template.FuncMap{
	"lower":   strings.ToLower,
	"upper":   strings.ToUpper,
	"snake":   snake,
	"kebab":   kebab,
	"camel":   camel,
	"pascal":  pascal,
	"replace": func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
}

// words splits s on non-alphanumerics and on lower-to-upper case boundaries.
func words(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = nil
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		if unicode.IsUpper(r) && len(cur) > 0 {
			prev := rs[i-1]
			nextLower := i+1 < len(rs) && unicode.IsLower(rs[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

func lowerAll(ws []string) []string {
	out := make([]string, len(ws))
	for i, w := range ws {
		out[i] = strings.ToLower(w)
	}
	return out
}

func snake(s string) string { return strings.Join(lowerAll(words(s)), "_") }
func kebab(s string) string { return strings.Join(lowerAll(words(s)), "-") }

func title(w string) string {
	rs := []rune(strings.ToLower(w))
	rs[0] = unicode.ToUpper(rs[0])
	return string(rs)
}

func pascal(s string) string {
	var b strings.Builder
	for _, w := range words(s) {
		b.WriteString(title(w))
	}
	return b.String()
}

func camel(s string) string {
	ws := words(s)
	var b strings.Builder
	for i, w := range ws {
		if i == 0 {
			b.WriteString(strings.ToLower(w))
		} else {
			b.WriteString(title(w))
		}
	}
	return b.String()
}

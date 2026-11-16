package manifest

import (
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	v := Variable{Pattern: `^[a-z]+$`, Message: "lowercase letters only", MaxLen: 4}
	for in, want := range map[string]string{"": "", "abc": "", "abcde": "at most 4", "AB": "lowercase letters only"} {
		err := v.Check(in)
		if want == "" && err != nil || want != "" && (err == nil || !strings.Contains(err.Error(), want)) {
			t.Errorf("Check(%q) = %v, want %q", in, err, want)
		}
	}
	if err := (Variable{Pattern: `^x$`}).Check("y"); err == nil || !strings.Contains(err.Error(), "must match ^x$") {
		t.Errorf("default message: %v", err)
	}
}

package guide

import (
	"bytes"
	"strings"
	"testing"
)

func TestTexts(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want []string
	}{
		{"agent-guide", []string{"sim", "browser", "annotate", "instructions", "check", "capture setup", "--platform ios", "brew install cameroncooke/axe/axe", "connect --serial", "launch <bundle id>", "screenshot is 2x", "coordinates from ui"}},
		{"setup", []string{"annotate check", "sim check", "browser check", "ffmpeg", "adb", "agent-browser", "phone-harness"}},
	} {
		var out, errb bytes.Buffer
		if code := Run([]string{tc.cmd}, &out, &errb); code != 0 {
			t.Fatalf("%s: exit %d", tc.cmd, code)
		}
		for _, w := range tc.want {
			if !strings.Contains(out.String(), w) {
				t.Errorf("%s: missing %q", tc.cmd, w)
			}
		}
	}
}

func TestUsageError(t *testing.T) {
	var out, errb bytes.Buffer
	if code := Run([]string{"nope"}, &out, &errb); code != 2 {
		t.Fatalf("exit %d", code)
	}
}

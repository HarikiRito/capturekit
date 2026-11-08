package main

import (
	"bytes"
	"testing"
)

func TestDispatch(t *testing.T) {
	for args, want := range map[string]int{"help": 0, "setup": 0, "agent-guide": 0, "bogus": 2} {
		var out, errb bytes.Buffer
		if code := run([]string{args}, &out, &errb); code != want {
			t.Errorf("%s: exit %d, want %d", args, code, want)
		}
		if want == 0 && out.Len() == 0 {
			t.Errorf("%s: empty output", args)
		}
	}
}

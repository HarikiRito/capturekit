package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

type call struct {
	env  []string
	name string
	args []string
}

type fakeExec struct {
	missing bool
	codes   []int // exit code per Run call, 0 when exhausted
	calls   []call
	out     string
}

func (f *fakeExec) LookPath(name string) (string, error) {
	if f.missing {
		return "", errors.New("not found")
	}
	return "/bin/" + name, nil
}

func (f *fakeExec) Run(env []string, name string, args []string, stdout, _ io.Writer) (int, error) {
	f.calls = append(f.calls, call{env, name, args})
	_, _ = io.WriteString(stdout, f.out)
	if len(f.codes) >= len(f.calls) {
		return f.codes[len(f.calls)-1], nil
	}
	return 0, nil
}

func TestPassthroughInjectsDefaults(t *testing.T) {
	f := &fakeExec{out: `{"success":true}`}
	var out, errb bytes.Buffer
	code := run(f, []string{"open", "https://example.com"}, &out, &errb)
	want := []string{"--session", "capture", "--json", "open", "https://example.com"}
	if code != 0 || len(f.calls) != 1 || !reflect.DeepEqual(f.calls[0].args, want) {
		t.Fatalf("code=%d calls=%+v", code, f.calls)
	}
	if f.calls[0].env[0] != "AGENT_BROWSER_NO_VERSION_CHECK=1" || out.String() != f.out {
		t.Fatalf("env/stdout wrong: %v %q", f.calls[0].env, out.String())
	}
}

func TestPassthroughKeepsUserSessionAndExitCode(t *testing.T) {
	f := &fakeExec{codes: []int{3}}
	code := run(f, []string{"--session", "mine", "--json", "snapshot"}, io.Discard, io.Discard)
	want := []string{"--session", "mine", "--json", "snapshot"}
	if code != 3 || !reflect.DeepEqual(f.calls[0].args, want) {
		t.Fatalf("code=%d args=%v", code, f.calls[0].args)
	}
}

func TestRunFlags(t *testing.T) {
	f := &fakeExec{}
	code := run(f, []string{"run", "--session", "s", "--cdp", "9222", "--headed", "--viewport", "800x600", "--no-json", "--", "snapshot"}, io.Discard, io.Discard)
	if code != 0 || len(f.calls) != 2 {
		t.Fatalf("code=%d calls=%+v", code, f.calls)
	}
	if got := strings.Join(f.calls[0].args, " "); got != "--session s --cdp 9222 --headed set viewport 800 600" {
		t.Fatalf("viewport call: %s", got)
	}
	if got := strings.Join(f.calls[1].args, " "); got != "--session s --cdp 9222 --headed snapshot" {
		t.Fatalf("main call: %s", got)
	}
}

func TestRunViewportFailureStops(t *testing.T) {
	f := &fakeExec{codes: []int{1}}
	code := run(f, []string{"run", "--viewport", "800x600", "--", "snapshot"}, io.Discard, io.Discard)
	if code != 1 || len(f.calls) != 1 {
		t.Fatalf("code=%d calls=%d", code, len(f.calls))
	}
}

func TestMissingBinary(t *testing.T) {
	for _, args := range [][]string{{"open", "x"}, {"check"}, {"install", "--yes"}} {
		var errb bytes.Buffer
		code := run(&fakeExec{missing: true}, args, io.Discard, &errb)
		if code != 1 || !strings.Contains(errb.String(), "npm i -g agent-browser") {
			t.Fatalf("%v: code=%d stderr=%q", args, code, errb.String())
		}
	}
}

func TestInstallPrintsUnlessYes(t *testing.T) {
	f := &fakeExec{}
	var out bytes.Buffer
	if code := run(f, []string{"install"}, &out, io.Discard); code != 0 || len(f.calls) != 0 || !strings.Contains(out.String(), "agent-browser install") {
		t.Fatalf("code=%d calls=%d out=%q", code, len(f.calls), out.String())
	}
	run(f, []string{"install", "--yes"}, io.Discard, io.Discard)
	if len(f.calls) != 1 || !reflect.DeepEqual(f.calls[0].args, []string{"install"}) {
		t.Fatalf("calls=%+v", f.calls)
	}
}

type checkReport struct {
	OK     bool          `json:"ok"`
	Checks []checkResult `json:"checks"`
}

func TestCheckJSON(t *testing.T) {
	cases := []struct {
		name   string
		fake   *fakeExec
		wantOK bool
		code   int
		status []string
	}{
		{"all ok", &fakeExec{out: "x\n"}, true, 0, []string{"ok", "ok", "ok"}},
		{"chrome fails", &fakeExec{out: "no chrome", codes: []int{0, 1}}, false, 1, []string{"ok", "ok", "fail"}},
		{"binary missing", &fakeExec{missing: true}, false, 1, []string{"fail"}},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		code := run(c.fake, []string{"check", "--json"}, &out, &errb)
		var r checkReport
		if err := json.Unmarshal(out.Bytes(), &r); err != nil {
			t.Fatalf("%s: bad json %q: %v", c.name, out.String(), err)
		}
		var got []string
		for _, ch := range r.Checks {
			got = append(got, ch.Status)
		}
		if code != c.code || r.OK != c.wantOK || !reflect.DeepEqual(got, c.status) || errb.Len() != 0 {
			t.Errorf("%s: code=%d ok=%v statuses=%v stderr=%q", c.name, code, r.OK, got, errb.String())
		}
	}
}

func TestCheckJSONDetails(t *testing.T) {
	f := &fakeExec{out: "agent-browser 0.41.2\n"}
	var out bytes.Buffer
	run(f, []string{"check", "--json"}, &out, io.Discard)
	var r checkReport
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Checks[0].Name != "binary" || r.Checks[0].Detail != "/bin/agent-browser" || r.Checks[1].Name != "version" || r.Checks[1].Detail != "agent-browser 0.41.2" {
		t.Errorf("%+v", r.Checks)
	}
	if len(f.calls) != 2 || !reflect.DeepEqual(f.calls[1].args, []string{"doctor", "--offline", "--quick"}) {
		t.Errorf("calls %+v", f.calls)
	}
}

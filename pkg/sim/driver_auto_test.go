package sim

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

const phBin = "/bin/ph"

func writeArgPath(f *fakeExec, content string) {
	for _, e := range f.env {
		if v, ok := strings.CutPrefix(e, "CAPTURE_SIM_ARG="); ok {
			var path string
			_ = json.Unmarshal([]byte(v), &path)
			_ = os.WriteFile(path, []byte(content), 0o600)
		}
	}
}

// autoApp wires a fake where phone-harness steps listed in phFail error and everything else succeeds.
func autoApp(t *testing.T, phInstalled bool, phFail ...string) (*app, *fakeExec, *bytes.Buffer) {
	t.Helper()
	f := &fakeExec{}
	f.reply = func(c call) (string, error) {
		joined := strings.Join(c.args, " ")
		if c.name == phBin {
			for _, step := range phFail {
				if strings.Contains(f.stdin, "# step: "+step+"\n") {
					return "", os.ErrInvalid
				}
			}
			switch {
			case strings.Contains(f.stdin, "# step: screenshot\n"):
				writeArgPath(f, "\x89PNG-ph")
			case strings.Contains(f.stdin, "# step: screen-size\n"):
				return "@@[1,2]\n", nil
			case strings.Contains(f.stdin, "# step: ui\n"):
				return "@@[]\n", nil
			}
			return "@@true\n", nil
		}
		switch {
		case strings.Contains(joined, "uiautomator"):
			return dump, nil
		case strings.Contains(joined, "screencap"):
			return "\x89PNG-adb", nil
		case strings.Contains(joined, "wm size"):
			return "Physical size: 1080x2400", nil
		}
		return "", nil
	}
	a := newTestAndroid(t, f)
	if phInstalled {
		a.LookPath = func(string) (string, error) { return phBin, nil }
	}
	var errb bytes.Buffer
	return &app{out: &bytes.Buffer{}, err: &errb, plat: a, getenv: a.Getenv}, f, &errb
}

func driverFor(t *testing.T, a *app, args ...string) (Driver, error) {
	t.Helper()
	f := a.flags("x")
	f.driverFlag(a.getenv)
	if err := f.parse(args); err != nil {
		t.Fatal(err)
	}
	return a.driver(f)
}

func countCalls(f *fakeExec, name string) (n int) {
	for _, c := range f.calls {
		if c.name == name {
			n++
		}
	}
	return n
}

func TestDriverSelection(t *testing.T) {
	driverVal := func(env map[string]string, args ...string) string {
		f := (&app{err: &bytes.Buffer{}}).flags("x")
		f.driverFlag(func(k string) string { return env[k] })
		if err := f.parse(args); err != nil {
			t.Fatal(err)
		}
		return *f.driver
	}
	if got := driverVal(nil); got != "auto" {
		t.Fatalf("default %q", got)
	}
	if got := driverVal(map[string]string{"CAPTURE_SIM_DRIVER": "adb"}); got != "adb" {
		t.Fatalf("env %q", got)
	}
	if got := driverVal(map[string]string{"CAPTURE_SIM_DRIVER": "adb"}, "--driver", "phoneharness"); got != "phoneharness" {
		t.Fatalf("flag %q", got)
	}
	a, _, _ := autoApp(t, true)
	if _, err := driverFor(t, a, "--driver", "bogus"); err == nil || !strings.Contains(err.Error(), "auto|adb|phoneharness") {
		t.Fatalf("unknown driver err: %v", err)
	}
}

func TestAutoFallsBackToAdbWhenPhMissing(t *testing.T) {
	a, f, errb := autoApp(t, false)
	d, err := driverFor(t, a)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Tap(1, 2); err != nil {
		t.Fatal(err)
	}
	if strings.Count(errb.String(), "\n") != 1 || !strings.Contains(errb.String(), "using adb") {
		t.Fatalf("want one fallback line, got %q", errb.String())
	}
	if countCalls(f, "/sdk/adb") == 0 {
		t.Fatal("tap did not go through adb")
	}
}

func TestAutoRetriesReadsOnAdb(t *testing.T) {
	a, f, _ := autoApp(t, true, "ui", "screenshot", "screen-size")
	d, err := driverFor(t, a)
	if err != nil {
		t.Fatal(err)
	}
	if nodes, err := d.UI(); err != nil || len(nodes) == 0 {
		t.Fatalf("ui: %v %v", nodes, err)
	}
	if png, err := d.Screenshot(); err != nil || string(png) != "\x89PNG-adb" {
		t.Fatalf("screenshot: %q %v", png, err)
	}
	if w, h, err := d.ScreenSize(); err != nil || w != 1080 || h != 2400 {
		t.Fatalf("size: %d %d %v", w, h, err)
	}
	if n := countCalls(f, phBin); n != 3 {
		t.Fatalf("ph called %d times, want once per read", n)
	}
}

func TestAutoActionErrorHasHintAndNoAdbCall(t *testing.T) {
	a, f, _ := autoApp(t, true, "tap", "type", "swipe", "key", "back", "shell")
	d, err := driverFor(t, a)
	if err != nil {
		t.Fatal(err)
	}
	_, shellErr := d.Shell("ls")
	for name, err := range map[string]error{
		"tap": d.Tap(1, 2), "type": d.Type("x"), "swipe": d.Swipe(1, 2, 3, 4, 5),
		"key": d.Key("ENTER"), "back": d.Back(), "shell": shellErr,
	} {
		if err == nil || !strings.Contains(err.Error(), "--driver adb") {
			t.Fatalf("%s: want hinted error, got %v", name, err)
		}
	}
	if n := countCalls(f, "/sdk/adb"); n != 0 {
		t.Fatalf("adb called %d times after failed ph action", n)
	}
}

func TestPhScreenshotRejectsNonPNG(t *testing.T) {
	a, f, _ := autoApp(t, true)
	f.reply = func(call) (string, error) {
		writeArgPath(f, "garbage")
		return "@@true\n", nil
	}
	d, err := a.plat.Driver("phoneharness")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Screenshot(); err == nil || !strings.Contains(err.Error(), "PNG") {
		t.Fatalf("want PNG error, got %v", err)
	}
}

func TestPhTypeRejectsPercentAndNonASCII(t *testing.T) {
	a, f, _ := autoApp(t, true)
	d, err := a.plat.Driver("phoneharness")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"50%", "café"} {
		if err := d.Type(s); err == nil {
			t.Fatalf("%q accepted", s)
		}
	}
	if n := countCalls(f, phBin); n != 0 {
		t.Fatalf("ph called %d times for rejected text", n)
	}
}

package sim

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type call struct {
	name string
	args []string
}

type fakeExec struct {
	calls []call
	reply func(c call) (string, error)
}

func (f *fakeExec) Run(_ []string, _ []byte, name string, args ...string) ([]byte, error) {
	c := call{name, args}
	f.calls = append(f.calls, c)
	if f.reply == nil {
		return nil, nil
	}
	out, err := f.reply(c)
	return []byte(out), err
}

func (f *fakeExec) Start(_ string, _ []string, name string, args ...string) error {
	f.calls = append(f.calls, call{"START " + name, args})
	return nil
}

func newTestAndroid(t *testing.T, f *fakeExec) *Android {
	t.Helper()
	env := map[string]string{"ADB": "/sdk/adb", "HOME": t.TempDir()}
	now := time.Unix(0, 0)
	return &Android{
		X: f, StatePath: filepath.Join(t.TempDir(), "sim.json"),
		Getenv:   func(k string) string { return env[k] },
		LookPath: func(string) (string, error) { return "", errors.New("none") },
		Now:      func() time.Time { return now },
		Sleep:    func(d time.Duration) { now = now.Add(d) },
	}
}

func TestAdbArgs(t *testing.T) {
	got := adbArgs(State{Server: "127.0.0.1:5038", Serial: "emulator-5554"}, true, "shell", "ls")
	want := []string{"-H", "127.0.0.1", "-P", "5038", "-s", "emulator-5554", "shell", "ls"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := adbArgs(State{Serial: "x"}, false, "devices"); !reflect.DeepEqual(got, []string{"devices"}) {
		t.Fatalf("no-serial/no-server: %v", got)
	}
}

func TestStateRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "sim.json")
	if s, err := loadState(p); err != nil || s != (State{}) {
		t.Fatalf("missing file: %v %v", s, err)
	}
	want := State{Server: "h:1", Serial: "s"}
	if err := saveState(p, want); err != nil {
		t.Fatal(err)
	}
	if got, _ := loadState(p); got != want {
		t.Fatalf("got %v", got)
	}
}

const dump = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
<node text="" content-desc="" class="android.widget.FrameLayout" clickable="false" bounds="[0,0][1000,2000]">
 <node text="" content-desc="Sign in" class="android.view.View" clickable="false" bounds="[100,200][300,260]"/>
 <node text="Sign in to continue" content-desc="" class="android.widget.TextView" clickable="false" bounds="[0,0][1000,100]"/>
 <node text="Sign in" content-desc="" class="android.widget.Button" clickable="true" bounds="[400,500][600,600]"/>
</node></hierarchy>
UI hierarchy dumped to: /dev/tty`

func TestParseAndFindNode(t *testing.T) {
	nodes, err := ParseUIXML([]byte(dump))
	if err != nil || len(nodes) != 4 {
		t.Fatalf("nodes=%d err=%v", len(nodes), err)
	}
	n, ok := FindNode(nodes, "sign in", false, false)
	if x, y := n.Center(); !ok || x != 500 || y != 550 {
		t.Fatalf("exact text match should win over substring: %+v", n)
	}
	n, ok = FindNode(nodes, "Sign in", true, false)
	if x, y := n.Center(); !ok || x != 200 || y != 230 {
		t.Fatalf("desc center: %+v", n)
	}
	if _, ok := FindNode(nodes, "nope", false, false); ok {
		t.Fatal("unexpected match")
	}
}

func TestFindNodeSearchesBothFields(t *testing.T) {
	nodes, _ := ParseUIXML([]byte(`<hierarchy>
<node text="" content-desc="Email" class="v" bounds="[0,0][100,50]"/>
<node text="Email address" content-desc="" class="v" bounds="[0,100][100,150]"/></hierarchy>`))
	if n, _ := FindNode(nodes, "email", false, false); n.Top != 0 {
		t.Fatalf("--text: exact desc match should beat substring text: %+v", n)
	}
	if n, ok := FindNode(nodes, "email address", true, false); !ok || n.Top != 100 {
		t.Fatalf("--desc should also match text: %+v", n)
	}
	if _, ok := FindNode(nodes, "mail", false, true); ok {
		t.Fatal("exact must not match substrings")
	}
}

func TestFieldForResolvesLabelToInputBelow(t *testing.T) {
	nodes, _ := ParseUIXML([]byte(`<hierarchy>
<node text="" content-desc="Email *" class="android.view.View" bounds="[0,0][100,20]"/>
<node text="" content-desc="" class="android.widget.EditText" bounds="[0,30][100,80]"/>
<node text="" content-desc="Password *" class="android.view.View" bounds="[0,90][100,110]"/>
<node text="" content-desc="" class="android.widget.EditText" bounds="[0,120][100,170]"/></hierarchy>`))
	label, _ := FindNode(nodes, "password", true, false)
	if got := FieldFor(nodes, label); got.Top != 120 {
		t.Fatalf("got %+v", got)
	}
}

func TestParseUIStates(t *testing.T) {
	nodes, _ := ParseUIXML([]byte(`<hierarchy><node text="x" class="android.widget.EditText" enabled="true" focused="true" password="true" checked="true" selected="true" bounds="[0,0][10,10]"/>
<node text="y" enabled="false" bounds="[0,0][10,10]"/></hierarchy>`))
	n := nodes[0]
	if !n.Focused || !n.Password || !n.Checked || !n.Selected || !n.Enabled || nodes[1].Enabled {
		t.Fatalf("states: %+v %+v", n, nodes[1])
	}
	if stateFlags(n) != " focused selected checked password" || stateFlags(nodes[1]) != " disabled" {
		t.Fatalf("flags %q", stateFlags(n))
	}
}

func TestEscapeInputText(t *testing.T) {
	got, _ := escapeInputText("it's a $x;y")
	if got != `'it'\''s%sa%s$x;y'` {
		t.Fatalf("got %s", got)
	}
	if _, err := escapeInputText("café"); err == nil {
		t.Fatal("non-ascii should fail")
	}
	if _, err := escapeInputText("50%"); err == nil {
		t.Fatal("percent should fail")
	}
	if normalizeKey("enter") != "KEYCODE_ENTER" || normalizeKey("66") != "66" || normalizeKey("KEYCODE_HOME") != "KEYCODE_HOME" {
		t.Fatal("normalizeKey")
	}
}

func TestTapTextUsesNodeCenter(t *testing.T) {
	f := &fakeExec{reply: func(c call) (string, error) {
		if strings.Contains(strings.Join(c.args, " "), "uiautomator") {
			return dump, nil
		}
		return "", nil
	}}
	a := newTestAndroid(t, f)
	_ = saveState(a.StatePath, State{Server: "h:5038", Serial: "emu"})
	var out, errb bytes.Buffer
	code := (&app{out: &out, err: &errb, plat: a, getenv: a.Getenv}).run([]string{"tap", "--text", "Sign in"})
	if code != 0 {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	last := f.calls[len(f.calls)-2] // the call before the verify dump
	want := []string{"-H", "h", "-P", "5038", "-s", "emu", "shell", "input", "tap", "500", "550"}
	if last.name != "/sdk/adb" || !reflect.DeepEqual(last.args, want) {
		t.Fatalf("last call %v", last)
	}
}

func init() { settle, keyboardSettle = 0, 0 }

func uiNode(text, desc string, focused bool, top int) string {
	return fmt.Sprintf(`<node text=%q content-desc=%q class="android.widget.EditText" focused="%v" bounds="[0,%d][100,%d]"/>`, text, desc, focused, top, top+50)
}

func hierarchy(nodes ...string) string {
	return "<hierarchy>" + strings.Join(nodes, "") + "</hierarchy>"
}

// scriptedApp serves successive UI dumps (last one repeats) and records other adb calls.
func scriptedApp(t *testing.T, dumps ...string) (*app, *fakeExec, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	i := 0
	f := &fakeExec{reply: func(c call) (string, error) {
		joined := strings.Join(c.args, " ")
		if strings.Contains(joined, "uiautomator") {
			d := dumps[min(i, len(dumps)-1)]
			i++
			return d, nil
		}
		if strings.Contains(joined, "dumpsys input_method") {
			return "mInputShown=true", nil
		}
		return "", nil
	}}
	a := newTestAndroid(t, f)
	_ = saveState(a.StatePath, State{Serial: "emu"})
	var out, errb bytes.Buffer
	return &app{out: &out, err: &errb, plat: a, getenv: a.Getenv}, f, &out, &errb
}

func TestTapReportsChange(t *testing.T) {
	a, _, out, _ := scriptedApp(t, hierarchy(uiNode("", "Go", false, 0)), hierarchy(uiNode("", "Go", false, 0)))
	if a.run([]string{"tap", "--text", "Go"}) != 0 || !strings.Contains(out.String(), "screen changed: no") {
		t.Fatalf("no-op tap: %s", out.String())
	}
	a, _, out, _ = scriptedApp(t, hierarchy(uiNode("", "Go", false, 0)), hierarchy(uiNode("abc", "Go", true, 0), uiNode("", "More", false, 60)))
	if a.run([]string{"tap", "--desc", "Go", "--json"}) != 0 {
		t.Fatal("tap failed")
	}
	for _, want := range []string{`"changed": true`, `"focused": "abc"`, `"nodes_after": 2`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s in %s", want, out.String())
		}
	}
}

func TestTapNoVerifyDumpsOnce(t *testing.T) {
	a, f, out, _ := scriptedApp(t, hierarchy(uiNode("", "Go", false, 0)))
	if a.run([]string{"tap", "--text", "Go", "--no-verify"}) != 0 || strings.Contains(out.String(), "changed") {
		t.Fatalf("out: %s", out.String())
	}
	if len(f.calls) != 2 {
		t.Fatalf("want 1 dump + 1 tap, got %v", f.calls)
	}
}

func TestTypeIntoClearsAndTypes(t *testing.T) {
	before := hierarchy(uiNode("", "Email", false, 0))
	focused := hierarchy(uiNode("old", "Email", true, 0))
	empty := hierarchy(uiNode("", "Email", true, 0))
	a, f, _, errb := scriptedApp(t, before, focused, focused, empty, hierarchy(uiNode("demo", "Email", true, 0)))
	if code := a.run([]string{"type", "--into", "--desc", "Email", "--clear", "demo"}); code != 0 {
		t.Fatalf("code %d: %s", code, errb.String())
	}
	var shell []string
	for _, c := range f.calls {
		shell = append(shell, c.args[len(c.args)-1])
	}
	if !strings.Contains(strings.Join(shell, "|"), "KEYCODE_MOVE_END KEYCODE_DEL KEYCODE_DEL KEYCODE_DEL") {
		t.Fatalf("expected 3 DELs for 'old': %v", f.calls)
	}
}

// typingFake emulates a field whose IME swallows multi-char `input text` calls.
type typingFake struct {
	text        string
	drop        int // multi-char calls still to swallow; <0 = always
	stray       int // input text calls that still append a stray "y"
	imeShown    bool
	backCalls   int
	backHidesIM bool
}

func (tf *typingFake) reply(c call) (string, error) {
	joined := strings.Join(c.args, " ")
	switch {
	case strings.Contains(joined, "uiautomator"):
		return hierarchy(uiNode(tf.text, "Email", true, 0)), nil
	case strings.Contains(joined, "dumpsys input_method"):
		return fmt.Sprintf("mInputShown=%v", tf.imeShown), nil
	case strings.Contains(joined, "input text"):
		s := strings.ReplaceAll(strings.Trim(c.args[len(c.args)-1], "'"), "%s", " ")
		if len(s) > 1 && tf.drop != 0 {
			tf.drop--
			return "", nil
		}
		tf.text += s
		if tf.stray > 0 && len(s) > 1 {
			tf.stray--
			tf.text += "y"
		}
	case strings.Contains(joined, "KEYCODE_DEL"):
		tf.text = tf.text[:max(0, len(tf.text)-strings.Count(joined, "KEYCODE_DEL"))]
	case strings.Contains(joined, "KEYCODE_BACK"):
		tf.backCalls++
		if tf.backHidesIM {
			tf.imeShown = false
		}
	}
	return "", nil
}

func typingApp(t *testing.T, tf *typingFake) (*app, *bytes.Buffer) {
	t.Helper()
	a := newTestAndroid(t, &fakeExec{reply: tf.reply})
	_ = saveState(a.StatePath, State{Serial: "emu"})
	var errb bytes.Buffer
	return &app{out: &bytes.Buffer{}, err: &errb, plat: a, getenv: a.Getenv}, &errb
}

func TestTypeRetriesDroppedChars(t *testing.T) {
	tf := &typingFake{drop: 2, imeShown: true}
	a, errb := typingApp(t, tf)
	if code := a.run([]string{"type", "user@example.com"}); code != 0 || tf.text != "user@example.com" {
		t.Fatalf("code %d text %q: %s", code, tf.text, errb.String())
	}
}

func TestTypeRemovesStrayChars(t *testing.T) {
	tf := &typingFake{stray: 1, imeShown: true}
	a, errb := typingApp(t, tf)
	if code := a.run([]string{"type", "Gym"}); code != 0 || tf.text != "Gym" {
		t.Fatalf("code %d text %q: %s", code, tf.text, errb.String())
	}
}

func TestTypeAppendsToExistingTextExactly(t *testing.T) {
	tf := &typingFake{text: "Gym", drop: 1, imeShown: true}
	a, errb := typingApp(t, tf)
	if code := a.run([]string{"type", "Gym"}); code != 0 || tf.text != "GymGym" {
		t.Fatalf("code %d text %q: %s", code, tf.text, errb.String())
	}
}

func TestTypeFailsWhenNeverConverges(t *testing.T) {
	tf := &typingFake{drop: -1, imeShown: true}
	a, errb := typingApp(t, tf)
	if code := a.run([]string{"type", "user@example.com"}); code != 1 || !strings.Contains(errb.String(), "but field shows") {
		t.Fatalf("want exit 1, got text %q err %s", tf.text, errb.String())
	}
}

func TestTypeIntoFailsWithoutKeyboard(t *testing.T) {
	tf := &typingFake{}
	a, errb := typingApp(t, tf)
	if code := a.run([]string{"type", "--into", "--desc", "Email", "x"}); code != 1 || tf.text != "" {
		t.Fatalf("typed %q without keyboard: %s", tf.text, errb.String())
	}
}

func TestBackHidesKeyboardOnly(t *testing.T) {
	tf := &typingFake{imeShown: true, backHidesIM: true}
	a, errb := typingApp(t, tf)
	if code := a.run([]string{"back"}); code != 0 || tf.imeShown || tf.backCalls != 1 {
		t.Fatalf("code %d ime %v: %s", code, tf.imeShown, errb.String())
	}
	tf.imeShown, tf.backHidesIM = true, false
	if code := a.run([]string{"back"}); code != 1 {
		t.Fatal("stuck keyboard must fail")
	}
}

func TestCheckOptionalSDKIsNotAFailure(t *testing.T) {
	f := &fakeExec{reply: func(call) (string, error) { return "Android Debug Bridge version 1.0.41\n", nil }}
	a := newTestAndroid(t, f)
	var out, errb bytes.Buffer
	code := (&app{out: &out, err: &errb, plat: a, getenv: a.Getenv}).run([]string{"check"})
	if code != 0 || !strings.Contains(out.String(), "optional") || strings.Contains(out.String(), "missing") {
		t.Fatalf("code %d out=%s", code, out.String())
	}
}

func TestArgParsing(t *testing.T) {
	f := &fakeExec{}
	a := newTestAndroid(t, f)
	run := func(args ...string) int {
		var o, e bytes.Buffer
		return (&app{out: &o, err: &e, plat: a, getenv: a.Getenv}).run(args)
	}
	if run("bogus") != 2 || run("tap") != 2 || run("install") != 2 || run("wait") != 2 {
		t.Fatal("usage errors should exit 2")
	}
	if run("install", "/nonexistent.apk", "--grant") != 1 {
		t.Fatal("missing apk should exit 1")
	}
	if run("shell", "--", "ls", "-la", "/sdcard") != 0 {
		t.Fatal("shell")
	}
	if got := f.calls[0].args; !reflect.DeepEqual(got, []string{"shell", "ls -la /sdcard"}) {
		t.Fatalf("shell args %v", got)
	}
}

func TestBootWaitsForBootCompleted(t *testing.T) {
	polls := 0
	f := &fakeExec{}
	f.reply = func(c call) (string, error) {
		joined := strings.Join(c.args, " ")
		switch {
		case joined == "-list-avds":
			return "capturekit\n", nil
		case joined == "devices":
			if polls > 0 {
				return "List of devices attached\nemulator-5556\tdevice\n", nil
			}
			polls++
			return "List of devices attached\n", nil
		case strings.Contains(joined, "getprop"):
			polls++
			if polls > 3 {
				return "1\n", nil
			}
			return "\n", nil
		}
		return "", nil
	}
	a := newTestAndroid(t, f)
	a.Getenv = func(k string) string { return map[string]string{"ADB": "/sdk/adb"}[k] }
	a.LookPath = func(n string) (string, error) { return "/bin/" + n, nil }
	serial, err := a.Boot(BootOpts{Timeout: 60})
	if err != nil || serial != "emulator-5556" {
		t.Fatalf("serial=%q err=%v", serial, err)
	}
	started := false
	for _, c := range f.calls {
		if c.name == "START /bin/emulator" && strings.Contains(strings.Join(c.args, " "), "-avd capturekit -no-window") {
			started = true
		}
	}
	if !started {
		t.Fatalf("emulator not started: %v", f.calls)
	}
	if s, _ := loadState(a.StatePath); s.Serial != "emulator-5556" {
		t.Fatalf("state %v", s)
	}
}

func TestBootTimeoutAndMissingAVD(t *testing.T) {
	f := &fakeExec{reply: func(c call) (string, error) {
		if strings.Join(c.args, " ") == "-list-avds" {
			return "other\n", nil
		}
		return "List of devices attached\n", nil
	}}
	a := newTestAndroid(t, f)
	a.LookPath = func(n string) (string, error) { return "/bin/" + n, nil }
	if _, err := a.Boot(BootOpts{Name: "capturekit", Timeout: 5}); err == nil || !strings.Contains(err.Error(), "--create") {
		t.Fatalf("want missing-avd error, got %v", err)
	}
	f.reply = func(c call) (string, error) {
		if strings.Join(c.args, " ") == "-list-avds" {
			return "capturekit\n", nil
		}
		return "List of devices attached\n", nil
	}
	if _, err := a.Boot(BootOpts{Timeout: 5}); err == nil || !strings.Contains(err.Error(), "did not finish booting") {
		t.Fatalf("want timeout, got %v", err)
	}
}

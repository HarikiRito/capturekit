package sim

import (
	"reflect"
	"strings"
	"testing"
)

func axeSetup(t *testing.T) (Driver, *fakeExec) {
	t.Helper()
	f := &fakeExec{}
	p := newTestIOS(t, f)
	if err := p.Connect("", false, "U1"); err != nil {
		t.Fatal(err)
	}
	d, err := p.Driver("axe")
	if err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	return d, f
}

func TestAxeActionsArgv(t *testing.T) {
	d, f := axeSetup(t)
	for _, err := range []error{d.Tap(10, 20), d.Type("Hi 'x' -1"), d.Swipe(1, 2, 3, 4, 500), d.Key("40")} {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := []call{
		{"/bin/axe", []string{"tap", "-x", "10", "-y", "20", "--udid", "U1"}},
		{"/bin/axe", []string{"type", "--stdin", "--udid", "U1"}},
		{"/bin/axe", []string{"swipe", "--start-x", "1", "--start-y", "2", "--end-x", "3", "--end-y", "4", "--duration", "0.5", "--udid", "U1"}},
		{"/bin/axe", []string{"key", "40", "--udid", "U1"}},
	}
	if !reflect.DeepEqual(f.calls, want) {
		t.Fatalf("got %v\nwant %v", f.calls, want)
	}
}

func TestAxeTypeSendsTextOnStdin(t *testing.T) {
	d, f := axeSetup(t)
	if err := d.Type("Hi 'x' -1"); err != nil {
		t.Fatal(err)
	}
	if f.stdin != "Hi 'x' -1" {
		t.Fatalf("stdin %q", f.stdin)
	}
}

func TestAxeRejectsInvalidInput(t *testing.T) {
	d, f := axeSetup(t)
	for name, err := range map[string]error{
		"non-ascii": d.Type("café"), "key name": d.Key("enter"), "key range": d.Key("300"), "back": d.Back(),
	} {
		if err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if _, err := d.Shell("ls"); err == nil {
		t.Fatal("shell accepted")
	}
	if len(f.calls) != 0 {
		t.Fatalf("exec ran: %v", f.calls)
	}
}

func TestAxeUIMapsDescribeUIJSON(t *testing.T) {
	d, f := axeSetup(t)
	nodes, err := d.UI()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"describe-ui", "--udid", "U1"}; !reflect.DeepEqual(f.calls[0].args, want) {
		t.Fatalf("argv %v", f.calls[0].args)
	}
	want := []Node{
		{Desc: "Demo", Class: "Application", Enabled: true, Right: 834, Bottom: 1194},
		{Desc: "Sign in", ID: "signin", Class: "Button", Left: 100, Top: 200, Right: 300, Bottom: 260},
		{Text: "a@b.c", Desc: "Email", Class: "TextField", Enabled: true, Top: 300, Right: 300, Bottom: 340},
	}
	if !reflect.DeepEqual(nodes, want) {
		t.Fatalf("got %+v\nwant %+v", nodes, want)
	}
	if w, h, err := d.ScreenSize(); err != nil || w != 834 || h != 1194 {
		t.Fatalf("size %d %d %v", w, h, err)
	}
}

func TestAxeScreenshotUsesSimctl(t *testing.T) {
	d, f := axeSetup(t)
	png, err := d.Screenshot()
	if err != nil || string(png) != "\x89PNG-sim" {
		t.Fatalf("got %q %v", png, err)
	}
	a := f.calls[0].args
	if f.calls[0].name != "xcrun" || !reflect.DeepEqual(a[:5], []string{"simctl", "io", "U1", "screenshot", "--type=png"}) {
		t.Fatalf("argv %v", f.calls[0])
	}
	f.reply = func(call) (string, error) { return "", nil }
	if _, err := d.Screenshot(); err == nil || !strings.Contains(err.Error(), "PNG") {
		t.Fatalf("want PNG error, got %v", err)
	}
}

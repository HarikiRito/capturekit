package sim

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const simList = `{"devices":{"com.apple.CoreSimulator.SimRuntime.iOS-26-5":[
 {"name":"iPad Pro 11","udid":"U1","state":"Booted","isAvailable":true},
 {"name":"capturekit","udid":"U2","state":"Shutdown","isAvailable":true},
 {"name":"stale","udid":"U3","state":"Shutdown","isAvailable":false}]}}`

const axeTree = `[{"type":"Application","AXLabel":"Demo","frame":{"x":0,"y":0,"width":834,"height":1194},"enabled":true,
 "children":[{"type":"Button","AXLabel":"Sign in","AXUniqueId":"signin","AXValue":null,"enabled":false,
  "frame":{"x":100.4,"y":200,"width":200,"height":60},"children":[]},
 {"type":"TextField","AXLabel":"Email","AXValue":"a@b.c","frame":{"x":0,"y":300,"width":300,"height":40}}]}]`

func newTestIOS(t *testing.T, f *fakeExec) *IOS {
	t.Helper()
	f.reply = func(c call) (string, error) {
		switch {
		case strings.Contains(strings.Join(c.args, " "), "list devices"):
			return simList, nil
		case c.name == "/bin/axe" && c.args[0] == "describe-ui":
			return axeTree, nil
		case len(c.args) > 1 && c.args[1] == "create":
			return "NEW\n", nil
		case len(c.args) > 3 && c.args[3] == "screenshot":
			return "", os.WriteFile(c.args[len(c.args)-1], []byte("\x89PNG-sim"), 0o600)
		}
		return "", nil
	}
	return &IOS{
		X: f, StatePath: filepath.Join(t.TempDir(), "sim.json"),
		Getenv:   func(string) string { return "" },
		LookPath: func(n string) (string, error) { return "/bin/" + n, nil },
	}
}

func simctlCalls(f *fakeExec) (out [][]string) {
	for _, c := range f.calls {
		if c.name == "xcrun" {
			out = append(out, c.args)
		}
	}
	return out
}

func TestIOSBootReusesExistingSim(t *testing.T) {
	f := &fakeExec{}
	p := newTestIOS(t, f)
	udid, err := p.Boot(BootOpts{Name: "capturekit"})
	if err != nil || udid != "U2" {
		t.Fatalf("got %q %v", udid, err)
	}
	want := [][]string{{"simctl", "list", "devices", "-j"}, {"simctl", "bootstatus", "U2", "-b"}}
	if got := simctlCalls(f); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if st := p.state(); st.Serial != "U2" {
		t.Fatalf("state %+v", st)
	}
}

func TestIOSBootCreatesOnlyWithCreate(t *testing.T) {
	f := &fakeExec{}
	p := newTestIOS(t, f)
	if _, err := p.Boot(BootOpts{Name: "new"}); err == nil || !strings.Contains(err.Error(), "--create") {
		t.Fatalf("want --create hint, got %v", err)
	}
	udid, err := p.Boot(BootOpts{Name: "new", Create: true})
	if err != nil || udid != "NEW" {
		t.Fatalf("got %q %v", udid, err)
	}
	got := simctlCalls(f)
	if want := []string{"simctl", "create", "new", defaultSimType}; !reflect.DeepEqual(got[len(got)-2], want) {
		t.Fatalf("create argv %v", got[len(got)-2])
	}
	if want := []string{"simctl", "bootstatus", "NEW", "-b"}; !reflect.DeepEqual(got[len(got)-1], want) {
		t.Fatalf("bootstatus argv %v", got[len(got)-1])
	}
}

func TestIOSAppLifecycleArgv(t *testing.T) {
	f := &fakeExec{}
	p := newTestIOS(t, f)
	if err := p.Connect("", false, "U1"); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	if err := p.Install("/x/App.app", true); err != nil {
		t.Fatal(err)
	}
	if out, err := p.Launch("com.x"); err != nil || out != "" {
		t.Fatalf("launch %q %v", out, err)
	}
	for _, err := range []error{p.Stop("com.x"), p.Clear("com.x"), p.Grant("com.x", []string{"photos", "location"})} {
		if err != nil {
			t.Fatal(err)
		}
	}
	want := [][]string{
		{"simctl", "install", "U1", "/x/App.app"},
		{"simctl", "launch", "U1", "com.x"},
		{"simctl", "terminate", "U1", "com.x"},
		{"simctl", "uninstall", "U1", "com.x"},
		{"simctl", "privacy", "U1", "grant", "photos", "com.x"},
		{"simctl", "privacy", "U1", "grant", "location", "com.x"},
	}
	if got := simctlCalls(f); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

func TestIOSDevicesListsAvailableSims(t *testing.T) {
	p := newTestIOS(t, &fakeExec{})
	devs, err := p.Devices()
	want := []Device{{"U1", "Booted", "iPad Pro 11"}, {"U2", "Shutdown", "capturekit"}}
	if err != nil || !reflect.DeepEqual(devs, want) {
		t.Fatalf("got %v %v", devs, err)
	}
}

func TestIOSRealDeviceModeRejectsLifecycleAndShell(t *testing.T) {
	f := &fakeExec{}
	p := newTestIOS(t, f)
	if err := p.Connect("my-iphone", true, ""); err != nil {
		t.Fatal(err)
	}
	_, bootErr := p.Boot(BootOpts{})
	_, shellErr := p.Shell("ls")
	for name, err := range map[string]error{
		"boot": bootErr, "install": p.Install("a.app", false), "stop": p.Stop("x"),
		"clear": p.Clear("x"), "grant": p.Grant("x", []string{"photos"}),
	} {
		if err == nil || !strings.Contains(err.Error(), "phone-harness") {
			t.Fatalf("%s: got %v", name, err)
		}
	}
	if shellErr == nil {
		t.Fatal("shell accepted")
	}
	if _, err := newTestIOS(t, f).Shell("ls"); err == nil {
		t.Fatal("shell accepted on simulator")
	}
	if len(f.calls) != 0 {
		t.Fatalf("exec ran: %v", f.calls)
	}
	if devs, _ := p.Devices(); len(devs) != 1 || devs[0].Serial != "my-iphone" {
		t.Fatalf("devices %v", devs)
	}
}

func TestIOSAutoDriver(t *testing.T) {
	f := &fakeExec{}
	p := newTestIOS(t, f)
	app := &app{out: &bytes.Buffer{}, err: &bytes.Buffer{}, plat: p, getenv: p.Getenv}
	d, err := driverFor(t, app)
	if _, ok := d.(*axeDriver); err != nil || !ok {
		t.Fatalf("sim: %T %v", d, err)
	}
	if err := p.Connect("my-iphone", true, ""); err != nil {
		t.Fatal(err)
	}
	d, err = driverFor(t, app)
	if _, ok := d.(*phoneHarness); err != nil || !ok {
		t.Fatalf("device: %T %v", d, err)
	}
	if _, err := driverFor(t, app, "--driver", "adb"); err == nil || !strings.Contains(err.Error(), "auto|axe|phoneharness") {
		t.Fatalf("got %v", err)
	}
}

func TestIOSPhoneHarnessEnv(t *testing.T) {
	f := &fakeExec{}
	p := newTestIOS(t, f)
	f.reply = func(call) (string, error) { return "@@true\n", nil }
	d, err := p.Driver("phoneharness")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Tap(1, 2); err != nil {
		t.Fatal(err)
	}
	env := strings.Join(f.env, " ")
	if !strings.Contains(env, "PHONE_HARNESS_PLATFORM=ios") || strings.Contains(env, "ADB") || strings.Contains(env, "ANDROID") {
		t.Fatalf("env %q", env)
	}
	if _, err := d.UI(); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("ui: %v", err)
	}
}

func TestAndroidPhoneHarnessEnvUnchanged(t *testing.T) {
	a, f, _ := autoApp(t, true)
	d, err := a.plat.Driver("phoneharness")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Tap(1, 2); err != nil {
		t.Fatal(err)
	}
	if env := strings.Join(f.env, " "); !strings.Contains(env, "PHONE_HARNESS_PLATFORM=android") || !strings.Contains(env, "PHONE_HARNESS_ANDROID_ADB=/sdk/adb") {
		t.Fatalf("env %q", env)
	}
}

func TestPlatformSelection(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		env      string
		wantName string
		wantRest []string
	}{
		{[]string{"ui"}, "", "android", []string{"ui"}},
		{[]string{"--platform", "ios", "ui"}, "", "ios", []string{"ui"}},
		{[]string{"--platform=ios", "ui"}, "", "ios", []string{"ui"}},
		{[]string{"ui"}, "ios", "ios", []string{"ui"}},
		{[]string{"--platform", "android", "ui"}, "ios", "android", []string{"ui"}},
	} {
		name, rest := splitPlatform(tc.args, tc.env)
		p, err := newPlatform(name)
		if err != nil || p.Name() != tc.wantName || !reflect.DeepEqual(rest, tc.wantRest) {
			t.Fatalf("%v: %v %v %v", tc.args, p, rest, err)
		}
	}
	if _, err := newPlatform("symbian"); err == nil || !strings.Contains(err.Error(), "android|ios") {
		t.Fatalf("got %v", err)
	}
}

func TestIOSCheckReportsTools(t *testing.T) {
	p := newTestIOS(t, &fakeExec{})
	p.LookPath = func(n string) (string, error) {
		if n == "xcrun" {
			return "/usr/bin/xcrun", nil
		}
		return "", errors.New("none")
	}
	byName := map[string]Tool{}
	for _, tool := range p.Check() {
		byName[tool.Name] = tool
	}
	if !byName["xcrun"].OK || byName["xcrun"].Optional || byName["axe"].OK || !byName["axe"].Optional || !byName["phone-harness"].Optional {
		t.Fatalf("%+v", byName)
	}
}

func TestIOSTypeSkipsAndroidChecks(t *testing.T) {
	old := keyboardSettle
	keyboardSettle = 0
	t.Cleanup(func() { keyboardSettle = old })
	f := &fakeExec{}
	p := newTestIOS(t, f)
	if err := p.Connect("", false, "U1"); err != nil {
		t.Fatal(err)
	}
	a := &app{out: &bytes.Buffer{}, err: &bytes.Buffer{}, plat: p, getenv: p.Getenv}
	f.calls = nil
	if err := a.typeText([]string{"--into", "--desc", "Email", "hello"}); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range f.calls {
		names = append(names, c.args[0])
	}
	if want := []string{"describe-ui", "tap", "type"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("calls %v", names)
	}
	if f.stdin != "hello" {
		t.Fatalf("stdin %q", f.stdin)
	}
	if err := a.typeText([]string{"--clear", "x"}); err == nil {
		t.Fatal("--clear accepted on iOS")
	}
	if err := a.clearField(nil); err == nil {
		t.Fatal("clear-field accepted on iOS")
	}
}

func TestStateIsPerPlatform(t *testing.T) {
	f := &fakeExec{}
	ios := newTestIOS(t, f)
	droid := newTestAndroid(t, f)
	droid.StatePath = ios.StatePath
	if err := ios.Connect("my-iphone", true, ""); err != nil {
		t.Fatal(err)
	}
	if s := droid.state(); s != (State{}) {
		t.Fatalf("android saw ios state %+v", s)
	}
	if err := saveState(droid.StatePath, State{Server: "h:1", Serial: "emulator-5554"}); err != nil {
		t.Fatal(err)
	}
	if s := ios.state(); s != (State{}) {
		t.Fatalf("ios saw android state %+v", s)
	}
	if s := droid.state(); s.Serial != "emulator-5554" {
		t.Fatalf("android lost its state %+v", s)
	}
}

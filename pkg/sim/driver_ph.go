package sim

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

// phoneHarness drives the device through the phone-harness Python CLI (experimental).
type phoneHarness struct {
	a   *Android
	bin string
}

func phoneHarnessBin(getenv func(string) string) string {
	if p := getenv("PHONE_HARNESS_BIN"); p != "" {
		return p
	}
	return "phone-harness"
}

func newPhoneHarness(a *Android) (Driver, error) {
	bin, err := a.LookPath(phoneHarnessBin(a.Getenv))
	if err != nil {
		return nil, errors.New("phone-harness not found in PATH (pip install phone-harness, or set PHONE_HARNESS_BIN)")
	}
	return &phoneHarness{a: a, bin: bin}, nil
}

// call runs a python snippet with `a` bound to arg; the snippet sets `result`.
func (p *phoneHarness) call(step string, arg any, snippet string) (json.RawMessage, error) {
	argJSON, _ := json.Marshal(arg)
	adb, err := p.a.adbBin()
	if err != nil {
		return nil, err
	}
	st := p.a.state()
	env := []string{
		"PHONE_HARNESS_TELEMETRY=0", "DO_NOT_TRACK=1", "PHONE_HARNESS_NO_UPDATE_CHECK=1",
		"PHONE_HARNESS_PLATFORM=android", "PHONE_HARNESS_ANDROID_ADB=" + adb,
		"CAPTURE_SIM_ARG=" + string(argJSON),
	}
	if host, port, ok := strings.Cut(st.Server, ":"); st.Server != "" {
		env = append(env, "ANDROID_ADB_SERVER_ADDRESS="+host)
		if ok {
			env = append(env, "ANDROID_ADB_SERVER_PORT="+port)
		}
	}
	if st.Serial != "" {
		env = append(env, "ANDROID_SERIAL="+st.Serial)
	}
	script := "# task: capture sim\n# step: " + step + "\nimport os, json\na = json.loads(os.environ['CAPTURE_SIM_ARG'])\n" +
		snippet + "\nprint('@@' + json.dumps(result))\n"
	out, err := p.a.X.Run(env, []byte(script), p.bin)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, "@@") {
		return nil, fmt.Errorf("phone-harness gave no result: %s", strings.TrimSpace(string(out)))
	}
	return json.RawMessage(last[2:]), nil
}

func (p *phoneHarness) do(step string, arg any, snippet string) error {
	_, err := p.call(step, arg, snippet+"\nresult = True")
	return err
}

func (p *phoneHarness) Tap(x, y int) error {
	return p.do("tap", []int{x, y}, "tap(a[0], a[1])")
}

func (p *phoneHarness) Type(text string) error {
	return p.do("type", text, "type_text(a)")
}

func (p *phoneHarness) Swipe(x1, y1, x2, y2, ms int) error {
	return p.do("swipe", []int{x1, y1, x2, y2, ms}, "shell('input swipe %d %d %d %d %d' % tuple(a))")
}

func (p *phoneHarness) Key(key string) error {
	return p.do("key", normalizeKey(key), "shell('input keyevent ' + a)")
}

func (p *phoneHarness) Back() error { return p.do("back", nil, "back()") }

func (p *phoneHarness) UI() ([]Node, error) {
	raw, err := p.call("ui", nil, "result = ui()")
	if err != nil {
		return nil, err
	}
	var items []struct {
		Text, Desc, ID, Class string
		Clickable             bool
		X, Y, W, H            int
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	nodes := make([]Node, len(items))
	for i, it := range items {
		nodes[i] = Node{Text: it.Text, Desc: it.Desc, ID: it.ID, Class: it.Class, Clickable: it.Clickable, Enabled: true,
			Left: it.X - it.W/2, Top: it.Y - it.H/2, Right: it.X - it.W/2 + it.W, Bottom: it.Y - it.H/2 + it.H}
	}
	return nodes, nil
}

func (p *phoneHarness) Screenshot() ([]byte, error) {
	f, err := os.CreateTemp("", "capture-sim-*.png")
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()
	if err := p.do("screenshot", f.Name(), "screenshot(a)"); err != nil {
		return nil, err
	}
	return os.ReadFile(f.Name())
}

func (p *phoneHarness) ScreenSize() (int, int, error) {
	raw, err := p.call("screen-size", nil, "b = ensure_device()\nresult = [b['w'], b['h']]")
	var wh [2]int
	if err == nil {
		err = json.Unmarshal(raw, &wh)
	}
	return wh[0], wh[1], err
}

func (p *phoneHarness) Shell(cmd string) (string, error) {
	raw, err := p.call("shell", cmd, "result = shell(a)")
	if err != nil {
		return "", err
	}
	var s string
	return s, json.Unmarshal(raw, &s)
}

package sim

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultAVD   = "capturekit"
	defaultImage = "system-images;android-35;google_apis;x86_64"
	defaultHW    = "pixel_tablet"
)

// Android is the adb/emulator backed Platform.
type Android struct {
	X         Exec
	StatePath string
	Getenv    func(string) string
	LookPath  func(string) (string, error)
	Now       func() time.Time
	Sleep     func(time.Duration)
}

func NewAndroid() *Android {
	a := &Android{X: osExec{}, Getenv: os.Getenv, LookPath: exec.LookPath, Now: time.Now, Sleep: time.Sleep}
	a.StatePath = defaultStatePath(a.Getenv)
	return a
}

func (a *Android) Name() string { return "android" }

func (a *Android) state() State {
	s, _ := loadState(a.StatePath)
	return s
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func (a *Android) sdkDirs() []string {
	var dirs []string
	for _, k := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := a.Getenv(k); v != "" {
			dirs = append(dirs, v)
		}
	}
	home := a.Getenv("HOME")
	return append(dirs, filepath.Join(home, "Android", "Sdk"), filepath.Join(home, "Library", "Android", "sdk"))
}

// sdkTool finds a tool under the SDK (rel) else in PATH.
func (a *Android) sdkTool(name string, rel ...string) string {
	for _, d := range a.sdkDirs() {
		for _, r := range rel {
			if p := filepath.Join(d, r); fileExists(p) {
				return p
			}
		}
	}
	p, _ := a.LookPath(name)
	return p
}

func (a *Android) adbBin() (string, error) {
	if p := a.Getenv("ADB"); p != "" {
		return p, nil
	}
	if p := a.sdkTool("adb", "platform-tools/adb"); p != "" {
		return p, nil
	}
	return "", errors.New("adb not found: set ADB or ANDROID_HOME, or add adb to PATH")
}

// adbArgs builds adb arguments with -H/-P from state and -s when withSerial.
func adbArgs(s State, withSerial bool, args ...string) []string {
	var out []string
	if s.Server != "" {
		host, port, err := net.SplitHostPort(s.Server)
		if err != nil {
			host = s.Server
		}
		out = append(out, "-H", host)
		if port != "" {
			out = append(out, "-P", port)
		}
	}
	if withSerial && s.Serial != "" {
		out = append(out, "-s", s.Serial)
	}
	return append(out, args...)
}

func (a *Android) adbRun(s State, withSerial bool, stdin []byte, args ...string) ([]byte, error) {
	bin, err := a.adbBin()
	if err != nil {
		return nil, err
	}
	out, err := a.X.Run(nil, stdin, bin, adbArgs(s, withSerial, args...)...)
	if err != nil && strings.Contains(err.Error(), "more than one") {
		err = fmt.Errorf("%w (pick one: capture sim connect --serial <serial>)", err)
	}
	return out, err
}

// dev runs an adb command against the active device.
func (a *Android) dev(args ...string) (string, error) {
	out, err := a.adbRun(a.state(), true, nil, args...)
	return string(out), err
}

func (a *Android) Check() []Tool {
	var tools []Tool
	if bin, err := a.adbBin(); err != nil {
		tools = append(tools, Tool{Name: "adb", Note: err.Error()})
	} else {
		t := Tool{Name: "adb", Path: bin, OK: true}
		if out, err := a.X.Run(nil, nil, bin, "version"); err == nil {
			t.Version = strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]
		} else {
			t.OK, t.Note = false, err.Error()
		}
		tools = append(tools, t)
	}
	sdk := Tool{Name: "android-sdk", Optional: true, Note: "optional (needed for boot only); set ANDROID_HOME or ANDROID_SDK_ROOT"}
	for _, k := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := a.Getenv(k); v != "" {
			sdk = Tool{Name: "android-sdk", Path: v, OK: true, Optional: true, Note: k + " (needed for boot only)"}
			break
		}
	}
	tools = append(tools, sdk)
	for _, t := range []struct{ name, path string }{
		{"emulator", a.sdkTool("emulator", "emulator/emulator")},
		{"avdmanager", a.sdkTool("avdmanager", "cmdline-tools/latest/bin/avdmanager")},
	} {
		tools = append(tools, Tool{Name: t.name, Path: t.path, OK: t.path != "", Optional: true, Note: "optional (needed for boot only)"})
	}
	ph := Tool{Name: "phone-harness", Optional: true, Note: "optional, default driver when present (--driver auto), else adb"}
	if p, err := a.LookPath(phoneHarnessBin(a.Getenv)); err == nil {
		ph.Path, ph.OK, ph.Note = p, true, "experimental"
		if out, err := a.X.Run(nil, nil, p, "--version"); err == nil {
			ph.Version = strings.TrimSpace(string(out))
		}
	}
	return append(tools, ph)
}

func parseDevices(out string) []Device {
	var devs []Device
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || line == "List of devices attached" || strings.HasPrefix(line, "*") {
			continue
		}
		devs = append(devs, Device{Serial: f[0], State: f[1], Info: strings.Join(f[2:], " ")})
	}
	return devs
}

func (a *Android) Devices() ([]Device, error) {
	out, err := a.adbRun(a.state(), false, nil, "devices", "-l")
	return parseDevices(string(out)), err
}

func (a *Android) Connect(target string, device bool, serial string) error {
	s := a.state()
	if target != "" && !device {
		s.Server = target
		if serial == "" {
			s.Serial = ""
		}
	}
	if serial != "" {
		s.Serial = serial
	}
	if device {
		if _, err := a.adbRun(s, false, nil, "connect", target); err != nil {
			return err
		}
		s.Serial = target
	}
	out, err := a.adbRun(s, false, nil, "devices", "-l")
	if err != nil {
		return err
	}
	if len(parseDevices(string(out))) == 0 {
		return fmt.Errorf("connected to adb but no devices are attached")
	}
	return saveState(a.StatePath, s)
}

func (a *Android) Install(path string, grant bool) error {
	if !fileExists(path) {
		return fmt.Errorf("apk not found: %s", path)
	}
	args := []string{"install", "-r"}
	if grant {
		args = append(args, "-g")
	}
	_, err := a.dev(append(args, path)...)
	return err
}

func (a *Android) Launch(app string) (string, error) {
	comp := app
	if !strings.Contains(app, "/") {
		out, err := a.dev("shell", "cmd", "package", "resolve-activity", "--brief", app)
		if err != nil {
			return "", err
		}
		comp = ""
		for _, l := range strings.Split(out, "\n") {
			if l = strings.TrimSpace(l); strings.Contains(l, "/") && !strings.Contains(l, " ") {
				comp = l
			}
		}
		if comp == "" {
			return "", fmt.Errorf("no launcher activity for %q (installed?)", app)
		}
	}
	out, err := a.dev("shell", "am", "start", "-n", comp)
	if err == nil && strings.Contains(out, "Error") {
		err = fmt.Errorf("am start %s: %s", comp, strings.TrimSpace(out))
	}
	return comp, err
}

func (a *Android) Stop(pkg string) error {
	_, err := a.dev("shell", "am", "force-stop", pkg)
	return err
}

func (a *Android) Clear(pkg string) error {
	out, err := a.dev("shell", "pm", "clear", pkg)
	if err == nil && !strings.Contains(out, "Success") {
		err = fmt.Errorf("pm clear %s: %s", pkg, strings.TrimSpace(out))
	}
	return err
}

func (a *Android) Grant(pkg string, perms []string) error {
	for _, p := range perms {
		if _, err := a.dev("shell", "pm", "grant", pkg, p); err != nil {
			return err
		}
	}
	return nil
}

func (a *Android) Shell(cmd string) (string, error) { return a.dev("shell", cmd) }

func (a *Android) Driver(name string) (Driver, error) {
	switch name {
	case "", "adb":
		return &adbDriver{a: a}, nil
	case "phoneharness":
		return newPhoneHarness(a)
	}
	return nil, fmt.Errorf("unknown driver %q (auto|adb|phoneharness)", name)
}

// Boot starts a headless native emulator and waits for sys.boot_completed.
func (a *Android) Boot(o BootOpts) (string, error) {
	if o.Name == "" {
		o.Name = defaultAVD
	}
	emu := a.sdkTool("emulator", "emulator/emulator")
	if emu == "" {
		return "", errors.New("emulator not found: install the Android SDK emulator and set ANDROID_HOME")
	}
	out, err := a.X.Run(nil, nil, emu, "-list-avds")
	if err != nil {
		return "", err
	}
	if !strings.Contains("\n"+string(out), "\n"+o.Name+"\n") {
		if !o.Create {
			return "", fmt.Errorf("avd %q not found (use --create)", o.Name)
		}
		if err := a.createAVD(o.Name); err != nil {
			return "", err
		}
	}
	local := State{} // a booted emulator registers with the local adb server
	before, err := a.adbRun(local, false, nil, "devices")
	if err != nil {
		return "", err
	}
	known := map[string]bool{}
	for _, d := range parseDevices(string(before)) {
		known[d.Serial] = true
	}
	log := filepath.Join(filepath.Dir(a.StatePath), "emulator-"+o.Name+".log")
	_ = os.MkdirAll(filepath.Dir(log), 0o755)
	err = a.X.Start(log, nil, emu, "-avd", o.Name, "-no-window", "-no-audio", "-no-snapshot",
		"-gpu", "swiftshader_indirect", "-accel", "auto")
	if err != nil {
		return "", err
	}
	serial, err := a.waitBoot(known, time.Duration(o.Timeout)*time.Second)
	if err != nil {
		return "", fmt.Errorf("%w (see %s)", err, log)
	}
	return serial, saveState(a.StatePath, State{Serial: serial})
}

func (a *Android) waitBoot(known map[string]bool, timeout time.Duration) (string, error) {
	deadline := a.Now().Add(timeout)
	local := State{}
	for a.Now().Before(deadline) {
		out, _ := a.adbRun(local, false, nil, "devices")
		for _, d := range parseDevices(string(out)) {
			if known[d.Serial] || !strings.HasPrefix(d.Serial, "emulator-") || d.State != "device" {
				continue
			}
			prop, _ := a.adbRun(State{Serial: d.Serial}, true, nil, "shell", "getprop", "sys.boot_completed")
			if strings.TrimSpace(string(prop)) == "1" {
				return d.Serial, nil
			}
		}
		a.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("emulator did not finish booting within %s", timeout)
}

func (a *Android) createAVD(name string) error {
	mgr := a.sdkTool("avdmanager", "cmdline-tools/latest/bin/avdmanager")
	if mgr == "" {
		return errors.New("avdmanager not found: install Android SDK cmdline-tools")
	}
	_, err := a.X.Run(nil, []byte("no\n"), mgr, "create", "avd", "-n", name, "-k", defaultImage, "-d", defaultHW, "--force")
	return err
}

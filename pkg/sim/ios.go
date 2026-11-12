package sim

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const (
	defaultSim     = "capturekit"
	defaultSimType = "com.apple.CoreSimulator.SimDeviceType.iPad-Air-11-inch-M2"
)

// IOS is the Platform for iOS simulators (xcrun simctl) and, with --device, a real iPhone via phone-harness.
type IOS struct {
	X         Exec
	StatePath string
	Getenv    func(string) string
	LookPath  func(string) (string, error)
}

func NewIOS() *IOS {
	p := &IOS{X: osExec{}, Getenv: os.Getenv, LookPath: exec.LookPath}
	p.StatePath = defaultStatePath(p.Getenv)
	return p
}

func (p *IOS) Name() string { return "ios" }

func (p *IOS) state() State {
	s, _ := loadState(p.StatePath)
	return s
}

func (p *IOS) simctl(args ...string) (string, error) {
	out, err := p.X.Run(nil, nil, "xcrun", append([]string{"simctl"}, args...)...)
	return strings.TrimSpace(string(out)), err
}

// simOnly rejects lifecycle commands in real-device mode.
func (p *IOS) simOnly(what string) error {
	if p.state().Device {
		return fmt.Errorf("%s is unsupported for a real device, use phone-harness", what)
	}
	return nil
}

type simDevice struct {
	Name, UDID, State string
	IsAvailable       bool
}

func (p *IOS) sims() ([]simDevice, error) {
	out, err := p.simctl("list", "devices", "-j")
	if err != nil {
		return nil, err
	}
	var l struct{ Devices map[string][]simDevice }
	if err := json.Unmarshal([]byte(out), &l); err != nil {
		return nil, fmt.Errorf("parse simctl list: %w", err)
	}
	runtimes := make([]string, 0, len(l.Devices))
	for r := range l.Devices {
		runtimes = append(runtimes, r)
	}
	sort.Strings(runtimes)
	var sims []simDevice
	for _, r := range runtimes {
		for _, d := range l.Devices[r] {
			if d.IsAvailable {
				sims = append(sims, d)
			}
		}
	}
	return sims, nil
}

// udid is the saved simulator, else the only booted one (axe needs an explicit UDID).
func (p *IOS) udid() (string, error) {
	if s := p.state().Serial; s != "" {
		return s, nil
	}
	sims, err := p.sims()
	if err != nil {
		return "", err
	}
	var booted []string
	for _, d := range sims {
		if d.State == "Booted" {
			booted = append(booted, d.UDID)
		}
	}
	if len(booted) != 1 {
		return "", fmt.Errorf("%d booted simulators (pick one: capture sim --platform ios connect --serial <udid>)", len(booted))
	}
	return booted[0], nil
}

func (p *IOS) Check() []Tool {
	var tools []Tool
	if bin, err := p.LookPath("xcrun"); err != nil {
		tools = append(tools, Tool{Name: "xcrun", Note: "install Xcode"})
	} else {
		t := Tool{Name: "xcrun", Path: bin, OK: true}
		if _, err := p.simctl("help"); err != nil {
			t.OK, t.Note = false, "simctl unavailable: "+err.Error()
		}
		tools = append(tools, t)
	}
	axe := Tool{Name: "axe", Optional: true, Note: "optional, simulator UI tree + input (brew install cameroncooke/axe/axe)"}
	if bin, err := p.LookPath("axe"); err == nil {
		axe.Path, axe.OK = bin, true
	}
	ph := Tool{Name: "phone-harness", Optional: true, Note: "optional, real iPhone via iPhone Mirroring (macOS 15+, iOS 18+)"}
	if bin, err := p.LookPath(phoneHarnessBin(p.Getenv)); err == nil {
		ph.Path, ph.OK = bin, true
	}
	return append(tools, axe, ph)
}

// Boot reuses a simulator named o.Name, else creates it (o.Create), then waits for boot. o.Timeout is not used.
func (p *IOS) Boot(o BootOpts) (string, error) {
	if err := p.simOnly("boot"); err != nil {
		return "", err
	}
	if o.Name == "" {
		o.Name = defaultSim
	}
	sims, err := p.sims()
	if err != nil {
		return "", err
	}
	udid := ""
	for _, d := range sims {
		if d.Name == o.Name {
			udid = d.UDID
			break
		}
	}
	if udid == "" {
		if !o.Create {
			return "", fmt.Errorf("simulator %q not found (use --create)", o.Name)
		}
		if udid, err = p.simctl("create", o.Name, defaultSimType); err != nil {
			return "", err
		}
	}
	if _, err := p.simctl("bootstatus", udid, "-b"); err != nil {
		return "", err
	}
	return udid, saveState(p.StatePath, State{Serial: udid})
}

// Connect picks a simulator by --serial <udid>, or with device a real iPhone (target is only a label).
func (p *IOS) Connect(target string, device bool, serial string) error {
	if device {
		return saveState(p.StatePath, State{Serial: target, Device: true})
	}
	if target != "" || serial == "" {
		return errors.New("ios has no remote server: use --serial <udid>, or <name> --device for a real iPhone")
	}
	sims, err := p.sims()
	if err != nil {
		return err
	}
	for _, d := range sims {
		if d.UDID == serial {
			return saveState(p.StatePath, State{Serial: serial})
		}
	}
	return fmt.Errorf("no simulator with udid %q", serial)
}

func (p *IOS) Devices() ([]Device, error) {
	if st := p.state(); st.Device {
		return []Device{{Serial: st.Serial, State: "mirroring"}}, nil
	}
	sims, err := p.sims()
	devs := make([]Device, len(sims))
	for i, d := range sims {
		devs[i] = Device{Serial: d.UDID, State: d.State, Info: d.Name}
	}
	return devs, err
}

// simctlApp runs `simctl <verb> <udid> <args>` on the active simulator.
func (p *IOS) simctlApp(verb string, args ...string) (string, error) {
	if err := p.simOnly(verb); err != nil {
		return "", err
	}
	udid, err := p.udid()
	if err != nil {
		return "", err
	}
	return p.simctl(append([]string{verb, udid}, args...)...)
}

// Install takes an .app bundle path; grant is ignored (use `grant`).
func (p *IOS) Install(path string, _ bool) error {
	_, err := p.simctlApp("install", path)
	return err
}

// Launch returns simctl's "<bundle>: <pid>" line.
func (p *IOS) Launch(bundleID string) (string, error) { return p.simctlApp("launch", bundleID) }

func (p *IOS) Stop(bundleID string) error {
	_, err := p.simctlApp("terminate", bundleID)
	return err
}

// Clear uninstalls the app: simctl has no per-app data wipe, so reinstall afterwards.
func (p *IOS) Clear(bundleID string) error {
	_, err := p.simctlApp("uninstall", bundleID)
	return err
}

func (p *IOS) Grant(bundleID string, services []string) error {
	for _, s := range services {
		if _, err := p.simctlApp("privacy", "grant", s, bundleID); err != nil {
			return err
		}
	}
	return nil
}

func (p *IOS) Shell(string) (string, error) { return "", errors.New("shell is unsupported on iOS") }

func (p *IOS) Driver(name string) (Driver, error) {
	switch name {
	case "", "auto":
		if p.state().Device {
			return newPhoneHarness(p.X, p.LookPath, p.Getenv, "ios", nil)
		}
		return newAxe(p)
	case "axe":
		return newAxe(p)
	case "phoneharness":
		return newPhoneHarness(p.X, p.LookPath, p.Getenv, "ios", nil)
	}
	return nil, fmt.Errorf("unknown driver %q (auto|axe|phoneharness)", name)
}

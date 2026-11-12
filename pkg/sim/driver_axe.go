package sim

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
)

// axeDriver drives an iOS simulator through AXe (https://github.com/cameroncooke/AXe); coordinates are points.
type axeDriver struct {
	p   *IOS
	bin string
}

func newAxe(p *IOS) (Driver, error) {
	bin, err := p.LookPath("axe")
	if err != nil {
		return nil, errors.New("axe not found in PATH (brew install cameroncooke/axe/axe)")
	}
	return &axeDriver{p: p, bin: bin}, nil
}

// run executes `axe <args> --udid <active simulator>`.
func (d *axeDriver) run(stdin []byte, args ...string) ([]byte, error) {
	udid, err := d.p.udid()
	if err != nil {
		return nil, err
	}
	return d.p.X.Run(nil, stdin, d.bin, append(args, "--udid", udid)...)
}

func (d *axeDriver) Tap(x, y int) error {
	_, err := d.run(nil, "tap", "-x", strconv.Itoa(x), "-y", strconv.Itoa(y))
	return err
}

func (d *axeDriver) Type(text string) error {
	for _, r := range text {
		if r < 0x20 || r > 0x7e {
			return fmt.Errorf("axe cannot type %q (US keyboard ASCII only)", r)
		}
	}
	if text == "" {
		return nil
	}
	_, err := d.run([]byte(text), "type", "--stdin")
	return err
}

func (d *axeDriver) Swipe(x1, y1, x2, y2, ms int) error {
	_, err := d.run(nil, "swipe", "--start-x", strconv.Itoa(x1), "--start-y", strconv.Itoa(y1),
		"--end-x", strconv.Itoa(x2), "--end-y", strconv.Itoa(y2),
		"--duration", strconv.FormatFloat(float64(ms)/1000, 'f', -1, 64))
	return err
}

// Key takes an HID usage code (0-255), e.g. 40 = Return.
func (d *axeDriver) Key(key string) error {
	if n, err := strconv.Atoi(key); err != nil || n < 0 || n > 255 {
		return fmt.Errorf("axe key takes an HID usage code 0-255 (40 = Return), got %q", key)
	}
	_, err := d.run(nil, "key", key)
	return err
}

func (d *axeDriver) Back() error {
	return errors.New("back is unsupported on iOS: tap the app's back button (see ui)")
}

func (d *axeDriver) Shell(string) (string, error) {
	return "", errors.New("shell is unsupported on iOS")
}

type axeNode struct {
	Type, AXLabel, AXValue, AXUniqueID string
	Enabled                            *bool
	Frame                              struct{ X, Y, Width, Height float64 }
	Children                           []axeNode
}

// UI maps `axe describe-ui` (a JSON array of nested nodes) to flat Nodes; AXLabel -> Desc, AXValue -> Text.
func (d *axeDriver) UI() ([]Node, error) {
	out, err := d.run(nil, "describe-ui")
	if err != nil {
		return nil, err
	}
	var roots []axeNode
	if err := json.Unmarshal(out, &roots); err != nil {
		return nil, fmt.Errorf("parse axe describe-ui: %w", err)
	}
	var nodes []Node
	var walk func([]axeNode)
	walk = func(ns []axeNode) {
		for _, n := range ns {
			f := n.Frame
			nodes = append(nodes, Node{Text: n.AXValue, Desc: n.AXLabel, ID: n.AXUniqueID, Class: n.Type,
				Enabled: n.Enabled == nil || *n.Enabled,
				Left:    int(math.Round(f.X)), Top: int(math.Round(f.Y)),
				Right: int(math.Round(f.X + f.Width)), Bottom: int(math.Round(f.Y + f.Height))})
			walk(n.Children)
		}
	}
	walk(roots)
	return nodes, nil
}

func (d *axeDriver) Screenshot() ([]byte, error) {
	udid, err := d.p.udid()
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "capture-sim-*.png")
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := d.p.simctl("io", udid, "screenshot", "--type=png", f.Name()); err != nil {
		return nil, err
	}
	png, err := os.ReadFile(f.Name())
	if err == nil && !bytes.HasPrefix(png, []byte("\x89PNG")) {
		return nil, fmt.Errorf("simctl screenshot is not a PNG (%d bytes)", len(png))
	}
	return png, err
}

// ScreenSize is the root node's size in points, the unit tap/swipe use (screenshots are in pixels).
func (d *axeDriver) ScreenSize() (int, int, error) {
	nodes, err := d.UI()
	if err != nil || len(nodes) == 0 {
		return 0, 0, errors.Join(err, errors.New("no screen size from axe describe-ui"))
	}
	return nodes[0].Right - nodes[0].Left, nodes[0].Bottom - nodes[0].Top, nil
}

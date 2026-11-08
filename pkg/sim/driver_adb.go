package sim

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type adbDriver struct{ a *Android }

// escapeInputText prepares text for `adb shell input text` (shell-quoted, spaces as %s).
func escapeInputText(s string) (string, error) {
	for _, r := range s {
		if r < 0x20 || r > 0x7e {
			return "", fmt.Errorf("adb input cannot type %q (ASCII only)", r)
		}
	}
	s = strings.ReplaceAll(s, " ", "%s")
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'", nil
}

// normalizeKey maps back/enter/KEYCODE_X/66 to an `input keyevent` argument.
func normalizeKey(k string) string {
	if _, err := strconv.Atoi(k); err == nil {
		return k
	}
	k = strings.ToUpper(k)
	if !strings.HasPrefix(k, "KEYCODE_") {
		k = "KEYCODE_" + k
	}
	return k
}

func (d *adbDriver) shell(args ...string) (string, error) {
	return d.a.dev(append([]string{"shell"}, args...)...)
}

func (d *adbDriver) Tap(x, y int) error {
	_, err := d.shell("input", "tap", strconv.Itoa(x), strconv.Itoa(y))
	return err
}

func (d *adbDriver) Type(text string) error {
	esc, err := escapeInputText(text)
	if err != nil {
		return err
	}
	_, err = d.shell("input", "text", esc)
	return err
}

func (d *adbDriver) Swipe(x1, y1, x2, y2, ms int) error {
	_, err := d.shell("input", "swipe", strconv.Itoa(x1), strconv.Itoa(y1), strconv.Itoa(x2), strconv.Itoa(y2), strconv.Itoa(ms))
	return err
}

func (d *adbDriver) Key(key string) error {
	_, err := d.shell("input", "keyevent", normalizeKey(key))
	return err
}

func (d *adbDriver) Back() error { return d.Key("BACK") }

func (d *adbDriver) UI() ([]Node, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		out, err := d.a.dev("exec-out", "uiautomator", "dump", "/dev/tty")
		if err == nil {
			var nodes []Node
			if nodes, err = ParseUIXML([]byte(out)); err == nil {
				return nodes, nil
			}
		}
		lastErr = err
		d.a.Sleep(time.Second)
	}
	return nil, lastErr
}

func (d *adbDriver) Screenshot() ([]byte, error) {
	out, err := d.a.dev("exec-out", "screencap", "-p")
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix([]byte(out), []byte("\x89PNG")) {
		return nil, fmt.Errorf("screencap did not return a PNG (%d bytes)", len(out))
	}
	return []byte(out), nil
}

func (d *adbDriver) ScreenSize() (int, int, error) {
	out, err := d.shell("wm", "size")
	if err != nil {
		return 0, 0, err
	}
	var w, h int
	// "Physical size: 1080x2400" (last line wins: an override beats physical)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if _, err := fmt.Sscanf(lines[len(lines)-1][strings.LastIndex(lines[len(lines)-1], ":")+1:], " %dx%d", &w, &h); err != nil {
		return 0, 0, fmt.Errorf("parse wm size %q", out)
	}
	return w, h, nil
}

func (d *adbDriver) Shell(cmd string) (string, error) { return d.a.Shell(cmd) }

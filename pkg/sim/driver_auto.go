package sim

import "fmt"

// autoDriver prefers phone-harness. Reads retry once on adb; actions never do (a failed action may have half-applied).
type autoDriver struct {
	ph, adb Driver
}

func (d *autoDriver) hint(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w (phone-harness driver; rerun with --driver adb)", err)
}

func (d *autoDriver) Tap(x, y int) error     { return d.hint(d.ph.Tap(x, y)) }
func (d *autoDriver) Type(text string) error { return d.hint(d.ph.Type(text)) }
func (d *autoDriver) Swipe(x1, y1, x2, y2, ms int) error {
	return d.hint(d.ph.Swipe(x1, y1, x2, y2, ms))
}
func (d *autoDriver) Key(key string) error { return d.hint(d.ph.Key(key)) }
func (d *autoDriver) Back() error          { return d.hint(d.ph.Back()) }

func (d *autoDriver) Shell(cmd string) (string, error) {
	out, err := d.ph.Shell(cmd)
	return out, d.hint(err)
}

func (d *autoDriver) UI() ([]Node, error) {
	if nodes, err := d.ph.UI(); err == nil {
		return nodes, nil
	}
	return d.adb.UI()
}

func (d *autoDriver) Screenshot() ([]byte, error) {
	if png, err := d.ph.Screenshot(); err == nil {
		return png, nil
	}
	return d.adb.Screenshot()
}

func (d *autoDriver) ScreenSize() (int, int, error) {
	if w, h, err := d.ph.ScreenSize(); err == nil {
		return w, h, nil
	}
	return d.adb.ScreenSize()
}

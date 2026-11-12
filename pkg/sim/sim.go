// Package sim installs, runs and drives apps on emulators for coding agents.
package sim

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type app struct {
	out, err io.Writer
	plat     Platform
	getenv   func(string) string
}

// Run executes `capture sim [--platform android|ios] <args>` and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	name, args := splitPlatform(args, os.Getenv("CAPTURE_SIM_PLATFORM"))
	plat, err := newPlatform(name)
	if err != nil {
		fmt.Fprintf(stderr, "capture sim: %v\n", err)
		return 2
	}
	return (&app{out: stdout, err: stderr, plat: plat, getenv: os.Getenv}).run(args)
}

// splitPlatform takes a leading --platform flag (default def, else android) off args.
func splitPlatform(args []string, def string) (string, []string) {
	if def == "" {
		def = "android"
	}
	switch {
	case len(args) >= 2 && args[0] == "--platform":
		return args[1], args[2:]
	case len(args) >= 1 && strings.HasPrefix(args[0], "--platform="):
		return strings.TrimPrefix(args[0], "--platform="), args[1:]
	}
	return def, args
}

func newPlatform(name string) (Platform, error) {
	switch name {
	case "android":
		return NewAndroid(), nil
	case "ios":
		return NewIOS(), nil
	}
	return nil, fmt.Errorf("unknown platform %q (android|ios)", name)
}

type command struct {
	usage string
	run   func(a *app, args []string) error
}

var errUsage = errors.New("usage")

var commands map[string]command

func init() {
	commands = map[string]command{
		"check":       {"check [--json]", (*app).check},
		"boot":        {"boot [--avd name] [--create] [--timeout 180s] [--json]", (*app).boot},
		"connect":     {"connect <host:port> [--device] [--serial s] | connect --serial s", (*app).connect},
		"devices":     {"devices [--json]", (*app).devices},
		"install":     {"install <apk> [--grant]", (*app).install},
		"launch":      {"launch <package>[/activity]", (*app).launch},
		"stop":        {"stop <package>", (*app).stop},
		"clear":       {"clear <package>", (*app).clear},
		"grant":       {"grant <package> <permission>...", (*app).grant},
		"shell":       {"shell -- <cmd>", (*app).shell},
		"ui":          {"ui [--json] [--all] [--driver d]", (*app).ui},
		"tap":         {"tap <x> <y> | tap --text T | tap --desc D [--exact] [--no-verify]", (*app).tap},
		"type":        {"type [--clear] <text> | type --into --text T | --desc D [--clear] [--exact] <text>", (*app).typeText},
		"clear-field": {"clear-field [--json]", (*app).clearField},
		"key":         {"key <KEYCODE|name>", (*app).key},
		"back":        {"back", (*app).back},
		"swipe":       {"swipe <x1> <y1> <x2> <y2> [ms] | swipe up|down|left|right", (*app).swipe},
		"wait":        {"wait --text T | --desc D [--exact] [--timeout 20s]", (*app).wait},
		"screenshot":  {"screenshot <out.png>", (*app).screenshot},
		"screen-size": {"screen-size [--json]", (*app).screenSize},
	}
}

func (a *app) run(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(a.out, Instructions())
		return 0
	}
	c, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(a.err, "capture sim: unknown command %q (try `capture sim help`)\n", args[0])
		return 2
	}
	err := c.run(a, args[1:])
	switch {
	case err == nil:
		return 0
	case errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errUsage):
		fmt.Fprintf(a.err, "usage: capture sim %s\n", c.usage)
		return 2
	}
	fmt.Fprintf(a.err, "capture sim %s: %v\n", args[0], err)
	return 1
}

// flags holds options shared by subcommands.
type flags struct {
	fs     *flag.FlagSet
	json   *bool
	driver *string
	pos    []string
}

func (a *app) flags(name string) *flags {
	fs := flag.NewFlagSet("capture sim "+name, flag.ContinueOnError)
	fs.SetOutput(a.err)
	return &flags{fs: fs, json: fs.Bool("json", false, "machine-readable JSON output")}
}

func (f *flags) driverFlag(getenv func(string) string) {
	def := getenv("CAPTURE_SIM_DRIVER")
	if def == "" {
		def = "auto"
	}
	f.driver = f.fs.String("driver", def, "auto|adb|axe|phoneharness (adb: android, axe: ios)")
}

// parse allows flags anywhere; everything after a literal "--" is positional.
func (f *flags) parse(args []string) error {
	var tail []string
	for i, s := range args {
		if s == "--" {
			args, tail = args[:i], args[i+1:]
			break
		}
	}
	for {
		if err := f.fs.Parse(args); err != nil {
			return err
		}
		rest := f.fs.Args()
		if len(rest) == 0 {
			break
		}
		f.pos = append(f.pos, rest[0])
		args = rest[1:]
	}
	f.pos = append(f.pos, tail...)
	return nil
}

func (a *app) emit(asJSON bool, v any, text string) error {
	if asJSON {
		enc := json.NewEncoder(a.out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
	if text != "" {
		fmt.Fprintln(a.out, text)
	}
	return nil
}

func (a *app) driver(f *flags) (Driver, error) {
	if *f.driver != "auto" || a.plat.Name() != "android" {
		return a.plat.Driver(*f.driver)
	}
	adb, err := a.plat.Driver("adb")
	if err != nil {
		return nil, err
	}
	ph, err := a.plat.Driver("phoneharness")
	if err != nil {
		fmt.Fprintf(a.err, "capture sim: phone-harness unavailable, using adb: %v\n", err)
		return adb, nil
	}
	return &autoDriver{ph: ph, adb: adb}, nil
}

func (a *app) check(args []string) error {
	f := a.flags("check")
	if err := f.parse(args); err != nil {
		return err
	}
	tools := a.plat.Check()
	var lines []string
	failed := false
	for _, t := range tools {
		mark := "ok      "
		switch {
		case t.OK:
		case t.Optional:
			mark = "optional"
		default:
			mark = "missing "
			failed = true
		}
		lines = append(lines, strings.TrimSpace(fmt.Sprintf("%s %-14s %s %s %s", mark, t.Name, t.Path, t.Version, t.Note)))
	}
	if err := a.emit(*f.json, tools, strings.Join(lines, "\n")); err != nil {
		return err
	}
	if failed {
		return errors.New("a required tool is missing")
	}
	return nil
}

func (a *app) boot(args []string) error {
	f := a.flags("boot")
	avd := f.fs.String("avd", "", "AVD name (default "+defaultAVD+")")
	create := f.fs.Bool("create", false, "create the AVD if missing")
	timeout := f.fs.Duration("timeout", 180*time.Second, "boot timeout")
	if err := f.parse(args); err != nil {
		return err
	}
	serial, err := a.plat.Boot(BootOpts{Name: *avd, Create: *create, Timeout: int(timeout.Seconds())})
	if err != nil {
		return err
	}
	return a.emit(*f.json, map[string]string{"serial": serial}, "booted "+serial)
}

func (a *app) connect(args []string) error {
	f := a.flags("connect")
	device := f.fs.Bool("device", false, "target is a device endpoint (adb connect), not an adb server")
	serial := f.fs.String("serial", "", "pick this device serial")
	if err := f.parse(args); err != nil {
		return err
	}
	if len(f.pos) > 1 || (len(f.pos) == 0 && *serial == "") {
		return errUsage
	}
	target := ""
	if len(f.pos) == 1 {
		target = f.pos[0]
	}
	if err := a.plat.Connect(target, *device, *serial); err != nil {
		return err
	}
	return a.devices([]string{jsonArg(*f.json)})
}

func jsonArg(on bool) string { return "--json=" + strconv.FormatBool(on) }

func (a *app) devices(args []string) error {
	f := a.flags("devices")
	if err := f.parse(args); err != nil {
		return err
	}
	devs, err := a.plat.Devices()
	if err != nil {
		return err
	}
	var lines []string
	for _, d := range devs {
		lines = append(lines, d.Serial+"\t"+d.State+"\t"+d.Info)
	}
	return a.emit(*f.json, devs, strings.Join(lines, "\n"))
}

func (a *app) ok(f *flags, msg string, extra map[string]string) error {
	if extra == nil {
		extra = map[string]string{}
	}
	extra["ok"] = "true"
	return a.emit(*f.json, extra, msg)
}

func (a *app) install(args []string) error {
	f := a.flags("install")
	grant := f.fs.Bool("grant", false, "grant all runtime permissions (-g)")
	if err := f.parse(args); err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return errUsage
	}
	if err := a.plat.Install(f.pos[0], *grant); err != nil {
		return err
	}
	return a.ok(f, "installed "+f.pos[0], nil)
}

func (a *app) launch(args []string) error {
	f := a.flags("launch")
	if err := f.parse(args); err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return errUsage
	}
	comp, err := a.plat.Launch(f.pos[0])
	if err != nil {
		return err
	}
	return a.ok(f, "launched "+comp, map[string]string{"component": comp})
}

func (a *app) onePackage(name string, do func(string) error, verb string, args []string) error {
	f := a.flags(name)
	if err := f.parse(args); err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return errUsage
	}
	if err := do(f.pos[0]); err != nil {
		return err
	}
	return a.ok(f, verb+" "+f.pos[0], nil)
}

func (a *app) stop(args []string) error  { return a.onePackage("stop", a.plat.Stop, "stopped", args) }
func (a *app) clear(args []string) error { return a.onePackage("clear", a.plat.Clear, "cleared", args) }

func (a *app) grant(args []string) error {
	f := a.flags("grant")
	if err := f.parse(args); err != nil {
		return err
	}
	if len(f.pos) < 2 {
		return errUsage
	}
	if err := a.plat.Grant(f.pos[0], f.pos[1:]); err != nil {
		return err
	}
	return a.ok(f, "granted "+strings.Join(f.pos[1:], " "), nil)
}

func (a *app) shell(args []string) error {
	f := a.flags("shell")
	if err := f.parse(args); err != nil {
		return err
	}
	if len(f.pos) == 0 {
		return errUsage
	}
	out, err := a.plat.Shell(strings.Join(f.pos, " "))
	if err != nil {
		return err
	}
	return a.emit(*f.json, map[string]string{"output": out}, strings.TrimRight(out, "\n"))
}

type nodeView struct {
	Node
	Center [2]int `json:"center"`
}

func (a *app) ui(args []string) error {
	f := a.flags("ui")
	f.driverFlag(a.getenv)
	all := f.fs.Bool("all", false, "include unlabeled, non-clickable nodes")
	if err := f.parse(args); err != nil {
		return err
	}
	d, err := a.driver(f)
	if err != nil {
		return err
	}
	nodes, err := d.UI()
	if err != nil {
		return err
	}
	var views []nodeView
	var lines []string
	for _, n := range nodes {
		if !*all && !n.Interesting() {
			continue
		}
		x, y := n.Center()
		views = append(views, nodeView{n, [2]int{x, y}})
		lines = append(lines, fmt.Sprintf("(%d,%d) [%d,%d][%d,%d] %s text=%q desc=%q id=%q clickable=%v%s",
			x, y, n.Left, n.Top, n.Right, n.Bottom, n.Class, n.Text, n.Desc, n.ID, n.Clickable, stateFlags(n)))
	}
	return a.emit(*f.json, views, strings.Join(lines, "\n"))
}

// stateFlags lists only the set states, e.g. " focused password"; empty when plain.
func stateFlags(n Node) string {
	var out string
	for _, s := range []struct {
		on   bool
		name string
	}{{n.Focused, "focused"}, {n.Selected, "selected"}, {n.Checked, "checked"}, {n.Password, "password"}, {!n.Enabled, "disabled"}} {
		if s.on {
			out += " " + s.name
		}
	}
	return out
}

func atoiAll(ss []string) ([]int, error) {
	out := make([]int, len(ss))
	for i, s := range ss {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("not a number: %q", s)
		}
		out[i] = n
	}
	return out, nil
}

// settle is the pause before re-reading the UI after an input.
var settle = 400 * time.Millisecond

// keyboardSettle is the longer pause after typing; some IMEs (MIUI) lag behind.
var keyboardSettle = 600 * time.Millisecond

func imeShown(d Driver) bool {
	out, err := d.Shell("dumpsys input_method | grep mInputShown")
	return err == nil && strings.Contains(out, "mInputShown=true")
}

func screenHash(nodes []Node) uint64 {
	h := fnv.New64a()
	for _, n := range nodes {
		fmt.Fprintf(h, "%+v\n", n)
	}
	return h.Sum64()
}

type tapVerify struct {
	Changed     bool   `json:"changed"`
	NodesBefore int    `json:"nodes_before"`
	NodesAfter  int    `json:"nodes_after"`
	Focused     string `json:"focused"`
}

type tapResult struct {
	X, Y int
	*tapVerify
}

func (r tapResult) MarshalJSON() ([]byte, error) {
	m := map[string]any{"x": r.X, "y": r.Y}
	if v := r.tapVerify; v != nil {
		m["changed"], m["nodes_before"], m["nodes_after"], m["focused"] = v.Changed, v.NodesBefore, v.NodesAfter, v.Focused
	}
	return json.Marshal(m)
}

func (a *app) tap(args []string) error {
	f := a.flags("tap")
	f.driverFlag(a.getenv)
	text := f.fs.String("text", "", "tap the node whose text or content-desc matches")
	desc := f.fs.String("desc", "", "tap the node whose content-desc or text matches")
	exact := f.fs.Bool("exact", false, "no substring matching")
	noVerify := f.fs.Bool("no-verify", false, "skip the post-tap ui dump and change report")
	if err := f.parse(args); err != nil {
		return err
	}
	d, err := a.driver(f)
	if err != nil {
		return err
	}
	var x, y int
	var before []Node
	switch {
	case *text != "" || *desc != "":
		query := *text
		if *desc != "" {
			query = *desc
		}
		var n Node
		if n, before, err = a.findNode(d, query, *desc != "", *exact); err != nil {
			return err
		}
		x, y = n.Center()
	case len(f.pos) == 2:
		xy, err := atoiAll(f.pos)
		if err != nil {
			return err
		}
		x, y = xy[0], xy[1]
		if !*noVerify {
			before, _ = d.UI()
		}
	default:
		return errUsage
	}
	if err := d.Tap(x, y); err != nil {
		return err
	}
	res := tapResult{X: x, Y: y}
	msg := fmt.Sprintf("tapped %d %d", x, y)
	if !*noVerify {
		time.Sleep(settle)
		after, err := d.UI()
		if err != nil {
			fmt.Fprintf(a.err, "capture sim tap: verify failed: %v\n", err)
		} else {
			v := &tapVerify{Changed: screenHash(before) != screenHash(after), NodesBefore: len(before), NodesAfter: len(after)}
			if n, ok := Focused(after); ok {
				v.Focused = n.Text
			}
			res.tapVerify = v
			msg += fmt.Sprintf("; nodes %d -> %d; screen changed: %s", v.NodesBefore, v.NodesAfter, yesNo(v.Changed))
			if v.Focused != "" {
				msg += fmt.Sprintf("; focused text: %q", v.Focused)
			}
		}
	}
	return a.emit(*f.json, res, msg)
}

func yesNo(b bool) string { return map[bool]string{false: "no", true: "yes"}[b] }

// findNode returns the match plus the full node list it was found in.
func (a *app) findNode(d Driver, query string, byDesc, exact bool) (Node, []Node, error) {
	nodes, err := d.UI()
	if err != nil {
		return Node{}, nil, err
	}
	n, ok := FindNode(nodes, query, byDesc, exact)
	if !ok {
		return n, nodes, fmt.Errorf("no node with text or desc matching %q", query)
	}
	return n, nodes, nil
}

// clearFocused deletes the focused field's content (END, then one DEL per character;
// 40 when the a11y text is empty, as Flutter may not expose it).
func (a *app) clearFocused(d Driver) error {
	nodes, err := d.UI()
	if err != nil {
		return err
	}
	n, ok := Focused(nodes)
	if !ok {
		return errors.New("no focused field (tap one first)")
	}
	dels := len([]rune(n.Text))
	if dels == 0 {
		dels = 40
	}
	keys := "input keyevent KEYCODE_MOVE_END" + strings.Repeat(" KEYCODE_DEL", dels)
	_, err = d.Shell(keys)
	return err
}

func (a *app) clearField(args []string) error {
	if a.plat.Name() == "ios" {
		return errors.New("clear-field is unsupported on iOS")
	}
	f := a.flags("clear-field")
	f.driverFlag(a.getenv)
	if err := f.parse(args); err != nil {
		return err
	}
	d, err := a.driver(f)
	if err != nil {
		return err
	}
	if err := a.clearFocused(d); err != nil {
		return err
	}
	time.Sleep(settle)
	field, err := a.focusedText(d)
	if err != nil {
		return err
	}
	if field != "" {
		fmt.Fprintf(a.err, "capture sim clear-field: field still shows %q\n", field)
	}
	return a.emit(*f.json, map[string]any{"ok": true, "field": field}, fmt.Sprintf("cleared; field shows %q", field))
}

func (a *app) focusedNode(d Driver) (Node, error) {
	nodes, err := d.UI()
	if err != nil {
		return Node{}, err
	}
	n, _ := Focused(nodes)
	return n, nil
}

func (a *app) focusedText(d Driver) (string, error) {
	n, err := a.focusedNode(d)
	return n.Text, err
}

// waitFocus polls until an EditText has focus and the IME is shown, else errors.
func (a *app) waitFocus(d Driver) error {
	for i := 0; i < 5; i++ {
		if nodes, err := d.UI(); err == nil {
			if n, ok := Focused(nodes); ok && strings.Contains(n.Class, "EditText") && imeShown(d) {
				return nil
			}
		}
		time.Sleep(settle)
	}
	return errors.New("field never got focus with the keyboard shown (tap it again, check 'ui')")
}

// reconcile compares the field with baseline+typed. It returns the missing suffix to
// type or the number of stray trailing chars to delete; ok=false when the field
// diverged. Masked fields compare length only.
func reconcile(field, baseline Node, typed string) (add string, extra int, ok bool) {
	if field.Password {
		n := len([]rune(field.Text)) - len([]rune(baseline.Text))
		switch {
		case n < 0:
			return "", 0, false
		case n > len(typed):
			return "", n - len(typed), true
		}
		return typed[n:], 0, true
	}
	want := baseline.Text + typed
	switch {
	case strings.HasPrefix(want, field.Text):
		return want[len(field.Text):], 0, true
	case strings.HasPrefix(field.Text, want):
		return "", len(field.Text) - len(want), true
	}
	return "", 0, false
}

func (a *app) typeText(args []string) error {
	f := a.flags("type")
	f.driverFlag(a.getenv)
	into := f.fs.Bool("into", false, "tap the node given by --text/--desc first")
	text := f.fs.String("text", "", "with --into: node text or content-desc")
	desc := f.fs.String("desc", "", "with --into: node content-desc or text")
	exact := f.fs.Bool("exact", false, "with --into: no substring matching")
	clearField := f.fs.Bool("clear", false, "clear the field before typing")
	if err := f.parse(args); err != nil {
		return err
	}
	if len(f.pos) == 0 || *into != (*text != "" || *desc != "") {
		return errUsage
	}
	d, err := a.driver(f)
	if err != nil {
		return err
	}
	if *into {
		query := *text
		if *desc != "" {
			query = *desc
		}
		n, nodes, err := a.findNode(d, query, *desc != "", *exact)
		if err != nil {
			return err
		}
		if err := d.Tap(FieldFor(nodes, n).Center()); err != nil {
			return err
		}
	}
	if a.plat.Name() == "ios" {
		return a.typeIOS(d, f, *clearField, strings.Join(f.pos, " "))
	}
	if err := a.waitFocus(d); err != nil {
		return err
	}
	if *clearField {
		if err := a.clearFocused(d); err != nil {
			return err
		}
		time.Sleep(settle)
	}
	baseline, err := a.focusedNode(d)
	if err != nil {
		return err
	}
	typed := strings.Join(f.pos, " ")
	if err := d.Type(typed); err != nil {
		return err
	}
	// A mismatch must be seen twice in a row before correcting: the a11y tree lags the IME.
	var shown, last string
	for fixes, reads := 0, 0; ; reads++ {
		time.Sleep(keyboardSettle)
		field, err := a.focusedNode(d)
		if err != nil {
			return err
		}
		shown = field.Text
		add, extra, ok := reconcile(field, baseline, typed)
		if ok && add == "" && extra == 0 {
			break
		}
		if !ok || reads >= 8 || fixes == 3 {
			return fmt.Errorf("typed %q onto %q but field shows %q", typed, baseline.Text, shown)
		}
		if shown != last {
			last = shown
			continue
		}
		fixes++
		last = ""
		if extra > 0 {
			_, err = d.Shell("input keyevent" + strings.Repeat(" KEYCODE_DEL", extra))
		} else {
			err = d.Type(add)
		}
		if err != nil {
			return err
		}
	}
	return a.ok(f, fmt.Sprintf("typed; field shows %q", shown), map[string]string{"field": shown})
}

// typeIOS types without the Android field verification (focus, IME and shell checks do not exist on iOS).
func (a *app) typeIOS(d Driver, f *flags, clearField bool, text string) error {
	if clearField {
		return errors.New("type --clear is unsupported on iOS")
	}
	time.Sleep(keyboardSettle)
	if err := d.Type(text); err != nil {
		return err
	}
	return a.ok(f, "typed", nil)
}

func (a *app) key(args []string) error {
	f := a.flags("key")
	f.driverFlag(a.getenv)
	if err := f.parse(args); err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return errUsage
	}
	d, err := a.driver(f)
	if err != nil {
		return err
	}
	if err := d.Key(f.pos[0]); err != nil {
		return err
	}
	return a.ok(f, "key "+f.pos[0], nil)
}

func (a *app) back(args []string) error {
	f := a.flags("back")
	f.driverFlag(a.getenv)
	if err := f.parse(args); err != nil {
		return err
	}
	d, err := a.driver(f)
	if err != nil {
		return err
	}
	shown := imeShown(d)
	if err := d.Back(); err != nil {
		return err
	}
	if !shown {
		return a.ok(f, "back", nil)
	}
	for i := 0; i < 5; i++ {
		time.Sleep(settle)
		if !imeShown(d) {
			return a.ok(f, "back (keyboard hidden)", nil)
		}
	}
	return errors.New("back: keyboard still shown")
}

func (a *app) swipe(args []string) error {
	f := a.flags("swipe")
	f.driverFlag(a.getenv)
	if err := f.parse(args); err != nil {
		return err
	}
	d, err := a.driver(f)
	if err != nil {
		return err
	}
	var x1, y1, x2, y2, ms int
	ms = 300
	switch len(f.pos) {
	case 1:
		w, h, err := d.ScreenSize()
		if err != nil {
			return err
		}
		if x1, y1, x2, y2, err = directionSwipe(f.pos[0], w, h); err != nil {
			return err
		}
	case 4, 5:
		n, err := atoiAll(f.pos)
		if err != nil {
			return err
		}
		x1, y1, x2, y2 = n[0], n[1], n[2], n[3]
		if len(n) == 5 {
			ms = n[4]
		}
	default:
		return errUsage
	}
	if err := d.Swipe(x1, y1, x2, y2, ms); err != nil {
		return err
	}
	return a.ok(f, fmt.Sprintf("swiped %d,%d -> %d,%d", x1, y1, x2, y2), nil)
}

// directionSwipe returns a finger path across the middle 40% of the screen.
func directionSwipe(dir string, w, h int) (x1, y1, x2, y2 int, err error) {
	cx, cy := w/2, h/2
	switch dir {
	case "up":
		return cx, h * 7 / 10, cx, h * 3 / 10, nil
	case "down":
		return cx, h * 3 / 10, cx, h * 7 / 10, nil
	case "left":
		return w * 7 / 10, cy, w * 3 / 10, cy, nil
	case "right":
		return w * 3 / 10, cy, w * 7 / 10, cy, nil
	}
	return 0, 0, 0, 0, fmt.Errorf("direction must be up|down|left|right, got %q", dir)
}

func (a *app) wait(args []string) error {
	f := a.flags("wait")
	f.driverFlag(a.getenv)
	text := f.fs.String("text", "", "wait for a node with this text or content-desc")
	desc := f.fs.String("desc", "", "wait for a node with this content-desc or text")
	exact := f.fs.Bool("exact", false, "no substring matching")
	timeout := f.fs.Duration("timeout", 20*time.Second, "give up after")
	if err := f.parse(args); err != nil {
		return err
	}
	if (*text == "") == (*desc == "") {
		return errUsage
	}
	query := *text + *desc
	d, err := a.driver(f)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(*timeout)
	for {
		nodes, err := d.UI()
		if err == nil {
			if n, ok := FindNode(nodes, query, *desc != "", *exact); ok {
				x, y := n.Center()
				return a.emit(*f.json, nodeView{n, [2]int{x, y}}, fmt.Sprintf("found %q at %d,%d", query, x, y))
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for %q", *timeout, query)
		}
		time.Sleep(time.Second)
	}
}

func (a *app) screenshot(args []string) error {
	f := a.flags("screenshot")
	f.driverFlag(a.getenv)
	if err := f.parse(args); err != nil {
		return err
	}
	if len(f.pos) != 1 {
		return errUsage
	}
	d, err := a.driver(f)
	if err != nil {
		return err
	}
	png, err := d.Screenshot()
	if err != nil {
		return err
	}
	if err := os.WriteFile(f.pos[0], png, 0o644); err != nil {
		return err
	}
	return a.emit(*f.json, map[string]any{"path": f.pos[0], "bytes": len(png)}, "saved "+f.pos[0])
}

func (a *app) screenSize(args []string) error {
	f := a.flags("screen-size")
	f.driverFlag(a.getenv)
	if err := f.parse(args); err != nil {
		return err
	}
	d, err := a.driver(f)
	if err != nil {
		return err
	}
	w, h, err := d.ScreenSize()
	if err != nil {
		return err
	}
	return a.emit(*f.json, map[string]int{"width": w, "height": h}, fmt.Sprintf("%dx%d", w, h))
}

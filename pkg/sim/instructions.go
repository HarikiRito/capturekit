package sim

// Instructions returns the agent-readable command reference.
func Instructions() string {
	return `capture sim - install, run and drive an Android or iOS app on an emulator/simulator

Platform: leading flag --platform android|ios (or $CAPTURE_SIM_PLATFORM), default android. Example: capture sim --platform ios boot
Flow: check -> boot|connect -> install -> launch -> ui/tap/type/screenshot -> clear
Every command takes --json (machine-readable). Exit 0 ok, 1 failure (stderr says why), 2 usage.

Setup
  check                              adb (required), Android SDK/emulator/avdmanager/phone-harness (optional, boot only)
  boot [--avd n] [--create]          headless native emulator, waits for boot (default avd "capturekit")
  connect <host:port>                use an adb SERVER (adb -H host -P port), saved in state file
  connect <host:port> --device       adb connect to a device endpoint instead
  connect --serial <s>               pick one device when several are attached
  devices                            list devices on the active adb server

App
  install <apk> [--grant]            adb install -r (-g with --grant)
  launch <pkg>[/activity]            resolves the launcher activity when none given
  stop <pkg> | clear <pkg>           force-stop | wipe app data
  grant <pkg> <perm>...              pm grant
  shell -- <cmd>                     raw device shell

iOS (--platform ios; needs Xcode, simulators only for the commands below)
  check                              xcrun/simctl (required), axe + phone-harness (optional)
  boot [--avd n] [--create]          reuse simulator named n (default "capturekit") else simctl create (iPad Air 11-inch M2), then bootstatus -b; saves its UDID
  connect --serial <udid>            pick a simulator; devices lists available ones
  install <App.app>                  simctl install; --grant is ignored, use grant
  launch <bundle id>                 simctl launch, prints "<bundle>: <pid>"
  stop <bundle id>                   simctl terminate
  clear <bundle id>                  simctl uninstall (no per-app data wipe exists): install again after
  grant <bundle id> <service>...     simctl privacy grant (photos, location, microphone, ...; see simctl privacy)
  shell, back                        unsupported
  Simulator UI/input run through AXe (brew install cameroncooke/axe/axe): ui = axe describe-ui (AXLabel -> desc,
  AXValue -> text; clickable is always false), tap/swipe use points (screen-size reports points, screenshots are pixels),
  type = US keyboard ASCII, key takes an HID usage code (40 = Return). Screenshot = simctl io screenshot.
  Real iPhone: connect <name> --device selects phone-harness over iPhone Mirroring (macOS 15+, iOS 18+, iPhone only).
  It supports screenshot, tap x y, type, swipe, key (press combo). No UI tree (ui and tap --text fail), no back, no shell,
  and boot/install/launch/stop/clear/grant are rejected: use phone-harness directly.
  --driver auto|axe|phoneharness on iOS: auto = axe for a simulator, phoneharness after connect --device.

Interact (all accept --driver auto|adb|phoneharness)
  ui [--all]                         visible nodes: center, bounds, text, desc, class, clickable,
                                     plus set states: focused selected checked password disabled
  tap <x> <y>                        tap point
  tap --text "Sign in"               tap center of node; --text and --desc both search text AND
  tap --desc "Sign in"               content-desc (exact beats substring, case-insensitive; the named
                                     field wins ties); --exact disables substring matching
                                     after a tap prints node count before/after, 'screen changed:
                                     yes/no' and focused text (--json: changed, focused); --no-verify skips
  type <text>                        into the focused field (ASCII only, no '%'); sent in chunks, checks the
                                     field afterwards, retypes the missing tail up to 2x, exit 1 if still short
  type --into --desc "Email" [--clear] <text>
                                     tap node (a label resolves to the input field below it), wait for EditText focus
                                     + keyboard shown (exit 1 if not), [clear], type, print the field's resulting text
  clear-field                        empty the focused field (END + one DEL per character, 40 if text unreadable)
  key <KEYCODE|name>                 e.g. ENTER, TAB, KEYCODE_HOME, 66
  back                               keyboard open: only hides it (exit 1 if it stays); else normal BACK
  swipe <x1> <y1> <x2> <y2> [ms]     or: swipe up|down|left|right (finger direction)
  wait --text T | --desc D [--exact] [--timeout 20s]   e.g. wait --desc "Sign in"
  screenshot <out.png>               then view the PNG
  screen-size

Tips
  Read 'ui' before tapping; Flutter buttons often report clickable=false but tap by text/desc works.
  Flutter exposes labels only as content-desc; --text and --desc both find them.
  Run 'ui' after every tap that may move layout: the keyboard shifts fields, so old coordinates go stale.
  Prefer 'type --into --desc X' over tap-then-type with remembered coordinates.
  'tap' says 'screen changed: no' when nothing happened (e.g. an already-open dropdown); re-check with 'ui'.
  Text input is ASCII only. Password fields show masked text; only the length is checked.
  Dropdown/menu items: tap by desc once the list is open.
  Flutter dropdowns: tapping a select opens a modal list whose rows may reuse the select's own label
  as content-desc. Tap the label twice (first opens, second picks) or tap coordinates from 'ui' after
  opening. 'tap' prints 'screen changed: yes' for both, so re-run 'ui' to see which state you are in.
  The selected value is NOT exposed in 'ui'; confirm a pick with 'screenshot'.
  'ui' hides nodes with no text, no desc and not clickable; 'ui --all' lists those too (layout containers).
  'back' on the login screen exits the app; relaunch with 'launch'.
  After an action verify with 'wait' or 'ui', never assume it landed.
  State file: $CAPTURE_SIM_STATE (default ~/.local/state/capturekit/sim.json).
  Driver: --driver or $CAPTURE_SIM_DRIVER; 'auto' is default: phone-harness when in PATH
  (else adb, one stderr line). ui/screenshot/screen-size retry once on adb if phone-harness fails; tap/type/swipe/key/back/shell
  never fall back (error says rerun with --driver adb). 'adb' or 'phoneharness' force one driver. Telemetry and update checks
  of phone-harness are disabled by this tool.
  adb: $ADB, else $ANDROID_HOME/platform-tools/adb, else PATH.
`
}

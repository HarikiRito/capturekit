package sim

// Instructions returns the agent-readable command reference.
func Instructions() string {
	return `capture sim - install, run and drive an Android app on an emulator (iOS: future)

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

Interact (all accept --driver adb|phoneharness)
  ui [--all]                         visible nodes: center, bounds, text, desc, class, clickable,
                                     plus set states: focused selected checked password disabled
  tap <x> <y>                        tap point
  tap --text "Sign in"               tap center of node; --text and --desc both search text AND
  tap --desc "Sign in"               content-desc (exact beats substring, case-insensitive; the named
                                     field wins ties); --exact disables substring matching
                                     after a tap prints node count before/after, 'screen changed:
                                     yes/no' and focused text (--json: changed, focused); --no-verify skips
  type <text>                        into the focused field (ASCII only)
  type --into --desc "Email" [--clear] <text>
                                     tap node (a label resolves to the input field below it), wait for focus, [clear], type, then print the field's
                                     resulting text (stderr warning when it differs from <text>)
  clear-field                        empty the focused field (END + one DEL per character)
  key <KEYCODE|name>                 e.g. ENTER, TAB, KEYCODE_HOME, 66
  back
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
  Driver: --driver or $CAPTURE_SIM_DRIVER; 'adb' is default. 'phoneharness' is experimental
  (needs the phone-harness CLI in PATH; telemetry and update checks are disabled by this tool).
  adb: $ADB, else $ANDROID_HOME/platform-tools/adb, else PATH.
`
}

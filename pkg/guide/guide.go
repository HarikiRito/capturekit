// Package guide prints the agent-facing overview and dependency setup text.
package guide

import (
	"fmt"
	"io"
)

// Run prints the text for args[0]: "setup" or "agent-guide".
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: capture setup | capture agent-guide")
		return 2
	}
	switch args[0] {
	case "setup":
		fmt.Fprint(stdout, setup)
	case "agent-guide":
		fmt.Fprint(stdout, Instructions())
	default:
		fmt.Fprintf(stderr, "unknown guide %q\n", args[0])
		return 2
	}
	return 0
}

// Instructions returns the overview for coding agents.
func Instructions() string { return agentGuide }

const agentGuide = `capture - screenshot toolkit for coding agents

Packages (independent, use only what you need)
  sim       drive an Android emulator/device or iOS simulator (--platform ios): install an app, tap, type, screenshot
  browser   drive a web page via agent-browser: navigate, click, screenshot
  annotate  draw colored boxes + labels on screenshots (ffmpeg)

Standard flow
  1. capture setup                  install missing dependencies (text instructions for you)
  2. capture sim ... | capture browser ...   take raw screenshots
  3. capture annotate ...           box + label the screenshots

iOS simulator (sim --platform ios; macOS + Xcode)
  capture sim --platform ios check              then: brew install cameroncooke/axe/axe
  capture sim --platform ios connect --serial <udid>   (udids: devices) or: boot --create
  install <App.app>, launch <bundle id>, then ui / tap / type / swipe / screenshot, stop, clear
  tap/swipe use points; screenshot is 2x pixels, never reuse its coordinates.
  When two nodes share a label (title + button) or a field value is not shown, tap coordinates from ui.

Rules
  - run "capture <pkg> instructions" before using a package: full commands, flags, schemas.
  - run "capture <pkg> check" to see whether its dependencies are present.
  - packages do not depend on each other; sim and browser only produce PNGs, annotate only consumes them.

Exit codes
  0 ok, 1 failure (stderr or JSON says why), 2 usage error (unknown command, bad flags).
`

const setup = `capture setup - dependency setup for a coding agent
The capture tool installs nothing by itself. You run the steps below, only for what is missing.

Agent rules
  - run each check first; install only what is missing.
  - after installing, re-run the same check to confirm.
  - never use sudo or system package installs (apt, brew) without asking the user first.
  - report anything still failing.

annotate  [required for annotate]
  purpose: ffmpeg draws boxes/labels; a TTF font renders label text.
  detect:  capture annotate check
  font:    default /usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf, or pass --font <ttf>
  install: Linux   apt install ffmpeg fonts-dejavu-core
           macOS   brew install ffmpeg; no DejaVu by default: pass --font <ttf> (or brew install --cask font-dejavu and use that path)
           Windows install ffmpeg and use --font C:\Windows\Fonts\arialbd.ttf
  verify:  capture annotate check

sim: adb  [required for sim]
  purpose: Android Debug Bridge, used by every sim command.
  detect:  capture sim check
  install: Linux   apt install adb
           macOS   brew install --cask android-platform-tools
           Windows download Android platform-tools, add to PATH
           or set ADB, or ANDROID_HOME (uses $ANDROID_HOME/platform-tools/adb)
  verify:  capture sim check   (adb must be ok)

sim: Android SDK emulator, avdmanager, system image  [optional, only for "capture sim boot"]
  purpose: boot a headless emulator. Not needed when you connect to an existing device/adb server.
  detect:  capture sim check   (android-sdk, emulator, avdmanager entries)
  install: install Android command-line tools (Android Studio, or brew install --cask android-commandlinetools),
           set ANDROID_HOME (or ANDROID_SDK_ROOT), then with sdkmanager install "emulator", "platform-tools"
           and a system image, e.g. "system-images;android-34;google_apis;x86_64" (arm64-v8a on Apple silicon).
           "capture sim boot --create" creates the AVD "capturekit" with avdmanager.
  verify:  capture sim check

sim: phone-harness  [optional, preferred driver when installed]
  purpose: default input driver when present (--driver auto); falls back to adb if missing or failing.
  detect:  capture sim check
  install: pipx install phone-harness   (or pip install phone-harness)
           set PHONE_HARNESS_BIN if it is not in PATH.
           disable telemetry and update checks in your own use:
           PHONE_HARNESS_TELEMETRY=0 DO_NOT_TRACK=1 PHONE_HARNESS_NO_UPDATE_CHECK=1
           (capture sim sets these itself)
  verify:  capture sim check

sim: iOS  [optional, only for "capture sim --platform ios"]
  purpose: simulator lifecycle via Xcode's simctl; UI tree + input via AXe; real iPhone via phone-harness.
  detect:  capture sim --platform ios check
  install: Xcode from the App Store (simctl: xcrun simctl help must work; macOS only)
           AXe, simulators only: brew install cameroncooke/axe/axe
           phone-harness, real iPhone only: see the phone-harness entry; needs macOS 15+, an iPhone on iOS 18+ and iPhone Mirroring.
  limits:  real iPhone = iPhone Mirroring only: screenshot, tap x y, type, swipe, key. No UI tree, no install, no back, no shell.
           select it with: capture sim --platform ios connect <name> --device
  verify:  capture sim --platform ios check

browser: agent-browser + node  [required for browser]
  purpose: agent-browser (npm package) drives Chrome; needs node and npm.
  detect:  capture browser check   (checks: binary, version, chrome; --json available)
  install: node and npm via your OS (Linux apt install nodejs npm; macOS brew install node; Windows nodejs.org)
           npm i -g agent-browser     (or brew install agent-browser)
           capture browser install --yes   downloads Chrome for Testing (never sudo)
           "capture browser install" without --yes only prints the steps.
           override binary: AGENT_BROWSER_BIN
  verify:  capture browser check

Order to run
  1. capture annotate check
  2. capture sim check   (and capture sim --platform ios check for iOS)
  3. capture browser check
Install what is red for the packages you need, re-run the same check, report what remains.
`

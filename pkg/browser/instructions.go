package browser

// Instructions returns the agent-readable reference for this package.
func Instructions() string {
	return `capture browser - drive a web page via agent-browser (https://github.com/vercel-labs/agent-browser, Apache-2.0)

Setup (once):
  capture browser check [--json]   binary + Chrome status (agent-browser doctor --offline --quick).
      Plain text by default. --json prints {"ok":bool,"checks":[{"name","status":"ok|fail","detail"}]}
      with checks binary, version, chrome; exit 1 if any failed.
  capture browser install     prints: npm i -g agent-browser (or brew), then agent-browser install
  capture browser install --yes   runs "agent-browser install" (Chrome for Testing download); never sudo
  Override binary: AGENT_BROWSER_BIN. Full upstream docs: capture browser skills get core

Usage:
  capture browser <agent-browser args>        shorthand, defaults injected
  capture browser run [--session n] [--cdp port|url] [--headed] [--viewport WxH] [--no-json] -- <args>
Defaults injected: --session capture, --json (skipped for --help/--version/skills or if you pass --json),
env AGENT_BROWSER_NO_VERSION_CHECK=1. stdout, stderr and exit code pass through unchanged.
--viewport WxH runs "set viewport W H" first, then your command.

Typical flow:
  capture browser open https://example.com
  capture browser set viewport 1280 800
  capture browser snapshot -i                  interactive elements only (links, buttons, inputs) with refs @e1, @e2
  capture browser snapshot                     full tree incl. non-interactive text and headings
  capture browser wait --load networkidle      wait for network to settle after open/navigation
  capture browser get box "#sel"               data {x,y,width,height} of one element
  capture browser click @e1 | fill @e2 "text" | press Enter | scroll down 500
  capture browser wait "#sel" | wait 1000 | wait --text "Done"
  capture browser eval "document.title"
  capture browser screenshot out.png           viewport only
  capture browser screenshot --full out.png    full page
  capture browser screenshot "#sel" out.png    one element (syntax: screenshot [selector] [path])
  capture browser screenshot --annotate out.png   numbered labels [N] = ref @eN on interactive elements in view
  capture browser close                        stop this session's daemon (close --all: every session)
Refs are invalidated by navigation/DOM change: re-run snapshot before using them.
Gotchas:
  - snapshot -i never lists headings or plain text; use plain snapshot to find them.
  - a page may have no heading at all: check the snapshot first; "get box h1" on a missing element
    fails with "Element not found".
  - get box returns viewport CSS pixels (relative to the current scroll position). For a
    screenshot --full add the scroll offset (eval "[scrollX,scrollY]"), or scroll to top first.
    Screenshot pixels = CSS px x deviceScaleFactor (see set viewport output).

Existing Chrome (CDP): start Chrome with --remote-debugging-port=9222, then
  capture browser run --cdp 9222 -- snapshot       or: capture browser connect 9222
Chrome must already have a loaded page; a blank tab can time out ("tab is not responding").
If connect fails, the daemon may fall back to launching its own headless Chrome: verify with
  capture browser get cdp-url

Sessions: each --session name is an isolated browser + background daemon that persists between
calls (cookies, tabs, page). "session list" lists them. Persist login state: --restore, --state file,
--profile. Always close when done.

JSON contract (probed on 0.41.2):
  success: {"success":true,"data":{...,"lifecycle":{...}},"error":null}
  failure: {"success":false,"data":null,"error":"<message>"}   (some add "code", e.g. page_wedged)
  open       data: url, title, targetId
  snapshot   data: snapshot (text tree), refs {"e1":{"name":"Learn more","role":"link"}}, origin
  eval       data: result, origin
  screenshot data: path; with --annotate also annotations [{number,ref,role,name,box:{x,y,width,height}}]
  set viewport data: width, height, deviceScaleFactor
  session list data: sessions ["capture"]
  Without --json the same commands print short text (e.g. "Screenshot saved to ...").
Exit codes: 0 success; 1 any failure (unknown ref "Unknown ref: e99", element not found, wait timeout
"Wait timed out after 1500ms", navigation error "net::ERR_...", bad Chrome path, unknown command).
Failures are reported on stdout as JSON (stderr stays empty) so parse stdout, then check the exit code.
Defaults: viewport 1280x720, scrollbars hidden. Linux Chrome has no CJK fonts: such text renders as boxes.
Network: only the requested pages; the upstream version check is disabled.

Annotating for docs: take plain screenshots here, then use "capture annotate" to draw boxes and labels
(see "capture annotate instructions"). Use snapshot refs or "get box <sel>" for coordinates:
  capture browser get box "#login"            -> data {"x":880,"y":640,"width":800,"height":120} (floats; round)
  capture annotate --in out.png --box 880,640,800,120 --label "Tap Sign in" --out step-01.png
For several elements, list each get box result under "boxes" in an annotate manifest.
`
}

// Package browser is a thin wrapper around the agent-browser CLI.
package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const (
	defaultBin     = "agent-browser"
	defaultSession = "capture"
)

// Exec abstracts running external programs so tests can fake them.
type Exec interface {
	LookPath(name string) (string, error)
	// Run returns the exit code; err is set only when the program could not start.
	Run(env []string, name string, args []string, stdout, stderr io.Writer) (int, error)
}

type osExec struct{}

func (osExec) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (osExec) Run(env []string, name string, args []string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	return 0, err
}

const usage = `usage: capture browser <command>

  check [--json]              is agent-browser installed, is Chrome available
  install [--yes]             print (or with --yes run) the Chrome download
  run [flags] -- <args...>    run agent-browser with defaults (flags: --session --cdp --headed --viewport WxH --no-json)
  instructions                full reference
  <anything else>             passed to agent-browser, e.g. open https://example.com
`

// Run executes the browser command and returns the exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	return run(osExec{}, args, stdout, stderr)
}

func run(ex Exec, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	case "instructions":
		fmt.Fprint(stdout, Instructions())
		return 0
	case "check":
		if len(args) == 2 && args[1] == "--json" {
			return checkJSON(ex, stdout)
		}
		return check(ex, stdout, stderr)
	case "install":
		return install(ex, args[1:], stdout, stderr)
	case "run":
		return runCmd(ex, args[1:], stdout, stderr)
	}
	return passthrough(ex, args, stdout, stderr)
}

func binary(ex Exec, stderr io.Writer) (string, bool) {
	bin := os.Getenv("AGENT_BROWSER_BIN")
	if bin == "" {
		bin = defaultBin
	}
	path, err := ex.LookPath(bin)
	if err != nil {
		fmt.Fprintf(stderr, "%s not found on PATH (set AGENT_BROWSER_BIN to override)\n%s", bin, installHints)
		return "", false
	}
	return path, true
}

const installHints = `install:
  npm i -g agent-browser     (or: brew install agent-browser)
  agent-browser install      (downloads Chrome for Testing; add --with-deps on Linux, needs root)
`

func check(ex Exec, stdout, stderr io.Writer) int {
	bin, ok := binary(ex, stderr)
	if !ok {
		return 1
	}
	fmt.Fprintf(stdout, "binary: %s\n", bin)
	env := []string{"AGENT_BROWSER_NO_VERSION_CHECK=1"}
	if code, err := ex.Run(env, bin, []string{"--version"}, stdout, stderr); err != nil || code != 0 {
		fmt.Fprintf(stderr, "%s --version failed\n", bin)
		return 1
	}
	code, err := ex.Run(env, bin, []string{"doctor", "--offline", "--quick"}, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if code != 0 {
		fmt.Fprint(stderr, "chrome or install problem, see above\n"+installHints)
	}
	return code
}

type checkResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// checkJSON prints {ok, checks:[{name,status,detail}]}; exit 1 if any check failed.
func checkJSON(ex Exec, stdout io.Writer) int {
	report := struct {
		OK     bool          `json:"ok"`
		Checks []checkResult `json:"checks"`
	}{Checks: []checkResult{}}
	add := func(name string, ok bool, detail string) {
		status := "ok"
		if !ok {
			status = "fail"
		}
		report.Checks = append(report.Checks, checkResult{name, status, strings.TrimSpace(detail)})
	}
	finish := func() int {
		report.OK = true
		for _, c := range report.Checks {
			report.OK = report.OK && c.Status == "ok"
		}
		_ = json.NewEncoder(stdout).Encode(report)
		if report.OK {
			return 0
		}
		return 1
	}

	var bin string
	if b := os.Getenv("AGENT_BROWSER_BIN"); b != "" {
		bin = b
	} else {
		bin = defaultBin
	}
	path, err := ex.LookPath(bin)
	if err != nil {
		add("binary", false, bin+" not found on PATH (set AGENT_BROWSER_BIN to override); "+strings.ReplaceAll(installHints, "\n", " "))
		return finish()
	}
	add("binary", true, path)
	env := []string{"AGENT_BROWSER_NO_VERSION_CHECK=1"}
	for _, step := range []struct {
		name string
		args []string
	}{{"version", []string{"--version"}}, {"chrome", []string{"doctor", "--offline", "--quick"}}} {
		var out bytes.Buffer
		code, err := ex.Run(env, path, step.args, &out, &out)
		if err != nil {
			add(step.name, false, err.Error())
		} else {
			add(step.name, code == 0, out.String())
		}
	}
	return finish()
}

func install(ex Exec, args []string, stdout, stderr io.Writer) int {
	yes := len(args) == 1 && args[0] == "--yes"
	if len(args) > 0 && !yes {
		fmt.Fprintln(stderr, "usage: capture browser install [--yes]")
		return 2
	}
	if !yes {
		fmt.Fprint(stdout, installHints)
		return 0
	}
	bin, ok := binary(ex, stderr)
	if !ok {
		return 1
	}
	code, err := ex.Run(nil, bin, []string{"install"}, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return code
}

func runCmd(ex Exec, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("browser run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	session := fs.String("session", "", "session name (default "+defaultSession+")")
	cdp := fs.String("cdp", "", "attach to Chrome via CDP port or url")
	headed := fs.Bool("headed", false, "show the browser window")
	viewport := fs.String("viewport", "", "viewport WxH, applied before the command")
	noJSON := fs.Bool("no-json", false, "plain text output instead of JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(stderr, "usage: capture browser run [flags] -- <agent-browser args...>")
		return 2
	}
	var global []string
	if *session != "" {
		global = append(global, "--session", *session)
	}
	if *cdp != "" {
		global = append(global, "--cdp", *cdp)
	}
	if *headed {
		global = append(global, "--headed")
	}
	bin, ok := binary(ex, stderr)
	if !ok {
		return 1
	}
	if *viewport != "" {
		w, h, ok := strings.Cut(*viewport, "x")
		if !ok {
			fmt.Fprintln(stderr, "--viewport must be WxH, e.g. 1280x800")
			return 2
		}
		if code := exec1(ex, bin, withDefaults(append(global, "set", "viewport", w, h), !*noJSON), stdout, stderr); code != 0 {
			return code
		}
	}
	return exec1(ex, bin, withDefaults(append(global, rest...), !*noJSON), stdout, stderr)
}

func passthrough(ex Exec, args []string, stdout, stderr io.Writer) int {
	bin, ok := binary(ex, stderr)
	if !ok {
		return 1
	}
	return exec1(ex, bin, withDefaults(args, true), stdout, stderr)
}

// withDefaults prepends --session and --json unless the caller already set them.
func withDefaults(args []string, json bool) []string {
	var pre []string
	if !has(args, "--session") {
		pre = append(pre, "--session", defaultSession)
	}
	if json && !has(args, "--json", "--help", "-h", "--version", "-V") && args[0] != "skills" {
		pre = append(pre, "--json")
	}
	return append(pre, args...)
}

func has(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f || strings.HasPrefix(a, f+"=") {
				return true
			}
		}
	}
	return false
}

func exec1(ex Exec, bin string, args []string, stdout, stderr io.Writer) int {
	code, err := ex.Run([]string{"AGENT_BROWSER_NO_VERSION_CHECK=1"}, bin, args, stdout, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return code
}

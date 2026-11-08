package main

import (
	"fmt"
	"io"
	"os"

	"github.com/HarikiRito/capturekit/pkg/annotate"
	"github.com/HarikiRito/capturekit/pkg/browser"
	"github.com/HarikiRito/capturekit/pkg/sim"
)

type pkg struct {
	run          func(args []string, stdout, stderr io.Writer) int
	instructions func() string
}

var pkgs = map[string]pkg{
	"sim":      {sim.Run, sim.Instructions},
	"browser":  {browser.Run, browser.Instructions},
	"annotate": {annotate.Run, annotate.Instructions},
}

const usage = `usage: capture <package> [args]

packages:
  sim       drive a simulator/emulator and take screenshots
  browser   drive a browser and take screenshots
  annotate  draw red box + label on screenshots

run "capture <package> instructions" for the full command reference
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	p, ok := pkgs[args[0]]
	if !ok {
		fmt.Fprintf(stderr, "unknown package %q\n\n%s", args[0], usage)
		return 2
	}
	if len(args) > 1 && args[1] == "instructions" {
		fmt.Fprint(stdout, p.instructions())
		return 0
	}
	return p.run(args[1:], stdout, stderr)
}

package sim

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Exec is the single seam for running external programs (faked in tests).
type Exec interface {
	// Run waits for the command and returns stdout; the error includes stderr.
	Run(env []string, stdin []byte, name string, args ...string) ([]byte, error)
	// Start launches a long-lived command detached, logging output to logPath.
	Start(logPath string, env []string, name string, args ...string) error
}

type osExec struct{}

func (osExec) Run(env []string, stdin []byte, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), env...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return out.Bytes(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
	}
	return out.Bytes(), nil
}

func (osExec) Start(logPath string, env []string, name string, args ...string) error {
	log, err := os.Create(logPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

package sim

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// State is the persisted active endpoint (adb server/serial, or iOS simulator UDID).
type State struct {
	Server string `json:"server,omitempty"` // adb server host:port (adb -H/-P)
	Serial string `json:"serial,omitempty"` // device serial (adb -s) or simulator UDID
	Device bool   `json:"device,omitempty"` // iOS: real iPhone via phone-harness
}

func defaultStatePath(getenv func(string) string) string {
	if p := getenv("CAPTURE_SIM_STATE"); p != "" {
		return p
	}
	dir := getenv("XDG_STATE_HOME")
	if dir == "" {
		dir = filepath.Join(getenv("HOME"), ".local", "state")
	}
	return filepath.Join(dir, "capturekit", "sim.json")
}

func loadState(path string) (State, error) {
	var s State
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

func saveState(path string, s State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

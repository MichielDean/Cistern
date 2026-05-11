// Package sessionlog provides a single source of truth for session log file
// paths and operations. The Castellarius liveness check, the CLI peek command,
// and the cataractae spawn logic all resolve session log paths through this
// package — no duplicated path strings, no inconsistent home-directory lookups.
package sessionlog

import (
	"os"
	"path/filepath"
	"time"
)

// LogDirFn returns the session log directory. Override in tests to control
// the log directory without creating real directories. Defaults to
// ~/.cistern/session-logs.
var LogDirFn = defaultLogDir

func defaultLogDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cistern", "session-logs"), nil
}

// Path returns the full path to the session log file for the given session ID.
// The session ID is typically "<repo>-<assignee>" (e.g. "myrepo-alpha").
func Path(sessionID string) (string, error) {
	dir, err := LogDirFn()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sessionID+".log"), nil
}

// MtimeFn is the function used by Mtime. Override in tests to control
// the returned mtime without creating real files.
var MtimeFn = defaultMtime

func defaultMtime(sessionID string) (time.Time, error) {
	p, err := Path(sessionID)
	if err != nil {
		return time.Time{}, err
	}
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return info.ModTime().UTC(), nil
}

// Mtime returns the modification time of the session log file.
// Used by the Castellarius liveness check to detect stalled agents:
// an active agent writes to its session log continuously via tmux pipe-pane,
// so a stale mtime indicates the agent is stuck or dead.
// Returns zero time if the log file does not exist.
func Mtime(sessionID string) (time.Time, error) {
	return MtimeFn(sessionID)
}

// Read returns the full contents of the session log file.
// Used by ct droplet peek --raw to display agent output without tmux.
// Returns an error if the file cannot be read. Callers should handle
// os.IsNotExist for missing logs.
func Read(sessionID string) ([]byte, error) {
	p, err := Path(sessionID)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

// EnsureDir creates the session log directory if it does not exist.
// Called by the cataractae spawn logic before setting up tmux pipe-pane.
func EnsureDir() error {
	dir, err := LogDirFn()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o750)
}
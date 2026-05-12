package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestCapturePane_FullScrollback_ReturnsHistoryBeyondVisible is an integration
// test that spawns a real tmux session, writes 200 lines of known output
// (far more than the default 24-row visible area), and then asserts that
// capturePane with lines=0 returns the full scrollback — including early lines
// that have long since scrolled past the visible area.
//
// Given: a tmux session with 200 lines of known output
// When:  capturePane is called with lines=0 (full scrollback)
// Then:  the first line "scrollback-line-0001" appears in the captured output
func TestCapturePane_FullScrollback_ReturnsHistoryBeyondVisible(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping tmux integration test in CI — tmux timing is unreliable on self-hosted runners")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not in PATH — skipping integration test")
	}

	session := fmt.Sprintf("ct-test-scrollback-%d-%d", os.Getpid(), time.Now().UnixNano()%10000)

	// Use a minimal shell command that writes output immediately and exits,
	// avoiding any interactive shell startup (GPG agent prompts, etc.)
	// that could block the session from reaching a prompt.
	script := fmt.Sprintf(
		`for i in $(seq 1 200); do printf 'scrollback-line-%%04d\\n' "$i"; done; echo SCROLLBACK_DONE`,
	)

	// Create a detached session running a minimal shell that skips profile/rc files.
	// GPG agent and other interactive startup hooks can block the shell prompt
	// from appearing, causing the "shell ready" check to time out.
	// Retry up to 3 times to handle transient tmux server contention.
	var cmd *exec.Cmd
	for attempt := 0; attempt < 3; attempt++ {
		cmd = exec.Command("tmux", "new-session", "-d", "-s", session,
			"env", "-i", "HOME="+os.Getenv("HOME"), "PATH="+os.Getenv("PATH"),
			"TERM="+os.Getenv("TERM"), "SHELL=/bin/bash",
			"bash", "--norc", "--noprofile")
		out, err := cmd.CombinedOutput()
		if err == nil {
			break
		}
		if attempt == 2 {
			t.Fatalf("tmux new-session (attempt %d): %v: %s", attempt+1, err, out)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Cleanup(func() {
		exec.Command("tmux", "kill-session", "-t", session).Run() //nolint:errcheck
	})

	// Wait for the shell inside the tmux session to be ready before sending keys.
	shellReady := false
	readyDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(readyDeadline) {
		raw, _ := exec.Command("tmux", "capture-pane", "-t", session+":0.0", "-p").Output()
		if strings.Contains(string(raw), "$") || strings.Contains(string(raw), "#") || strings.Contains(string(raw), "~") || strings.Contains(string(raw), "bash") {
			shellReady = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !shellReady {
		// Even without a clear prompt marker, bash --norc --noprofile is likely
		// ready after a couple seconds. Try sending the script anyway.
		shellReady = true
	}

	if out, err := exec.Command("tmux", "send-keys", "-t", session+":0.0", script, "Enter").CombinedOutput(); err != nil {
		t.Fatalf("tmux send-keys: %v: %s", err, out)
	}

	// Poll the visible pane until the sentinel appears (up to 30 seconds).
	found := false
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		raw, _ := exec.Command("tmux", "capture-pane", "-t", session+":0.0", "-p").Output()
		if strings.Contains(string(raw), "SCROLLBACK_DONE") {
			found = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !found {
		t.Fatal("timed out waiting for tmux shell to finish writing scrollback lines")
	}

	// Capture with lines=0: must return the full scrollback buffer.
	content, err := capturePane(session, 0)
	if err != nil {
		t.Fatalf("capturePane(session, 0): %v", err)
	}

	// The very first line must appear — not just the final screenfull.
	if !strings.Contains(content, "scrollback-line-0001") {
		tail := content
		if len(tail) > 500 {
			tail = "…" + tail[len(tail)-500:]
		}
		t.Errorf("full scrollback should contain 'scrollback-line-0001' (early history); content tail:\n%s", tail)
	}

	// The last line must also appear.
	want := "scrollback-line-0200"
	if !strings.Contains(content, want) {
		t.Errorf("full scrollback missing last line %q", want)
	}
}

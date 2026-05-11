// fakeagent is a minimal fake agent binary used in tests to exercise the
// Cistern session spawn → isAlive → outcome pipeline without a real LLM CLI.
//
// It accepts the same flags as the opencode CLI:
//
//	--dangerously-skip-permissions (ignored)
//	--model <model>                (ignored)
//	--agent <name>                 (identity name, triggers RESPONSE.md write)
//	--format json                  (when present, triggers non-interactive mode)
//	--output-format <format>       (legacy: also triggers non-interactive mode)
//	--resume <session-id>          (ignored; accepted for flag compatibility)
//
// Non-interactive mode (when --format or --output-format is present in os.Args):
//
//	When --format or --output-format is present, the agent prints a JSON envelope
//	containing a hardcoded proposal array and a test session_id. This is the
//	behaviour expected by callFilterAgent() in filter.go.
//
//	When FAKEAGENT_MODE=raw_fallback is set, prints the hardcoded proposal array
//	directly. This exercises the JSON-fallback path in callFilterAgent().
//
//	We scan os.Args directly because flag.Parse stops at the first positional
//	arg (e.g. a subcommand like "run"), which would otherwise prevent
//	--format/--output-format from being parsed when it appears after the
//	subcommand.
//
// Interactive mode (when --format and --output-format are both absent):
//
//	Environment variables read:
//	  CT_CATARACTA_NAME   identity passed by the session runner (ignored)
//
//	When --agent filter is present (filter mode), the agent writes its output
//	to RESPONSE.md in the current directory instead of printing to stdout.
//
//	When CONTEXT.md (in the current working directory) contains a line:
//	  ## Item: <droplet-id>
//	The binary sleeps 200 ms to simulate work, then calls:
//	  ct droplet pass <id> --notes 'fakeagent: ok'
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// hardcodedProposals is the raw text output for FAKEAGENT_MODE=raw_fallback,
// exercising the non-JSON fallback path in callFilterAgent where stdout
// becomes filterSessionResult.Text directly.
const hardcodedProposals = `[{"title":"mock proposal","description":"test description","depends_on":[]}]`

// hardcodedJSONEnvelope is returned when --format or --output-format is present in
// non-interactive mode. The result field becomes filterSessionResult.Text;
// session_id is a stable test value used to verify session_id extraction.
const hardcodedJSONEnvelope = `{"type":"result","subtype":"success","is_error":false,"result":"[{\"title\":\"mock proposal\",\"description\":\"test description\",\"depends_on\":[]}]","session_id":"test-session-id-abc123"}`

// hardcodedErrorEnvelope is returned in FAKEAGENT_MODE=error_envelope.
// is_error is true so callFilterAgent returns an error for the envelope.IsError path.
const hardcodedErrorEnvelope = `{"type":"result","subtype":"error","is_error":true,"result":"agent encountered an error","session_id":"error-session-id"}`

func main() {
	// Pre-scan os.Args for flags before calling flag.Parse.
	hasOutputFormat := false
	hasAgentFilter := false
	for _, arg := range os.Args[1:] {
		if arg == "--output-format" || arg == "--format" {
			hasOutputFormat = true
		}
		if arg == "filter" {
			// Check if previous arg was --agent
			for i, a := range os.Args[1:] {
				if a == "--agent" && i+1 < len(os.Args[1:]) && os.Args[1:][i+1] == "filter" {
					hasAgentFilter = true
				}
			}
		}
	}

	if hasOutputFormat {
		// Capture all args for tests that need to inspect which flags were passed.
		if argsFile := os.Getenv("FAKEAGENT_ARGS_FILE"); argsFile != "" {
			_ = os.WriteFile(argsFile, []byte(strings.Join(os.Args[1:], "\n")), 0o644)
		}
		// Capture the prompt for tests that need to inspect what was sent.
		if promptFile := os.Getenv("FAKEAGENT_PROMPT_FILE"); promptFile != "" {
			if len(os.Args) > 1 {
				_ = os.WriteFile(promptFile, []byte(os.Args[len(os.Args)-1]), 0o644)
			}
		}
		mode := os.Getenv("FAKEAGENT_MODE")
		switch {
		case mode == "error_envelope":
			fmt.Println(hardcodedErrorEnvelope)
		case mode != "raw_fallback":
			fmt.Println(hardcodedJSONEnvelope)
		default:
			fmt.Println(hardcodedProposals)
		}
		return
	}

	// Accept flags so flag.Parse does not reject them.
	flag.Bool("dangerously-skip-permissions", false, "")
	flag.String("model", "", "")
	flag.String("p", "", "")
	flag.String("format", "", "")
	flag.String("output-format", "", "")
	flag.String("resume", "", "")
	flag.String("s", "", "")
	flag.String("agent", "", "")
	flag.Parse()

	mode := os.Getenv("FAKEAGENT_MODE")

	// Interactive mode: optionally dump environment variables for env-hygiene integration tests.
	if mode == "env_dump" {
		fmt.Println("=== FAKEAGENT ENV DUMP ===")
		for _, e := range os.Environ() {
			fmt.Println(e)
		}
		fmt.Println("=== END ENV DUMP ===")
	}

	// Filter mode: write RESPONSE.md for tmux-based filter tests.
	// This is the path used by filterAgentTmux — the agent writes its response
	// to RESPONSE.md instead of producing output on stdout.
	if hasAgentFilter {
		responseContent := `1. Add user authentication

   Title: Add user authentication
   Description: Implement JWT-based authentication with refresh tokens
   Dependencies: none
   Complexity: standard

2. Set up role-based access control

   Title: Set up role-based access control
   Description: Define roles and permissions for admin, editor, and viewer
   Dependencies: droplet 1
   Complexity: standard

Ready to file
`
		_ = os.WriteFile("RESPONSE.md", []byte(responseContent), 0o644)
		return
	}

	// Interactive mode: read CONTEXT.md from the working directory to find the droplet ID.
	data, err := os.ReadFile("CONTEXT.md")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fakeagent: cannot read CONTEXT.md: %v\n", err)
		os.Exit(1)
	}

	re := regexp.MustCompile(`(?m)^##\s+Item:\s+(\S+)`)
	m := re.FindSubmatch(data)
	if m == nil {
		fmt.Fprintln(os.Stderr, "fakeagent: cannot find '## Item: <id>' in CONTEXT.md")
		os.Exit(1)
	}
	dropletID := string(m[1])

	// no_signal mode: exit without signaling — used for dead-session recovery tests.
	if mode == "no_signal" {
		os.Exit(0)
	}

	// Simulate work.
	time.Sleep(200 * time.Millisecond)

	// Signal outcome via ct.
	ctBin := "ct"
	if v := os.Getenv("CT_BIN"); v != "" {
		ctBin = v
	}
	cmd := exec.Command(ctBin, "droplet", "pass", dropletID, "--notes", "fakeagent: ok")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fakeagent: ct droplet pass %s: %v\n", dropletID, err)
		os.Exit(1)
	}
}

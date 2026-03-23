// fakeagent is a minimal fake agent binary used in tests to exercise the
// Cistern session spawn → isAlive → outcome pipeline without a real LLM CLI.
//
// It accepts the same flags as the claude CLI:
//
//	--dangerously-skip-permissions (ignored)
//	--add-dir <dir>                (ignored)
//	--model <model>                (ignored)
//	--print                        (triggers non-interactive mode)
//	-p <prompt>                    (interactive prompt, also used with --print)
//
// Non-interactive mode (when --print is present in os.Args):
//
//	Prints a hardcoded valid JSON proposal array to stdout and exits 0.
//	This is the behaviour expected by runNonInteractive() in refine.go.
//	We scan os.Args directly because flag.Parse stops at the first positional
//	arg (e.g. a subcommand like "exec"), which would otherwise prevent --print
//	from being parsed when it appears after the subcommand.
//
// Interactive mode (when --print is absent):
//
//	Environment variables read:
//	  CT_CATARACTA_NAME   identity passed by the session runner (ignored)
//
//	CONTEXT.md (in the current working directory) must contain a line:
//	  ## Item: <droplet-id>
//
//	The binary sleeps 200 ms to simulate work, then calls:
//	  ct droplet pass <id> --notes 'fakeagent: ok'
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"
)

// hardcodedProposals is the JSON array printed in non-interactive mode.
// Its content must match mockllm.HardcodedProposalsJSON so that tests can
// verify the full round-trip without importing the mockllm package.
const hardcodedProposals = `[{"title":"mock proposal","description":"test description","complexity":"standard","depends_on":[]}]`

func main() {
	// Pre-scan os.Args for --print before calling flag.Parse.
	// flag.Parse stops at the first positional arg (e.g. a subcommand such as
	// "exec" or "run"), so --print could appear later in the arg list without
	// being registered by the flag package.
	for _, arg := range os.Args[1:] {
		if arg == "--print" {
			fmt.Println(hardcodedProposals)
			return
		}
	}

	// Accept the same flags as claude so the command string built by
	// buildClaudeCmd() / buildPresetCmd() is valid when the binary is
	// substituted via CLAUDE_PATH. Return values are discarded — the
	// flags exist only so flag.Parse does not reject them.
	flag.Bool("dangerously-skip-permissions", false, "")
	flag.String("add-dir", "", "")
	flag.String("model", "", "")
	flag.Bool("print", false, "")
	flag.String("p", "", "")
	flag.Parse()

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

	// Simulate work.
	time.Sleep(200 * time.Millisecond)

	// Signal outcome via ct.
	cmd := exec.Command("ct", "droplet", "pass", dropletID, "--notes", "fakeagent: ok")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "fakeagent: ct droplet pass %s: %v\n", dropletID, err)
		os.Exit(1)
	}
}

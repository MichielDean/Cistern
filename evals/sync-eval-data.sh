#!/usr/bin/env bash
# sync-eval-data.sh — extract live instruction prompts from Go sources into
# eval-data/. Single source of truth: const blocks in cmd/ct/*.go. If the
# prompt changes, this re-extracts; evals always grade what ships.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$DIR/.." && pwd)"
OUT="$DIR/eval-data"
mkdir -p "$OUT"

extract_go_string_block() {
    # $1 = source file, $3 = out path. Grabs `const NAME = ` + the ENTIRE
    # backtick block (greedy to the closing backtick) — the prompts contain
    # inline backticked code, so non-greedy would truncate mid-prompt.
    python3 - "$1" "$3" <<'PYEOF'
import re, sys
path, out = sys.argv[1], sys.argv[2]
text = open(path).read()
m = re.search(r"const \w+ = `(.*?)`[,\]})]?\s*(\n//|\n\}|$)", text, re.S)
if not m:
    # fall back: greedy to last backtick in file
    m = re.search(r"const \w+ = `(.*)`", text, re.S)
if not m:
    sys.exit(f"no const backtick string found in {path}")
open(out, "w").write(m.group(1).strip())
PYEOF
}

extract_go_string_block "$REPO/cmd/ct/refine.go"  "filterSystemPrompt" "$OUT/filtration.txt"
extract_go_string_block "$REPO/cmd/ct/triage.go"  "triageSystemPrompt" "$OUT/triage.txt"
extract_go_string_block "$REPO/cmd/ct/wizard.go"  "wizardSystemPrompt" "$OUT/wizard.txt"
echo "synced: $(ls "$OUT" | tr '\n' ' ')"

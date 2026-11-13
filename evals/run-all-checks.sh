#!/usr/bin/env bash
# run-all-checks.sh — Cistern instruction-change eval gate.
#
# 1. Deterministic checks (fail fast, no LLM):
#    - sync-eval-data.sh extracted something for every entrypoint prompt
#    - aqueduct skill references ship in internal/skills/
#    - no retired default models in shipped configs
# 2. promptfoo configs, sequentially (LLM-graded; needs local ollama).
#
# Exit non-zero on any error or, per config, >2 rubric flakes.
set -u

DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$DIR/.." && pwd)"

echo "==> Deterministic checks"
bash "$DIR/sync-eval-data.sh" || exit 1
for f in filtration.txt triage.txt wizard.txt; do
    if [ ! -s "$DIR/eval-data/$f" ]; then
        echo "FAIL: eval-data/$f missing or empty after sync" >&2
        exit 1
    fi
done
echo "    eval-data extracted: filtration/triage/wizard"

# Verify cataractae INSTRUCTIONS.md exist on disk (read at runtime, not synced)
for role in architect implementer reviewer qa security docs_writer delivery; do
    if [ ! -s "$REPO_ROOT/cataractae/$role/INSTRUCTIONS.md" ]; then
        echo "FAIL: cataractae/$role/INSTRUCTIONS.md missing or empty" >&2
        exit 1
    fi
done
echo "    cataractae instructions present: architect/implementer/reviewer/qa/security/docs_writer/delivery"

if grep -rn "glm-5.1:cloud" "$REPO_ROOT/cmd/ct/assets/cistern.yaml" "$REPO_ROOT/cistern.yaml" "$REPO_ROOT/aqueduct/aqueduct.yaml" 2>/dev/null; then
    echo "FAIL: retired model glm-5.1:cloud found in shipped configs (retired 2026-09-25 — see session log 15:27)" >&2
    exit 1
fi
echo "    no retired default models in shipped configs"

if ! grep -q "Finding facts is your job" "$DIR/eval-data/filtration.txt"; then
    echo "FAIL: filtration prompt lost the fact-finding duty directive (grilling adaptation)" >&2
    exit 1
fi
for directive in "frontier" "design tree" "ct droplet add"; do
    if ! grep -q "$directive" "$DIR/eval-data/filtration.txt"; then
        echo "FAIL: filtration prompt missing grilling directive '$directive'" >&2
        exit 1
    fi
done
for state in "needs-triage" "needs-info" "ready-for-agent" "ready-for-human" "wontfix"; do
    if ! grep -q "$state" "$DIR/eval-data/triage.txt"; then
        echo "FAIL: triage prompt missing state '$state'" >&2
        exit 1
    fi
done
if ! grep -q "template library" "$DIR/eval-data/wizard.txt"; then
    echo "FAIL: wizard prompt missing template-library contract" >&2
    exit 1
fi
echo "    methodology directives intact (filter/triage/wizard)"

echo "==> promptfoo evals (sequential)"
FLAKES_ALLOWED=2
FAIL_TOTAL=0
for cfg in \
    cistern-filter-promptfooconfig.yaml \
    cistern-triage-promptfooconfig.yaml \
    cistern-wizard-promptfooconfig.yaml \
    cistern-architect-promptfooconfig.yaml \
    cistern-implementer-promptfooconfig.yaml \
    cistern-reviewer-promptfooconfig.yaml \
    cistern-qa-promptfooconfig.yaml \
    cistern-security-promptfooconfig.yaml \
    cistern-docs-writer-promptfooconfig.yaml \
    cistern-delivery-promptfooconfig.yaml; do
    echo "--- $cfg"
    (cd "$DIR" && promptfoo eval -c "$cfg" --no-cache > /tmp/pf-"$cfg".out 2>&1)
    code=$?
    fails=$(grep -E "✗ [0-9]+ failed" /tmp/pf-"$cfg".out 2>/dev/null | head -1 | grep -oE "[0-9]+" || echo 0)
    errors=$(grep -c "❌\|ERROR\|Error:" /tmp/pf-"$cfg".out 2>/dev/null || true)
    echo "    exit=$code fails=$fails errors=$errors (details: /tmp/pf-$cfg.out)"
    if [ "$code" -ne 0 ]; then
        if [ "$fails" -le "$FLAKES_ALLOWED" ] && [ "$errors" -eq 0 ]; then
            echo "    WARN: $fails rubric flake(s) tolerated (limit $FLAKES_ALLOWED)"
        else
            FAIL_TOTAL=$((FAIL_TOTAL + 1))
            echo "    FAIL: config exited non-zero with $fails fails / $errors errors" >&2
        fi
    fi
done
if [ "$FAIL_TOTAL" -gt 0 ]; then
    echo "==> NOT GREEN: $FAIL_TOTAL config(s) failed" >&2
    exit 1
fi
echo "==> ALL GREEN"

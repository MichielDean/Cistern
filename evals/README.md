# Cistern Evals

Testing ground for instruction changes — the same practice the lobsterdog
career-ops-eval suite established: **prompts are requests; evals are the
enforcement.** Any change to filtration/triage/wizard prompts, cataractae
instructions, or stage-skill wiring must pass this suite before merge.

## What's here

| Path | Purpose |
|---|---|
| `providers/glm_flash.py` | Grader model (ollama glm-5.3-flash:cloud, retry-hardened). Graders only — never the system under test |
| `cistern-filter-promptfooconfig.yaml` | Filtration behavior under the grilling methodology |
| `cistern-triage-promptfooconfig.yaml` | Triage state-machine decisions incl. verify-before-grill |
| `cistern-wizard-promptfooconfig.yaml` | Wizard staging discipline (scoped stages, values, secrets) |
| `sync-eval-data.sh` | Extracts live prompts from Go sources into `eval-data/` (single source of truth) |
| `eval-data/` | Generated — don't hand-edit |
| `metrics/metrics.py` | Per-cataractae flow statistics from the cistern events table |
| `metrics/BASELINE-*.md` | Frozen pre/post-adaptation measurements |
| `metrics/REPORT-*.md` | Generated cohort reports (write with `--report <path>`) |

## Running

```bash
cd evals
./run-all-checks.sh            # sync + deterministic checks + all promptfoo configs, sequential
promptfoo eval -c cistern-filter-promptfooconfig.yaml --no-cache   # single config
python3 metrics/metrics.py     # current pipeline flow statistics
```

## When evals gate a change

Run `./run-all-checks.sh` before pushing a PR that touches:

- `cmd/ct/refine.go`, `cmd/ct/triage.go`, `cmd/ct/wizard.go` (entrypoint prompts)
- `cataractae/*/INSTRUCTIONS.md`, `cataractae/*/PERSONA.md` (stage identities)
- `internal/skills/**` (vendored/adapted skills)
- `aqueduct/aqueduct.yaml`, `cmd/ct/assets/cistern.yaml` (stage-skill wiring)

**CI limitation (deliberate):** graders call the local ollama endpoint, which
GitHub Actions cannot reach. The LLM-graded configs run locally pre-merge;
CI runs the deterministic Go tests (`TestAqueductSkillsShip`, model sanity)
plus this suite's structure whenever someone runs it locally. Instruction
changes without a local eval run are a review blocker — say so in the PR body.

## Rubric discipline (learned from career-ops)

- Grader = glm-5.3-flash with a strict JSON rubricPrompt; the system under
  test is never the grader.
- Sequential config runs (congestion flakes when batched concurrently).
- ≤2 rubric flakes per config tolerated; >2 = fix prompt or revert.
- Deterministic checks first: anything assertable by grep/parse lives in
  `run-all-checks.sh` or a Go test, never in the rubric.

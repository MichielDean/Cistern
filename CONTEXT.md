# Context

## Item: ci-g6so3

**Title:** Implement Jira tracker provider
**Status:** in_progress
**Priority:** 1

### Description

Implement the Jira provider in internal/tracker/jira/. Uses Jira REST API v3 with Basic Auth (email + API token). Config reads from cistern.yaml trackers.jira section (url, email, token - token supports env var via ${JIRA_API_TOKEN} syntax). FetchIssue(key) calls GET /rest/api/3/issue/{key} with fields=summary,description,priority,labels,status. Maps: summary->Title, description (ADF to plain text)->Description, priority name mapped to int (Highest/High=1, Medium=2, Low=3, Lowest=4), labels->Labels, constructs SourceURL from base URL + browse + key. Returns ExternalIssue. Includes unit tests with HTTP test server mocking Jira responses. Depends on ci-xrgv2.

## Current Step: docs

- **Type:** agent
- **Role:** docs_writer
- **Context:** full_codebase

## ⚠️ REVISION REQUIRED — Fix these issues before anything else

This droplet was recirculated. The following issues were found and **must** be fixed.
Do not proceed to implementation until you have read and understood each issue.

### Issue 1 (from: reviewer)

♻ 3 findings. (1) jira.go:51 — SSRF/path traversal: issue key interpolated into URL without validation or url.PathEscape; attacker-controlled key can rewrite the request path. (2) jira.go:31 — HTTP client has no timeout (zero = infinite), violating the established codebase pattern (skills.go:25 sets 30s timeout); a non-responding Jira server hangs the goroutine forever. (3) jira.go:73 — unbounded response body decode with no io.LimitReader, violating the codebase pattern (skills.go:240); a huge response could OOM the process.

### Issue 2 (from: reviewer)

All 3 prior issues verified fixed: (1) url.PathEscape prevents SSRF, (2) 30s HTTP timeout set, (3) io.LimitReader caps response body. Fresh review of full diff found no new issues. All tests pass.

### Issue 3 (from: qa)

♻ Tests pass but 3 error-path/coverage gaps found:\n1. ci-g6so3-4ivrz: jira.go:68-70 transport error path (httpClient.Do failure) is untested — needs a test injecting a failing transport.\n2. ci-g6so3-xezaq: jira.go:79-81 malformed JSON error path (200 with non-JSON body) is untested — needs a test where the server returns invalid JSON.\n3. ci-g6so3-aak4f: TestADFToPlainText has no cases for heading, bulletList/orderedList/listItem, codeBlock, or blockquote node types — these are all in the switch at jira.go:149 but only paragraph is exercised.

### Issue 4 (from: reviewer)

Phase 1: all 3 prior QA issues (ci-g6so3-4ivrz, ci-g6so3-xezaq, ci-g6so3-aak4f) verified fixed with evidence — transport error test, malformed JSON test, and ADF node type coverage all present and passing. Phase 2: fresh adversarial review of full diff found no new issues. Security mitigations (PathEscape, 30s timeout, LimitReader) solid. All error paths covered. Race detector clean. Full repo test suite (15 packages) passes.

### Issue 5 (from: qa)

Phase 1: all 3 prior QA issues verified fixed with evidence — transport error test (TestProvider_FetchIssue_ReturnsError_OnTransportFailure), malformed JSON test (TestProvider_FetchIssue_ReturnsError_OnMalformedJSON), and ADF node type coverage (Heading, BulletListWithListItem, OrderedListWithListItem, CodeBlock, Blockquote) all present and passing. Phase 2: fresh adversarial review found no new issues. Security mitigations (PathEscape, 30s timeout, LimitReader) solid. Error paths fully covered. All 23 jira tests and 15 packages pass with race detector clean.

### Issue 6 (from: security)

Phase 1: all 3 prior security findings (SSRF/PathEscape, 30s HTTP timeout, LimitReader 1MiB) verified fixed in jira.go:52,33,74. QA coverage gaps (transport error, malformed JSON, ADF node types) verified present in tests. Phase 2: fresh adversarial review of full diff — no new issues. Auth credentials not leaked in errors, url.PathEscape prevents injection, response body bounded, timeouts set, defer Close in place. All tests pass with race detector clean.

### Issue 7 (from: security)

No security issues found. All prior findings (SSRF, timeout, unbounded body) verified fixed. Fresh adversarial review clean — no new vulnerabilities across auth, injection, secrets, data exposure, or resource safety.

---

## Recent Step Notes

### From: security

No security issues found. All prior findings (SSRF, timeout, unbounded body) verified fixed. Fresh adversarial review clean — no new vulnerabilities across auth, injection, secrets, data exposure, or resource safety.

### From: security

Phase 1: all 3 prior security findings (SSRF/PathEscape, 30s HTTP timeout, LimitReader 1MiB) verified fixed in jira.go:52,33,74. QA coverage gaps (transport error, malformed JSON, ADF node types) verified present in tests. Phase 2: fresh adversarial review of full diff — no new issues. Auth credentials not leaked in errors, url.PathEscape prevents injection, response body bounded, timeouts set, defer Close in place. All tests pass with race detector clean.

### From: qa

Phase 1: all 3 prior QA issues verified fixed with evidence — transport error test (TestProvider_FetchIssue_ReturnsError_OnTransportFailure), malformed JSON test (TestProvider_FetchIssue_ReturnsError_OnMalformedJSON), and ADF node type coverage (Heading, BulletListWithListItem, OrderedListWithListItem, CodeBlock, Blockquote) all present and passing. Phase 2: fresh adversarial review found no new issues. Security mitigations (PathEscape, 30s timeout, LimitReader) solid. Error paths fully covered. All 23 jira tests and 15 packages pass with race detector clean.

### From: reviewer

Phase 1: all 3 prior QA issues (ci-g6so3-4ivrz, ci-g6so3-xezaq, ci-g6so3-aak4f) verified fixed with evidence — transport error test, malformed JSON test, and ADF node type coverage all present and passing. Phase 2: fresh adversarial review of full diff found no new issues. Security mitigations (PathEscape, 30s timeout, LimitReader) solid. All error paths covered. Race detector clean. Full repo test suite (15 packages) passes.

<available_skills>
  <skill>
    <name>cistern-droplet-state</name>
    <description>Manage droplet state in the Cistern agentic pipeline using the `ct` CLI.</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-droplet-state/SKILL.md</location>
  </skill>
  <skill>
    <name>cistern-git</name>
    <description>---</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-git/SKILL.md</location>
  </skill>
</available_skills>

## Signaling Completion

When your work is done, signal your outcome using the `ct` CLI:

**Pass (work complete, move to next step):**
    ct droplet pass ci-g6so3

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-g6so3
    ct droplet recirculate ci-g6so3 --to implement

**Pool (cannot currently proceed):**
    ct droplet pool ci-g6so3

Add notes before signaling:
    ct droplet note ci-g6so3 "What you did / found"

The `ct` binary is on your PATH.

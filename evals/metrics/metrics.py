#!/usr/bin/env python3
"""Cistern pipeline flow metrics — measures instruction-adaptation impact.

Reads the cistern events table (read-only) and reports per-cataractae
dispatch/outcome statistics, split at an adaptation date.

Usage:
  python3 metrics.py                                  # current stats to stdout
  python3 metrics.py --report metrics/REPORT-2026-11-11.md
  SPLIT_AT=2026-11-11T14:42:00 python3 metrics.py     # cohort split (UTC)

Outcome model (events.event_type):
  dispatch          a cataractae stage started
  exit_no_outcome   session died without signaling (loop indicator)
  pass              stage completed and advanced
  recirculate       sent back upstream (payload: cataractae + target)
  pool              cannot currently proceed
  fail              stage failed

Interpretation (adapted-methodology tracking):
  - exit_no_outcome/dispatch ratio  = session-death/loop rate (lower is better)
  - recirculate per pass            = rework rate per stage (spec-ambiguity probe)
  - pass/dispatch                   = throughput per stage

Caveats: the live DB retains only unpurged droplets (ct droplet purge); long
ranges must be reconstructed from GitHub PR history (delivery cataractae PRs).
Single-stage deltas are noise until a cohort has 5+ delivered droplets.
"""
import argparse
import json
import os
import sqlite3
import sys
from collections import defaultdict
from datetime import datetime, timezone

DEFAULT_DB = os.path.expanduser("~/.cistern/cistern.db")
SPLIT_AT = os.environ.get("SPLIT_AT", "")
TRACKED = ("dispatch", "exit_no_outcome", "pass", "recirculate", "pool", "fail")


def split_at():
    if not SPLIT_AT:
        return None
    return datetime.fromisoformat(SPLIT_AT.replace("Z", "+00:00"))


def cohort_of(created_at: str) -> str:
    if not SPLIT_AT:
        return "all"
    # DB timestamps use "2026-11-11 22:34:08.9+00:00" — normalize both sides.
    try:
        ts = datetime.fromisoformat(created_at.replace(" ", "T"))
        return "pre" if ts < split_at() else "post"
    except ValueError:
        return "all"


def cataractae_of(payload: str, fallback: str = "?") -> str:
    try:
        data = json.loads(payload) if payload else {}
        for key in ("cataractae", "step", "target"):
            if data.get(key):
                return data[key]
    except (json.JSONDecodeError, TypeError):
        pass
    return fallback


def collect(db_path):
    con = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    try:
        rows = con.execute(
            "SELECT event_type, payload, created_at FROM events ORDER BY created_at"
        ).fetchall()
    finally:
        con.close()
    stats = defaultdict(int)  # "cohort/cataractae/etype" -> count
    for etype, payload, created in rows:
        if etype not in TRACKED:
            continue
        cat = cataractae_of(payload)
        stats[f"{cohort_of(created)}/{cat}/{etype}"] += 1
    return stats, rows


def rows_by_droplet(rows):
    by = defaultdict(lambda: defaultdict(int))
    for etype, payload, created in rows:
        by[etype][cohort_of(created)] += 1
    return by


def stage_table(stats, cohort):
    stages = sorted(
        {key.split("/")[1] for key in stats if key.startswith(cohort + "/")}
    )
    out = ["| cataractae | dispatch | pass | recirculate | pool | fail | exit_no_outcome | loops |",
           "|---|---|---|---|---|---|---|---|"]
    flagged = False
    for stage in stages:
        counts = {e: stats.get(f"{cohort}/{stage}/{e}", 0) for e in TRACKED}
        loop_pct = (
            100.0 * counts["exit_no_outcome"] / counts["dispatch"]
            if counts["dispatch"]
            else 0.0
        )
        if loop_pct > 20:
            flagged = True
        out.append(
            f"| {stage} | {counts['dispatch']} | {counts['pass']} | "
            f"{counts['recirculate']} | {counts['pool']} | {counts['fail']} | "
            f"{counts['exit_no_outcome']} | {loop_pct:.1f}% |"
        )
    return "\n".join(out), flagged


def stage_duration_stats(db_rows):
    """Mean dispatch->next-event duration per cataractae (proxy for stage time)."""
    return None  # durations live in events payload timestamps; refined later


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--db", default=DEFAULT_DB)
    ap.add_argument("--report", help="write markdown report to this path")
    args = ap.parse_args()

    stats, rows = collect(args.db)
    lines = ["# Cistern flow metrics", ""]
    lines.append(f"Generated: {datetime.now(timezone.utc).isoformat()}")
    if split_at():
        lines.append(f"Cohort split at: {split_at().isoformat()} (pre = pre-adaptation)")
    lines.append("")

    cohorts = ["all"] if not SPLIT_AT else ["pre", "post"]
    flagged = False
    for cohort in cohorts:
        if any(k.startswith(cohort + "/") for k in stats):
            lines.append(f"## Cohort: {cohort}")
            table, did_flag = stage_table(stats, cohort)
            flagged = flagged or did_flag
            lines += [table, ""]
    if flagged:
        lines.append("> ⚠ exit_no_outcome loops >20% on one or more stages — "
                     "pipeline is not completing stages; see session logs "
                     "(~/.cistern/session-logs/) before trusting any other metric.")
        lines.append("")

    report = "\n".join(lines) + "\n"
    if args.report:
        os.makedirs(os.path.dirname(args.report) or ".", exist_ok=True)
        with open(args.report, "w") as f:
            f.write(report)
        print(f"wrote {args.report}")
    else:
        sys.stdout.write(report)

    # Non-zero exit when a fatal loop is detected — usable as a health gate.
    sys.exit(1 if flagged else 0)


if __name__ == "__main__":
    main()

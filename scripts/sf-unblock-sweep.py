#!/usr/bin/env python3
"""Detect and repair stale-blocked Stoneforge tasks.

A task is *stale-blocked* when its status is `blocked` but every one of its
"blocks"-type dependencies is already closed. The dispatch daemon only hands out
`open` tasks, so a stale-blocked task is never picked up — and anything behind it
in the dependency chain stalls with it. On 2026-07-21 four stale roots held up
seven downstream tasks and idled ten workers for hours.

Usage:
    scripts/sf-unblock-sweep.py            # report only (safe, default)
    scripts/sf-unblock-sweep.py --apply    # flip stale-blocked tasks to open
    scripts/sf-unblock-sweep.py --json     # machine-readable report

Exit codes:
    0  no stale-blocked tasks found (or --apply repaired them all)
    1  stale-blocked tasks found while in report-only mode
    2  a repair failed, or sf could not be queried
"""

import argparse
import json
import re
import subprocess
import sys
import tempfile

TASK_LIMIT = 500
BLOCKER_LINE = re.compile(r"\s*(el-[a-z0-9]+)\s+(\S+)")


def sf(*args):
    """Run an sf command, returning stdout. Raises on non-zero exit."""
    proc = subprocess.run(
        ("sf",) + args, capture_output=True, text=True
    )
    if proc.returncode != 0:
        raise RuntimeError(f"sf {' '.join(args)} failed: {proc.stderr.strip()}")
    return proc.stdout


def unwrap(payload):
    """sf --json nests the element list under varying keys; find the list."""
    if isinstance(payload, list):
        return payload
    if isinstance(payload, dict):
        for key in ("data", "items", "elements", "tasks", "results"):
            if key in payload:
                return unwrap(payload[key])
    return []


def load_tasks():
    """Load every task as JSON.

    sf truncates its --json output at ~64KB when stdout is a pipe but not when
    it is a regular file, so the full listing is routed through a temp file.
    """
    with tempfile.NamedTemporaryFile("w+", suffix=".json") as handle:
        proc = subprocess.run(
            ("sf", "task", "list", "--limit", str(TASK_LIMIT), "--json"),
            stdout=handle, stderr=subprocess.PIPE, text=True,
        )
        if proc.returncode != 0:
            raise RuntimeError(f"sf task list failed: {proc.stderr.strip()}")
        handle.flush()
        handle.seek(0)
        return unwrap(json.load(handle))


def blockers_of(task_id):
    """Return the ids this task is blocked BY (outgoing "blocks" deps only).

    `sf dependency list` prints outgoing deps first, then an "Incoming" section.
    Only the outgoing side gates dispatch, and only "Blocks"-type edges do —
    Parent-Child edges link a task to its plan and never block it.
    """
    text = sf("dependency", "list", task_id).split("Incoming")[0]
    found = []
    for line in text.splitlines():
        match = BLOCKER_LINE.match(line)
        if match and match.group(2).lower().startswith("block"):
            found.append(match.group(1))
    return found


def find_stale(tasks):
    status = {t["id"]: t.get("status") for t in tasks}
    stale, genuine = [], []
    for task in tasks:
        if task.get("status") != "blocked":
            continue
        blockers = blockers_of(task["id"])
        states = [(b, status.get(b, "unknown")) for b in blockers]
        record = {
            "id": task["id"],
            "title": task.get("title", ""),
            "priority": task.get("priority"),
            "blockers": states,
        }
        # No blockers at all is also stale: nothing is holding it back.
        if all(s == "closed" for _, s in states):
            stale.append(record)
        else:
            genuine.append(record)
    return stale, genuine


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--apply", action="store_true",
                        help="flip stale-blocked tasks to open (default: report only)")
    parser.add_argument("--json", action="store_true", help="machine-readable output")
    args = parser.parse_args()

    try:
        tasks = load_tasks()
        stale, genuine = find_stale(tasks)
    except (RuntimeError, json.JSONDecodeError) as exc:
        print(f"sweep failed: {exc}", file=sys.stderr)
        return 2

    repaired, failed = [], []
    if args.apply:
        for item in stale:
            try:
                sf("task", "update", item["id"], "--status", "open")
                repaired.append(item["id"])
            except RuntimeError as exc:
                failed.append((item["id"], str(exc)))

    if args.json:
        print(json.dumps({
            "stale": stale,
            "genuine_blocked": genuine,
            "repaired": repaired,
            "failed": failed,
        }, indent=2))
    else:
        if not stale:
            print(f"clean — no stale-blocked tasks ({len(genuine)} genuinely blocked)")
        for item in stale:
            closed = ", ".join(b for b, _ in item["blockers"]) or "none"
            mark = "REPAIRED" if item["id"] in repaired else "STALE"
            print(f"{mark:9} {item['id']} P{item['priority']} {item['title'][:55]}"
                  f"  (all blockers closed: {closed})")
        for task_id, err in failed:
            print(f"FAILED    {task_id}: {err}", file=sys.stderr)
        if stale and not args.apply:
            print(f"\n{len(stale)} stale-blocked task(s). Re-run with --apply to repair.")

    if failed:
        return 2
    return 1 if stale and not args.apply else 0


if __name__ == "__main__":
    sys.exit(main())

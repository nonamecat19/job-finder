#!/usr/bin/env bash
#
# Fail if apps/api depends on a Go module with a published advisory that this
# module's own code can actually reach.
#
# Why a wrapper rather than calling govulncheck directly: govulncheck has no
# ignore file, by deliberate upstream choice. An advisory with no released fix
# still needs a way to be acknowledged that is reviewable in a diff and expires
# on its own — the alternative in practice is `|| true`, which disables the
# whole gate rather than one finding. That mechanism lives here, in
# apps/api/.govulncheck-ignore.
#
# Reachability is the point of the tool. A vulnerable module sitting in the
# dependency graph that no code path reaches is reported and does NOT fail the
# build, so the gate stays actionable instead of becoming noise.
#
# Fix a failure by bumping the dependency:
#   cd apps/api && go get <module>@<fixed-version> && go mod tidy
#
# If no fix exists, add an expiring entry to apps/api/.govulncheck-ignore and
# record the reason. See specs/domains/platform-operations.md.
#
# Exit codes:
#   0  clean — no findings, or every finding unreachable or validly excepted
#   1  at least one reachable, non-excepted finding
#   2  the ignore file is malformed, or an entry is expired or stale
#   3  govulncheck is missing or its version differs from the pin
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
API_DIR="$REPO_ROOT/apps/api"
VERSION_FILE="$API_DIR/.govulncheck-version"
IGNORE_FILE="$API_DIR/.govulncheck-ignore"

# ---------------------------------------------------------------------------
# Ignore-file parsing
#
# Grammar (see specs/039-supply-chain-ci/contracts/file-formats.md):
#   <GO-YYYY-NNNN>  <expiry YYYY-MM-DD>  <reason to end of line>
#
# A malformed line is never skipped silently. A bad line means somebody
# believes an advisory is suppressed when it is not, or the reverse, and both
# directions are dangerous.
#
# Populates the IGNORED associative array: id -> expiry.
declare -A IGNORED=()

parse_ignore_file() {
  local file="$1" today lineno=0 line id expiry reason
  today="$(date -u +%F)"

  [[ -f "$file" ]] || return 0

  while IFS= read -r line || [[ -n "$line" ]]; do
    lineno=$((lineno + 1))

    # Comments and blank lines.
    [[ "$line" =~ ^[[:space:]]*(#.*)?$ ]] && continue

    if ! [[ "$line" =~ ^(GO-[0-9]{4}-[0-9]{4,})[[:space:]]+([0-9]{4}-[0-9]{2}-[0-9]{2})[[:space:]]+([^[:space:]].*)$ ]]; then
      echo "error: $file:$lineno: malformed entry" >&2
      echo "  expected: <GO-YYYY-NNNN>  <expiry YYYY-MM-DD>  <reason>" >&2
      echo "  got:      $line" >&2
      return 2
    fi

    id="${BASH_REMATCH[1]}"
    expiry="${BASH_REMATCH[2]}"
    reason="${BASH_REMATCH[3]}"

    if ! date -u -d "$expiry" >/dev/null 2>&1; then
      echo "error: $file:$lineno: '$expiry' is not a real date" >&2
      return 2
    fi

    if [[ -n "${IGNORED[$id]+set}" ]]; then
      echo "error: $file:$lineno: duplicate entry for $id" >&2
      return 2
    fi

    # An expired exception fails the build rather than lapsing into silence.
    # Renewing it is a reviewed diff; that is the whole design.
    if [[ "$expiry" < "$today" ]]; then
      echo "error: $file:$lineno: exception for $id expired on $expiry" >&2
      echo "  reason on record: $reason" >&2
      echo "  Either the advisory now has a fix (bump it) or the exception needs renewing in a reviewed change." >&2
      return 2
    fi

    IGNORED["$id"]="$expiry"
  done < "$file"

  return 0
}

# ---------------------------------------------------------------------------
# Self-test: exercises every row of the parser table above against in-script
# fixtures. No network, no real advisory, so the parser's failure modes are
# provable in CI on any tree.
self_test() {
  local tmp rc failures=0 future past
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  future="$(date -u -d '+30 days' +%F)"
  past="$(date -u -d '-1 day' +%F)"

  expect() {
    local desc="$1" want="$2" content="$3"
    IGNORED=()
    printf '%s\n' "$content" > "$tmp/ignore"
    set +e
    parse_ignore_file "$tmp/ignore" 2>/dev/null
    rc=$?
    set -e
    if [[ "$rc" -eq "$want" ]]; then
      echo "  ok    $desc"
    else
      echo "  FAIL  $desc (want exit $want, got $rc)"
      failures=$((failures + 1))
    fi
  }

  echo "govulncheck-check.sh --self-test"
  expect "comments and blanks only"      0 "# just a comment"
  expect "valid entry"                   0 "GO-2026-1234  $future  no upstream fix yet"
  expect "malformed id"                  2 "GO-26-1  $future  bad id"
  expect "missing expiry"                2 "GO-2026-1234  no upstream fix yet"
  expect "unparseable date"              2 "GO-2026-1234  2026-13-45  bad month"
  expect "empty reason"                  2 "GO-2026-1234  $future  "
  expect "expired entry"                 2 "GO-2026-1234  $past  lapsed"
  expect "duplicate id"                  2 "$(printf 'GO-2026-1234  %s  first\nGO-2026-1234  %s  second' "$future" "$future")"

  if [[ "$failures" -gt 0 ]]; then
    echo "$failures self-test failure(s)" >&2
    return 1
  fi
  echo "all parser cases pass"
  return 0
}

if [[ "${1:-}" == "--self-test" ]]; then
  self_test
  exit $?
fi

# ---------------------------------------------------------------------------
# Version guard. Structure mirrors scripts/sqlc-check.sh: pinned version,
# installed version, why it matters, install line.
if [[ ! -f "$VERSION_FILE" ]]; then
  echo "error: missing version pin at $VERSION_FILE" >&2
  exit 3
fi

PINNED_VERSION="$(tr -d '[:space:]' < "$VERSION_FILE")"

if ! command -v govulncheck >/dev/null 2>&1; then
  cat >&2 <<EOF
error: govulncheck is not installed.

Install the pinned version:

  go install golang.org/x/vuln/cmd/govulncheck@v${PINNED_VERSION}

The version is pinned so a local run and a CI run reach the same verdict.
EOF
  exit 3
fi

# ---------------------------------------------------------------------------
parse_ignore_file "$IGNORE_FILE" || exit $?

REPORT="$(mktemp)"
trap 'rm -f "$REPORT"' EXIT

# -format json streams a sequence of JSON objects; the "finding" entries carry
# the trace. A finding whose deepest frame has a function is reachable; one
# that stops at the module level is not.
set +e
(cd "$API_DIR" && govulncheck -format json ./...) > "$REPORT" 2>"$REPORT.err"
scan_status=$?
set -e

if [[ "$scan_status" -ne 0 ]] && [[ ! -s "$REPORT" ]]; then
  echo "error: govulncheck failed to run (exit $scan_status)" >&2
  cat "$REPORT.err" >&2 || true
  exit 3
fi

python3 - "$REPORT" "${!IGNORED[@]}" <<'PY'
import json
import sys

report_path = sys.argv[1]
ignored = set(sys.argv[2:])

with open(report_path) as fh:
    text = fh.read()

decoder = json.JSONDecoder()
messages, idx = [], 0
while idx < len(text):
    while idx < len(text) and text[idx].isspace():
        idx += 1
    if idx >= len(text):
        break
    obj, idx = decoder.raw_decode(text, idx)
    messages.append(obj)

# An OSV message declares the advisory; a finding message ties it to this
# module. A finding with a function in its deepest frame is call-graph
# reachable — the distinction the whole gate rests on.
osvs = {m["osv"]["id"]: m["osv"] for m in messages if "osv" in m}
findings = [m["finding"] for m in messages if "finding" in m]

# govulncheck emits several findings per advisory: a module-level one, a
# package-level one, and — only when the vulnerable symbol is actually called
# — a symbol-level one carrying a function in its deepest frame. They must be
# folded per advisory, keeping the most specific, or the same advisory lands in
# both buckets and the report contradicts itself.
folded = {}
for f in findings:
    osv_id = f.get("osv")
    if not osv_id:
        continue
    trace = f.get("trace") or []
    is_reachable = bool(trace and trace[0].get("function"))
    prev = folded.get(osv_id)
    if prev is None:
        folded[osv_id] = (is_reachable, f)
        continue
    prev_reachable, prev_finding = prev
    # A symbol-level finding always wins; otherwise keep whichever carries the
    # version metadata (the module-level finding is the one that has it).
    if is_reachable and not prev_reachable:
        folded[osv_id] = (is_reachable, f)
    elif is_reachable == prev_reachable and not prev_finding.get("fixed_version"):
        folded[osv_id] = (is_reachable, f)

reachable = {k: v[1] for k, v in folded.items() if v[0]}
unreachable = {k: v[1] for k, v in folded.items() if not v[0]}

# The version metadata lives on the module-level finding, which is not
# necessarily the one kept above, so index it separately. The version in use is
# on the trace frame (`trace[0].version`), not on the finding itself — only the
# fixed version sits at the top level, and an advisory with no released fix
# omits even that.
versions = {}
for f in findings:
    osv_id = f.get("osv")
    if not osv_id or osv_id in versions:
        continue
    trace = f.get("trace") or []
    found = trace[0].get("version") if trace else None
    if found or f.get("fixed_version"):
        versions[osv_id] = (found, f.get("fixed_version"))

def describe(osv_id, finding):
    osv = osvs.get(osv_id, {})
    summary = osv.get("summary") or osv.get("details", "").split("\n")[0]
    trace = finding.get("trace") or []
    module = trace[0].get("module") if trace else None
    found, fixed = versions.get(osv_id, (None, None))
    found = found or finding.get("found_version") or "unknown"
    fixed = fixed or finding.get("fixed_version") or "none published"
    lines = [
        f"  {osv_id}  {module or '(unknown module)'}",
        f"    summary: {summary}",
        f"    found:   {found}",
        f"    fixed:   {fixed}",
    ]
    if trace and trace[0].get("function"):
        frame = trace[0]
        where = f"{frame.get('module','')}.{frame.get('function','')}"
        if frame.get("position"):
            p = frame["position"]
            where += f" ({p.get('filename')}:{p.get('line')})"
        lines.append(f"    reached: {where}")
    return "\n".join(lines)

# Stale exceptions: an entry for an advisory that no longer appears at all
# reads as live risk while suppressing nothing.
seen = set(reachable) | set(unreachable)
stale = sorted(ignored - seen)
if stale:
    print("error: apps/api/.govulncheck-ignore has entries for advisories that no", file=sys.stderr)
    print("longer appear in any finding. Remove them — a dead entry reads as live risk:", file=sys.stderr)
    for osv_id in stale:
        print(f"  {osv_id}", file=sys.stderr)
    sys.exit(2)

if unreachable:
    print(f"{len(unreachable)} advisory/advisories affect a dependency but are NOT reachable")
    print("from this module's code. Reported, not failing:")
    for osv_id, finding in sorted(unreachable.items()):
        print(describe(osv_id, finding))
    print()

excepted = {k: v for k, v in reachable.items() if k in ignored}
failing = {k: v for k, v in reachable.items() if k not in ignored}

if excepted:
    print(f"{len(excepted)} reachable advisory/advisories suppressed by an expiring")
    print("exception in apps/api/.govulncheck-ignore:")
    for osv_id, finding in sorted(excepted.items()):
        print(describe(osv_id, finding))
    print()

if failing:
    print(f"error: {len(failing)} reachable advisory/advisories with no exception:", file=sys.stderr)
    for osv_id, finding in sorted(failing.items()):
        print(describe(osv_id, finding), file=sys.stderr)
    print(file=sys.stderr)
    print("Fix by bumping the dependency:", file=sys.stderr)
    print("  cd apps/api && go get <module>@<fixed-version> && go mod tidy", file=sys.stderr)
    print("If no fix exists, add an expiring entry to apps/api/.govulncheck-ignore.", file=sys.stderr)
    sys.exit(1)

print("govulncheck: no reachable, non-excepted advisories.")
PY

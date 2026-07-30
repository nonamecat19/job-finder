# Quickstart: Verifying the Consolidation

The risk here is a silent regression — types that get *weaker* while all suites stay green. Every check below is aimed at that.

## Prerequisites

```bash
pnpm install
pnpm --filter @job-finder/shared build
go install github.com/gzuidhof/tygo@v$(tr -d '[:space:]' < apps/api/.tygo-version)
```

## Baseline — capture before touching anything

```bash
# Duplicated shapes and their drift
python3 scripts/compare-shared-types.py    # committed as part of this feature
# EXPECT before: pairs=56 identical=17 drifted=39
# EXPECT after:  pairs=0

# Consumer surface
grep -rl "@job-finder/shared" apps/dashboard/src | wc -l    # EXPECT: 47, before and after
git rev-parse HEAD > /tmp/pre-consolidation                  # for the import diff below
```

---

## P1 — `enum_style: union`

```bash
# add `enum_style: union` to apps/api/tygo.yaml, then:
make tygo-generate
git diff --stat packages/shared/src/generated.ts
```

```bash
grep -n "export type SourceKind" packages/shared/src/generated.ts
# EXPECT: 'api' | 'scrape' | 'sidecar'    NOT: string

pnpm typecheck && pnpm --filter @job-finder/dashboard test
# EXPECT: green. Any failure here is a real narrowing the codebase was violating.
```

Re-measure before writing a single narrowing — this step removes entries from the Class B list for free:

```bash
python3 scripts/compare-shared-types.py
# EXPECT: literal-union-lost drops to 0; the "other" class shrinks
```

---

## P2 — Deduplication

**No shape is defined twice.**

```bash
python3 scripts/compare-shared-types.py     # EXPECT: pairs=0
grep -c "^export interface" packages/shared/src/index.ts    # EXPECT: 0
```

**The public surface is frozen** (FR-005, SC-004):

```bash
git diff --name-only $(cat /tmp/pre-consolidation) -- apps/dashboard/src | wc -l
# EXPECT: 0 — not one consumer file changed

pnpm typecheck                                     # EXPECT: green
pnpm --filter @job-finder/dashboard test           # EXPECT: green
make test-go                                       # EXPECT: green
./scripts/tygo-check.sh                            # EXPECT: green
```

**Strictness did not regress** (SC-005) — the check that matters most, because everything above can pass while types quietly weaken:

```bash
# Nullability survived: no field that was `T | null` became `T?`
python3 scripts/compare-shared-types.py --check-strictness
# EXPECT: 0 weakened fields
```

Spot-check by hand in the editor — automation can miss a widened union:

```ts
const r: ActivityRunDto = /* … */;
r.error;       // EXPECT hover: string | null      NOT string | undefined
r.elapsedMs;   // EXPECT hover: number | null
const j: JobDto = /* … */;
j.status;      // EXPECT hover: ApplicationStatus | 'hidden'   NOT string
j.application; // EXPECT: exists — it was missing from index.ts before
```

**A reintroduced duplicate is rejected** (FR-008):

```bash
cat >> packages/shared/src/index.ts <<'EOF'
export interface JobDto { id: string; title: string; }
EOF
python3 scripts/compare-shared-types.py    # EXPECT: non-zero exit, names JobDto
git checkout -- packages/shared/src/index.ts
```

**Consumer-only types are real** (FR-004):

```bash
# For each type in consumer-only.ts, confirm it has consumers:
for t in $(grep -oP '^export (interface|type) \K\w+' packages/shared/src/consumer-only.ts); do
  n=$(grep -rl "\b$t\b" apps/dashboard/src | wc -l)
  echo "$t: $n consumers"
done
# EXPECT: every count >= 1. A zero means dead code — delete it, don't label it.
```

---

## P3 — Document consistency

**No false statements remain** (SC-007):

```bash
grep -rn -i "python" AGENTS.md package.json
# EXPECT: no output

grep -n "hand-maintained" AGENTS.md
# EXPECT: no output — the practice no longer exists
```

**Required rules are present** (FR-013, FR-014):

```bash
grep -n -i "branch\|pull request" AGENTS.md    # EXPECT: the branch-and-PR rule
grep -n -i "worktree" AGENTS.md                # EXPECT: lifecycle rules
```

**No topic has two owners** (SC-006): walk the Owners table in `contracts/doc-ownership.md`. For each topic, search `AGENTS.md`, `constitution.md` and `README.md`. Exactly one may state the rule; others may only point at it.

**The described command matches the recipe**: read the `make test-lint` line in `AGENTS.md` against the `Makefile`. They must agree exactly — including after feature 023 changes what that target runs.

---

## P4 — Single agent stack

**Configuration names the stack that is present** (FR-022):

```bash
grep -n '"ai"' .specify/init-options.json         # EXPECT: "claude"
grep -n 'installed_integrations' .specify/integration.json   # EXPECT: ["claude"]
ls .specify/integrations/                         # EXPECT: no opencode.manifest.json
grep -rn "opencode" . --exclude-dir={node_modules,.git,specs}
# EXPECT: no output (FR-023)
```

**Commands are committed** (FR-021, the reproducibility failure this fixes):

```bash
git ls-files .claude/skills | wc -l    # EXPECT: 12 skill files. Today: 0.
```

**Local-only settings stay local**:

```bash
git ls-files .claude/settings.local.json    # EXPECT: empty — holds a DB password
git ls-files .claude/worktrees | wc -l      # EXPECT: 0
```

**The real test — a fresh clone works** (FR-024, FR-025):

```bash
git clone . /tmp/clone-check && cd /tmp/clone-check
ls .claude/skills/            # EXPECT: the speckit-* skills are present
cat .specify/integration.json # EXPECT: claude
bash .specify/scripts/bash/check-prerequisites.sh --json
# EXPECT: runs and resolves templates
cd - && rm -rf /tmp/clone-check
```

This is the check that fails today: a fresh clone gets a manifest naming a stack that is not there and no speckit commands at all.

---

## Full-loop check

The feature is done when this works:

1. Add a field to a Go DTO in `apps/api/internal/dto/`.
2. Run `make tygo-generate`. **Stop there** — no second file to edit.
3. The dashboard sees the new field; `tsc --noEmit` is green.
4. `./scripts/tygo-check.sh` is green with no hand edit.
5. Attempt to paste a duplicate interface into `index.ts` — the duplicate check rejects it.
6. Read `AGENTS.md` and `constitution.md` end to end: every rule appears once, and every claim is true.

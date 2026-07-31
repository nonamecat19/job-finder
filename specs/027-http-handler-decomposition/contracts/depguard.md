# Contract: Arrangement Enforcement

Two mechanisms, because one is not enough:

| Mechanism | Enforces | Covers |
|---|---|---|
| `depguard` rules (below, §1) | *what a package may import* | FR-004, FR-010, FR-012, FR-011 half one |
| Placement test (below, §2) | *where a handler may live* | FR-011 half two |

`depguard` matches import paths, not file locations. A handler at `internal/jobs/handler.go` — inside its feature module but outside `interfaces/http` — violates FR-011 and passes every import rule. That is the gap §2 closes.

---

# §1 — `depguard` Arrangement Rules

Added to the existing `apps/api/.golangci.yml`. No new tooling, no new CI job — `make lint-go` and the `lint-go` CI job already run golangci-lint with a pinned version.

**`depguard` is not in the `standard` linter set**, so it must be added to an explicit `enable` list alongside the existing `linters.default: standard` / `disable: [errcheck]` configuration.

## Configuration

```yaml
linters:
  default: standard
  enable:
    # 027-http-handler-decomposition: keep HTTP adapters inside their feature
    # modules and keep the router free of feature dependencies. These rules are
    # what stops the flat-httpapi arrangement from growing back — it took 23
    # handlers to accumulate the first time, one convenient exception at a time.
    - depguard
  disable:
    - errcheck   # unchanged, see existing rationale

  settings:
    depguard:
      rules:
        # Rule 1 — nothing imports the router except the composition root.
        # Enforces FR-004/SC-001 in both directions: the router cannot reach
        # into a feature, and a feature cannot reach back into the router.
        router-is-not-a-library:
          list-mode: lax
          files:
            - "!**/cmd/server/**"
            - "!**/internal/httpapi/**"
          deny:
            - pkg: github.com/job-finder/api/internal/httpapi
              desc: >-
                internal/httpapi is the router, not a library. Put HTTP handlers
                in internal/<feature>/interfaces/http and register them via
                NewRouter's variadic mounts in cmd/server. Shared response
                helpers live in internal/httpx.

        # Rule 2 — the shared helper package stays a leaf.
        # If httpx grows an internal dependency it stops being safely importable
        # from every feature and the cycle risk returns.
        httpx-stays-a-leaf:
          list-mode: lax
          files:
            - "**/internal/httpx/**"
          deny:
            - pkg: github.com/job-finder/api/internal
              desc: >-
                internal/httpx must depend on the standard library only. It is
                imported by every feature's HTTP adapter; any internal
                dependency here risks an import cycle and couples all features
                to it.

        # Rule 3 — feature adapters do not reach into another feature's guts.
        # Prefix-based and therefore approximate (see Limitations); it catches
        # the mechanical regressions, which are the ones that actually occur.
        no-cross-feature-internals:
          list-mode: lax
          files:
            - "**/internal/*/interfaces/**"
          deny:
            - pkg: github.com/job-finder/api/internal/*/infrastructure
              desc: >-
                An adapter must not import another feature's infrastructure.
                Go through that feature's exported application service, or
                declare a local port it satisfies.
```

## What each rule prevents

| Rule | Prevents | Spec requirement |
|---|---|---|
| `router-is-not-a-library` | A new handler being added to `httpapi`; a feature importing the router | FR-004, FR-010, **FR-011 half one**, SC-001, SC-005 |
| `httpx-stays-a-leaf` | The helper package accreting dependencies and reintroducing cycle risk | FR-003 |
| `no-cross-feature-internals` | An adapter reaching past another feature's boundary | FR-012 |
| **placement test (§2)** | A handler inside a feature module but outside its adapter layer | **FR-011 half two** |

## Verification

The rules must be seen **rejecting** something. A rule never observed failing has not been tested.

```bash
# Rule 1
cat > internal/jobs/interfaces/http/violation.go <<'EOF'
package http
import _ "github.com/job-finder/api/internal/httpapi"
EOF
make lint-go   # must fail naming internal/httpapi and the depguard rule
rm internal/jobs/interfaces/http/violation.go

# Rule 2
cat > internal/httpx/violation.go <<'EOF'
package httpx
import _ "github.com/job-finder/api/internal/dto"
EOF
make lint-go   # must fail
rm internal/httpx/violation.go
```

Each check belongs in the task list as its own verification step (FR-010, SC-005).

---

# §2 — Placement Test

Covers FR-011's second half: request handling inside a feature module but outside its adapter layer.

**Location**: `apps/api/internal/arch_test.go` (package `internal_test`). Lives in the normal suite, runs under `go test ./...` and therefore under `make test-lint` — no new tooling, no new CI job.

**Rule**: any `.go` file under `internal/<feature>/` that imports the router library (`github.com/go-chi/chi/v5`) must be inside an `interfaces/` package.

```go
// Walk internal/, parse imports with go/parser (ImportsOnly), and for every
// file importing chi, assert its path contains "/interfaces/".
//
// Exemptions, each justified rather than merely listed:
//   internal/httpapi  — is the router itself
//   internal/httpx    — must not import chi at all; rule 2 above already
//                       enforces that, so it is exempt here only to keep the
//                       two mechanisms from reporting the same violation twice
//
// Failure message must name the file and the required destination, e.g.:
//   internal/jobs/handler.go imports chi outside an interfaces/ package.
//   HTTP handlers belong in internal/jobs/interfaces/http/.
```

**Why a test rather than a linter plugin**: golangci-lint has no built-in check for "file location implies allowed imports", and a custom plugin needs building, pinning and distributing. A test is ~20 lines, is debuggable with `go test -run`, and fails in the same place every other regression fails.

**Verification** — the test must be seen failing:

```bash
cat > internal/jobs/violation.go <<'EOF'
package jobs
import _ "github.com/go-chi/chi/v5"
EOF
go test ./internal/ -run TestHandlersLiveInInterfaces   # MUST fail, naming the file
rm internal/jobs/violation.go
```

**Limitation**: keyed on the chi import, so a handler written against `net/http` alone with no router import would not be caught. Accepted — every existing handler mounts via chi, and a handler that never touches the router cannot be registered.

---

## Limitations — stated, not discovered later

- **`depguard` matches import paths, not intent.** `no-cross-feature-internals` cannot distinguish a legitimate cross-feature port from an illegitimate reach-in; it only blocks `infrastructure` packages by prefix. A feature importing another feature's `application` package still passes. That is deliberate — `application` is the exported boundary, which FR-012 permits.
- **`depguard` matches import paths, not file locations.** This is why §2 exists. Without the placement test, FR-011 would be half-enforced while appearing satisfied.
- **File-pattern matching is by path glob.** A feature that does not follow the `internal/<feature>/interfaces/` layout is not covered by rule 3. FR-002's requirement that every module adopt the layout is therefore load-bearing for this rule, not merely cosmetic.
- **Rules are additive only.** They prevent regression; they do not verify the initial move was complete. SC-006 is checked by the inventory audit in quickstart.md, not by the linter.
- **Verify glob syntax against the pinned golangci-lint version** before relying on it. `depguard`'s `files` patterns and `list-mode` semantics have changed across major versions; the config above targets v2.x, matching `apps/api/.golangci-version`. Confirm with a deliberate violation (above) rather than assuming the config parses as intended — a `depguard` rule that silently matches nothing is indistinguishable from a passing build.

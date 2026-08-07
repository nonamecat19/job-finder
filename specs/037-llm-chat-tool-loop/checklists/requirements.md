# Requirements Quality Checklist: Multi-Turn Conversations and a Typed Tool Loop

**Purpose**: Validate that the requirements in spec.md are complete, unambiguous, testable, and
consistent before implementation begins.
**Created**: 2026-08-07
**Revised**: 2026-08-07 after audit. The original checklist passed a specification whose consumer did
not exist, whose loop returned a type nothing could consume, and whose capability mechanism
contradicted a binding domain document — because every one of its thirty items asked whether a
requirement was *written*, and none asked whether what it asserted was *true*. § "Verification against
the tree" exists to close that.
**Feature**: [spec.md](../spec.md)

## Verification against the tree — **the class of error this feature actually made**

Check these first. Everything below them is worthless if one of them fails.

- [ ] CHK031 Every package, file and line number named in any document has been **opened**, and the
      reader can say what is in it. The original R8 named `internal/interviewprep/application` across
      five documents; it does not exist, and nothing in that tree makes an LLM call at all
- [ ] CHK032 The chosen consumer was verified to (a) hold an LLM provider, (b) have a test suite the
      conversion can regress, and (c) return a type worth validating — each confirmed by reading the
      package, not by recalling what it does
- [ ] CHK033 Every task naming a file to edit was checked against `ls`. T038–T044 named four files in
      a package with none of them
- [ ] CHK034 Every cited **precedent** was opened and its mechanism confirmed to be the mechanism
      claimed. `internal/arch_test.go` is a direct-import scan against one constant using
      `parser.ImportsOnly`; `outreach/nosend_test.go` is a string-token grep that touches no imports.
      Neither is the transitive import walk they were cited as precedent for
- [ ] CHK035 Every claim about **vendor or proxy behaviour** was verified against the running
      configuration or the vendor's source, not asserted. `drop_params: true` (`gateway/config.yaml:213`)
      makes the tool-capability annotation inert at runtime, and the trap was already documented one
      section above in `specs/domains/llm-routing.md`
- [ ] CHK036 Every configuration key the feature introduces was checked for whether it **already
      exists** in the file. No `model_info` block existed in `gateway/config.yaml`; the feature
      introduces the first, so its meaning must be written down rather than inferred from convention
- [ ] CHK037 Every new type's **consumers were counted**, not assumed. `Result{Content string}` had
      zero: all fourteen structured call sites go through `CompleteStructured[T]`
- [ ] CHK038 Every requirement asserting a **capability of another system** was checked against the
      contract this repository has already written down for it. FR-017's original mechanism required
      knowing which upstream served a request, which `specs/domains/llm-routing.md` (030-FR-004)
      states the application never learns
- [ ] CHK039 Every count and enumeration in the documents was **recounted**. "Thirteen packages call
      the structured path and four call `Complete`" is six packages over fourteen call sites, and two
      direct callers; "Nine scenarios" introduced ten
- [ ] CHK040 Every claim that a check "fails the build" **names the job that runs it**, and that job's
      trigger was checked to fire for the files the check guards. `.github/workflows/api-ci.yml`'s
      `go` paths filter does not include `gateway/`, so a config-only pull request skips `go test`
      entirely

## Backward compatibility

- [ ] CHK001 A requirement states that every existing caller keeps working **without being modified**
      (FR-003), not merely that the old method still exists
- [ ] CHK002 The measurable bar for compatibility is mechanical — byte-identical requests (SC-001) —
      rather than "no regressions observed"
- [ ] CHK003 The structured-output path is named explicitly as unchanged (FR-004), including strict
      schemas, the retry loop and the validator hook
- [ ] CHK004 The differences between the two existing entry points are **enumerated with file
      references and ranked by consequence** (FR-003a, contracts C1-2), not summarised as "the
      temperature defaults differ". The temperature split is the fourth-largest of seven
- [ ] CHK004a The golden comparison covers **both adapters**. The highest-ranked difference —
      `num_predict` present on the local plain path and absent on the local JSON path — is entirely
      outside a gateway-only test, and it is a wire change on the terminal tier every chain ends at
- [ ] CHK004b Behaviours a golden file cannot see are covered separately: the retry's full trigger set
      (it fires on an unparsable 200 body and on zero choices, not only on a 400/422), the case where
      a schema-parse failure **skips** the retry, and the doubled `logServed` / `ReportServedModel` /
      `ReportUsage` counts when the retry fires (FR-005a, FR-005b)
- [ ] CHK005 A requirement forbids turns being merged, dropped or reordered (FR-005)

## The return type

- [ ] CHK041 A requirement states the exchange produces a **typed, schema-validated** value, not text
      (FR-023), and names the existing structured path as the mechanism
- [ ] CHK042 The type is the **consumer's own** result type, so conversion changes no signature
      (FR-023a)
- [ ] CHK043 A requirement forbids falling back to prose when the terminal step fails (FR-023b)
- [ ] CHK044 A requirement or contract makes a non-answered stop return the zero value **and** an
      error, so a truncated exchange cannot be mistaken for an answer (contracts C4-16)

## The read-only fence

- [ ] CHK006 A requirement forbids any tool from acting outward or changing state (FR-007), phrased as
      a prohibition rather than a design intention
- [ ] CHK007 The prohibition has an automated enforcement requirement attached (FR-008), not review
      convention
- [ ] CHK008 The enforcement's **limits** are stated — all three, not one: a hand-built `net/http`
      request, **a closure over an already-injected capability**, and packages rather than call paths
      (FR-008c, contracts C5-3). The closure hole is the largest and was previously undocumented
- [ ] CHK009 The sequencing requirement is explicit: the fence lands with the loop, never after it
      (contracts C5-4)
- [ ] CHK010 The relationship to Constitution Principle I is stated in the plan, not left for a
      reviewer to infer
- [ ] CHK045 The check's **mechanism** is specified concretely enough to implement, and its
      dependency cost was checked: `go list -deps` via `os/exec` rather than
      `golang.org/x/tools/go/packages`, which is not in `go.mod` and whose addition would break
      SC-009 with the control protecting Principle I (contracts C5-1)
- [ ] CHK046 The check **fails rather than skips** when its own mechanism is unavailable (C5-1a)
- [ ] CHK047 The **enumeration** mechanism is specified, not merely required: how the test discovers a
      tool-registering package it was not told about, and why an import walk alone cannot (FR-008a,
      contracts C5-2)
- [ ] CHK048 The forbidden set includes every package that reaches the internet, not only those that
      send messages. `internal/retrieval` drives a headless browser and calls FlareSolverr and was
      missing from the original list (FR-008b)
- [ ] CHK049 The check's **location** does not force the platform layer to enumerate its consumers'
      packages, which would contradict the plan's own structure decision (contracts C5-5)

## Bounds

- [ ] CHK011 Every bound is a separate, named requirement: rounds (FR-010), deadline (FR-011),
      per-lookup time (FR-012), result size (FR-014), total spend (FR-016a)
- [ ] CHK012 The round limit's bar is exact — stops at the limit, never one past it (SC-003)
- [ ] CHK013 A requirement forbids any bound from being influenced by a prompt or a model response
      (research R6, contracts C4-9)
- [ ] CHK014 A requirement or contract forbids a second overall deadline competing with the caller's
      context (contracts C4-3)
- [ ] CHK014a …**and** a requirement bounds wall time anyway, by requiring the caller's context to
      carry a deadline (FR-011a). "No second timer" is not the same as "bounded": the proxy's own
      documented worst case is 600s per call
- [ ] CHK014b A requirement bounds total spend, and the record carries a running total (FR-016a).
      Per-call cost is already captured by an existing hook; nothing was accumulating it
- [ ] CHK015 Truncation is required to be **reported**, not just performed (FR-014)
- [ ] CHK016 Every failure mode is required to become a message rather than abort the exchange
      (FR-013), and the list of failure modes is enumerated
- [ ] CHK050 The exchange record carries the **served model and the cost per round** (FR-016). A
      record without them makes a multi-round exchange invisible to the per-tier and per-run
      visibility features 035 and 036 provide

## Untrusted tool output

*Absent from the original checklist entirely, as it was from the original specification.*

- [ ] CHK051 A requirement states that tool **output** is untrusted, distinct from the tool being
      read-only (FR-024). Read-only bounds what a lookup does, not what its output can talk the model
      into
- [ ] CHK052 A requirement covers framing and delimiting results as data rather than instructions
      (FR-025)
- [ ] CHK053 A requirement fixes the toolset, the bounds and the answer schema before the exchange and
      forbids any result content from changing them (FR-026)
- [ ] CHK054 A requirement makes injection attempts **visible**, not merely ineffective (FR-027)
- [ ] CHK055 The documents state plainly that the heuristic is a **detector, not a filter**, so nobody
      reads it as sanitisation

## Provider wire details

- [ ] CHK056 The **encoding** of tool-call arguments is specified: the wire form is a JSON string
      containing JSON and must be decoded before validation, or every well-formed call is refused by
      the mechanism meant to refuse malformed ones (FR-009a)
- [ ] CHK057 **Tool-call identity on every provider path** is specified. The requirement that every
      result carry a provider-assigned id is unsatisfiable on the local path, whose native format has
      no id field; the adapter's synthesis is authorised explicitly (FR-015a, contracts C2-10)
- [ ] CHK058 Every field a document reads from a response was checked to **exist in the response
      struct**. `ChatResult.FinishReason` had no source: neither adapter's response type declared
      `finish_reason` or `tool_calls`

## Honest failure

- [ ] CHK017 A requirement forbids returning an answer when the serving model cannot call tools
      (FR-017), stated as a prohibition on the wrong behaviour and not only as a preference for the
      right one
- [ ] CHK017a The detection **mechanism** works with information the application is permitted to have.
      It requires a tool call on round one rather than comparing the served model against an
      expectation, which 030-FR-004 forbids and which cannot distinguish a dropped `tools` array from
      a model that chose not to look anything up (research R12)
- [ ] CHK017b The mechanism's **cost** is stated rather than hidden: round one always performs at
      least one lookup (FR-017a)
- [ ] CHK018 Tool capability is required to be a declared property of configuration (FR-018) with a
      test — **and the requirement says plainly that the declaration is documentation a test reads,
      not a control the proxy enforces**. Any wording implying it prevents the drop is wrong
- [ ] CHK018a The **coupling** through the single shared terminal deployment is stated: declaring one
      tool-using chain declares the local tier tool-capable for every task (FR-018a)
- [ ] CHK019 The local terminal tier's behaviour is specified for both cases — tool-capable and not
      (FR-019)
- [ ] CHK020 The measurable bar counts the forbidden outcome at zero (SC-007)

## Consumer

- [ ] CHK021 A requirement forces a real consumer to ship with the capability (FR-021) — **and the
      consumer named exists** (CHK031, CHK032)
- [ ] CHK022 The consumer's failure behaviour is specified — decline rather than invent (FR-022) —
      which is the same fabrication boundary Principle II protects, and here it means **persisting
      nothing**, since this consumer writes to the database
- [ ] CHK023 A requirement or contract protects the consumer's existing behaviour from regression
      (SC-008, contracts C7-4), naming the specific paths at risk rather than "existing tests"
- [ ] CHK059 Where the consumer's own test fake is also one of the fakes the interface change breaks,
      that overlap is noted so the consumer's suite is not counted as an independent check
      (contracts C7-6)

## Scope discipline

- [ ] CHK024 A requirement forbids introducing a third-party agent framework (FR-020) with a
      measurable check (SC-009)
- [ ] CHK025 The reasoning behind that prohibition is recorded in a document that survives the feature
      directory being deleted on ship (tasks T048)
- [ ] CHK026 Streaming, persistence, memory and context-window management are explicitly out of scope
      rather than silently absent
- [ ] CHK027 No requirement introduces a database change, a DTO, or a cross-language type
- [ ] CHK060 No control introduced to satisfy one requirement violates another. The fence's
      implementation must not add a module requirement that breaks SC-009

## Testability

- [ ] CHK028 Every FR has an acceptance scenario or quickstart step that would fail if the requirement
      were violated
- [ ] CHK029 The fence has a test that has been **seen to fail** (tasks T025) — and specifically seen
      to fail on a **transitive** import, which is the only case that distinguishes it from the
      direct-import scan it was mistakenly claimed to be an instance of
- [ ] CHK030 No success criterion depends on a subjective reading of model output quality
- [ ] CHK061 Every scenario count, list length and enumeration in the documents matches the items it
      counts

## Notes

- Check items off as completed: `[x]`
- CHK006–CHK010 and CHK045–CHK049 are the gate. A gap there is not a documentation defect; it is a
  request to put a language model inside the trust boundary the constitution declares non-negotiable.
- **CHK031–CHK040 are the meta-gate**, and they are new because the original checklist could not have
  caught what went wrong. Thirty items asked whether requirements were written clearly. Every one of
  them passed. The specification still rested on a fabricated consumer, an unusable return type and a
  mechanism forbidden by a domain document in the same repository — because "is this well specified?"
  and "is this true?" are different questions, and only the first was being asked.

# Requirements Quality Checklist: Self-Hosted LLM Observability

**Purpose**: Validate that the requirements in spec.md are complete, unambiguous, testable, and
consistent before implementation begins.
**Created**: 2026-08-07
**Feature**: [spec.md](../spec.md)

## Scope & framing

- [ ] CHK001 The spec states plainly which parts are config-only and which are not, rather than
      inheriting the original one-line claim uncritically (US1 vs US2)
- [ ] CHK002 Observability is scoped as an operator tool; no end-user dashboard surface leaks into
      the requirements
- [ ] CHK003 Coverage is stated as an explicit enumeration (FR-013), not implied by "everything goes
      through the proxy"

## Safety & blast radius

- [ ] CHK004 A requirement forbids the collector from delaying, failing or altering any AI call
      (FR-004), and a measurable bar backs it (SC-003)
- [ ] CHK005 Both failure modes are covered — collector *absent* and collector *hung* — not just the
      easy one
- [ ] CHK006 A requirement guarantees the platform starts and its suites pass with nothing configured
      (FR-015, SC-007)
- [ ] CHK007 No requirement makes inference depend on the collector's ordering, availability or
      acknowledgement

## Privacy & credentials

- [ ] CHK008 A requirement forbids prompt or completion content leaving the deployment (FR-005) with
      a verification method attached (SC-006), not just a stated intention
- [ ] CHK009 Retention is a stated, bounded, documented number rather than a vendor default (FR-008)
- [ ] CHK010 Credential isolation is required in the direction that matters — the application must
      not be able to read collector keys (FR-007)
- [ ] CHK011 The decision to retain prompt bodies rather than redact them is recorded with its
      rationale, so it is a choice and not an oversight

## Completeness of the record

- [ ] CHK012 Every field an operator needs is required by name: requested key, serving model,
      duration, both token counts, cost, outcome (FR-002)
- [ ] CHK013 Failed calls are required to produce records, not only successful ones (FR-002)
- [ ] CHK014 Zero-cost local calls are required to produce a record rather than being absent (FR-014)
- [ ] CHK015 Fallback service is observable by comparing requested key against serving model (FR-003),
      with no separate flag needed

## Correlation

- [ ] CHK016 Run grouping is required (FR-009) and its identifier is tied to the platform's own
      activity record so the cross-reference works both ways (FR-010)
- [ ] CHK017 Concurrency safety is required explicitly (FR-011), and the measurable bar names a
      concurrency level (SC-005)
- [ ] CHK018 Retries and escalations are required to land inside the run, not beside it (FR-009)
- [ ] CHK019 A requirement or contract forbids the correlation id from reaching a prompt

## Testability

- [ ] CHK020 Every FR has at least one acceptance scenario or quickstart step that would fail if the
      requirement were violated
- [ ] CHK021 SC-001's "100% of calls" has a stated counting method, not just a percentage
- [ ] CHK022 SC-002's "zero Go change" has a mechanical check (a clean diff), not a reviewer's opinion
- [ ] CHK023 No success criterion depends on reading the collector's UI subjectively

## Consistency

- [ ] CHK024 No requirement contradicts the constitution's local-first principle
- [ ] CHK025 No requirement changes the routing chains, timeouts or retry counts that feature 030 and
      035 depend on (contracts C1-4)
- [ ] CHK026 Terminology matches the rest of the repository: "task key", "serving model",
      "fallback tier", "activity run"

## Vendor reality (added 2026-08-07 after audit)

These exist because the original checklist asked whether the spec *stated* things, never whether the
stated things were *true*. Every item below corresponds to a claim that passed the checklist and
failed against the vendor's source.

- [ ] CHK027 Every field the spec names is verified against the integration's actual output, not
      assumed from how the request was constructed
- [ ] CHK028 The record's grouping key is verified: does the collector group by requested key or by
      serving deployment? Two task keys served by one model must not collapse (SC-004a)
- [ ] CHK029 Every configuration setting the spec depends on is verified to exist in the chosen
      edition — not the product generally, and not a paid tier
- [ ] CHK030 The chosen version's support horizon is stated, and any requirement resting on a feature
      newer than that version is identified
- [ ] CHK031 Any metadata key with append-vs-overwrite semantics is verified against the vendor's own
      documentation of that distinction

## Repository reality (added 2026-08-07 after audit)

- [ ] CHK032 Every file path named in plan, contracts or tasks is confirmed to exist, and to contain
      the code the document claims
- [ ] CHK033 Every service named in a compose instruction is confirmed present in that compose file
- [ ] CHK034 Every test file a task says to "add" is checked for prior existence
- [ ] CHK035 Coverage claims stated as conditional are verified to actually be conditional, rather
      than unconditional facts described as configurable
- [ ] CHK036 Any cited precedent ("works the same way as X") is read, not recalled

## Blast radius (added 2026-08-07 after audit)

- [ ] CHK037 The feature's storage growth cannot exhaust a resource the platform depends on
- [ ] CHK038 The collector's own authentication and network exposure are specified, not just its
      egress
- [ ] CHK039 Deletion in the platform is either propagated to the collector or documented as not
      propagating
- [ ] CHK040 Failure records — which carry upstream error bodies — are considered as a distinct
      privacy surface from success records

## Notes

- Check items off as completed: `[x]`
- CHK004–CHK007 are the gate. A failure in that group is not a documentation defect, it is a
  reason not to ship the feature.
- **CHK027–CHK036 exist because the first pass of this checklist passed a spec whose central
  mechanism was wrong.** The checklist and the spec shared an author, so items testing whether the
  spec was *internally coherent* all passed while its premises about external software and about this
  repository were false. An item asking "is this requirement stated clearly" cannot catch that; only
  "has this been opened and read" can.

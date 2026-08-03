# Contract: Profile Resume Editing API

Extends the existing `ProfilesHandler` (`apps/api/internal/httpapi/profiles.go`, mounted
routes at line 30-38). Existing routes (`GET/POST /profiles`, `GET/PUT/DELETE /profiles/{id}`,
`POST /profiles/config`, `GET /profiles/config/status`) are unchanged and remain available
— config upload stays a supported, optional pre-fill path (FR-002).

## GET /profiles/{id}/resume

Returns the structured `Resume` (see data-model.md) for the given profile, derived from
its current `rendercvConfig`/`rendercvYaml`. Unrecognized data is included under each
`unrecognized` field, never dropped.

- **200**: `{ "resume": Resume }`
- **404**: profile does not exist
- **200 with empty Resume** (`sections: []`, only `name` populated or even empty) when the
  profile has no config yet — this is the User Story 1 "start from scratch" state
  (FR-012), not an error.

## PUT /profiles/{id}/resume

Replaces the profile's structured resume in full (whole-document replace, matching the
existing whole-blob update model — no partial-field PATCH semantics, consistent with the
current `UpdateProfile` COALESCE pattern for the identity fields only).

- **Request body**: `{ "resume": Resume }`
- **Behavior**: server validates per data-model.md validation rules; on success, converts
  `Resume` → `RendercvMaster` map (`resume_mapping.go`) → YAML text via
  `PrepareMasterForMarshal` (preserving section order per research.md #2) → persists via
  the existing `rendercvYaml`/`rendercvConfig` update path.
- **200**: `{ "resume": Resume }` (the saved, re-read-back state, so client shows the
  authoritative persisted value)
- **400**: validation failure — response includes a machine-readable field path (e.g.
  `sections[2].entries[0].endDate`) so the client can point at the specific offending
  field (FR-007), not just a generic message.
- **404**: profile does not exist

## POST /profiles/config (existing, behavior clarified)

Unchanged endpoint, but the plan formalizes its interaction with the above: if the target
profile already has non-empty resume content (any section with entries, or any identity
field beyond a bare name), the client MUST have already obtained user confirmation
(FR-010) before calling this endpoint — the server does not itself prompt (no UI concept at
that layer), but MAY return a `409`-style advisory flag (`"hasExistingContent": true`) in
the current `GET /profiles/config/status` response so the client can decide whether to
show the confirmation dialog before calling upload. No breaking change to the endpoint's
existing success/error shape.

## Not introduced by this feature

- No endpoints for partial/single-entry or single-section mutation — the whole-`Resume`
  PUT is the only write path, matching the existing whole-blob persistence model and
  keeping the mapping layer (research.md #2-#3) as the single place order/round-trip
  correctness is enforced. Per-entry/section add/edit/delete/reorder all happen
  client-side against the in-memory `Resume` document, then are saved via one PUT.

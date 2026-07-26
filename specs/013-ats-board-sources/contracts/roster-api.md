# Contract: Employer Roster HTTP API

New endpoints on the existing `apps/api` chi router (`apps/api/internal/httpapi/`), mounted
alongside `SourcesHandler` (`sources.go`). The five board vendors reuse the *existing* `/sources`
endpoints unchanged (`GET /sources`, `PUT /sources/{key}`, `POST /sources/{key}/test`,
`POST /sources/{key}/run`) — FR-022 is satisfied by registering each vendor as a normal
`JobSource` row, no new source-level contract needed.

This file covers only the roster/candidate surface that is new.

## `GET /api/roster`

List the employer roster (FR-024).

Response `200`:
```json
{
  "employers": [
    {
      "id": "uuid",
      "vendor": "greenhouse",
      "employerIdentifier": "acme",
      "displayName": "Acme Inc",
      "addedVia": "proposed",
      "enabled": true,
      "lastSuccessAt": "2026-07-24T10:00:00Z",
      "lastPostingCount": 12,
      "stale": false
    }
  ]
}
```

## `POST /api/roster`

Register an employer board by pasting a URL (FR-011).

Request:
```json
{ "url": "https://boards.greenhouse.io/acme" }
```

Response `201` — same shape as one `employers[]` entry above, `addedVia: "pasted"`.

Response `422` — url does not match a supported vendor, or the board failed its live read
health-check:
```json
{ "error": "unsupported_vendor", "message": "...", "supportedVendors": ["greenhouse", "lever", "ashby", "workable", "smartrecruiters"] }
```
or
```json
{ "error": "unreadable", "message": "board did not respond with a valid posting list" }
```

## `DELETE /api/roster/{id}`

Remove an employer from the roster (FR-012). Does not delete previously ingested `Job` rows.

Response `204`.

## `GET /api/roster/candidates`

List proposed candidates not yet decided (FR-009).

Response `200`:
```json
{
  "candidates": [
    {
      "id": "uuid",
      "vendor": "lever",
      "employerIdentifier": "beta-co",
      "displayName": "Beta Co",
      "inferredFromJobId": "uuid",
      "state": "proposed"
    }
  ]
}
```

## `POST /api/roster/candidates/{id}/accept`

Accept a candidate: creates/enables the matching `EmployerBoard` row (FR-010 scenario 2).

Response `200` — the created `EmployerBoard` entry (same shape as `POST /api/roster`).

## `POST /api/roster/candidates/{id}/reject`

Reject a candidate: marks it `rejected`, terminal — never re-proposed (FR-010 scenario 3).

Response `204`.

## `POST /api/roster/discover`

Trigger candidate discovery over existing `Job` rows (US2 Independent Test). Idempotent — running
twice does not duplicate `proposed` rows for the same employer, and does not resurrect `rejected`
ones (Edge Cases: "candidate discovery finds an employer already in the roster: not offered
again").

Response `200`:
```json
{ "newCandidates": 4, "skippedAlreadyKnown": 11 }
```

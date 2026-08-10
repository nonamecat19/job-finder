# Contract: The generation workspace REST surface

`apps/api/internal/generation/interfaces/http/generations.go`, registered through the existing
variadic `httpapi.NewRouter(...)` call in `cmd/server/compose.go` — one line, no edit to
`router.go` (027-FR-005). Versioned and unversioned mounts come from that single registration
(027-FR-007). Auth, CORS, requestId middleware and the `{error, path?, message?}` error shape are
inherited. Dates are ISO-8601 UTC. Every response body is a DTO in
`internal/dto/generation_workspace.go`, tygo-generated into `packages/shared` — no hand-written
TS shape (Constitution III).

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/v1/generations` | Start a run. `202 {runId, activityId}` |
| `GET` | `/v1/generations/{runId}` | The whole workspace: run, sections, items |
| `GET` | `/v1/generations?jobId=&limit=` | Recent runs, newest first |
| `PATCH` | `/v1/generations/{runId}/items/{itemId}` | Toggle / edit / reorder one item |
| `PATCH` | `/v1/generations/{runId}/sections/{sectionId}/order` | Reorder a section in one call |
| `POST` | `/v1/generations/{runId}/rerun` | `202`; body `{sections?: string[]}` — omit for the whole run |
| `POST` | `/v1/generations/{runId}/export` | `202` rendering, or `200` with the overflow report |
| `GET` | `/v1/generations/{runId}/export` | Idempotent short-poll for export status |
| `DELETE` | `/v1/generations/{runId}` | `204` |

---

## POST /v1/generations

```jsonc
{
  "profileId": "uuid",
  "jobId": "uuid",                  // or omit and send vacancy
  "vacancy": { "company": "Acme", "title": "Senior Engineer", "text": "…" },
  "groundingLevel": "moderate",     // optional; governs the SUMMARY only on this route (R1)
  "summaryOptionId": "premium"      // optional; absent = stored default (034)
}
```

`202 {"runId": "…", "activityId": "…"}`. The run is enqueued on the existing asynq `generate`
queue with `GeneratePayload.GenerationRunID` set; `generation.Handler.ProcessTask` dispatches on
that field, the same wire-nullable additive pattern 020 specified for `tailoringDraftId`.

| Status | Condition |
|---|---|
| `400` | neither `jobId` nor `vacancy.text`; or the profile has no master content |
| `404` | unknown `profileId` / `jobId` |

**No `409` for an existing run.** Unlike 020's drafts, concurrent runs against one vacancy are
legal (data-model §1).

---

## GET /v1/generations/{runId}

```jsonc
{
  "id": "uuid",
  "state": "ready",                       // running | ready | partial | failed
  "vacancy": { "company": "Acme", "title": "Senior Engineer" },
  "jobId": "uuid|null",
  "groundingLevel": "moderate",
  "summaryOptionId": "premium",
  "summarySubstituted": false,            // 035 provenance, surfaced as it is today
  "masterChanged": false,                 // FR-022: snapshot hash vs the profile's current hash
  "shapeConfig": { /* ResumeShapeConfigDto, as resolved at run start */ },
  "export": { "status": "blocked", "documentId": null, "report": { /* below */ } },
  "sections": [
    {
      "id": "uuid", "kind": "summary", "entryKey": null, "entryLabel": null,
      "position": 0, "targetCount": 0, "state": "ready",
      "error": null, "fallbackUsed": false,
      "items": [ /* below */ ]
    },
    {
      "id": "uuid", "kind": "experience", "entryKey": "Acme Inc.",
      "entryLabel": "Senior Engineer · 2021–2024",
      "position": 1, "targetCount": 8, "state": "ready",
      "error": null, "fallbackUsed": false,
      "items": [ /* below */ ]
    }
  ]
}
```

**Item**

```jsonc
{
  "id": "uuid",
  "origin": "profile",        // "profile" | "ai"
  "kind": "achievement",      // achievement | skill_group | summary — drives the badge
  "text": "Cut p95 latency 40% by …",   // effective text: edited_text ?? source_text
  "sourceIndex": 3,           // null for origin="ai"
  "rank": 0,
  "position": 0,
  "selected": true,
  "edited": false,            // true when the user has edited an AI item
  "unavailable": false
}
```

Three response rules the UI depends on and the handler must guarantee:

1. **Items are returned in `position` order**, profile-origin and AI-origin interleaved by
   position but tagged by `origin` — the client groups them, the server does not pre-group. A
   included-and-repositioned AI item must be able to sit between two profile items (FR-014's
   "including after inclusion", AS-3 of User Story 3).
2. **Every master bullet for a rendered entry appears exactly once**, selected or not, ranked or
   in master order (R2). A client can assert this without consulting the profile.
3. **`text` for `origin="profile"` is byte-identical to the master's bullet.** This is the
   assertion SC-001 is measured with, and the contract test that fails if R1 ever regresses.

---

## PATCH /v1/generations/{runId}/items/{itemId}

```jsonc
{ "selected": true, "position": 2, "text": "…" }   // any subset
```

`200` with the updated item.

| Status | Condition |
|---|---|
| `403` | `text` sent for an `origin="profile"` item — **FR-009 at the API boundary** |
| `409` | the run is `running`, or the item is `unavailable` |
| `404` | item not on this run |

Idempotent: re-sending the same body is a no-op returning the same row. Every write takes a
row-level `SELECT … FOR UPDATE` on the run first, the discipline 020 specified.

---

## POST /v1/generations/{runId}/rerun

```jsonc
{ "sections": ["<sectionId>", "…"] }   // omit for the whole run
```

`202 {"runId": "…", "activityId": "…"}` — **the same run id**. A rerun replaces the named
sections' items in place (data-model §4); it does not fork a new run, because forking would
detach the user's selections on the sections they did not rerun.

`409` when the run is already `running`.

The client is responsible for the FR-021 warning ("re-running replaces the AI's ordering for this
section") before it calls this. The server preserves matched selections and edits regardless of
whether the warning was shown.

---

## POST /v1/generations/{runId}/export

No body. Assembles from the current selection, renders once (research R5).

- `202 {"status": "rendering"}` — poll `GET …/export`.
- `200 {"status": "blocked", "report": {…}}` — over the page budget.
- `409` — the run is `running`, or every section has zero selected items with no summary and no
  skills (the "every item deselected" edge case is a client-side warning; the server refuses only
  a wholly empty document).

**Overflow report** (FR-019):

```jsonc
{
  "pagesRendered": 3,
  "pagesTarget": 2,
  "candidates": [
    { "itemId": "uuid", "sectionId": "uuid", "label": "Acme Inc. · bullet 8", "rank": 7 }
  ]
}
```

`candidates` are the lowest-ranked **selected** items, worst-ranked first. The server never acts
on them.

`GET /v1/generations/{runId}/export` returns `{"status": …, "documentId": …, "report": …}` and is
safe to poll.

---

## Client wiring

`apps/dashboard/src/lib/api.ts` gains a `generations` group mirroring the existing `settings`
group's shape. Query keys follow the established convention:

```ts
generations.all  = ['generations'];
generations.get  = (id: string) => ['generations', id];
```

Mutations invalidate `generations.get(id)`. The run poll while `state === 'running'` uses the
existing activity-polling interval; there is no new polling mechanism.

## Route

`/generate` in `apps/dashboard/src/app/routes.tsx`, wrapped in `RequireProfileConfig` (a run
without master content is a `400`, and the guard is how every other profile-dependent route
avoids that). `routeLayoutModes['/generate'] = 'fit'` — the two-pane workspace owns the viewport
and scrolls its panes independently, like `/tracker` and `/status`, rather than flowing the page.

Entry points (FR-001): a nav item, and a "Tailor for this job" action on `JobDetailPage` that
posts with the `jobId` and navigates to `/generate?runId=…`.

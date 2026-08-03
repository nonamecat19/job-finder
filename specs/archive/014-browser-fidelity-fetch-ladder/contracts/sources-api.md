# Contract: Sources-screen API additions (`apps/api/internal/httpapi`)

Extends the existing sources endpoints consumed by `apps/dashboard/src/features/sources/`.
DTOs shared via `packages/shared` per Constitution III.

## `GET /api/hosts/:host/retrieval-status` → `HostRetrievalStatusDto`

Read-only projection of `HostRetrievalState` for one host (FR-033, FR-034). 404 if the host has
no state row yet (never contacted).

```ts
type HostRetrievalStatusDto = {
  host: string;
  identityVersion: string;
  currentRung: "direct" | "browser" | "flaresolverr";
  lastBlockAt: string | null;      // ISO 8601
  lastBlockReason: string | null;
  coolingOffUntil: string | null;  // null ⇒ not cooling off
  budgetUsed: number;
  budgetLimit: number;
  budgetResetsAt: string;          // ISO 8601
};
```

## `POST /api/hosts/:host/clear-rung-preference` → 204

Operator action (FR-015, User Story 4 scenario 3). Idempotent.

## `POST /api/hosts/:host/clear-cookies` → 204

Operator action (FR-015, User Story 4 scenario 4). Idempotent.

## `POST /api/hosts/:host/override-cooling-off` → `{ remainingSeconds: number }`

On-demand override (FR-027, User Story 3 scenario 5). Response states the risk (remaining
cooling-off duration) for the dashboard to display before/after the override is applied; the
stored `coolingOffUntil` is unchanged by this call.

## `RecentRunsPanel` data extension

Existing `SourceRun`-shaped rows gain, without a new endpoint:

```ts
type RunVerdictDto = {
  verdict: "success" | "partial" | "blocked";
  blockedCount: number;
  blockReason: string | null;
};
```

Merged into the existing recent-runs response shape consumed by
`apps/dashboard/src/features/sources/SourcesPage.tsx`'s `RecentRunsPanel`.

# Quickstart: Manual Vacancy Add by URL

How to run this feature, add a vacancy end to end, and verify each success criterion.

---

## Run it

```sh
make up                 # Postgres, Redis, Ollama, LiteLLM, API, dashboard
```

Migrations are embedded and applied by goose on API start (`db.Migrate`, called from
`cmd/server/main.go:34`) — `00041_manual_add.sql` applies when the API comes up. There is no
separate migrate target.

After changing SQL or Go types, regenerate before building — never hand-edit generated files:

```sh
make sqlc-generate      # db/sqlcgen from queries/
make tygo-generate      # packages/shared/src/generated.ts from dto/
pnpm --filter @job-finder/shared build
```

`make sqlc-check` and `make tygo-check` verify the generated output is in sync; both are CI
gates, so run them before calling the feature done.

---

## Add a vacancy end to end

Dashboard: open the feed, paste a Djinni posting URL into **Add vacancy**, submit. Within a
few seconds the vacancy appears at the top of the feed.

Same thing over HTTP:

```sh
curl -sS -X POST localhost:8080/api/jobs/manual \
  -H 'content-type: application/json' \
  -d '{"url":"https://djinni.co/jobs/123456-senior-go-engineer/"}' | jq
```

```json
{
  "outcome": "created",
  "job": { "id": "…", "sourceKey": "djinni", "title": "Senior Go Engineer", "…": "…" }
}
```

Submit the same URL again — the second call returns `duplicate` with the *existing* vacancy,
not an error:

```sh
curl -sS -X POST localhost:8080/api/jobs/manual \
  -H 'content-type: application/json' \
  -d '{"url":"https://djinni.co/jobs/123456-senior-go-engineer/"}' | jq -r .outcome
# duplicate
```

Try a search URL — the host is known, the shape is not:

```sh
curl -sS -X POST localhost:8080/api/jobs/manual \
  -H 'content-type: application/json' \
  -d '{"url":"https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang"}' | jq -r .kind
# not_a_posting
```

---

## Inspect what it wrote

```sh
docker compose exec postgres psql -U postgres -d jobfinder -c '
  SELECT j."title", j."sourceKey", s."kind", j."postedAt", j."ingestedAt"
  FROM "Job" j JOIN "Subscription" s ON s."id" = j."subscriptionId"
  WHERE s."kind" = '"'"'manual'"'"' ORDER BY j."ingestedAt" DESC LIMIT 5;'
```

`postedAt` must be the posting's real date (or null) — never the add time. Then the run
record:

```sh
docker compose exec postgres psql -U postgres -d jobfinder -c '
  SELECT r."trigger", r."ok", r."found", r."new", r."error", r."startedAt"
  FROM "SourceRun" r WHERE r."trigger" = '"'"'manual'"'"'
  ORDER BY r."startedAt" DESC LIMIT 5;'
```

Every add — created, duplicate, or failed — is one row here.

---

## Verification checklist

Tied to the spec's success criteria. Each is checkable by hand in a few minutes.

| # | Check | Expect |
|---|---|---|
| SC-001 | Paste → submit → see the vacancy | Under 30 s, 3 interactions. A slow host returns `timed_out` rather than hanging. |
| SC-002 | Add a Djinni URL by hand; separately let a crawl collect the same posting | Title, company and description match |
| SC-003 | Submit the same URL twice; then with `?utm_source=x`; then twice concurrently | One vacancy, `duplicate` reported each time |
| SC-003a | Add a posting whose title differs slightly from one already in the feed | Two separate vacancies — never a silent merge |
| SC-004 | Force each failure kind (bad scheme, search URL, unknown host, dead URL) | Distinct `kind`, no vacancy left behind |
| SC-005 | Open a manually added vacancy | Match, tailor, tracker all work with no extra step |
| SC-006 | Filter the feed to Manual | Only manual adds, across all sources |
| SC-006a | Add a three-week-old posting to a busy feed | Appears at the top, still showing its real age |
| SC-006b | Compare post-age / ghost signals against the same posting crawled | Identical age |
| SC-007 | Fail a manual add three times in a row against one host | Source stays healthy; no crawl triggered |
| SC-007a | Check `SourceRun` after a failed add | The attempt and its reason are there |
| SC-008 | Add a URL on a host with no reader, complete the form, save | Vacancy stored, under 2 minutes, URL never re-entered |

Scheduler behaviour, worth checking once:

```sh
curl -sS -X POST localhost:8080/api/subscriptions/run-all | jq
# manual subscriptions are skipped

curl -sS -X POST localhost:8080/api/subscriptions/<manual-id>/run -i | head -1
# HTTP/1.1 400 Bad Request
```

---

## Tests

```sh
cd apps/api && go test ./internal/manualadd/... ./internal/jobsources/... ./internal/subscriptions/...
cd apps/dashboard && pnpm test
make test-integration      # real Postgres: persistence, dedupe, migration
make sqlc-check tygo-check # generated output in sync
make test-lint             # required — this feature spans api, dashboard and shared
```

What each layer covers:

- **Adapter** — `MatchesPostingURL` accept/reject tables, and `ReadPosting` against a saved
  fixture of a real Djinni posting page.
- **Service** — the four outcomes, all six failure kinds, timeout behaviour, and that a
  failure writes a run but stores no vacancy.
- **Integration** — a manual add and a crawl of the same posting produce one vacancy;
  concurrent adds produce one vacancy; manual runs do not flag a source unhealthy.
- **Dashboard** — the add form's outcome handling, the fill-in dialog's required-field
  validation, the Manual filter.

---

## Rollback

Goose runs `Up` only at startup; a Down has to be driven by hand against the container:

```sh
docker compose exec api goose -dir internal/db/migrations postgres "$DATABASE_URL" down
```

The `manual` `JobSource` row and any hand-entered vacancies survive by design — dropping the
source would cascade-delete real data. Re-applying the migration is safe and idempotent.

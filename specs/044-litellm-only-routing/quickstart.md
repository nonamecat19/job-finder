# Quickstart: verifying 044 end to end

**Feature**: 044-litellm-only-routing · **Date**: 2026-08-12

Ordered so each step's failure tells you something the next step would only confuse.

---

## 0. Prerequisites

```sh
cp .env.example .env      # then fill:
#   GATEWAY_URL=http://localhost:4000
#   LITELLM_MASTER_KEY=<any non-empty string>
#   COHERE_API_KEY=<real>            # embeddings run on this
#   CEREBRAS_API_KEY / GROQ_API_KEY / OPENROUTER_API_KEY  # chat tiers
```

## 1. The routing catalogue is valid before anything runs

```sh
cd apps/api && go test ./internal/platform/llm/ -run TestGatewayConfig -v
```

Expect: chain arity, provider diversity, absence of `local`/`default`, `salary` tool declarations,
`embed` width and `input_type`, no literal credentials. **This test reads a file the binary never
loads — it is the only thing between a forgotten chain and a request-time failure.**

## 2. Startup refuses to run without a gateway

```sh
GATEWAY_URL= go run ./cmd/server
```

Expect: exit within a second or two, message naming `GATEWAY_URL`. Anything that boots and serves AI
work has reintroduced the second path (SC-002, FR-002).

## 3. Bring the stack up

```sh
make up
docker compose ps litellm          # healthy
curl -s localhost:4000/health/liveliness
```

## 4. Smoke one chat scenario and one embedding

```sh
cd apps/api
go run ./cmd/llmsmoke -task match
go run ./cmd/llmsmoke -embed "senior go engineer, remote"
```

Expect from the second: a vector of exactly `EMBED_DIMS` (1024) floats, and a log line carrying
`served_model`. A length other than 1024 is E2-2 firing — the deployment and the application
disagree, and it must fail here rather than in Postgres.

## 5. The embedding is not accidentally asymmetric

The failure this catches is silent: vectors that are merely *worse*.

```sh
go run ./cmd/llmsmoke -embed-check
```

Expect: identical text → byte-identical vectors; a related pair (`"golang backend engineer"` vs
`"go developer, backend"`) scoring above an unrelated pair (`"golang backend engineer"` vs
`"pastry chef, Lyon"`). If the related pair does not win clearly, check that the `embed` deployment
declares `input_type: search_document` (research.md R2) before touching the match threshold.

## 6. Migrate and re-embed lazily

```sh
make migrate
psql "$DATABASE_URL" -c 'select atttypmod from pg_attribute where attrelid = '"'"'"Job"'"'"'::regclass and attname = '"'"'embedding'"'"';'
```

Expect `1028` (pgvector stores `dims + 4`). Then run one match:

```sh
curl -XPOST localhost:3000/api/jobs/<id>/match
psql "$DATABASE_URL" -c 'select "embedModel", ("embedding" is not null) from "Job" where id = '"'"'<id>'"'"';'
```

Expect the configured `EMBED_MODEL_ID` and a non-null vector. That is the lazy re-embed path working;
no backfill job exists or is needed (research.md R5).

## 7. Every scenario is independently routed

```sh
docker compose logs litellm | grep -oE '"model": *"[a-z-]+"' | sort | uniq -c
```

Run a match, a resume generation, a salary inference, an outreach draft. Expect `salary`, `outreach`
and `recruiter` as distinct groups — not `default`, which must no longer appear anywhere (SC-005).

Then change one scenario's lead model in `gateway/config.yaml`, `docker compose restart litellm`,
re-run, and confirm exactly that scenario's `served_model` changed (SC-004).

Finally, confirm an undeclared name fails loudly (FR-009) — inherited proxy behaviour that has never
been re-tested and is now load-bearing, because there is no `default` group left to absorb a typo:

```sh
curl -s -o /dev/null -w '%{http_code}\n' localhost:4000/chat/completions \
  -H "Authorization: Bearer $LITELLM_MASTER_KEY" -H 'Content-Type: application/json' \
  -d '{"model":"defualt","messages":[{"role":"user","content":"hi"}]}'
```

Expect a 4xx. A 200 means a typo'd scenario is being served by something, which is the failure mode
FR-009 exists to prevent.

## 8. Observability covers embeddings

Open the collector UI. Filter to a single run id.

Expect the embedding call in the same trace as that run's chat calls, named `embed`, with a cost
figure. Its absence means metadata is not being sent on the embedding path — the row in
`llm-routing.md` §7.3 that reads *"Embeddings — always: No"* is what this feature revokes, and this
is where the revocation is verified (SC-001).

## 9. Full suites

```sh
make test-lint
make test-integration
```

## 10. Confirm the pins (FR-026, SC-009)

Declare the two candidates as temporary groups in `gateway/config.yaml` (research.md R10 shows the
shape), `docker compose restart litellm`, then compare the **groups** — the flag takes task keys, not
model ids:

```sh
cd apps/api
go test -tags eval_live ./internal/generation/application/ \
  -run TestLiveComparison \
  -eval.models generation-summary-candidate-a,generation-summary-candidate-b
```

Remove the candidate groups once the winner is pinned. Left behind, they are unrequested groups and
the FR-009 guardrail fails the build — which is how this scaffolding is prevented from becoming
permanent.

Record the artifact beside the assignment table. Do the same for each quality-writing scenario with a
corpus. `outreach` has none — generate the same three drafts under both candidates and record a dated
side-by-side note instead, labelled as a judgement rather than a score (research.md R10).

---

## Rollback

`make migrate-down` retypes the columns to 768 and nulls them; the vectors do not come back either
way. Restoring the previous behaviour is a code revert, not a configuration change — which is the
honest consequence of deleting the second path, and is stated here so nobody discovers it during an
incident.

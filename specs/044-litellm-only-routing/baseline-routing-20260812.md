# SC-004/SC-005 routing baseline — before 044-litellm-only-routing (2026-08-12)

Captured against the CURRENT (unmodified) code on `master` at commit `3b515a5`, with the real
`litellm` container (port 4000) and Postgres/Redis already up, and `GATEWAY_URL`/`LITELLM_MASTER_KEY`
configured in `.env` (so `internal/queue/policy.go`'s `TaskPolicy`/`internal/cmd/server/compose.go`
wiring routes through the gateway, not local Ollama).

## How this was captured

`apps/api/cmd/llmsmoke/main.go` was read first, as instructed, and turned out to be unusable for
this task: its `run()` calls `llm.New(cfg)` (`apps/api/internal/platform/llm/llm.go:97`), which
**always** returns a plain `ollama.New(...)` provider regardless of `GATEWAY_URL` —

```go
func New(cfg *config.Config) (Provider, error) {
	return ollama.New(cfg.OllamaURL, cfg.OllamaKey, cfg.LLMModel, cfg.EmbedModel, cfg.EmbedURL), nil
}
```

— so it never reaches the gateway adapter and never emits the `served_model` structured log line
this task needs. The real routing only happens through `llm.Router` (`internal/platform/llm/application/router.go`),
which `cmd/server/compose.go` constructs per task key and which is what the live server actually
uses.

Rather than driving the running HTTP server (queued/async, harder to correlate 1:1 with a single
task key) or leaving temporary scaffolding in the tree, a small temporary program was added at
`apps/api/cmd/routingsmoke044/main.go`, deleted immediately after this capture. It:

- calls `llm.NewProviders(cfg)` and `llm.NewRouter(taskKey, gateway, local, model)` exactly as
  `cmd/server/compose.go` does for `match`, `generation` and `default`,
- issues one `Complete(ctx, "Reply with the single word: pong", nil)` per router,
- relies on the gateway adapter's existing `logServed` call
  (`internal/platform/llm/infrastructure/gateway/gateway.go:419`) to emit the `"gateway request"`
  slog line with `task`, `requested_group`, `served_model`, `duration_ms`, `outcome`,
  `litellm_model_id`.

Command run (from `apps/api/`, after the binary's own `.env` loader picked up the repo's `.env`):

```
go run ./cmd/routingsmoke044
```

## Finding: salary, outreach and recruiter do not have distinct task keys today

This is the single most important thing this baseline records. Reading
`apps/api/cmd/server/compose.go` shows salary, outreach and recruiter are **all** wired to the same
`DefaultRouter` (task key `"default"`):

```go
DefaultRouter: llm.NewRouter("default", gatewayIface, ollamaProvider, cfg.LLMModel),
...
salaryService := salary.NewService(p.DB.Queries, defaultRouter, levelsFyiLoader, "")
recruiterSvc := recruiter.NewService(p.DB.Queries, defaultRouter, "", p.Scraping, p.Config.LinkedInScrapeEnabled)
outreachSvc := outreach.NewService(recruiterSvc, companyIntelSvc, defaultRouter, "")
```

`internal/queue/policy.go` confirms the same for the async worker path: `TypeSalaryInfer`'s
`TaskPolicy.LLMTaskKey` is `"default"`, and there is no `TypeOutreach`/`TypeRecruiter` policy or
`LLMTaskKey` at all — outreach and recruiter aren't queue-driven scenarios in the current code, they
run inline off HTTP requests, also through `defaultRouter`. `gateway/config.yaml`'s `model_list` has
no `salary`, `outreach` or `recruiter` group; it has `match`, `generation` (+ its stages), `rephrase`,
`ghost`, and `default`.

This is exactly the gap 044/US2 (per-scenario model assignment) exists to close. The before-side of
SC-005 ("salary, outreach and recruiter are observably served by independently changeable models")
is: **they are not** — today all three are one indistinguishable task key, so changing "the salary
model" today necessarily also changes the outreach and recruiter model, because there is only one
knob for all three.

## Captured `served_model` log lines

```
=== match ===
task=match requested_group=match served_model=cerebras/gpt-oss-120b duration_ms=261 outcome=ok litellm_model_id=05ae17f5df18f5afbaaf83a539d072a4096f97b9bc81ee61df591b90eded7f7e

=== generation ===
task=generation requested_group=generation served_model=openrouter/anthropic/claude-haiku-4.5 duration_ms=1547 outcome=ok litellm_model_id=49c7f31e9f46d6c4e7e8ffd8b5212546e0c9a7ae9eb8e0411c1a343514c21a25

=== salary (routes via shared "default" task key) ===
task=default requested_group=default served_model=cerebras/gpt-oss-120b duration_ms=115753 outcome=ok litellm_model_id=254f0230e2e4963c4aceafdf54270a502f51bf137f2d09590c580d353632b11d

=== outreach (routes via shared "default" task key) ===
task=default requested_group=default served_model=cerebras/gpt-oss-120b duration_ms=451 outcome=ok litellm_model_id=254f0230e2e4963c4aceafdf54270a502f51bf137f2d09590c580d353632b11d

=== recruiter (routes via shared "default" task key) ===
task=default requested_group=default served_model=cerebras/gpt-oss-120b duration_ms=252 outcome=ok litellm_model_id=254f0230e2e4963c4aceafdf54270a502f51bf137f2d09590c580d353632b11d
```

(The 115753ms duration on the first `default` call is a cold-start/rate-limit artifact of Cerebras,
not something meaningful to the routing question — the two subsequent `default` calls in the same
run came back in ~250-450ms from what is presumably the same warmed connection/cache.)

## Summary table

| Scenario  | Task key sent to gateway | Served model               | Distinct from the other two? |
|-----------|---------------------------|-----------------------------|-------------------------------|
| match     | `match`                    | `cerebras/gpt-oss-120b`     | n/a (own key)                 |
| generation| `generation`                | `openrouter/anthropic/claude-haiku-4.5` | n/a (own key)   |
| salary    | `default`                   | `cerebras/gpt-oss-120b`     | **No** — shares key+model with outreach and recruiter |
| outreach  | `default`                   | `cerebras/gpt-oss-120b`     | **No** — shares key+model with salary and recruiter |
| recruiter | `default`                   | `cerebras/gpt-oss-120b`     | **No** — shares key+model with salary and outreach |

## Gaps / what could not be captured differently

Nothing was fabricated. All five lines above are real gateway responses from the live `litellm`
container captured in this run. The one caveat worth being explicit about: since salary, outreach
and recruiter share one task key in the current code, there is no way to capture three
*independently* distinct served-model lines for them pre-migration — the correct and honest
before-baseline is that they are indistinguishable today, which is what SC-004/SC-005's after-side
comparison needs to show changed.

Cleanup: `apps/api/cmd/routingsmoke044/` was a temporary scaffold for this capture and has been
deleted; it must not appear in the working tree or any commit.

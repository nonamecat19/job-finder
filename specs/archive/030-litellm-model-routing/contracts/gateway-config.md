# Contract: Gateway Routing Configuration

**File**: `gateway/config.yaml` (mounted read-only into the `litellm` service)
**Supersedes**: `specs/029-litellm-proxy-gateway/contracts/gateway-config.md`

## C1 — Request contract (Go app → proxy)

- `POST {GATEWAY_URL}/chat/completions`, `Authorization: Bearer {LITELLM_MASTER_KEY}`.
- `model` is **always** one of `match`, `generation`, `rephrase`, `ghost`, `default`. Never a provider or upstream model name.
- The app sends `temperature`, optional `max_completion_tokens`, and `response_format: {"type":"json_object"}` for structured calls. `stream` is always false.
- The proxy MUST accept every one of the five groups. An unknown group MUST fail loudly (4xx), never silently route to a default.

## C2 — Response contract (proxy → Go app)

- On success: OpenAI chat-completion shape with at least one choice. The `x-litellm-model-name` response header names the **resolved deployment model**; the body's `model` field is not reliably the resolved model on a primary-tier (no-fallback) hit (verified 2026-07-31 against `main-stable` — see research.md R4). The adapter reads the header first, the body field second, for FR-012 logging.
- On failure after the whole chain is exhausted: a non-2xx the adapter classifies through `infrastructure/shared` (`ErrRateLimited`, `ErrCredentialRejected`, `ErrProviderUnavailable`, …). Existing worker retry/skip semantics are unchanged.

## C3 — Group and chain requirements

For every task key there MUST be:
1. A public group named exactly the task key, whose deployment is a **free-tier** provider.
2. An ordered fallback list, declared in `litellm_settings.fallbacks`, of tier groups.

Order constraints (MUST hold for every task):
- All free-tier tiers (Cerebras, Groq, Cohere) come before any OpenRouter tier — FR-006.
- The final tier is the Ollama deployment — FR-008.
- No chain is empty and no chain terminates on a hosted provider.

Shape (illustrative — the constraints above are what must hold, not the literal keys):

```yaml
general_settings:
  master_key: os.environ/LITELLM_MASTER_KEY

model_list:
  - model_name: match                       # public group, free tier 1
    litellm_params:
      model: cerebras/<verified-model>
      api_key: os.environ/CEREBRAS_API_KEY
  - model_name: match-groq                  # free tier 2
    litellm_params:
      model: groq/<verified-model>
      api_key: os.environ/GROQ_API_KEY
  - model_name: match-cohere                # free tier 3
    litellm_params:
      model: cohere_chat/<verified-model>
      api_key: os.environ/COHERE_API_KEY
  - model_name: match-openrouter            # aggregator
    litellm_params:
      model: openrouter/<verified-model>
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: local                       # terminal tier, shared by all tasks
    litellm_params:
      model: ollama_chat/<LLM_MODEL>
      api_base: os.environ/OLLAMA_URL
      api_key: os.environ/OLLAMA_KEY
  # … same five-tier pattern for generation, rephrase, ghost, default

litellm_settings:
  drop_params: true
  num_retries: 1
  request_timeout: 110          # < the adapter's 120s client timeout
  allowed_fails: 3
  cooldown_time: 60
  fallbacks:
    - match:      [match-groq, match-cohere, match-openrouter, local]
    - generation: [generation-groq, generation-cohere, generation-openrouter, local]
    - rephrase:   [rephrase-groq, rephrase-cohere, rephrase-openrouter, local]
    - ghost:      [ghost-groq, ghost-cohere, ghost-openrouter, local]
    - default:    [default-groq, default-cohere, default-openrouter, local]
```

## C4 — Credential handling

- Every `api_key` is an `os.environ/…` reference. No literal key may appear in the file.
- The `litellm` service in `docker-compose.yml` MUST pass `CEREBRAS_API_KEY`, `GROQ_API_KEY`, `COHERE_API_KEY`, `OPENROUTER_API_KEY`, `OLLAMA_URL`, `OLLAMA_KEY`, `LITELLM_MASTER_KEY`, each with an empty default (`${VAR:-}`), so a missing variable never aborts config load (FR-011).
- An empty key produces an auth failure at request time, which advances the chain (FR-007).

## C5 — Capability requirement

Every model listed in a chain for a JSON-consuming task MUST support `response_format: {"type":"json_object"}`. `drop_params: true` means an unsupported param is dropped silently, so a non-JSON-capable model degrades into prose the app cannot parse — a failure mode the chain will *not* rescue. Model IDs are verified against each provider's live catalog at implementation time and pinned with a dated comment.

## C6 — Change contract

Changing which model serves a task MUST require only: edit this file → `docker compose restart litellm`. No application rebuild, no migration, no dashboard action (FR-005, SC-003).

# Contract: LiteLLM Proxy Gateway Configuration

**Feature**: 029-litellm-proxy-gateway
**Date**: 2026-07-31

## Overview

The LiteLLM proxy is configured via a YAML file (`gateway/config.yaml`) mounted into the container. This contract defines the required structure, the model naming convention, and the environment variables the proxy expects.

## Config File Contract

### Location

`gateway/config.yaml` at the repository root. Mounted into the container at `/app/config.yaml`.

### Schema

```yaml
general_settings:
  master_key: os.environ/LITELLM_MASTER_KEY  # required: proxy auth key

model_list:
  - model_name: <task-key>         # required: one of match|generation|rephrase|ghost|default
    litellm_params:
      model: <provider/model-id>   # required: LiteLLM provider prefix + model ID
      api_key: os.environ/<VAR>     # required: env var holding the provider's API key
      # Optional fallback chain (FR-005):
      fallbacks:
        - <provider/fallback-model-id>

litellm_settings:
  drop_params: true                 # recommended: strip unknown params before forwarding
  set_verbose: false                # recommended: reduce log noise
```

### Task Key to Model Mapping Convention

| Task Key | LiteLLM `model_name` | Purpose |
|---|---|---|
| `match` | `match` | Job matching/scoring |
| `generation` | `generation` | Resume/cover letter generation |
| `rephrase` | `rephrase` | Keyword rephrase suggestions |
| `ghost` | `ghost` | Ghost job detection |
| `default` | `default` | Salary, outreach, recruiter extraction |

The Go backend sends the task key as the `model` field in the OpenAI-compatible request. The proxy resolves it to the actual provider+model.

### Provider Prefixes

| Provider | LiteLLM Prefix | Example |
|---|---|---|
| OpenRouter | `openrouter/` | `openrouter/deepseek/deepseek-v4-pro` |
| Cerebras | `cerebras/` | `cerebras/gpt-oss-120b` |
| OpenAI | `openai/` | `openai/gpt-4o-mini` |
| Anthropic | `anthropic/` | `anthropic/claude-sonnet-4` |
| Google | `gemini/` | `gemini/gemini-2.5-flash` |
| Groq | `groq/` | `groq/llama-3.3-70b` |

### Example: Production Config

```yaml
general_settings:
  master_key: os.environ/LITELLM_MASTER_KEY

model_list:
  - model_name: match
    litellm_params:
      model: openrouter/deepseek/deepseek-v4-pro
      api_key: os.environ/OPENROUTER_API_KEY
      fallbacks:
        - cerebras/gpt-oss-120b

  - model_name: generation
    litellm_params:
      model: openrouter/qwen/qwen3.7-max
      api_key: os.environ/OPENROUTER_API_KEY
      fallbacks:
        - cerebras/gpt-oss-120b

  - model_name: rephrase
    litellm_params:
      model: openrouter/openai/gpt-4o-mini
      api_key: os.environ/OPENROUTER_API_KEY

  - model_name: ghost
    litellm_params:
      model: openrouter/deepseek/deepseek-v4-pro
      api_key: os.environ/OPENROUTER_API_KEY
      fallbacks:
        - cerebras/gpt-oss-120b

  - model_name: default
    litellm_params:
      model: openrouter/openai/gpt-4o-mini
      api_key: os.environ/OPENROUTER_API_KEY

litellm_settings:
  drop_params: true
  set_verbose: false
```

## Environment Variables

The proxy container expects these environment variables (set in `docker-compose.yml` or `.env`):

| Variable | Required | Purpose |
|---|---|---|
| `LITELLM_MASTER_KEY` | Yes | Authenticates the Go backend to the proxy |
| `OPENROUTER_API_KEY` | If using OpenRouter models | OpenRouter API key |
| `CEREBRAS_API_KEY` | If using Cerebras as fallback | Cerebras API key |

## API Contract (Go Backend → Proxy)

The Go backend's `GatewayProvider` sends standard OpenAI-compatible requests:

### Chat Completion Request

```http
POST /v1/chat/completions
Authorization: Bearer <LITELLM_MASTER_KEY>
Content-Type: application/json

{
  "model": "<task-key>",
  "messages": [
    {"role": "system", "content": "<system prompt>"},
    {"role": "user", "content": "<user prompt>"}
  ],
  "temperature": 0.3,
  "stream": false,
  "response_format": {"type": "json_object"}
}
```

### Chat Completion Response

```json
{
  "choices": [
    {
      "message": {
        "content": "<model response>"
      }
    }
  ],
  "usage": {
    "prompt_tokens": 1234,
    "completion_tokens": 567,
    "total_tokens": 1801
  }
}
```

### Error Response

```json
{
  "error": {
    "message": "<human-readable error>",
    "type": "<error type>",
    "code": 429
  }
}
```

HTTP status codes map to Go sentinel errors per the existing `classifyProviderError` taxonomy:
- 429 → `ErrRateLimited`
- 401/403 → `ErrCredentialRejected`
- 402 → `ErrInsufficientCredits`
- 404/400/422 → `ErrModelUnavailable`
- 5xx → `ErrProviderUnavailable`

## Hot Reload

The proxy supports config reload without restart:

```bash
# Via the proxy's admin API (if enabled):
curl -X POST http://litellm:4000/config/reload -H "Authorization: Bearer $LITELLM_MASTER_KEY"
```

Alternatively, restart the container: `docker compose restart litellm`

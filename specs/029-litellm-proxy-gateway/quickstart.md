# Quickstart: LiteLLM Proxy Gateway

**Feature**: 029-litellm-proxy-gateway
**Date**: 2026-07-31

## Prerequisites

- Docker and Docker Compose installed
- OpenRouter API key (get one at https://openrouter.ai/keys)
- Existing job-finder stack running (`docker compose up -d`)

## 1. Configure Environment

Add to `.env`:

```bash
# LiteLLM proxy
GATEWAY_URL=http://litellm:4000
LITELLM_MASTER_KEY=sk-litellm-proxy-dev-key
OPENROUTER_API_KEY=sk-or-v1-your-key-here
```

## 2. Create Gateway Config

Create `gateway/config.yaml`:

```yaml
general_settings:
  master_key: os.environ/LITELLM_MASTER_KEY

model_list:
  - model_name: match
    litellm_params:
      model: openrouter/deepseek/deepseek-v4-pro
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: generation
    litellm_params:
      model: openrouter/qwen/qwen3.7-max
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: rephrase
    litellm_params:
      model: openrouter/openai/gpt-4o-mini
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: ghost
    litellm_params:
      model: openrouter/deepseek/deepseek-v4-pro
      api_key: os.environ/OPENROUTER_API_KEY
  - model_name: default
    litellm_params:
      model: openrouter/openai/gpt-4o-mini
      api_key: os.environ/OPENROUTER_API_KEY

litellm_settings:
  drop_params: true
  set_verbose: false
```

## 3. Start the Stack

```bash
docker compose up -d
```

Wait for the litellm service to be healthy:

```bash
docker compose ps litellm
# Should show "healthy" after ~40s
```

## 4. Verify Proxy Health

```bash
curl http://localhost:4000/health/liveliness
# Expected: {"status": "healthy"}
```

## 5. Test Proxy Directly

```bash
curl -s http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer sk-litellm-proxy-dev-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "default",
    "messages": [{"role": "user", "content": "Say hello in one word."}],
    "stream": false
  }' | jq '.choices[0].message.content'
# Expected: "Hello" (or similar)
```

## 6. Switch a Task to Gateway

```bash
# Set the "match" task to use the gateway provider
curl -X PUT http://localhost:3000/v1/settings/llm \
  -H "Content-Type: application/json" \
  -d '{
    "tasks": [
      {"taskKey": "match", "provider": "gateway", "model": "match"}
    ]
  }'
```

## 7. Trigger an AI Task Through the Gateway

```bash
# Score a job posting (requires a job in the database)
curl -X POST http://localhost:3000/v1/jobs/<job-id>/score
```

Check the proxy logs to confirm routing:

```bash
docker compose logs litellm | grep "match"
# Expected: log line showing the request routed to openrouter/deepseek/deepseek-v4-pro
```

## 8. Verify Embeddings Stay Local

```bash
# Check that embedding calls don't appear in proxy logs
docker compose logs litellm | grep "embed"
# Expected: no embedding-related log lines
```

## 9. Verify Existing Providers Still Work

```bash
# Switch match back to Ollama
curl -X PUT http://localhost:3000/v1/settings/llm \
  -H "Content-Type: application/json" \
  -d '{
    "tasks": [
      {"taskKey": "match", "provider": "ollama", "model": ""}
    ]
  }'

# Trigger a match — should use local Ollama
curl -X POST http://localhost:3000/v1/jobs/<job-id>/score
```

## 10. Test Fallback (Optional)

Edit `gateway/config.yaml` to add a fallback chain:

```yaml
  - model_name: match
    litellm_params:
      model: openrouter/deepseek/deepseek-v4-pro
      api_key: os.environ/OPENROUTER_API_KEY
      fallbacks:
        - cerebras/gpt-oss-120b
```

Reload the proxy config:

```bash
docker compose restart litellm
```

To test fallback, temporarily use an invalid OpenRouter key and verify the request falls through to Cerebras.

## Validation Checklist

- [ ] `docker compose ps litellm` shows healthy
- [ ] `curl localhost:4000/health/liveliness` returns healthy
- [ ] Direct proxy chat completion works
- [ ] Dashboard Settings shows "Gateway" as a provider option
- [ ] Setting a task to Gateway persists across page reload
- [ ] AI task completes through the gateway (check proxy logs)
- [ ] Embedding calls do not appear in proxy logs
- [ ] Switching back to Ollama/Cerebras works immediately
- [ ] `make test-lint` passes (no regressions)

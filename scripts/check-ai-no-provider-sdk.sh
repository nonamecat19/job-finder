#!/usr/bin/env bash
#
# Fail if the Python dependency tree gains a provider-specific SDK (C7-3,
# FR-011). The AI service reaches every model through the LiteLLM gateway's
# OpenAI-compatible endpoint (research R3) — `openai` (and its adapter,
# `langchain-openai`) is therefore not itself a violation: it is the generic,
# provider-agnostic HTTP client pointed at GATEWAY_URL, the same pattern the
# Go backend already uses (029-FR-011). What must never appear is a package
# whose entire purpose is talking to one named provider directly, which would
# be a code path that *could* read a provider credential.
#
# Reads apps/ai/uv.lock rather than an installed environment, so it needs no
# `uv sync` and cannot be fooled by a stale local venv.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOCKFILE="$REPO_ROOT/apps/ai/uv.lock"

if [[ ! -f "$LOCKFILE" ]]; then
  echo "::error::$LOCKFILE not found" >&2
  exit 2
fi

# Provider-specific SDKs. NOT here, deliberately: `openai` — see header.
BANNED_PACKAGES=(
  anthropic
  google-generativeai
  google-genai
  google-cloud-aiplatform
  cohere
  mistralai
  groq
  ai21
  together
  replicate
  boto3
  botocore
  huggingface-hub
)

found=()
for pkg in "${BANNED_PACKAGES[@]}"; do
  if grep -qiE "^name = \"${pkg}\"\$" "$LOCKFILE"; then
    found+=("$pkg")
  fi
done

if ((${#found[@]} > 0)); then
  echo "::error::provider SDK(s) found in apps/ai's dependency tree: ${found[*]}" >&2
  echo "The AI service must reach every model through the LiteLLM gateway (FR-009, FR-011)." >&2
  exit 1
fi

echo "No provider SDK in apps/ai's dependency tree."

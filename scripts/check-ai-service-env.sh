#!/usr/bin/env bash
#
# Fail if the `ai` service's environment in either compose file gains
# `DATABASE_URL`, any Postgres credential, or any provider credential
# (K2-2, K2-3, FR-008). The AI service reads no database and holds no
# inference credential beyond `LITELLM_MASTER_KEY` — the same permission the
# backend already has (K2-4).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Keys the `ai` service block may legitimately reference (K2-1). Anything
# else naming a credential or a database is a violation.
FORBIDDEN_PATTERN='DATABASE_URL|POSTGRES_|DB_PASSWORD|DB_USER|CEREBRAS_API_KEY|GROQ_API_KEY|COHERE_API_KEY|OPENROUTER_API_KEY|OPENAI_API_KEY|ANTHROPIC_API_KEY|GOOGLE_API_KEY|AZURE_API_KEY'

check_file() {
  local file="$1"
  # Isolate the `ai:` service block: from its own top-level key to the next
  # top-level (2-space-indented) key, or end of file.
  local block
  block=$(awk '/^  ai:/{flag=1; print; next} /^  [A-Za-z]/{flag=0} flag' "$file")
  if [[ -z "$block" ]]; then
    echo "::error::no 'ai:' service found in $file" >&2
    return 2
  fi
  if grep -qE "$FORBIDDEN_PATTERN" <<<"$block"; then
    echo "::error::$file's ai service references a forbidden variable:" >&2
    grep -E "$FORBIDDEN_PATTERN" <<<"$block" >&2
    return 1
  fi
}

status=0
for f in docker-compose.yml docker-compose.prod.yml; do
  check_file "$REPO_ROOT/$f" || status=1
done

if ((status != 0)); then
  echo "The ai service must not receive a database credential or a provider credential (K2-2, K2-3)." >&2
  exit 1
fi

echo "ai service environment clean in docker-compose.yml and docker-compose.prod.yml."

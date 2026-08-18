"""Startup configuration for the AI service.

Validates presence and shape of required environment variables only — never
reachability (K3-1, K3-2). An unreachable broker or gateway must fail work,
not the boot.
"""

from __future__ import annotations

import os
from collections.abc import Mapping
from functools import lru_cache

from pydantic import BaseModel

REQUIRED_ENV_KEYS: tuple[str, ...] = (
    "GATEWAY_URL",
    "LITELLM_MASTER_KEY",
    "RABBITMQ_URL",
    "AI_SERVICE_TOKEN",
)


class ConfigurationError(RuntimeError):
    """Raised at startup when required configuration is missing (K3-1)."""


class Settings(BaseModel):
    gateway_url: str
    litellm_master_key: str
    rabbitmq_url: str
    ai_service_token: str
    langfuse_public_key: str | None = None
    langfuse_secret_key: str | None = None
    langfuse_host: str | None = None
    log_level: str = "INFO"


def load_settings(env: Mapping[str, str] | None = None) -> Settings:
    """Build Settings from `env` (defaults to the process environment).

    Fails naming every missing required key at once, rather than the first
    one found, so a misconfigured stack is fixed in one pass.
    """
    source: Mapping[str, str] = env if env is not None else os.environ
    missing = [key for key in REQUIRED_ENV_KEYS if not source.get(key)]
    if missing:
        raise ConfigurationError("missing required environment variable(s): " + ", ".join(missing))
    return Settings(
        gateway_url=source["GATEWAY_URL"],
        litellm_master_key=source["LITELLM_MASTER_KEY"],
        rabbitmq_url=source["RABBITMQ_URL"],
        ai_service_token=source["AI_SERVICE_TOKEN"],
        langfuse_public_key=source.get("LANGFUSE_PUBLIC_KEY") or None,
        langfuse_secret_key=source.get("LANGFUSE_SECRET_KEY") or None,
        langfuse_host=source.get("LANGFUSE_HOST") or None,
        log_level=source.get("LOG_LEVEL") or "INFO",
    )


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    """Process-wide singleton, loaded from the real environment on first use."""
    return load_settings()

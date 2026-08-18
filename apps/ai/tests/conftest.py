"""Test-only defaults for the required environment variables (K3-1).

`jobfinder_ai.settings.get_settings()` is called at import time by
`jobfinder_ai.main`, so these must be set before any test collects a module
that imports it. Real values are never read from here — only shape.
"""

import os

_TEST_ENV = {
    "GATEWAY_URL": "http://litellm:4000",
    "LITELLM_MASTER_KEY": "sk-test",
    "RABBITMQ_URL": "amqp://guest:guest@localhost:5672/",
    "AI_SERVICE_TOKEN": "test-shared-secret",
}

for _key, _value in _TEST_ENV.items():
    os.environ.setdefault(_key, _value)

import pytest

from jobfinder_ai.settings import ConfigurationError, load_settings

FULL_ENV = {
    "GATEWAY_URL": "http://litellm:4000",
    "LITELLM_MASTER_KEY": "sk-test",
    "RABBITMQ_URL": "amqp://ai_service:pw@rabbitmq:5672/",
    "AI_SERVICE_TOKEN": "shared-secret",
}


def test_load_settings_succeeds_with_every_required_key() -> None:
    settings = load_settings(FULL_ENV)
    assert settings.gateway_url == FULL_ENV["GATEWAY_URL"]
    assert settings.litellm_master_key == FULL_ENV["LITELLM_MASTER_KEY"]
    assert settings.rabbitmq_url == FULL_ENV["RABBITMQ_URL"]
    assert settings.ai_service_token == FULL_ENV["AI_SERVICE_TOKEN"]
    assert settings.langfuse_public_key is None


@pytest.mark.parametrize("missing_key", sorted(FULL_ENV))
def test_load_settings_fails_naming_the_missing_key(missing_key: str) -> None:
    env = {k: v for k, v in FULL_ENV.items() if k != missing_key}
    with pytest.raises(ConfigurationError) as exc_info:
        load_settings(env)
    assert missing_key in str(exc_info.value)


def test_load_settings_fails_naming_every_missing_key_at_once() -> None:
    with pytest.raises(ConfigurationError) as exc_info:
        load_settings({})
    message = str(exc_info.value)
    for key in FULL_ENV:
        assert key in message


def test_load_settings_treats_empty_string_as_missing() -> None:
    env = {**FULL_ENV, "AI_SERVICE_TOKEN": ""}
    with pytest.raises(ConfigurationError) as exc_info:
        load_settings(env)
    assert "AI_SERVICE_TOKEN" in str(exc_info.value)


def test_load_settings_never_probes_reachability(monkeypatch: pytest.MonkeyPatch) -> None:
    """K3-2: an unresolvable-looking URL is still accepted at load time."""
    env = {**FULL_ENV, "GATEWAY_URL": "http://does-not-exist.invalid:4000"}
    settings = load_settings(env)
    assert settings.gateway_url == "http://does-not-exist.invalid:4000"

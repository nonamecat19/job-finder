from jobfinder_ai.gateway import REQUEST_TIMEOUT_SECONDS, chat_model, embeddings_model
from jobfinder_ai.settings import Settings

SETTINGS = Settings(
    gateway_url="http://litellm:4000",
    litellm_master_key="sk-test",
    rabbitmq_url="amqp://ai_service:pw@rabbitmq:5672/",
    ai_service_token="shared-secret",
)

GATEWAY_REQUEST_TIMEOUT_SECONDS = 60


def test_request_timeout_exceeds_the_gateways_own_timeout() -> None:
    assert REQUEST_TIMEOUT_SECONDS > GATEWAY_REQUEST_TIMEOUT_SECONDS


def test_chat_model_is_selected_by_task_key_only() -> None:
    model = chat_model("ghost", settings=SETTINGS)
    assert model.model_name == "ghost"
    assert str(model.openai_api_base) == SETTINGS.gateway_url
    assert model.max_retries == 0
    assert model.request_timeout == REQUEST_TIMEOUT_SECONDS


def test_embeddings_model_is_selected_by_task_key_only() -> None:
    model = embeddings_model("embed", settings=SETTINGS)
    assert model.model == "embed"
    assert str(model.openai_api_base) == SETTINGS.gateway_url
    assert model.max_retries == 0


def test_chat_model_never_reads_a_provider_credential() -> None:
    model = chat_model("match", settings=SETTINGS)
    assert model.openai_api_key is not None
    assert model.openai_api_key.get_secret_value() == SETTINGS.litellm_master_key

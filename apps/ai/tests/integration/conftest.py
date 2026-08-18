"""Docker-backed RabbitMQ for the broker contract tests.

Everything else under `tests/integration/` runs in-process against stubs,
which can prove how this service's own functions compose but can never prove
anything about the broker: whether the topology this side declares is
actually compatible with the one `apps/api/internal/events/topology.go`
declares, what RabbitMQ really does with a delivery whose handler raised, or
whether a result lands on the exchange the Go consumer reads. Those facts
only exist on a real broker, so these tests bring one up.

There is deliberately no skip path. A missing or broken Docker daemon fails
the suite exactly the way the Go side's container-backed tests do — a test
that silently disappears when the environment is wrong protects nothing.

Isolation is per test, by vhost: each test gets a freshly created, empty
vhost on the one shared container, so a test that declares topology "Go side
first" is genuinely first, and no test can see another's queues.
"""

from __future__ import annotations

import base64
import json
import re
from collections.abc import Callable, Iterator
from typing import Any
from urllib.parse import quote
from urllib.request import Request, urlopen

import pytest
from testcontainers.community.rabbitmq import RabbitMqContainer

RABBITMQ_IMAGE = "rabbitmq:4.3.4-management-alpine"
MANAGEMENT_PORT = 15672


@pytest.fixture(scope="session", name="rabbitmq_container")
def _rabbitmq_container() -> Iterator[RabbitMqContainer]:
    container = RabbitMqContainer(RABBITMQ_IMAGE)
    container.with_exposed_ports(MANAGEMENT_PORT)
    container.start()
    try:
        yield container
    finally:
        container.stop()


def _exec(container: RabbitMqContainer, *args: str) -> None:
    exit_code, output = container.exec(list(args))
    if exit_code != 0:
        raise RuntimeError(f"{' '.join(args)} failed ({exit_code}): {output!r}")


@pytest.fixture(name="vhost")
def _vhost(rabbitmq_container: RabbitMqContainer, request: pytest.FixtureRequest) -> Iterator[str]:
    """A fresh, empty vhost for this test, dropped again afterwards."""
    name = re.sub(r"[^A-Za-z0-9_]", "_", request.node.name)[:200]
    _exec(rabbitmq_container, "rabbitmqctl", "add_vhost", name)
    _exec(
        rabbitmq_container,
        "rabbitmqctl",
        "set_permissions",
        "-p",
        name,
        rabbitmq_container.username,
        ".*",
        ".*",
        ".*",
    )
    try:
        yield name
    finally:
        _exec(rabbitmq_container, "rabbitmqctl", "delete_vhost", name)


@pytest.fixture(name="broker_url")
def _broker_url(rabbitmq_container: RabbitMqContainer, vhost: str) -> str:
    """An `amqp://` URL pointing at this test's own vhost."""
    params = rabbitmq_container.get_connection_params()
    return (
        f"amqp://{rabbitmq_container.username}:{rabbitmq_container.password}"
        f"@{params.host}:{params.port}/{vhost}"
    )


@pytest.fixture(name="queue_arguments")
def _queue_arguments(
    rabbitmq_container: RabbitMqContainer, vhost: str
) -> Callable[[str], dict[str, Any]]:
    """Reads back the `arguments` RabbitMQ recorded for a queue.

    A passive AMQP declare answers "does it exist", never "with what
    arguments", so the only way to check a queue's real definition against
    what `consumers.py` claims is the management HTTP API.
    """
    host = rabbitmq_container.get_container_host_ip()
    port = rabbitmq_container.get_exposed_port(MANAGEMENT_PORT)
    credentials = f"{rabbitmq_container.username}:{rabbitmq_container.password}"
    authorization = base64.b64encode(credentials.encode()).decode()

    def read(queue_name: str) -> dict[str, Any]:
        url = (
            f"http://{host}:{port}/api/queues/{quote(vhost, safe='')}/{quote(queue_name, safe='')}"
        )
        request = Request(url, headers={"Authorization": f"Basic {authorization}"})  # noqa: S310
        with urlopen(request, timeout=10) as response:  # noqa: S310
            payload = json.load(response)
        arguments = payload["arguments"]
        assert isinstance(arguments, dict)
        return dict(arguments)

    return read

import sys
import types
from pathlib import Path
from unittest.mock import MagicMock

import pandas as pd
import pytest
from fastapi.testclient import TestClient

# Make ``from main import …`` work when pytest runs from the tests/ dir.
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from main import app  # noqa: E402


# ---------------------------------------------------------------------------
# Lightweight fake ``jobspy`` module so the ``from jobspy import scrape_jobs``
# inside ``/search`` resolves without the real package installed.
# ---------------------------------------------------------------------------
_fake_jobspy = types.ModuleType("jobspy")
_fake_jobspy.scrape_jobs = MagicMock()  # type: ignore[attr-defined]
sys.modules.setdefault("jobspy", _fake_jobspy)


@pytest.fixture()
def client():
    """Yield a fresh ``TestClient`` for every test."""
    return TestClient(app, raise_server_exceptions=False)


@pytest.fixture()
def mock_scrape(monkeypatch):
    """Patch ``jobspy.scrape_jobs`` and return the mock for easy configuration."""
    mock = MagicMock()
    monkeypatch.setattr("main.scrape_jobs", mock, raising=False)

    # Re-wire the lazy import inside /search by patching the module-level lookup
    import jobspy
    monkeypatch.setattr(jobspy, "scrape_jobs", mock)
    return mock


@pytest.fixture()
def sample_df():
    """Return a small DataFrame mimicking jobspy's output."""
    return pd.DataFrame(
        {
            "id": [101, 102],
            "site": ["linkedin", "indeed"],
            "title": ["Backend Dev", "Frontend Dev"],
            "company": ["Acme", "Globex"],
            "location": ["Remote", "NYC"],
            "is_remote": [True, False],
            "min_amount": [80000, 70000],
            "max_amount": [120000, 100000],
            "interval": ["yearly", "yearly"],
            "currency": ["USD", "USD"],
            "job_url": ["https://example.com/1", "https://example.com/2"],
            "description": ["Desc A", "Desc B"],
            "date_posted": ["2026-01-01", "2026-01-02"],
        }
    )


@pytest.fixture()
def empty_df():
    """Return an empty DataFrame (no results)."""
    return pd.DataFrame()


@pytest.fixture()
def df_with_nan():
    """DataFrame with NaN values to exercise the ``_get`` NaN guard."""
    import numpy as np

    return pd.DataFrame(
        {
            "id": [201],
            "site": ["glassdoor"],
            "title": ["ML Engineer"],
            "company": ["DeepMind"],
            "location": [np.nan],
            "is_remote": [np.nan],
            "min_amount": [90000],
            "max_amount": [np.nan],
            "interval": [np.nan],
            "currency": [np.nan],
            "job_url": ["https://example.com/3"],
            "description": ["Deep work"],
            "date_posted": [np.nan],
        }
    )


@pytest.fixture()
def df_missing_title():
    """DataFrame where one row has an empty title — should be filtered out."""
    return pd.DataFrame(
        {
            "id": [301, 302],
            "site": ["linkedin", "linkedin"],
            "title": ["", "Good Title"],
            "company": ["X", "Y"],
            "location": ["A", "B"],
            "is_remote": [False, False],
            "min_amount": [None, None],
            "max_amount": [None, None],
            "interval": [None, None],
            "currency": [None, None],
            "job_url": ["https://a.test", "https://b.test"],
            "description": [None, None],
            "date_posted": [None, None],
        }
    )


@pytest.fixture()
def df_no_salary():
    """DataFrame where salary columns are all None."""
    return pd.DataFrame(
        {
            "id": [401],
            "site": ["indeed"],
            "title": ["Designer"],
            "company": ["Startup"],
            "location": ["LA"],
            "is_remote": [False],
            "min_amount": [None],
            "max_amount": [None],
            "interval": [None],
            "currency": [None],
            "job_url": ["https://design.test/1"],
            "description": ["Creative role"],
            "date_posted": ["2026-06-01"],
        }
    )

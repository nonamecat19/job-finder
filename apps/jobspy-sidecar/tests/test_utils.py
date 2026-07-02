"""Tests for the salary-formatting logic extracted from the /search handler.

The inline ``_get`` + salary logic is currently embedded in the route.  These
tests exercise the same edge cases by verifying the *output* of the endpoint,
plus a few pure-logic checks on a standalone helper for easier future
refactoring.
"""
import numpy as np
import pandas as pd

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))


# ── Standalone helper mirrors the salary line built inside /search ──────────


def build_salary(row: pd.Series) -> str | None:
    """Reproduce the salary-building logic from main.py /search."""
    def _get(key):
        val = row.get(key)
        return None if val is None or (isinstance(val, float) and pd.isna(val)) else val

    min_a, max_a = _get("min_amount"), _get("max_amount")
    if not (min_a or max_a):
        return None
    interval = _get("interval") or ""
    return f"{min_a or '?'} – {max_a or '?'} {(_get('currency') or '')} {interval}".strip()


class TestBuildSalary:
    def test_both_amounts(self):
        row = pd.Series(
            {
                "min_amount": 80000,
                "max_amount": 120000,
                "interval": "yearly",
                "currency": "USD",
            }
        )
        s = build_salary(row)
        assert s == "80000 – 120000 USD yearly"

    def test_only_min(self):
        row = pd.Series(
            {"min_amount": 50000, "max_amount": None, "interval": "monthly", "currency": "EUR"}
        )
        s = build_salary(row)
        assert "50000" in s
        assert "?" in s
        assert "EUR" in s

    def test_only_max(self):
        row = pd.Series(
            {"min_amount": None, "max_amount": 200000, "interval": "yearly", "currency": "GBP"}
        )
        s = build_salary(row)
        assert "?" in s
        assert "200000" in s
        assert "GBP" in s

    def test_none_amounts_returns_none(self):
        row = pd.Series(
            {"min_amount": None, "max_amount": None, "interval": None, "currency": None}
        )
        assert build_salary(row) is None

    def test_nan_amounts_returns_none(self):
        row = pd.Series(
            {"min_amount": np.nan, "max_amount": np.nan, "interval": np.nan, "currency": np.nan}
        )
        assert build_salary(row) is None

    def test_nan_interval_treated_as_empty(self):
        row = pd.Series(
            {"min_amount": 100, "max_amount": 200, "interval": np.nan, "currency": "USD"}
        )
        s = build_salary(row)
        assert "USD" in s
        # interval is stripped away
        assert s == "100 – 200 USD"

    def test_zero_min_zero_max_returns_none(self):
        """Both zero should be treated as falsy → no salary."""
        row = pd.Series(
            {"min_amount": 0, "max_amount": 0, "interval": "yearly", "currency": "USD"}
        )
        # 0 is falsy in Python so the `if not (min_a or max_a)` fires.
        assert build_salary(row) is None


# ── NaN / None normalisation in _get ───────────────────────────────────────


class TestGetHelper:
    """The ``_get`` closure rejects NaN; verify via the endpoint output."""

    def test_nan_location_becomes_none(self, client, mock_scrape):
        df = pd.DataFrame(
            {
                "id": [1],
                "site": ["linkedin"],
                "title": ["Dev"],
                "company": ["X"],
                "location": [np.nan],
                "is_remote": [True],
                "min_amount": [None],
                "max_amount": [None],
                "interval": [None],
                "currency": [None],
                "job_url": ["https://x.test/1"],
                "description": [None],
                "date_posted": [None],
            }
        )
        mock_scrape.return_value = df
        resp = client.post("/search", json={"search_term": "dev"})
        assert resp.json()["jobs"][0]["location"] is None

    def test_nan_company_becomes_none(self, client, mock_scrape):
        df = pd.DataFrame(
            {
                "id": [1],
                "site": ["linkedin"],
                "title": ["Dev"],
                "company": [np.nan],
                "location": ["R"],
                "is_remote": [True],
                "min_amount": [None],
                "max_amount": [None],
                "interval": [None],
                "currency": [None],
                "job_url": ["https://x.test/1"],
                "description": [None],
                "date_posted": [None],
            }
        )
        mock_scrape.return_value = df
        resp = client.post("/search", json={"search_term": "dev"})
        assert resp.json()["jobs"][0]["company"] is None

    def test_nan_description_becomes_none(self, client, mock_scrape):
        df = pd.DataFrame(
            {
                "id": [1],
                "site": ["linkedin"],
                "title": ["Dev"],
                "company": ["X"],
                "location": ["R"],
                "is_remote": [True],
                "min_amount": [None],
                "max_amount": [None],
                "interval": [None],
                "currency": [None],
                "job_url": ["https://x.test/1"],
                "description": [np.nan],
                "date_posted": [None],
            }
        )
        mock_scrape.return_value = df
        resp = client.post("/search", json={"search_term": "dev"})
        assert resp.json()["jobs"][0]["description"] is None

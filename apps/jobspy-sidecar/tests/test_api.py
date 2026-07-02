"""Tests for FastAPI endpoints: /health and /search."""
from unittest.mock import MagicMock

import pandas as pd
import pytest


# ── /health ────────────────────────────────────────────────────────────────


class TestHealth:
    def test_returns_ok(self, client):
        resp = client.get("/health")
        assert resp.status_code == 200
        assert resp.json() == {"ok": True}


# ── /search ────────────────────────────────────────────────────────────────


class TestSearch:
    def _post(self, client, **overrides):
        body = {"search_term": "python", **overrides}
        return client.post("/search", json=body)

    # Happy paths ----------------------------------------------------------

    def test_basic_search(self, client, mock_scrape, sample_df):
        mock_scrape.return_value = sample_df
        resp = self._post(client)
        assert resp.status_code == 200
        data = resp.json()
        assert len(data["jobs"]) == 2
        assert data["jobs"][0]["title"] == "Backend Dev"
        assert data["jobs"][1]["title"] == "Frontend Dev"

    def test_passes_params_to_scrape_jobs(self, client, mock_scrape, empty_df):
        mock_scrape.return_value = empty_df
        self._post(
            client,
            site="indeed",
            search_term="rust dev",
            location="Berlin",
            is_remote=True,
            results_wanted=10,
            hours_old=48,
            country_indeed="de",
        )
        mock_scrape.assert_called_once_with(
            site_name=["indeed"],
            search_term="rust dev",
            location="Berlin",
            is_remote=True,
            results_wanted=10,
            hours_old=48,
            country_indeed="de",
            description_format="markdown",
        )

    def test_empty_dataframe_returns_empty_list(self, client, mock_scrape, empty_df):
        mock_scrape.return_value = empty_df
        resp = self._post(client)
        assert resp.status_code == 200
        assert resp.json() == {"jobs": []}

    def test_none_dataframe_returns_empty_list(self, client, mock_scrape):
        mock_scrape.return_value = None
        resp = self._post(client)
        assert resp.status_code == 200
        assert resp.json() == {"jobs": []}

    # Salary formatting ----------------------------------------------------

    def test_salary_formatted_with_both_amounts(self, client, mock_scrape, sample_df):
        mock_scrape.return_value = sample_df
        resp = self._post(client)
        salary = resp.json()["jobs"][0]["salary"]
        assert "80000" in salary
        assert "120000" in salary
        assert "USD" in salary
        assert "yearly" in salary

    def test_salary_with_only_min_amount(self, client, mock_scrape):
        df = pd.DataFrame(
            {
                "id": [1],
                "site": ["linkedin"],
                "title": ["Dev"],
                "company": ["X"],
                "location": ["R"],
                "is_remote": [True],
                "min_amount": [50000],
                "max_amount": [None],
                "interval": ["monthly"],
                "currency": ["EUR"],
                "job_url": ["https://x.test/1"],
                "description": [None],
                "date_posted": [None],
            }
        )
        mock_scrape.return_value = df
        resp = self._post(client)
        salary = resp.json()["jobs"][0]["salary"]
        assert "50000" in salary
        assert "?" in salary  # max is None → '?'
        assert "EUR" in salary

    def test_salary_with_only_max_amount(self, client, mock_scrape):
        df = pd.DataFrame(
            {
                "id": [1],
                "site": ["linkedin"],
                "title": ["Dev"],
                "company": ["X"],
                "location": ["R"],
                "is_remote": [True],
                "min_amount": [None],
                "max_amount": [200000],
                "interval": ["yearly"],
                "currency": ["GBP"],
                "job_url": ["https://x.test/1"],
                "description": [None],
                "date_posted": [None],
            }
        )
        mock_scrape.return_value = df
        resp = self._post(client)
        salary = resp.json()["jobs"][0]["salary"]
        assert "?" in salary  # min is None → '?'
        assert "200000" in salary
        assert "GBP" in salary

    def test_no_salary_when_both_amounts_none(self, client, mock_scrape, df_no_salary):
        mock_scrape.return_value = df_no_salary
        resp = self._post(client)
        assert resp.json()["jobs"][0]["salary"] is None

    # NaN handling ---------------------------------------------------------

    def test_nan_values_normalised_to_none(self, client, mock_scrape, df_with_nan):
        mock_scrape.return_value = df_with_nan
        resp = self._post(client)
        job = resp.json()["jobs"][0]
        assert job["location"] is None
        assert job["is_remote"] is None
        assert job["date_posted"] is None
        # salary should exist because min_amount is set
        assert job["salary"] is not None

    # Filtering ------------------------------------------------------------

    def test_rows_without_title_are_excluded(self, client, mock_scrape, df_missing_title):
        mock_scrape.return_value = df_missing_title
        resp = self._post(client)
        assert len(resp.json()["jobs"]) == 1
        assert resp.json()["jobs"][0]["title"] == "Good Title"

    def test_rows_without_url_are_excluded(self, client, mock_scrape):
        df = pd.DataFrame(
            {
                "id": [1],
                "site": ["linkedin"],
                "title": ["Dev"],
                "company": ["X"],
                "location": ["R"],
                "is_remote": [False],
                "min_amount": [None],
                "max_amount": [None],
                "interval": [None],
                "currency": [None],
                "job_url": [""],
                "description": [None],
                "date_posted": [None],
            }
        )
        mock_scrape.return_value = df
        resp = self._post(client)
        assert resp.json() == {"jobs": []}

    # Error handling -------------------------------------------------------

    def test_scrape_exception_returns_502(self, client, mock_scrape):
        mock_scrape.side_effect = RuntimeError("network timeout")
        resp = self._post(client)
        assert resp.status_code == 502
        assert "jobspy scrape failed" in resp.json()["detail"]

    def test_missing_body_returns_422(self, client):
        resp = client.post("/search", json={})
        assert resp.status_code == 422  # validation error for missing search_term

    def test_invalid_site_returns_422(self, client):
        resp = client.post("/search", json={"search_term": "x", "site": "monster"})
        assert resp.status_code == 422

    # id field conversion --------------------------------------------------

    def test_id_converted_to_string(self, client, mock_scrape):
        df = pd.DataFrame(
            {
                "id": [999],
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
                "description": [None],
                "date_posted": [None],
            }
        )
        mock_scrape.return_value = df
        resp = self._post(client)
        assert resp.json()["jobs"][0]["id"] == "999"

    def test_none_id_stays_none(self, client, mock_scrape):
        df = pd.DataFrame(
            {
                "id": [None],
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
                "description": [None],
                "date_posted": [None],
            }
        )
        mock_scrape.return_value = df
        resp = self._post(client)
        assert resp.json()["jobs"][0]["id"] is None

    # date_posted ----------------------------------------------------------

    def test_date_posted_stringified(self, client, mock_scrape):
        df = pd.DataFrame(
            {
                "id": [1],
                "site": ["linkedin"],
                "title": ["Dev"],
                "company": ["X"],
                "location": ["R"],
                "is_remote": [False],
                "min_amount": [None],
                "max_amount": [None],
                "interval": [None],
                "currency": [None],
                "job_url": ["https://x.test/1"],
                "description": [None],
                "date_posted": ["2026-07-15"],
            }
        )
        mock_scrape.return_value = df
        resp = self._post(client)
        assert resp.json()["jobs"][0]["date_posted"] == "2026-07-15"

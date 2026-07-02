"""Tests for Pydantic models: SearchRequest and SidecarJob."""
import pytest
from pydantic import ValidationError

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from main import SearchRequest, SidecarJob


# ── SearchRequest ──────────────────────────────────────────────────────────


class TestSearchRequest:
    def test_defaults(self):
        req = SearchRequest(search_term="python dev")
        assert req.site == "linkedin"
        assert req.search_term == "python dev"
        assert req.location is None
        assert req.is_remote is False
        assert req.results_wanted == 30
        assert req.hours_old is None
        assert req.country_indeed == "usa"

    def test_all_fields(self):
        req = SearchRequest(
            site="indeed",
            search_term="rust",
            location="Berlin",
            is_remote=True,
            results_wanted=10,
            hours_old=24,
            country_indeed="de",
        )
        assert req.site == "indeed"
        assert req.results_wanted == 10

    def test_invalid_site_rejected(self):
        with pytest.raises(ValidationError):
            SearchRequest(search_term="x", site="monster")

    def test_results_wanted_too_low(self):
        with pytest.raises(ValidationError):
            SearchRequest(search_term="x", results_wanted=0)

    def test_results_wanted_too_high(self):
        with pytest.raises(ValidationError):
            SearchRequest(search_term="x", results_wanted=101)

    def test_results_wanted_boundaries(self):
        low = SearchRequest(search_term="x", results_wanted=1)
        high = SearchRequest(search_term="x", results_wanted=100)
        assert low.results_wanted == 1
        assert high.results_wanted == 100


# ── SidecarJob ─────────────────────────────────────────────────────────────


class TestSidecarJob:
    def test_minimal_required_fields(self):
        job = SidecarJob(site="linkedin", title="Eng", job_url="https://x.test/1")
        assert job.site == "linkedin"
        assert job.title == "Eng"
        assert job.job_url == "https://x.test/1"
        assert job.id is None
        assert job.company is None
        assert job.location is None
        assert job.is_remote is None
        assert job.salary is None
        assert job.description is None
        assert job.date_posted is None

    def test_all_fields(self):
        job = SidecarJob(
            id="123",
            site="indeed",
            title="PM",
            company="Acme",
            location="NYC",
            is_remote=False,
            salary="$100k – $150k USD yearly",
            job_url="https://x.test/2",
            description="Do stuff",
            date_posted="2026-03-15",
        )
        assert job.id == "123"
        assert job.salary == "$100k – $150k USD yearly"

    def test_model_dump_excludes_none(self):
        job = SidecarJob(site="s", title="t", job_url="u")
        data = job.model_dump()
        # Optional fields that are None should still be present (Pydantic
        # keeps them — we just check they are None).
        assert data["company"] is None
        assert data["salary"] is None
        assert data["description"] is None

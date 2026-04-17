"""Thin FastAPI wrapper around python-jobspy.

Mirrors the TS `NormalizedJob`-adjacent shape the JobSpyAdapter expects
(see apps/api/src/modules/job-sources/adapters/jobspy.adapter.ts — keep in sync).
"""
from typing import Literal, Optional

import pandas as pd
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

app = FastAPI(title="jobspy-sidecar")


class SearchRequest(BaseModel):
    site: Literal["linkedin", "indeed", "glassdoor"] = "linkedin"
    search_term: str
    location: Optional[str] = None
    is_remote: bool = False
    results_wanted: int = Field(default=30, ge=1, le=100)
    hours_old: Optional[int] = None
    country_indeed: str = "usa"


class SidecarJob(BaseModel):
    id: Optional[str] = None
    site: str
    title: str
    company: Optional[str] = None
    location: Optional[str] = None
    is_remote: Optional[bool] = None
    salary: Optional[str] = None
    job_url: str
    description: Optional[str] = None
    date_posted: Optional[str] = None


@app.get("/health")
def health():
    return {"ok": True}


@app.post("/search")
def search(req: SearchRequest):
    try:
        from jobspy import scrape_jobs
    except ImportError as e:  # pragma: no cover
        raise HTTPException(500, f"python-jobspy not installed: {e}")

    try:
        df = scrape_jobs(
            site_name=[req.site],
            search_term=req.search_term,
            location=req.location,
            is_remote=req.is_remote,
            results_wanted=req.results_wanted,
            hours_old=req.hours_old,
            country_indeed=req.country_indeed,
            description_format="markdown",
        )
    except Exception as e:
        # upstream scrapers break periodically — surface a clean 502 so the
        # NestJS adapter marks this source unhealthy without crashing ingestion
        raise HTTPException(502, f"jobspy scrape failed: {e}")

    if df is None or df.empty:
        return {"jobs": []}

    jobs: list[SidecarJob] = []
    for _, row in df.iterrows():
        def _get(key: str):
            val = row.get(key)
            return None if val is None or (isinstance(val, float) and pd.isna(val)) else val

        salary = None
        min_a, max_a = _get("min_amount"), _get("max_amount")
        if min_a or max_a:
            interval = _get("interval") or ""
            salary = f"{min_a or '?'} – {max_a or '?'} {(_get('currency') or '')} {interval}".strip()

        date_posted = _get("date_posted")
        jobs.append(
            SidecarJob(
                id=str(_get("id")) if _get("id") is not None else None,
                site=str(_get("site") or req.site),
                title=str(_get("title") or ""),
                company=str(_get("company")) if _get("company") is not None else None,
                location=str(_get("location")) if _get("location") is not None else None,
                is_remote=bool(_get("is_remote")) if _get("is_remote") is not None else None,
                salary=salary,
                job_url=str(_get("job_url") or ""),
                description=str(_get("description")) if _get("description") is not None else None,
                date_posted=str(date_posted) if date_posted is not None else None,
            )
        )

    jobs = [j for j in jobs if j.title and j.job_url]
    return {"jobs": [j.model_dump() for j in jobs]}

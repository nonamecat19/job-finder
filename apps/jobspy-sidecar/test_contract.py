"""Contract test: sidecar response shape ↔ what JobSpyAdapter expects.

Run: pytest test_contract.py  (does not hit real job boards — validates the
Pydantic model fields against the TS adapter's SidecarJob interface).
"""
from main import SidecarJob

# Fields the NestJS JobSpyAdapter reads (jobspy.adapter.ts SidecarJob interface).
TS_ADAPTER_FIELDS = {
    "id",
    "site",
    "title",
    "company",
    "location",
    "is_remote",
    "salary",
    "job_url",
    "description",
    "date_posted",
}


def test_sidecar_job_matches_ts_contract():
    py_fields = set(SidecarJob.model_fields.keys())
    assert TS_ADAPTER_FIELDS == py_fields, (
        f"contract drift: only-in-ts={TS_ADAPTER_FIELDS - py_fields}, "
        f"only-in-py={py_fields - TS_ADAPTER_FIELDS}"
    )


def test_serialization_roundtrip():
    job = SidecarJob(site="linkedin", title="Dev", job_url="https://x.test/1")
    data = job.model_dump()
    assert data["title"] == "Dev"
    assert data["job_url"] == "https://x.test/1"
    assert data["company"] is None

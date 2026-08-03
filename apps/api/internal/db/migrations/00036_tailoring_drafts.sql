-- +goose Up
-- Constrained AI resume tailoring (specs/020-ai-resume-tailoring): a
-- tailored_draft is one review-lifecycle unit for tailoring a profile's
-- resume against one job (or one ad-hoc, pasted vacancy). edit_proposals
-- are the atomic, reviewable changes the AI proposes against the draft's
-- baseline (the persisted RendercvMaster snapshot), one row per
-- summary/highlight/skill-group/skill change, each carrying a
-- traceability pointer back to the master resume or the job posting
-- (FR-003/SC-005).
--
-- Follows the snake_case/unquoted-identifier convention established by
-- host_retrieval_state (00026), not the legacy PascalCase Drizzle tables.
-- There is no updated_at trigger anywhere in this repo (checked: no
-- CREATE TRIGGER in any prior migration) — every other table maintains
-- updated_at by setting it explicitly in each UPDATE query, so this
-- migration follows that same convention instead of introducing the
-- first trigger.
CREATE TABLE tailored_drafts (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id           uuid NOT NULL REFERENCES "Profile"("id") ON DELETE CASCADE,
    job_id               uuid NULL REFERENCES "Job"("id") ON DELETE CASCADE,
    vacancy_company      text NULL,
    vacancy_title        text NULL,
    vacancy_text         text NULL,
    baseline             jsonb NOT NULL,
    baseline_content_hash text NOT NULL,
    state                text NOT NULL DEFAULT 'drafting'
        CHECK (state IN ('drafting', 'proposing', 'review', 'finalized', 'expounded', 'abandoned')),
    parent_draft_id      uuid NULL REFERENCES tailored_drafts(id) ON DELETE SET NULL,
    model                text NOT NULL,
    activity_id          uuid NULL REFERENCES "ActivityRun"("id") ON DELETE SET NULL,
    export_document_id   uuid NULL REFERENCES "GeneratedDocument"("id") ON DELETE SET NULL,
    export_status        text NULL
        CHECK (export_status IS NULL OR export_status IN ('pending', 'fitting', 'fit', 'blocked', 'error')),
    export_feedback      jsonb NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- One draft per (profile, job), with a NULL job_id — the ad-hoc pasted-vacancy
-- case — collapsing to a single slot per profile.
--
-- This is a unique *index*, not the table-level `UNIQUE (profile_id,
-- (COALESCE(job_id, ...)))` constraint this migration originally carried.
-- Postgres permits expressions only in index definitions; as a table
-- constraint it is a syntax error (SQLSTATE 42601), so the migration failed
-- for everyone who ran it and no database ever got past 00035. Editing it in
-- place rather than issuing a follow-up migration is therefore safe: there is
-- no deployment holding the broken version.
CREATE UNIQUE INDEX tailored_drafts_profile_job_key
    ON tailored_drafts (profile_id, COALESCE(job_id, '00000000-0000-0000-0000-000000000000'::uuid));

CREATE TABLE edit_proposals (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    draft_id       uuid NOT NULL REFERENCES tailored_drafts(id) ON DELETE CASCADE,
    field_type     text NOT NULL
        CHECK (field_type IN ('summary', 'experience_highlights', 'skill_change', 'skill_group_add', 'skill_group_remove')),
    field_key      text NOT NULL,
    before_value   text NOT NULL,
    after_value    text NOT NULL,
    traceability   jsonb NOT NULL,
    status         text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'rejected', 'dropped')),
    dropped_reason text NULL,
    accepted_at    timestamptz NULL,
    rejected_at    timestamptz NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX edit_proposals_draft_status_idx ON edit_proposals (draft_id, status);

-- +goose Down
DROP TABLE IF EXISTS edit_proposals;
DROP TABLE IF EXISTS tailored_drafts;

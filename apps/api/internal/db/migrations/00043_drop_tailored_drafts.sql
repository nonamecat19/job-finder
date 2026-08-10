-- +goose Up
-- 042 supersedes the 020 accept/reject review model (research.md R8): the
-- ranked-item workspace (generation_runs/generation_sections/generation_items,
-- 00042) and the tailored_drafts/edit_proposals draft/proposal surface
-- (00036) are two answers to the same "review AI-tailored content" question,
-- and no code path writes these tables — internal/tailoring/ has validators
-- but no handler, no service, no route registration, and
-- GeneratePayload.TailoringDraftID (queue.go) is declared and read nowhere.
-- Dropping them here rather than leaving two dead tables and a
-- never-true REST surface documented against the tree (024-FR-015).
--
-- Explicit user approval was obtained before this migration was written
-- (specs/042-resume-generation-workspace/tasks.md T083,
-- specs/042-resume-generation-workspace/research.md R8).
DROP TABLE IF EXISTS edit_proposals;
DROP TABLE IF EXISTS tailored_drafts;

-- +goose Down
-- Recreates 00036's shape exactly, so a rollback lands on the same schema
-- 00036 produced (including the corrected unique index it already carried).
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

-- +goose Up
-- Resume Generation Workspace (specs/042-resume-generation-workspace): a
-- generation_run is one tailoring attempt against one vacancy for one
-- profile. generation_sections collapse "Section" and "Work Entry Block"
-- into one row per section (a nullable entry_key distinguishes the two
-- granularities). generation_items are the ranked candidates for
-- inclusion — the spec's "Ranked Item" — each either origin='profile'
-- (an index into the master's bullet list, text always byte-identical to
-- the master) or origin='ai' (an unverified suggestion, editable, never
-- selected by default).
--
-- Follows the snake_case/unquoted-identifier convention 00036 established
-- for this feature area. There is no updated_at trigger anywhere in this
-- repo — every table maintains updated_at by setting it explicitly in
-- each UPDATE query, so this migration follows that convention too.
CREATE TABLE generation_runs (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    profile_id           uuid NOT NULL REFERENCES "Profile"("id") ON DELETE CASCADE,
    job_id               uuid NULL REFERENCES "Job"("id") ON DELETE CASCADE,
    vacancy_company      text NULL,
    vacancy_title        text NULL,
    vacancy_text         text NOT NULL,
    master_snapshot      jsonb NOT NULL,
    master_content_hash  text NOT NULL,
    shape_config         jsonb NOT NULL,
    grounding_level      text NOT NULL,
    summary_option_id    text NULL,
    analysis             jsonb NULL,
    state                text NOT NULL DEFAULT 'running'
        CHECK (state IN ('running', 'ready', 'partial', 'failed')),
    activity_id          uuid NULL REFERENCES "ActivityRun"("id") ON DELETE SET NULL,
    export_document_id   uuid NULL REFERENCES "GeneratedDocument"("id") ON DELETE SET NULL,
    export_status        text NULL
        CHECK (export_status IS NULL OR export_status IN ('rendering', 'exported', 'blocked', 'error')),
    export_report        jsonb NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- Deliberately no uniqueness constraint on (profile_id, job_id), unlike
-- 00036's tailored_drafts_profile_job_key: a run here is immutable input
-- plus per-item selections, so several runs against the same vacancy are a
-- comparison, not a conflict (data-model.md §1).
CREATE INDEX generation_runs_profile_created_idx ON generation_runs (profile_id, created_at DESC);
CREATE INDEX generation_runs_job_idx ON generation_runs (job_id) WHERE job_id IS NOT NULL;

CREATE TABLE generation_sections (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id         uuid NOT NULL REFERENCES generation_runs(id) ON DELETE CASCADE,
    kind           text NOT NULL CHECK (kind IN ('summary', 'experience', 'skills')),
    -- For kind='experience', the exact master company name (norm()-addressed
    -- elsewhere in this pipeline); NULL for summary/skills.
    entry_key      text NULL,
    entry_label    text NULL,
    position       int NOT NULL,
    target_count   int NOT NULL,
    state          text NOT NULL DEFAULT 'running' CHECK (state IN ('running', 'ready', 'failed')),
    error          text NULL,
    fallback_used  bool NOT NULL DEFAULT false,
    CHECK (kind <> 'experience' OR entry_key IS NOT NULL)
);

-- Postgres permits expressions only in index definitions, not table-level
-- UNIQUE constraints (SQLSTATE 42601 otherwise — the same pitfall 00036's
-- header documents for tailored_drafts_profile_job_key), hence a unique
-- index rather than an inline UNIQUE(...).
CREATE UNIQUE INDEX generation_sections_run_kind_entry_key
    ON generation_sections (run_id, kind, COALESCE(entry_key, ''));

CREATE TABLE generation_items (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    section_id    uuid NOT NULL REFERENCES generation_sections(id) ON DELETE CASCADE,
    origin        text NOT NULL CHECK (origin IN ('profile', 'ai')),
    -- For origin='profile': index into that entry's master bullet list.
    source_index  int NULL,
    -- For origin='profile': the master's text, copied at run time from the
    -- snapshot. For origin='ai': the model's suggestion.
    source_text   text NOT NULL,
    -- The user's edit. Only legal when origin='ai' — this is FR-009 and
    -- FR-015 in one constraint: profile-sourced text is never editable.
    edited_text   text NULL,
    rank          int NOT NULL,
    position      int NOT NULL,
    selected      bool NOT NULL,
    unavailable   bool NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (origin <> 'profile' OR source_index IS NOT NULL),
    CHECK (origin <> 'profile' OR edited_text IS NULL),
    UNIQUE (section_id, origin, source_index)
);

CREATE INDEX generation_items_section_pos_idx ON generation_items (section_id, position);

-- +goose Down
DROP TABLE IF EXISTS generation_items;
DROP TABLE IF EXISTS generation_sections;
DROP TABLE IF EXISTS generation_runs;

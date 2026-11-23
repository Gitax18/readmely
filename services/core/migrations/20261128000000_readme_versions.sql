-- +goose Up

-- A project's README, one row per version. The newest version is the current one; editing,
-- regenerating a section and restoring an older version each add a row, so nothing is overwritten
-- and the history is the table.
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('ingest', 'summarise', 'generate_readme', 'readme_section', 'generate_diagram', 'build_context_pack'));

CREATE TABLE readme_versions (
    id          uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    project_id  uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    version     integer     NOT NULL,
    content     text        NOT NULL,
    source      text        NOT NULL,
    template    text,
    job_id      uuid        REFERENCES jobs (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),

    -- Two writers racing for the next number: one wins, the other is told to retry.
    CONSTRAINT readme_versions_project_id_version_key UNIQUE (project_id, version),
    CONSTRAINT readme_versions_version_check CHECK (version >= 1),
    CONSTRAINT readme_versions_content_check CHECK (octet_length(content) <= 102400),
    CONSTRAINT readme_versions_source_check CHECK (source IN ('generated', 'edited', 'regenerated', 'restored')),
    CONSTRAINT readme_versions_template_check CHECK (template IN ('standard', 'minimal', 'detailed'))
);
CREATE INDEX readme_versions_job_id_idx ON readme_versions (job_id);

-- The AI worker writes what it generates; it never edits or removes a version.
GRANT SELECT, INSERT ON readme_versions TO readmely_ai;

-- +goose Down
REVOKE SELECT, INSERT ON readme_versions FROM readmely_ai;
DROP TABLE readme_versions;
DELETE FROM jobs WHERE kind = 'readme_section';
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN ('ingest', 'summarise', 'generate_readme', 'generate_diagram', 'build_context_pack'));

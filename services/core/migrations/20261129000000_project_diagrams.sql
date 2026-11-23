-- +goose Up

-- A project's architecture diagram: the current one only, replaced when a new one is generated.
-- The SVG is made by the AI worker's own renderer from a validated graph, never taken from the model.
CREATE TABLE project_diagrams (
    project_id  uuid        PRIMARY KEY REFERENCES projects (id) ON DELETE CASCADE,
    title       text        NOT NULL,
    svg         text        NOT NULL,
    node_count  integer     NOT NULL,
    commit_sha  text        NOT NULL,
    job_id      uuid        REFERENCES jobs (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT project_diagrams_svg_check CHECK (octet_length(svg) <= 200000),
    CONSTRAINT project_diagrams_title_check CHECK (char_length(title) BETWEEN 1 AND 120),
    CONSTRAINT project_diagrams_node_count_check CHECK (node_count BETWEEN 1 AND 12)
);
CREATE INDEX project_diagrams_job_id_idx ON project_diagrams (job_id);

GRANT SELECT, INSERT, UPDATE ON project_diagrams TO readmely_ai;

-- +goose Down
REVOKE SELECT, INSERT, UPDATE ON project_diagrams FROM readmely_ai;
DROP TABLE project_diagrams;

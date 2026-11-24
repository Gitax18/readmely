-- +goose Up

-- A project can hold one diagram of each kind: its architecture, and its database (an entity
-- relationship diagram drawn from the schema and SQL files). Generating one kind replaces only that kind.
ALTER TABLE project_diagrams
    ADD COLUMN kind text NOT NULL DEFAULT 'architecture',
    ADD CONSTRAINT project_diagrams_kind_check CHECK (kind IN ('architecture', 'database')),
    DROP CONSTRAINT project_diagrams_project_id_key,
    -- A database can have far more tables than an architecture has boxes.
    DROP CONSTRAINT project_diagrams_node_count_check,
    ADD CONSTRAINT project_diagrams_node_count_check CHECK (node_count BETWEEN 1 AND 40);
CREATE UNIQUE INDEX project_diagrams_project_id_kind_key ON project_diagrams (project_id, kind);

-- +goose Down
DROP INDEX project_diagrams_project_id_kind_key;
DELETE FROM project_diagrams WHERE kind <> 'architecture';
ALTER TABLE project_diagrams
    DROP CONSTRAINT project_diagrams_node_count_check,
    ADD CONSTRAINT project_diagrams_node_count_check CHECK (node_count BETWEEN 1 AND 12),
    ADD CONSTRAINT project_diagrams_project_id_key UNIQUE (project_id),
    DROP CONSTRAINT project_diagrams_kind_check,
    DROP COLUMN kind;

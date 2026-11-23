-- +goose Up

-- What analysing a repository produces (Phase 4). Everything is owned by a project and goes with it:
-- deleting the project deletes the index, the graph and the facts in one statement.

ALTER TABLE projects
    ADD COLUMN commit_sha text,
    ADD CONSTRAINT projects_commit_sha_check CHECK (commit_sha ~ '^[0-9a-f]{40}$');

-- One row per file in the repository. Text files keep their (secret-redacted) content so later
-- phases can read them; everything else is recorded with the reason it was left out.
CREATE TABLE files (
    id             uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    project_id     uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    path           text        NOT NULL,
    language       text,
    size_bytes     integer     NOT NULL,
    line_count     integer     NOT NULL DEFAULT 0,
    content        text,
    skipped_reason text,
    created_at     timestamptz NOT NULL DEFAULT now(),

    -- Also the index for the project foreign key.
    CONSTRAINT files_project_id_path_key UNIQUE (project_id, path),
    -- A path from an archive is untrusted: never absolute, never climbing out, bounded.
    CONSTRAINT files_path_check CHECK (
        char_length(path) BETWEEN 1 AND 400 AND path !~ '^/' AND path !~ '(^|/)\.\.(/|$)' AND path !~ '[[:cntrl:]]'
    ),
    CONSTRAINT files_language_check CHECK (language ~ '^[A-Za-z0-9+#. -]{1,40}$'),
    CONSTRAINT files_size_bytes_check CHECK (size_bytes >= 0),
    CONSTRAINT files_line_count_check CHECK (line_count >= 0),
    CONSTRAINT files_skipped_reason_check CHECK (
        skipped_reason IN ('binary', 'too_large', 'sensitive', 'generated', 'vendored', 'budget')
    ),
    -- Content is stored exactly when the file was not skipped.
    CONSTRAINT files_content_check CHECK ((content IS NULL) = (skipped_reason IS NOT NULL)),
    CONSTRAINT files_content_size_check CHECK (content IS NULL OR octet_length(content) <= 262144)
);

-- Functions, classes and types found in source files.
CREATE TABLE symbols (
    id         uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    project_id uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    file_id    uuid        NOT NULL REFERENCES files (id) ON DELETE CASCADE,
    name       text        NOT NULL,
    kind       text        NOT NULL,
    line       integer     NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT symbols_name_check CHECK (char_length(name) BETWEEN 1 AND 200),
    CONSTRAINT symbols_kind_check CHECK (kind IN ('function', 'method', 'class', 'type', 'interface', 'constant')),
    CONSTRAINT symbols_line_check CHECK (line >= 1)
);
CREATE INDEX symbols_file_id_idx ON symbols (file_id);
CREATE INDEX symbols_project_id_name_idx ON symbols (project_id, name);

-- The import graph. A node is a source file, a package directory, or an external dependency.
CREATE TABLE graph_nodes (
    id         uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    project_id uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    kind       text        NOT NULL,
    key        text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT graph_nodes_project_id_kind_key_key UNIQUE (project_id, kind, key),
    CONSTRAINT graph_nodes_kind_check CHECK (kind IN ('file', 'package', 'external')),
    CONSTRAINT graph_nodes_key_check CHECK (char_length(key) BETWEEN 1 AND 400)
);

CREATE TABLE graph_edges (
    id         uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    project_id uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    from_node  uuid        NOT NULL REFERENCES graph_nodes (id) ON DELETE CASCADE,
    to_node    uuid        NOT NULL REFERENCES graph_nodes (id) ON DELETE CASCADE,
    kind       text        NOT NULL DEFAULT 'imports',
    created_at timestamptz NOT NULL DEFAULT now(),

    -- Also the index for the from_node foreign key.
    CONSTRAINT graph_edges_from_node_to_node_kind_key UNIQUE (from_node, to_node, kind),
    CONSTRAINT graph_edges_kind_check CHECK (kind IN ('imports')),
    CONSTRAINT graph_edges_no_self_loop_check CHECK (from_node <> to_node)
);
CREATE INDEX graph_edges_to_node_idx ON graph_edges (to_node);
CREATE INDEX graph_edges_project_id_idx ON graph_edges (project_id);

-- What the project page shows: one JSON document per project, replaced on every analysis. Its
-- shape belongs to contracts/core-public.yaml (ProjectFacts), so it is a document, not columns.
CREATE TABLE project_facts (
    id          uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    project_id  uuid        NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    commit_sha  text        NOT NULL,
    facts       jsonb       NOT NULL,
    analysed_at timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT project_facts_project_id_key UNIQUE (project_id),
    CONSTRAINT project_facts_commit_sha_check CHECK (commit_sha ~ '^[0-9a-f]{40}$'),
    CONSTRAINT project_facts_facts_check CHECK (jsonb_typeof(facts) = 'object' AND octet_length(facts::text) <= 262144)
);

CREATE TRIGGER project_facts_set_updated_at BEFORE UPDATE ON project_facts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE project_facts;
DROP TABLE graph_edges;
DROP TABLE graph_nodes;
DROP TABLE symbols;
DROP TABLE files;
ALTER TABLE projects
    DROP CONSTRAINT projects_commit_sha_check,
    DROP COLUMN commit_sha;

-- +goose Up
CREATE TABLE projects (
    id         uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       text        NOT NULL,
    status     text        NOT NULL DEFAULT 'draft',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT projects_name_check CHECK (name = btrim(name) AND char_length(name) BETWEEN 1 AND 100),
    CONSTRAINT projects_status_check CHECK (status IN ('draft', 'importing', 'ready', 'failed')),
    -- Lets other tables prove that a project belongs to a given user (see jobs).
    CONSTRAINT projects_id_user_id_key UNIQUE (id, user_id)
);

-- A user cannot have two projects with the same name. The leading user_id column also serves
-- as the index for the foreign key.
CREATE UNIQUE INDEX projects_user_id_name_lower_key ON projects (user_id, lower(name));

CREATE TRIGGER projects_set_updated_at BEFORE UPDATE ON projects
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE projects;

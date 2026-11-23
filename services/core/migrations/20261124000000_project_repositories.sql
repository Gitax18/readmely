-- +goose Up

-- A project is a public GitHub repository (Phase 3). The columns are all-or-nothing: a project made
-- by hand (seed data, older rows) has none of them. Everything here came from GitHub, so every value
-- is held to a pattern: nothing in these columns can carry markup or a path.
ALTER TABLE projects
    ADD COLUMN repo_owner       text,
    ADD COLUMN repo_name        text,
    ADD COLUMN repo_description text,
    ADD COLUMN default_branch   text,
    ADD COLUMN repo_size_kb     integer,
    ADD CONSTRAINT projects_repo_all_or_none_check CHECK (
        (repo_owner IS NULL) = (repo_name IS NULL)
        AND (repo_owner IS NULL) = (default_branch IS NULL)
        AND (repo_owner IS NULL) = (repo_size_kb IS NULL)
    ),
    ADD CONSTRAINT projects_repo_owner_check CHECK (repo_owner ~ '^[A-Za-z0-9][A-Za-z0-9-]{0,38}$'),
    ADD CONSTRAINT projects_repo_name_check CHECK (repo_name ~ '^[A-Za-z0-9._-]{1,100}$' AND repo_name NOT IN ('.', '..')),
    ADD CONSTRAINT projects_repo_description_check CHECK (char_length(repo_description) <= 500),
    ADD CONSTRAINT projects_default_branch_check CHECK (default_branch ~ '^[A-Za-z0-9._/-]{1,255}$'),
    ADD CONSTRAINT projects_repo_size_kb_check CHECK (repo_size_kb >= 0);

-- One project per repository per user, however the address was typed.
CREATE UNIQUE INDEX projects_user_id_repo_key
    ON projects (user_id, lower(repo_owner), lower(repo_name)) WHERE repo_owner IS NOT NULL;

-- What a running job is doing, for the progress bar: a short step name and a percentage.
ALTER TABLE jobs
    ADD COLUMN step     text,
    ADD COLUMN progress smallint NOT NULL DEFAULT 0,
    ADD CONSTRAINT jobs_step_check CHECK (step ~ '^[a-z][a-z_]{0,39}$'),
    ADD CONSTRAINT jobs_progress_check CHECK (progress BETWEEN 0 AND 100);

-- +goose Down
ALTER TABLE jobs
    DROP CONSTRAINT jobs_progress_check,
    DROP CONSTRAINT jobs_step_check,
    DROP COLUMN progress,
    DROP COLUMN step;
DROP INDEX projects_user_id_repo_key;
ALTER TABLE projects
    DROP CONSTRAINT projects_repo_size_kb_check,
    DROP CONSTRAINT projects_default_branch_check,
    DROP CONSTRAINT projects_repo_description_check,
    DROP CONSTRAINT projects_repo_name_check,
    DROP CONSTRAINT projects_repo_owner_check,
    DROP CONSTRAINT projects_repo_all_or_none_check,
    DROP COLUMN repo_size_kb,
    DROP COLUMN default_branch,
    DROP COLUMN repo_description,
    DROP COLUMN repo_name,
    DROP COLUMN repo_owner;

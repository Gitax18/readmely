-- +goose Up

-- Every table has a uuid primary key named id (the schema conventions test); github_access was keyed by
-- user_id. The person is still the one row's owner, so user_id stays unique.
ALTER TABLE github_access
    ADD COLUMN id uuid NOT NULL DEFAULT uuid_generate_v7(),
    DROP CONSTRAINT github_access_pkey,
    ADD PRIMARY KEY (id),
    ADD CONSTRAINT github_access_user_id_key UNIQUE (user_id);

-- +goose Down
ALTER TABLE github_access
    DROP CONSTRAINT github_access_user_id_key,
    DROP CONSTRAINT github_access_pkey,
    DROP COLUMN id,
    ADD PRIMARY KEY (user_id);

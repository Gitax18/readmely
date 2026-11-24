-- +goose Up

-- A person's GitHub token, for reading their private repositories (plans/phase-9b-private-repos.md).
-- The token is sealed (AES-256-GCM, the user id bound in as additional data) before it gets here, so
-- this table alone cannot be read and a row cannot be moved to another user. One row per user; it is
-- deleted when they remove access, disconnect GitHub, or delete their account.
CREATE TABLE github_access (
    user_id    uuid        PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    sealed     bytea       NOT NULL,
    scope      text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT github_access_sealed_check CHECK (octet_length(sealed) BETWEEN 29 AND 8192),
    CONSTRAINT github_access_scope_check CHECK (char_length(scope) <= 200)
);

-- True when the repository was read with the owner's token, so it may be private.
ALTER TABLE projects ADD COLUMN is_private boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE projects DROP COLUMN is_private;
DROP TABLE github_access;

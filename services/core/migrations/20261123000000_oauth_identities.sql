-- +goose Up

-- A provider account (Google or GitHub) connected to a user (plans/phase-2b-oauth-login.md).
-- The key is the provider's own stable id (Google "sub", GitHub numeric id), never the email: an
-- email can change hands, an id cannot. email is only a lower-case snapshot shown to the owner.
-- No provider token is stored anywhere. A provider account belongs to one user, and a user holds
-- at most one account per provider; both are enforced here so a code bug cannot merge two users.
CREATE TABLE oauth_identities (
    id               uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    user_id          uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider         text        NOT NULL,
    provider_user_id text        NOT NULL,
    email            text        NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    last_login_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT oauth_identities_provider_check CHECK (provider IN ('google', 'github')),
    CONSTRAINT oauth_identities_provider_user_id_check CHECK (char_length(provider_user_id) BETWEEN 1 AND 255),
    CONSTRAINT oauth_identities_email_check CHECK (
        email = lower(email) AND char_length(email) BETWEEN 3 AND 254 AND position('@' IN email) > 1
    ),
    CONSTRAINT oauth_identities_provider_account_key UNIQUE (provider, provider_user_id),
    CONSTRAINT oauth_identities_user_provider_key UNIQUE (user_id, provider)
);
-- The (user_id, provider) unique index also serves the user_id foreign key and "list my accounts".

-- +goose Down
DROP TABLE oauth_identities;

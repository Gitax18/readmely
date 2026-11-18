-- +goose Up
CREATE TABLE users (
    id                uuid        PRIMARY KEY DEFAULT uuid_generate_v7(),
    email             text        NOT NULL,
    email_verified_at timestamptz,
    display_name      text        NOT NULL DEFAULT '',
    status            text        NOT NULL DEFAULT 'active',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT users_email_check CHECK (
        email = btrim(email) AND char_length(email) BETWEEN 3 AND 254 AND strpos(email, '@') > 1
    ),
    CONSTRAINT users_display_name_check CHECK (char_length(display_name) <= 100),
    CONSTRAINT users_status_check CHECK (status IN ('active', 'suspended'))
);

-- One account per address, however it is capitalised.
CREATE UNIQUE INDEX users_email_lower_key ON users (lower(email));

CREATE TRIGGER users_set_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE users;

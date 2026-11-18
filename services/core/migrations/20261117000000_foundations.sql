-- +goose Up

-- Time-ordered UUIDs (version 7): 48 bits of milliseconds, then random bits. Postgres 17 has no
-- built-in generator; Postgres 18 does (uuidv7()), and this function can be dropped then.
-- +goose StatementBegin
CREATE FUNCTION uuid_generate_v7() RETURNS uuid
LANGUAGE sql VOLATILE PARALLEL SAFE AS $$
    SELECT encode(
        set_bit(
            set_bit(
                overlay(
                    uuid_send(gen_random_uuid())
                    PLACING substring(int8send((extract(epoch FROM clock_timestamp()) * 1000)::bigint) FROM 3)
                    FROM 1 FOR 6
                ),
                52, 1
            ),
            53, 1
        ),
        'hex'
    )::uuid
$$;
-- +goose StatementEnd

-- Shared trigger so no application code ever sets updated_at.
-- +goose StatementBegin
CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = clock_timestamp();
    RETURN NEW;
END
$$;
-- +goose StatementEnd

-- Roles are created without a password and cannot log in. `migrate roles` sets the passwords
-- from the environment, so no secret ever appears in a migration. The handler makes concurrent
-- first runs (parallel test databases) safe.
-- +goose StatementBegin
DO $$
BEGIN
    BEGIN
        CREATE ROLE readmely_app NOLOGIN;
    EXCEPTION WHEN duplicate_object THEN
        NULL;
    END;
    BEGIN
        CREATE ROLE readmely_ai NOLOGIN;
    EXCEPTION WHEN duplicate_object THEN
        NULL;
    END;
END
$$;
-- +goose StatementEnd

-- The application changes data but never the schema. Tables created later by the migrating
-- role are readable and writable by the app role automatically; a migration that needs
-- stricter rules (the ledger) revokes what it must. The AI role gets nothing by default.
GRANT USAGE ON SCHEMA public TO readmely_app, readmely_ai;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO readmely_app;

-- +goose Down
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLES FROM readmely_app;
REVOKE USAGE ON SCHEMA public FROM readmely_app, readmely_ai;
DROP FUNCTION set_updated_at();
DROP FUNCTION uuid_generate_v7();
-- The roles are cluster-wide and may serve other databases, so they are left in place.

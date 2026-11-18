-- +goose Up

-- Bounds for the service roles in this database (audit F4). A runaway query is cancelled, a
-- statement waiting on a lock gives up, and a transaction left open by a crashed or stuck client
-- is ended, so the per-user credit lock and claimed job rows can never be held forever.
-- They apply at login; migrations (the owner) are not limited. Settings are per database, so
-- other databases in the cluster are unaffected.
-- +goose StatementBegin
DO $$
DECLARE
    r text;
BEGIN
    FOREACH r IN ARRAY ARRAY['readmely_app', 'readmely_ai'] LOOP
        EXECUTE format('ALTER ROLE %I IN DATABASE %I SET statement_timeout = %L', r, current_database(), '30s');
        EXECUTE format('ALTER ROLE %I IN DATABASE %I SET lock_timeout = %L', r, current_database(), '10s');
        EXECUTE format('ALTER ROLE %I IN DATABASE %I SET idle_in_transaction_session_timeout = %L', r, current_database(), '60s');
    END LOOP;
END
$$;
-- +goose StatementEnd

-- Postgres lets everyone execute a new function unless told otherwise. Functions created by
-- later migrations are private until granted explicitly (audit F12), so a SECURITY DEFINER
-- function can never be exposed by accident. Existing ones are covered by their own grants.
ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC;

-- +goose Down
ALTER DEFAULT PRIVILEGES GRANT EXECUTE ON FUNCTIONS TO PUBLIC;
-- +goose StatementBegin
DO $$
DECLARE
    r text;
BEGIN
    FOREACH r IN ARRAY ARRAY['readmely_app', 'readmely_ai'] LOOP
        EXECUTE format('ALTER ROLE %I IN DATABASE %I RESET ALL', r, current_database());
    END LOOP;
END
$$;
-- +goose StatementEnd

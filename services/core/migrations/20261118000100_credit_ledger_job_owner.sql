-- +goose Up

-- A ledger entry may only point at a job of the same user (audit F5), the rule jobs already
-- follow for projects. The composite key is checked whenever both columns are set; once an
-- account is deleted user_id is NULL and the key no longer applies. The single-column key stays:
-- it is the one that clears job_id when a job is deleted after its owner's rows were anonymised.
ALTER TABLE jobs ADD CONSTRAINT jobs_id_user_id_key UNIQUE (id, user_id);

ALTER TABLE credit_ledger
    ADD CONSTRAINT credit_ledger_job_id_user_id_fkey FOREIGN KEY (job_id, user_id)
        REFERENCES jobs (id, user_id) ON DELETE SET NULL (job_id);

-- One index serves both keys (it starts with job_id).
DROP INDEX credit_ledger_job_id_idx;
CREATE INDEX credit_ledger_job_id_user_id_idx ON credit_ledger (job_id, user_id);

-- +goose Down
DROP INDEX credit_ledger_job_id_user_id_idx;
CREATE INDEX credit_ledger_job_id_idx ON credit_ledger (job_id);
ALTER TABLE credit_ledger DROP CONSTRAINT credit_ledger_job_id_user_id_fkey;
ALTER TABLE jobs DROP CONSTRAINT jobs_id_user_id_key;

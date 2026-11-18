-- Development seed data. Every person is fictional and uses the reserved example.invalid domain.
-- Fixed ids plus ON CONFLICT DO NOTHING make it safe to run any number of times. It writes
-- through the real constraints, so it also proves the schema accepts every state it allows.

-- Users: one verified, one not yet verified, one suspended.
INSERT INTO users (id, email, email_verified_at, display_name, status) VALUES
    ('01900000-0000-7000-8000-000000000001', 'ada@example.invalid',   now(), 'Ada Lovelace',    'active'),
    ('01900000-0000-7000-8000-000000000002', 'bob@example.invalid',   NULL,  'Bob Builder',     'active'),
    ('01900000-0000-7000-8000-000000000003', 'carol@example.invalid', now(), 'Carol Suspended', 'suspended')
ON CONFLICT DO NOTHING;

INSERT INTO projects (id, user_id, name, status) VALUES
    ('01900000-0000-7000-8000-000000000101', '01900000-0000-7000-8000-000000000001', 'Readmely',     'ready'),
    ('01900000-0000-7000-8000-000000000102', '01900000-0000-7000-8000-000000000001', 'Side project', 'draft'),
    ('01900000-0000-7000-8000-000000000103', '01900000-0000-7000-8000-000000000002', 'Dotfiles',     'importing')
ON CONFLICT DO NOTHING;

-- One job in each state a worker or the UI has to handle.
INSERT INTO jobs (id, user_id, project_id, kind, payload, status, attempts, max_attempts, idempotency_key,
                  locked_by, lease_expires_at, error_code, finished_at) VALUES
    ('01900000-0000-7000-8000-000000000201', '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000101',
     'ingest', '{"source": "github"}', 'succeeded', 1, 3, 'seed-job-ingest-1',
     NULL, NULL, NULL, now() - interval '2 hours'),
    ('01900000-0000-7000-8000-000000000202', '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000101',
     'generate_readme', '{"depth": 2}', 'running', 1, 3, 'seed-job-readme-1',
     'seed-worker', now() + interval '1 day', NULL, NULL),
    ('01900000-0000-7000-8000-000000000203', '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000101',
     'generate_diagram', '{}', 'queued', 0, 3, 'seed-job-diagram-1',
     NULL, NULL, NULL, NULL),
    ('01900000-0000-7000-8000-000000000204', '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000101',
     'build_context_pack', '{}', 'failed', 3, 3, 'seed-job-context-1',
     NULL, NULL, 'provider_timeout', now() - interval '30 minutes'),
    ('01900000-0000-7000-8000-000000000205', '01900000-0000-7000-8000-000000000001', '01900000-0000-7000-8000-000000000102',
     'ingest', '{}', 'cancelled', 0, 3, 'seed-job-ingest-2',
     NULL, NULL, NULL, now() - interval '10 minutes'),
    ('01900000-0000-7000-8000-000000000206', '01900000-0000-7000-8000-000000000002', '01900000-0000-7000-8000-000000000103',
     'ingest', '{}', 'queued', 0, 3, 'seed-job-ingest-3',
     NULL, NULL, NULL, NULL)
ON CONFLICT DO NOTHING;

-- Credit history that uses every kind of entry: ada ends with 470 credits, bob with 100.
INSERT INTO credit_ledger (id, user_id, delta, kind, job_id, reservation_id, idempotency_key, note) VALUES
    ('01900000-0000-7000-8000-000000000301', '01900000-0000-7000-8000-000000000001',  500, 'grant',   NULL,                                   NULL, 'seed-grant-ada-1',     'welcome credits'),
    ('01900000-0000-7000-8000-000000000302', '01900000-0000-7000-8000-000000000001',  -20, 'reserve', '01900000-0000-7000-8000-000000000202', NULL, 'seed-reserve-readme',  'running job'),
    ('01900000-0000-7000-8000-000000000303', '01900000-0000-7000-8000-000000000001',  -10, 'reserve', '01900000-0000-7000-8000-000000000204', NULL, 'seed-reserve-context', 'job that later failed'),
    ('01900000-0000-7000-8000-000000000305', '01900000-0000-7000-8000-000000000001',  -15, 'reserve', '01900000-0000-7000-8000-000000000201', NULL, 'seed-reserve-ingest',  'job that finished'),
    ('01900000-0000-7000-8000-000000000307', '01900000-0000-7000-8000-000000000002',  100, 'grant',   NULL,                                   NULL, 'seed-grant-bob-1',     'welcome credits')
ON CONFLICT DO NOTHING;

INSERT INTO credit_ledger (id, user_id, delta, kind, job_id, reservation_id, idempotency_key, note) VALUES
    ('01900000-0000-7000-8000-000000000304', '01900000-0000-7000-8000-000000000001',   10, 'refund', '01900000-0000-7000-8000-000000000204', '01900000-0000-7000-8000-000000000303', 'seed-refund-context',  'the whole reserve returned'),
    ('01900000-0000-7000-8000-000000000306', '01900000-0000-7000-8000-000000000001',    5, 'settle', '01900000-0000-7000-8000-000000000201', '01900000-0000-7000-8000-000000000305', 'seed-settle-ingest',   'cost 10 of the 15 held')
ON CONFLICT DO NOTHING;

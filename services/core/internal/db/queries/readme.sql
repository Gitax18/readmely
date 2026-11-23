-- A project's README versions. Callers check the project belongs to the user first (GetProject);
-- these are keyed by project.

-- name: GetLatestReadme :one
SELECT * FROM readme_versions WHERE project_id = @project_id ORDER BY version DESC LIMIT 1;

-- name: GetReadmeVersion :one
SELECT * FROM readme_versions WHERE project_id = @project_id AND version = @version;

-- name: ListReadmeVersions :many
SELECT id, project_id, version, source, template, created_at FROM readme_versions
WHERE project_id = @project_id ORDER BY version DESC LIMIT 50;

-- name: AddReadmeVersion :one
-- Adds the next version. With a base version, it adds only if that is still the newest (no rows
-- otherwise), so an edit made on top of an out-of-date copy is refused instead of overwriting.
INSERT INTO readme_versions (project_id, version, content, source, template, job_id)
SELECT @project_id::uuid, COALESCE(max(version), 0) + 1, @content::text, @source::text, sqlc.narg(template)::text, sqlc.narg(job_id)::uuid
FROM readme_versions WHERE project_id = @project_id::uuid
HAVING (@base_version::int = -1 OR COALESCE(max(version), 0) = @base_version::int)
RETURNING *;

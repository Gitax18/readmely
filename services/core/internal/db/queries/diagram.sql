-- A project's diagrams, one per kind. Callers check the project belongs to the user first (GetProject).

-- name: GetProjectDiagram :one
SELECT * FROM project_diagrams WHERE project_id = @project_id AND kind = @kind::text;

-- name: GetLatestDiagramJob :one
-- The newest diagram job of one kind. A job's kind is in its payload; jobs queued before kinds
-- existed have none and are architecture jobs.
SELECT * FROM jobs
WHERE project_id = @project_id AND user_id = @user_id AND kind = 'generate_diagram'
  AND COALESCE(payload ->> 'kind', 'architecture') = @diagram_kind::text
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- A project's diagram. Callers check the project belongs to the user first (GetProject).

-- name: GetProjectDiagram :one
SELECT * FROM project_diagrams WHERE project_id = @project_id;

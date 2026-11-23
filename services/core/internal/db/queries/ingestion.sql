-- What an analysis stores and the project page reads. Every read is scoped to the owner through
-- the project, so someone else's project finds nothing.

-- name: GetProjectFacts :one
SELECT f.* FROM project_facts f
JOIN projects p ON p.id = f.project_id
WHERE f.project_id = @project_id AND p.user_id = @user_id;

-- name: ListProjectFiles :many
SELECT f.path, f.language, f.size_bytes, f.line_count, f.skipped_reason FROM files f
JOIN projects p ON p.id = f.project_id
WHERE f.project_id = @project_id AND p.user_id = @user_id
ORDER BY f.path
LIMIT @row_limit;

-- name: SetProjectCommit :exec
UPDATE projects SET commit_sha = @commit_sha WHERE id = @id AND user_id = @user_id;

-- A new analysis replaces the old one completely. Symbols go with their files, edges with their nodes.

-- name: ClearProjectFiles :exec
DELETE FROM files WHERE project_id = @project_id;

-- name: ClearProjectGraph :exec
DELETE FROM graph_nodes WHERE project_id = @project_id;

-- name: UpsertProjectFacts :exec
INSERT INTO project_facts (project_id, commit_sha, facts)
VALUES (@project_id, @commit_sha, @facts)
ON CONFLICT (project_id) DO UPDATE SET commit_sha = EXCLUDED.commit_sha, facts = EXCLUDED.facts, analysed_at = now();

-- name: ListProjectHubs :many
-- The nodes other files import most. Scoped to the owner through the project.
SELECT n.kind, n.key, count(*)::int AS imported_by
FROM graph_edges e
JOIN graph_nodes n ON n.id = e.to_node
JOIN projects p ON p.id = e.project_id
WHERE e.project_id = @project_id AND p.user_id = @user_id AND n.kind <> 'external'
GROUP BY n.kind, n.key
ORDER BY imported_by DESC, n.key
LIMIT 10;

-- name: GetProjectSummary :one
SELECT s.* FROM project_summaries s
JOIN projects p ON p.id = s.project_id
WHERE s.project_id = @project_id AND p.user_id = @user_id;

-- name: ClearProjectSummary :exec
-- A new analysis starts without the old summary, so a summary never describes another commit.
DELETE FROM project_summaries WHERE project_id = @project_id;

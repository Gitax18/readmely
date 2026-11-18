-- Every project query is scoped to its owner (@user_id): asking for someone else's project
-- finds nothing, the same as asking for one that does not exist.

-- name: CreateProject :one
INSERT INTO projects (user_id, name) VALUES (@user_id, @name) RETURNING *;

-- name: GetProject :one
SELECT * FROM projects WHERE id = @id AND user_id = @user_id;

-- name: ListProjects :many
SELECT * FROM projects
WHERE user_id = @user_id
ORDER BY created_at DESC, id DESC
LIMIT @row_limit OFFSET @row_offset;

-- name: RenameProject :one
UPDATE projects SET name = @name WHERE id = @id AND user_id = @user_id RETURNING *;

-- name: SetProjectStatus :one
UPDATE projects SET status = @status WHERE id = @id AND user_id = @user_id RETURNING *;

-- name: DeleteProject :execrows
DELETE FROM projects WHERE id = @id AND user_id = @user_id;

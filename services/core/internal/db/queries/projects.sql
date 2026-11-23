-- Every project query is scoped to its owner (@user_id): asking for someone else's project
-- finds nothing, the same as asking for one that does not exist.

-- name: CreateProject :one
INSERT INTO projects (user_id, name) VALUES (@user_id, @name) RETURNING *;

-- name: CreateRepositoryProject :one
-- A project for a GitHub repository. A unique violation on (user, repository) means it is already added.
INSERT INTO projects (user_id, name, repo_owner, repo_name, repo_description, default_branch, repo_size_kb)
VALUES (@user_id, @name, @repo_owner, @repo_name, @repo_description, @default_branch, @repo_size_kb)
RETURNING *;

-- name: CountUserProjects :one
SELECT count(*) FROM projects WHERE user_id = @user_id;

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

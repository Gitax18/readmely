-- name: CreateUser :one
INSERT INTO users (email, display_name)
VALUES (@email, @display_name)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = @id;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(@email::text);

-- name: UpdateUserDisplayName :one
UPDATE users SET display_name = @display_name WHERE id = @id RETURNING *;

-- name: SetUserStatus :one
UPDATE users SET status = @status WHERE id = @id RETURNING *;

-- name: MarkUserEmailVerified :one
UPDATE users SET email_verified_at = COALESCE(email_verified_at, now()) WHERE id = @id RETURNING *;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = @id;

-- name: InsertWorkspace :exec
INSERT INTO workspaces (id, project_id, name, slug, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = ?;

-- name: ListWorkspaceSlugs :many
SELECT slug FROM workspaces WHERE project_id = ?;

-- name: UpdateWorkspaceStatus :execrows
UPDATE workspaces SET status = @to_status, updated_at = @updated_at WHERE id = @id AND status = @from_status;

-- name: DeleteWorkspace :exec
DELETE FROM workspaces WHERE id = ?;

-- name: ListWorkspaces :many
SELECT * FROM workspaces WHERE project_id = ? ORDER BY created_at, id;

-- name: ListWorkspacesByStatus :many
SELECT * FROM workspaces WHERE status = ? ORDER BY created_at, id;

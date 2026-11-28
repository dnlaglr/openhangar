-- name: InsertProject :exec
INSERT INTO projects (
  id, name, remote_url, default_branch, setup_script, run_script, sandbox_image, merge_mode, code_remote, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetProject :one
SELECT * FROM projects WHERE id = ?;

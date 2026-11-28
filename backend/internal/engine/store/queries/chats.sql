-- name: InsertChat :exec
INSERT INTO chats (id, workspace_id, title, provider_session_id, created_at) VALUES (?, ?, ?, ?, ?);

-- name: ListChats :many
SELECT * FROM chats WHERE workspace_id = ? ORDER BY created_at, id;

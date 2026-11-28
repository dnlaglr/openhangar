-- name: InsertChat :exec
INSERT INTO chats (id, workspace_id, title, provider, model, effort, provider_session_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetChat :one
SELECT * FROM chats WHERE id = ?;

-- name: ListChats :many
SELECT * FROM chats WHERE workspace_id = ? ORDER BY created_at, id;

-- name: SetChatProviderSessionID :exec
UPDATE chats SET provider_session_id = ? WHERE id = ?;
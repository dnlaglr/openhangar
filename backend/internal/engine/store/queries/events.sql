-- name: InsertEvent :exec
INSERT INTO events (id, chat_id, seq, author, machine_id, type, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListEvents :many
SELECT * FROM events WHERE chat_id = ? ORDER BY rowid;

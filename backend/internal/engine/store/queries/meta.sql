-- name: GetMeta :one
SELECT value FROM meta WHERE key = ?;

-- name: InsertMetaIfAbsent :exec
INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT (key) DO NOTHING;

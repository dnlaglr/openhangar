-- name: InsertEvent :exec
INSERT INTO events (id, chat_id, seq, author, machine_id, type, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: ListEvents :many
SELECT * FROM events WHERE chat_id = ? ORDER BY rowid;

-- name: ListOpenTurns :many
SELECT s.chat_id, CAST(json_extract(s.payload, '$.turn_id') AS TEXT) AS turn_id
FROM events s
WHERE s.type = 'turn_started'
  AND NOT EXISTS (
    SELECT 1 FROM events e
    WHERE e.chat_id = s.chat_id
      AND e.type = 'turn_ended'
      AND json_extract(e.payload, '$.turn_id') = json_extract(s.payload, '$.turn_id')
  )
ORDER BY s.created_at, s.id;

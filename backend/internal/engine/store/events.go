package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store/db"
)

type OpenTurn struct {
	ChatID string
	TurnID string
}

func (s *Store) AppendEvent(ctx context.Context, e domain.Event) error {
	return s.q.InsertEvent(ctx, db.InsertEventParams{
		ID:        e.ID,
		ChatID:    e.ChatID,
		Seq:       e.Seq,
		Author:    e.Author,
		MachineID: e.MachineID,
		Type:      e.Type,
		Payload:   string(e.Payload),
		CreatedAt: e.CreatedAt.UnixMilli(),
	})
}

func (s *Store) Events(ctx context.Context, chatID string) ([]domain.Event, error) {
	rows, err := s.q.ListEvents(ctx, chatID)

	return mapRows(rows, err, eventFromRow)
}

// EventsAfter returns the chat events stored after afterID, or all of them when afterID is empty.
// TODO: reads the whole chat because sqlc rejects rowid predicates; move to a hand-written rowid query if long chats make this slow.
func (s *Store) EventsAfter(ctx context.Context, chatID, afterID string) ([]domain.Event, error) {
	events, err := s.Events(ctx, chatID)
	if err != nil || afterID == "" {
		return events, err
	}

	i := slices.IndexFunc(events, func(e domain.Event) bool { return e.ID == afterID })
	if i < 0 {
		return nil, fmt.Errorf("event %s in chat %s: %w", afterID, chatID, domain.ErrNotFound)
	}

	return events[i+1:], nil
}

// OpenTurns lists turns that started but never ended, which only a crash mid-turn leaves behind.
func (s *Store) OpenTurns(ctx context.Context) ([]OpenTurn, error) {
	rows, err := s.q.ListOpenTurns(ctx)

	return mapRows(rows, err, func(r db.ListOpenTurnsRow) OpenTurn { return OpenTurn(r) })
}

func eventFromRow(row db.Event) domain.Event {
	return domain.Event{
		ID:        row.ID,
		ChatID:    row.ChatID,
		Seq:       row.Seq,
		Author:    row.Author,
		MachineID: row.MachineID,
		Type:      row.Type,
		Payload:   json.RawMessage(row.Payload),
		CreatedAt: fromMillis(row.CreatedAt),
	}
}

package store

import (
	"context"
	"encoding/json"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store/db"
)

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
	if err != nil {
		return nil, err
	}

	events := make([]domain.Event, len(rows))
	for i, row := range rows {
		events[i] = domain.Event{
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

	return events, nil
}

package store

import (
	"context"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store/db"
)

func (s *Store) CreateChat(ctx context.Context, c domain.Chat) error {
	return s.q.InsertChat(ctx, db.InsertChatParams{
		ID:                c.ID,
		WorkspaceID:       c.WorkspaceID,
		Title:             c.Title,
		Provider:          c.Agent.Provider,
		Model:             c.Agent.Model,
		Effort:            c.Agent.Effort,
		ProviderSessionID: nullable(c.ProviderSessionID),
		CreatedAt:         c.CreatedAt.UnixMilli(),
	})
}

func (s *Store) Chat(ctx context.Context, id string) (domain.Chat, error) {
	row, err := s.q.GetChat(ctx, id)
	if err != nil {
		return domain.Chat{}, notFound(err, "chat", id)
	}

	return chatFromRow(row), nil
}

func (s *Store) Chats(ctx context.Context, workspaceID string) ([]domain.Chat, error) {
	rows, err := s.q.ListChats(ctx, workspaceID)

	return mapRows(rows, err, chatFromRow)
}

func (s *Store) SetChatProviderSessionID(ctx context.Context, id, sessionID string) error {
	return s.q.SetChatProviderSessionID(ctx, db.SetChatProviderSessionIDParams{ID: id, ProviderSessionID: nullable(sessionID)})
}

func chatFromRow(row db.Chat) domain.Chat {
	return domain.Chat{
		ID:                row.ID,
		WorkspaceID:       row.WorkspaceID,
		Title:             row.Title,
		Agent:             domain.Agent{Provider: row.Provider, Model: row.Model, Effort: row.Effort},
		ProviderSessionID: deref(row.ProviderSessionID),
		CreatedAt:         fromMillis(row.CreatedAt),
	}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

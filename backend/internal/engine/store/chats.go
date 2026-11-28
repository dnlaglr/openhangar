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
		ProviderSessionID: nullable(c.ProviderSessionID),
		CreatedAt:         c.CreatedAt.UnixMilli(),
	})
}

func (s *Store) Chats(ctx context.Context, workspaceID string) ([]domain.Chat, error) {
	rows, err := s.q.ListChats(ctx, workspaceID)
	if err != nil {
		return nil, err
	}

	chats := make([]domain.Chat, len(rows))
	for i, row := range rows {
		chats[i] = domain.Chat{
			ID:                row.ID,
			WorkspaceID:       row.WorkspaceID,
			Title:             row.Title,
			ProviderSessionID: deref(row.ProviderSessionID),
			CreatedAt:         fromMillis(row.CreatedAt),
		}
	}

	return chats, nil
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

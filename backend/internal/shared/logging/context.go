package logging

import (
	"context"
	"log/slog"
)

type field int

const (
	workspaceID field = iota
	chatID
	turnID
	hostID
	fieldCount
)

var fieldKeys = [fieldCount]string{"workspace_id", "chat_id", "turn_id", "host_id"}

type ids [fieldCount]string

type ctxKey struct{}

func WithWorkspace(ctx context.Context, id string) context.Context { return with(ctx, workspaceID, id) }

func WithChat(ctx context.Context, id string) context.Context { return with(ctx, chatID, id) }

func WithTurn(ctx context.Context, id string) context.Context { return with(ctx, turnID, id) }

func WithHost(ctx context.Context, id string) context.Context { return with(ctx, hostID, id) }

func with(ctx context.Context, f field, id string) context.Context {
	v, _ := ctx.Value(ctxKey{}).(ids)
	v[f] = id

	return context.WithValue(ctx, ctxKey{}, v)
}

type contextHandler struct {
	slog.Handler
}

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	v, _ := ctx.Value(ctxKey{}).(ids)
	for f, id := range v {
		if id != "" {
			r.AddAttrs(slog.String(fieldKeys[f], id))
		}
	}

	return h.Handler.Handle(ctx, r)
}

func (h contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return contextHandler{h.Handler.WithAttrs(attrs)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}

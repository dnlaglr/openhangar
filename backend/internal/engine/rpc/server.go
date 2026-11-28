package rpc

import (
	"context"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	"github.com/dnlaglr/openhangar/backend/gen/hangar/v1/hangarv1connect"
	"github.com/dnlaglr/openhangar/backend/internal/engine/chat"
	"github.com/dnlaglr/openhangar/backend/internal/engine/diagnostics"
	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/project"
	"github.com/dnlaglr/openhangar/backend/internal/engine/workspace"
)

// Server implements every client-facing service of the engine.
type Server struct {
	Projects    project.Service
	Workspaces  workspace.Service
	Chats       *chat.Service
	LogLevel    *slog.LevelVar
	Diagnostics diagnostics.Options
}

var (
	_ hangarv1connect.ProjectServiceHandler   = (*Server)(nil)
	_ hangarv1connect.WorkspaceServiceHandler = (*Server)(nil)
	_ hangarv1connect.ChatServiceHandler      = (*Server)(nil)
	_ hangarv1connect.EngineServiceHandler    = (*Server)(nil)
)

// Handler serves all services and rejects any request that does not carry the bearer token.
func (s *Server) Handler(token string) http.Handler {
	opts := connect.WithInterceptors(errorMapper{})
	mux := http.NewServeMux()
	mux.Handle(hangarv1connect.NewProjectServiceHandler(s, opts))
	mux.Handle(hangarv1connect.NewWorkspaceServiceHandler(s, opts))
	mux.Handle(hangarv1connect.NewChatServiceHandler(s, opts))
	mux.Handle(hangarv1connect.NewEngineServiceHandler(s, opts))

	return authenticate(token, mux)
}

func authenticate(token string, next http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	errs := connect.NewErrorWriter()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			_ = errs.Write(w, r, connect.NewError(connect.CodeUnauthenticated, errors.New("missing or invalid token")))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type errorMapper struct{}

func (errorMapper) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		res, err := next(ctx, req)

		return res, connectError(ctx, req.Spec().Procedure, err)
	}
}

func (errorMapper) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (errorMapper) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		return connectError(ctx, conn.Spec().Procedure, next(ctx, conn))
	}
}

func connectError(ctx context.Context, procedure string, err error) error {
	var ce *connect.Error
	switch {
	case err == nil, errors.As(err, &ce), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, domain.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, domain.ErrInvalidTransition):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, domain.ErrInvalid):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		slog.ErrorContext(ctx, "rpc failed", "procedure", procedure, "err", err)
		return connect.NewError(connect.CodeInternal, err)
	}
}

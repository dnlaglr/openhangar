package rpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"

	v1 "github.com/dnlaglr/openhangar/backend/gen/hangar/v1"
	"github.com/dnlaglr/openhangar/backend/internal/engine/diagnostics"
	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
)

func (s *Server) AddProject(ctx context.Context, req *connect.Request[v1.AddProjectRequest]) (*connect.Response[v1.AddProjectResponse], error) {
	p, err := s.Projects.Add(ctx, projectFromProto(req.Msg))
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.AddProjectResponse{Project: projectToProto(p)}), nil
}

func (s *Server) ListProjects(ctx context.Context, _ *connect.Request[v1.ListProjectsRequest]) (*connect.Response[v1.ListProjectsResponse], error) {
	projects, err := s.Projects.Store.Projects(ctx)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.ListProjectsResponse{Projects: toProtos(projects, projectToProto)}), nil
}

func (s *Server) CreateWorkspace(ctx context.Context, req *connect.Request[v1.CreateWorkspaceRequest]) (*connect.Response[v1.CreateWorkspaceResponse], error) {
	agent := agentFromProto(req.Msg.GetAgent())
	if err := s.Chats.CheckAgent(agent); err != nil {
		return nil, err
	}

	w, c, err := s.Workspaces.Create(ctx, req.Msg.GetProjectId(), req.Msg.GetName(), agent)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.CreateWorkspaceResponse{Workspace: workspaceToProto(w), Chat: chatToProto(c)}), nil
}

func (s *Server) ListWorkspaces(ctx context.Context, req *connect.Request[v1.ListWorkspacesRequest]) (*connect.Response[v1.ListWorkspacesResponse], error) {
	workspaces, err := s.Workspaces.Store.Workspaces(ctx, req.Msg.GetProjectId())
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.ListWorkspacesResponse{Workspaces: toProtos(workspaces, workspaceToProto)}), nil
}

func (s *Server) ActivateWorkspace(ctx context.Context, req *connect.Request[v1.ActivateWorkspaceRequest]) (*connect.Response[v1.ActivateWorkspaceResponse], error) {
	w, err := s.Workspaces.Activate(ctx, req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.ActivateWorkspaceResponse{Workspace: workspaceToProto(w)}), nil
}

func (s *Server) IdleWorkspace(ctx context.Context, req *connect.Request[v1.IdleWorkspaceRequest]) (*connect.Response[v1.IdleWorkspaceResponse], error) {
	w, err := s.Workspaces.Idle(ctx, req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.IdleWorkspaceResponse{Workspace: workspaceToProto(w)}), nil
}

func (s *Server) ArchiveWorkspace(ctx context.Context, req *connect.Request[v1.ArchiveWorkspaceRequest]) (*connect.Response[v1.ArchiveWorkspaceResponse], error) {
	w, err := s.Workspaces.Archive(ctx, req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.ArchiveWorkspaceResponse{Workspace: workspaceToProto(w)}), nil
}

func (s *Server) CreateChat(ctx context.Context, req *connect.Request[v1.CreateChatRequest]) (*connect.Response[v1.CreateChatResponse], error) {
	c, err := s.Chats.Create(ctx, req.Msg.GetWorkspaceId(), req.Msg.GetTitle(), agentFromProto(req.Msg.GetAgent()))
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.CreateChatResponse{Chat: chatToProto(c)}), nil
}

func (s *Server) ListChats(ctx context.Context, req *connect.Request[v1.ListChatsRequest]) (*connect.Response[v1.ListChatsResponse], error) {
	chats, err := s.Chats.Store.Chats(ctx, req.Msg.GetWorkspaceId())
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.ListChatsResponse{Chats: toProtos(chats, chatToProto)}), nil
}

func (s *Server) PostMessage(ctx context.Context, req *connect.Request[v1.PostMessageRequest]) (*connect.Response[v1.PostMessageResponse], error) {
	e, err := s.Chats.Post(ctx, req.Msg.GetChatId(), req.Msg.GetText())
	if err != nil {
		return nil, err
	}

	pe, err := eventToProto(e)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.PostMessageResponse{Event: pe}), nil
}

func (s *Server) InterruptTurn(_ context.Context, req *connect.Request[v1.InterruptTurnRequest]) (*connect.Response[v1.InterruptTurnResponse], error) {
	return connect.NewResponse(&v1.InterruptTurnResponse{Interrupted: s.Chats.Interrupt(req.Msg.GetChatId())}), nil
}

func (s *Server) SubscribeChat(ctx context.Context, req *connect.Request[v1.SubscribeChatRequest], stream *connect.ServerStream[v1.SubscribeChatResponse]) error {
	sub, err := s.Chats.Subscribe(ctx, req.Msg.GetChatId(), req.Msg.GetAfterEventId())
	if err != nil {
		return err
	}
	defer sub.Close()

	// Flushing headers now lets the client know it is subscribed even when there is nothing to replay.
	if err := stream.Send(nil); err != nil {
		return err
	}

	send := func(e domain.Event) error {
		pe, err := eventToProto(e)
		if err != nil {
			return err
		}

		return stream.Send(&v1.SubscribeChatResponse{Event: pe})
	}

	for _, e := range sub.Replay {
		if err := send(e); err != nil {
			return err
		}
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case e, ok := <-sub.Events:
			if !ok {
				return connect.NewError(connect.CodeUnavailable, errors.New("subscriber fell behind, resubscribe after the last received event"))
			}
			if err := send(e); err != nil {
				return err
			}
		}
	}
}

func (s *Server) ExportDiagnostics(_ context.Context, req *connect.Request[v1.ExportDiagnosticsRequest]) (*connect.Response[v1.ExportDiagnosticsResponse], error) {
	opts := s.Diagnostics
	opts.AppVersion, opts.WSLVersion = req.Msg.GetAppVersion(), req.Msg.GetWslVersion()

	var buf bytes.Buffer
	if err := diagnostics.Write(&buf, opts); err != nil {
		return nil, err
	}

	return connect.NewResponse(&v1.ExportDiagnosticsResponse{Zip: buf.Bytes()}), nil
}

var logLevels = map[v1.LogLevel]slog.Level{
	v1.LogLevel_LOG_LEVEL_DEBUG: slog.LevelDebug,
	v1.LogLevel_LOG_LEVEL_INFO:  slog.LevelInfo,
	v1.LogLevel_LOG_LEVEL_WARN:  slog.LevelWarn,
	v1.LogLevel_LOG_LEVEL_ERROR: slog.LevelError,
}

func (s *Server) SetLogLevel(_ context.Context, req *connect.Request[v1.SetLogLevelRequest]) (*connect.Response[v1.SetLogLevelResponse], error) {
	level, ok := logLevels[req.Msg.GetLevel()]
	if !ok {
		return nil, fmt.Errorf("log level %s: %w", req.Msg.GetLevel(), domain.ErrInvalid)
	}
	s.LogLevel.Set(level)

	return connect.NewResponse(&v1.SetLogLevelResponse{}), nil
}

package rpc

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/dnlaglr/openhangar/backend/gen/hangar/v1"
	"github.com/dnlaglr/openhangar/backend/gen/hangar/v1/hangarv1connect"
	"github.com/dnlaglr/openhangar/backend/internal/engine/chat"
	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
	"github.com/dnlaglr/openhangar/backend/internal/engine/gittest"
	"github.com/dnlaglr/openhangar/backend/internal/engine/project"
	"github.com/dnlaglr/openhangar/backend/internal/engine/provider"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox/worktree"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store"
	"github.com/dnlaglr/openhangar/backend/internal/engine/workspace"
)

const token = "test-token"

type turnFunc func(ctx context.Context, req provider.Request, emit func(domain.Payload) error) (provider.Result, error)

func (f turnFunc) Run(ctx context.Context, req provider.Request, emit func(domain.Payload) error) (provider.Result, error) {
	return f(ctx, req, emit)
}

type env struct {
	chats      hangarv1connect.ChatServiceClient
	workspaces hangarv1connect.WorkspaceServiceClient
	store      *store.Store
	url        string
	ws         *v1.Workspace
	chat       *v1.Chat
}

func newEnv(t *testing.T, turn turnFunc) env {
	t.Helper()
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.com")
	}

	root := t.TempDir()
	st, err := store.Open(t.Context(), filepath.Join(root, "engine.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	repos := git.Repos{Root: filepath.Join(root, "repos")}
	rt := worktree.Runtime{Root: filepath.Join(root, "worktrees")}
	chats := &chat.Service{Store: st, Repos: repos, Runtime: rt, Providers: map[string]provider.Provider{"fake": turn}}
	srv := &Server{
		Projects:   project.Service{Store: st, Repos: repos},
		Workspaces: workspace.Service{Store: st, Repos: repos, Runtime: rt},
		Chats:      chats,
		LogLevel:   new(slog.LevelVar),
	}

	hs := httptest.NewServer(srv.Handler(token))
	t.Cleanup(hs.Close)
	t.Cleanup(chats.Close)

	e := env{
		chats:      hangarv1connect.NewChatServiceClient(authed(token), hs.URL),
		workspaces: hangarv1connect.NewWorkspaceServiceClient(authed(token), hs.URL),
		store:      st,
		url:        hs.URL,
	}

	projects := hangarv1connect.NewProjectServiceClient(authed(token), hs.URL)
	p, err := projects.AddProject(t.Context(), connect.NewRequest(&v1.AddProjectRequest{Name: "hangar", RemoteUrl: gittest.NewRemote(t)}))
	if err != nil {
		t.Fatal(err)
	}
	ws, err := e.workspaces.CreateWorkspace(t.Context(), connect.NewRequest(&v1.CreateWorkspaceRequest{
		ProjectId: p.Msg.GetProject().GetId(),
		Name:      "Fix login",
		Agent:     &v1.Agent{Provider: "fake"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	e.ws, e.chat = ws.Msg.GetWorkspace(), ws.Msg.GetChat()

	return e
}

type bearer string

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+string(b))

	return http.DefaultTransport.RoundTrip(r)
}

func authed(token string) *http.Client {
	return &http.Client{Transport: bearer(token)}
}

func (e env) post(t *testing.T, text string) *v1.Event {
	t.Helper()
	res, err := e.chats.PostMessage(t.Context(), connect.NewRequest(&v1.PostMessageRequest{ChatId: e.chat.GetId(), Text: text}))
	if err != nil {
		t.Fatal(err)
	}

	return res.Msg.GetEvent()
}

func (e env) subscribe(t *testing.T, afterID string) *connect.ServerStreamForClient[v1.SubscribeChatResponse] {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	stream, err := e.chats.SubscribeChat(ctx, connect.NewRequest(&v1.SubscribeChatRequest{ChatId: e.chat.GetId(), AfterEventId: afterID}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stream.Close() })

	return stream
}

func receive(t *testing.T, stream *connect.ServerStreamForClient[v1.SubscribeChatResponse], n int) []*v1.Event {
	t.Helper()
	var events []*v1.Event
	for len(events) < n && stream.Receive() {
		events = append(events, stream.Msg().GetEvent())
	}
	if len(events) < n {
		t.Fatalf("received %d events, want %d: %v", len(events), n, stream.Err())
	}

	return events
}

func ids(events []*v1.Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.GetId()
	}

	return out
}

func (e env) storedIDs(t *testing.T) []string {
	t.Helper()
	events, err := e.store.Events(t.Context(), e.chat.GetId())
	if err != nil {
		t.Fatal(err)
	}

	out := make([]string, len(events))
	for i, ev := range events {
		out[i] = ev.ID
	}

	return out
}

func TestRejectsMissingOrWrongToken(t *testing.T) {
	e := newEnv(t, nil)

	for name, client := range map[string]*http.Client{"missing": http.DefaultClient, "wrong": authed("nope")} {
		projects := hangarv1connect.NewProjectServiceClient(client, e.url)
		_, err := projects.ListProjects(t.Context(), connect.NewRequest(&v1.ListProjectsRequest{}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s token, unary: err = %v, want unauthenticated", name, err)
		}

		chats := hangarv1connect.NewChatServiceClient(client, e.url)
		stream, err := chats.SubscribeChat(t.Context(), connect.NewRequest(&v1.SubscribeChatRequest{ChatId: e.chat.GetId()}))
		if err == nil {
			for stream.Receive() {
				t.Errorf("%s token: stream delivered an event", name)
			}
			err = stream.Err()
		}
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("%s token, stream: err = %v, want unauthenticated", name, err)
		}
	}
}

func TestSubscriberReceivesPostedTurn(t *testing.T) {
	e := newEnv(t, func(_ context.Context, req provider.Request, emit func(domain.Payload) error) (provider.Result, error) {
		calls := []domain.Payload{
			domain.ToolCall{TurnID: req.TurnID, CallID: "c1", Name: "Read", Input: []byte(`{"file_path":"README.md"}`)},
			domain.ToolResult{TurnID: req.TurnID, CallID: "c1", Output: "hangar"},
			domain.AgentText{TurnID: req.TurnID, Text: "It says hangar."},
		}
		for _, p := range calls {
			if err := emit(p); err != nil {
				return provider.Result{}, err
			}
		}

		return provider.Result{DurationMS: 7}, nil
	})

	stream := e.subscribe(t, "")
	posted := e.post(t, "What does the README say?")
	got := receive(t, stream, 6)

	if got[0].GetId() != posted.GetId() || got[0].GetUserMessage().GetText() != "What does the README say?" {
		t.Fatalf("first event = %v, want the posted message", got[0])
	}
	if started := got[1].GetTurnStarted(); !slices.Equal(started.GetMessageIds(), []string{posted.GetId()}) || started.GetAgent().GetProvider() != "fake" {
		t.Fatalf("turn_started = %v", started)
	}
	if path := got[2].GetToolCall().GetInput().GetFields()["file_path"].GetStringValue(); path != "README.md" {
		t.Fatalf("tool_call input file_path = %q", path)
	}
	if got[3].GetToolResult().GetOutput() != "hangar" || got[4].GetAgentText().GetText() != "It says hangar." {
		t.Fatalf("tool_result/agent_text = %v / %v", got[3], got[4])
	}
	if ended := got[5].GetTurnEnded(); ended.GetOutcome() != v1.Outcome_OUTCOME_COMPLETED || ended.GetDurationMs() != 7 {
		t.Fatalf("turn_ended = %v", ended)
	}
}

func TestSubscribeReplaysThenStreamsLiveWithoutGapOrDuplicate(t *testing.T) {
	emitted, release := make(chan struct{}), make(chan struct{})
	e := newEnv(t, func(_ context.Context, req provider.Request, emit func(domain.Payload) error) (provider.Result, error) {
		if err := emit(domain.AgentText{TurnID: req.TurnID, Text: "one"}); err != nil {
			return provider.Result{}, err
		}
		close(emitted)
		<-release

		return provider.Result{}, emit(domain.AgentText{TurnID: req.TurnID, Text: "two"})
	})

	e.post(t, "count")
	<-emitted

	full := e.subscribe(t, "")
	replayed := receive(t, full, 3)
	resumed := e.subscribe(t, replayed[1].GetId())

	close(release)
	all := append(replayed, receive(t, full, 2)...)
	if want := e.storedIDs(t); !slices.Equal(ids(all), want) {
		t.Fatalf("replay + live = %v, want stored order %v", ids(all), want)
	}
	if all[3].GetAgentText().GetText() != "two" || all[4].GetTurnEnded() == nil {
		t.Fatalf("live events = %v", all[3:])
	}

	if got := ids(receive(t, resumed, 3)); !slices.Equal(got, ids(all[2:])) {
		t.Fatalf("resume after %s = %v, want %v", replayed[1].GetId(), got, ids(all[2:]))
	}
}

func TestDomainErrorsMapToConnectCodes(t *testing.T) {
	e := newEnv(t, func(context.Context, provider.Request, func(domain.Payload) error) (provider.Result, error) {
		return provider.Result{}, nil
	})
	ctx := t.Context()

	subscribeErr := func(chatID, afterID string) error {
		stream, err := e.chats.SubscribeChat(ctx, connect.NewRequest(&v1.SubscribeChatRequest{ChatId: chatID, AfterEventId: afterID}))
		if err != nil {
			return err
		}
		defer stream.Close()
		stream.Receive()

		return stream.Err()
	}

	tests := []struct {
		name string
		call func() error
		want connect.Code
	}{
		{"post to unknown chat", func() error {
			_, err := e.chats.PostMessage(ctx, connect.NewRequest(&v1.PostMessageRequest{ChatId: domain.NewID(), Text: "hi"}))
			return err
		}, connect.CodeNotFound},
		{"post empty message", func() error {
			_, err := e.chats.PostMessage(ctx, connect.NewRequest(&v1.PostMessageRequest{ChatId: e.chat.GetId(), Text: "  "}))
			return err
		}, connect.CodeInvalidArgument},
		{"workspace with unknown provider", func() error {
			_, err := e.workspaces.CreateWorkspace(ctx, connect.NewRequest(&v1.CreateWorkspaceRequest{ProjectId: e.ws.GetProjectId(), Name: "x", Agent: &v1.Agent{Provider: "nope"}}))
			return err
		}, connect.CodeInvalidArgument},
		{"archive active workspace", func() error {
			_, err := e.workspaces.ArchiveWorkspace(ctx, connect.NewRequest(&v1.ArchiveWorkspaceRequest{WorkspaceId: e.ws.GetId()}))
			return err
		}, connect.CodeFailedPrecondition},
		{"subscribe to unknown chat", func() error { return subscribeErr(domain.NewID(), "") }, connect.CodeNotFound},
		{"subscribe after unknown event", func() error { return subscribeErr(e.chat.GetId(), domain.NewID()) }, connect.CodeNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			var ce *connect.Error
			if !errors.As(err, &ce) || ce.Code() != tt.want {
				t.Fatalf("err = %v, want %s", err, tt.want)
			}
		})
	}
}

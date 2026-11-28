package chat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
	"github.com/dnlaglr/openhangar/backend/internal/engine/gittest"
	"github.com/dnlaglr/openhangar/backend/internal/engine/project"
	"github.com/dnlaglr/openhangar/backend/internal/engine/provider"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox/worktree"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store"
	"github.com/dnlaglr/openhangar/backend/internal/engine/workspace"
)

type turnFunc func(ctx context.Context, req provider.Request, emit func(domain.Payload) error) (provider.Result, error)

type fakeProvider struct {
	turn turnFunc

	mu        sync.Mutex
	calls     []provider.Request
	active    int
	maxActive int
}

func (f *fakeProvider) Run(ctx context.Context, req provider.Request, emit func(domain.Payload) error) (provider.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.active++
	f.maxActive = max(f.maxActive, f.active)
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()

	return f.turn(ctx, req, emit)
}

type env struct {
	svc  *Service
	fake *fakeProvider
	ws   domain.Workspace
	chat domain.Chat
	dir  string
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
	p, err := project.Service{Store: st, Repos: repos}.Add(t.Context(), domain.Project{
		Name:       "hangar",
		RemoteURL:  gittest.NewRemote(t),
		MergeMode:  domain.MergePR,
		CodeRemote: domain.CodeRemoteProject,
	})
	if err != nil {
		t.Fatal(err)
	}

	ws, chat, err := workspace.Service{Store: st, Repos: repos, Runtime: rt}.Create(t.Context(), p.ID, "Fix login", domain.Agent{Provider: "fake"})
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakeProvider{turn: turn}
	svc := &Service{Store: st, Repos: repos, Runtime: rt, Providers: map[string]provider.Provider{"fake": fake}}
	t.Cleanup(svc.Close)

	return env{svc: svc, fake: fake, ws: ws, chat: chat, dir: rt.Dir(ws.ID)}
}

func (e env) post(t *testing.T, chatID, text string) domain.Event {
	t.Helper()
	ev, err := e.svc.Post(t.Context(), chatID, text)
	if err != nil {
		t.Fatal(err)
	}

	return ev
}

func (e env) events(t *testing.T, chatID string, typ string) []domain.Event {
	t.Helper()
	all, err := e.svc.Store.Events(t.Context(), chatID)
	if err != nil {
		t.Fatal(err)
	}

	var out []domain.Event
	for _, ev := range all {
		if typ == "" || ev.Type == typ {
			out = append(out, ev)
		}
	}

	return out
}

func decode[T any](t *testing.T, e domain.Event) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(e.Payload, &v); err != nil {
		t.Fatal(err)
	}

	return v
}

func (e env) write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTurnStoresEventsAndCheckpoints(t *testing.T) {
	var e env
	e = newEnv(t, func(_ context.Context, req provider.Request, emit func(domain.Payload) error) (provider.Result, error) {
		e.write(t, "notes.txt", "fixed\n")
		if err := emit(domain.AgentText{TurnID: req.TurnID, Text: "done"}); err != nil {
			return provider.Result{}, err
		}

		return provider.Result{SessionID: "sess-1", DurationMS: 42}, nil
	})

	msg := e.post(t, e.chat.ID, "Fix the login bug\nIt fails on empty passwords.")
	e.svc.wg.Wait()

	var types []string
	for _, ev := range e.events(t, e.chat.ID, "") {
		types = append(types, ev.Type)
	}
	want := []string{domain.EventUserMessage, domain.EventTurnStarted, domain.EventAgentText, domain.EventTurnEnded}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("event types = %v, want %v", types, want)
	}

	started := decode[domain.TurnStarted](t, e.events(t, e.chat.ID, domain.EventTurnStarted)[0])
	if !reflect.DeepEqual(started.MessageIDs, []string{msg.ID}) || started.Provider != "fake" {
		t.Fatalf("turn_started = %+v", started)
	}

	ended := decode[domain.TurnEnded](t, e.events(t, e.chat.ID, domain.EventTurnEnded)[0])
	repo := e.svc.Repos.Dir(e.ws.ProjectID)
	if ended.Outcome != domain.OutcomeCompleted || ended.Commit == "" || ended.TurnID != started.TurnID {
		t.Fatalf("turn_ended = %+v", ended)
	}
	if head := gittest.Must(t, repo, "rev-parse", e.ws.Branch()); head != ended.Commit {
		t.Fatalf("branch head %s, want checkpoint %s", head, ended.Commit)
	}
	if subj := gittest.Must(t, repo, "log", "-1", "--format=%s", e.ws.Branch()); subj != "Fix the login bug" {
		t.Fatalf("checkpoint subject = %q", subj)
	}

	c, err := e.svc.Store.Chat(t.Context(), e.chat.ID)
	if err != nil || c.ProviderSessionID != "sess-1" {
		t.Fatalf("chat = %+v, %v; want provider session sess-1", c, err)
	}

	e.post(t, e.chat.ID, "Anything else?")
	e.svc.wg.Wait()

	if e.fake.calls[1].SessionID != "sess-1" {
		t.Fatalf("second turn session = %q, want resume of sess-1", e.fake.calls[1].SessionID)
	}
	if again := decode[domain.TurnEnded](t, e.events(t, e.chat.ID, domain.EventTurnEnded)[1]); again.Commit != "" {
		t.Fatalf("turn without changes committed %s", again.Commit)
	}
}

func TestQueuedMessagesRunInOrderOneTurnAtATime(t *testing.T) {
	started, release := make(chan struct{}, 3), make(chan struct{})
	e := newEnv(t, func(ctx context.Context, req provider.Request, _ func(domain.Payload) error) (provider.Result, error) {
		started <- struct{}{}
		<-release

		return provider.Result{}, nil
	})

	other := domain.Chat{ID: domain.NewID(), WorkspaceID: e.ws.ID, Title: "Review", Agent: domain.Agent{Provider: "fake"}, CreatedAt: domain.Now()}
	if err := e.svc.Store.CreateChat(t.Context(), other); err != nil {
		t.Fatal(err)
	}

	e.post(t, e.chat.ID, "a1")
	<-started
	e.post(t, other.ID, "b1")
	a2 := e.post(t, e.chat.ID, "a2")
	a3 := e.post(t, e.chat.ID, "a3")

	if queue := e.events(t, e.chat.ID, domain.EventUserMessage); len(queue) != 3 {
		t.Fatalf("user messages = %d, want 3 stored while the turn runs", len(queue))
	}

	close(release)
	e.svc.wg.Wait()

	var prompts [][]string
	for _, req := range e.fake.calls {
		prompts = append(prompts, req.Prompts)
	}
	if want := [][]string{{"a1"}, {"b1"}, {"a2", "a3"}}; !reflect.DeepEqual(prompts, want) {
		t.Fatalf("turn prompts = %v, want %v", prompts, want)
	}
	if e.fake.maxActive != 1 {
		t.Fatalf("%d turns ran at once in one workspace", e.fake.maxActive)
	}

	second := decode[domain.TurnStarted](t, e.events(t, e.chat.ID, domain.EventTurnStarted)[1])
	if !reflect.DeepEqual(second.MessageIDs, []string{a2.ID, a3.ID}) {
		t.Fatalf("second turn consumed %v, want [%s %s]", second.MessageIDs, a2.ID, a3.ID)
	}
}

func TestInterruptCheckpointsPartialWork(t *testing.T) {
	started := make(chan struct{})
	var e env
	e = newEnv(t, func(ctx context.Context, _ provider.Request, _ func(domain.Payload) error) (provider.Result, error) {
		e.write(t, "partial.txt", "half done\n")
		close(started)
		<-ctx.Done()

		return provider.Result{SessionID: "sess-1"}, ctx.Err()
	})

	e.post(t, e.chat.ID, "Long task")
	<-started
	if !e.svc.Interrupt(e.chat.ID) {
		t.Fatal("no running turn to interrupt")
	}
	e.svc.wg.Wait()

	ended := decode[domain.TurnEnded](t, e.events(t, e.chat.ID, domain.EventTurnEnded)[0])
	if ended.Outcome != domain.OutcomeInterrupted || ended.Error != "" || ended.Commit == "" {
		t.Fatalf("turn_ended = %+v, want interrupted with a checkpoint", ended)
	}
	if files := gittest.Must(t, e.dir, "show", "--name-only", "--format=", ended.Commit); !strings.Contains(files, "partial.txt") {
		t.Fatalf("checkpoint files = %q", files)
	}
}

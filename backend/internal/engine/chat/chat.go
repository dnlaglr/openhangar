package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
	"github.com/dnlaglr/openhangar/backend/internal/engine/git"
	"github.com/dnlaglr/openhangar/backend/internal/engine/provider"
	"github.com/dnlaglr/openhangar/backend/internal/engine/sandbox"
	"github.com/dnlaglr/openhangar/backend/internal/engine/store"
	"github.com/dnlaglr/openhangar/backend/internal/shared/logging"
)

// Service runs chat turns one at a time per workspace, because all chats of a workspace share one worktree.
type Service struct {
	Store     *store.Store
	Repos     git.Repos
	Runtime   sandbox.Runtime
	Providers map[string]provider.Provider

	mu      sync.Mutex
	base    context.Context
	stop    context.CancelFunc
	workers map[string]bool
	turns   map[string]context.CancelFunc
	wg      sync.WaitGroup
}

// TODO: on engine start, end turns left open by a crash and kick workspaces that still have queued messages.
func (s *Service) Post(ctx context.Context, chatID, text string) (domain.Event, error) {
	if strings.TrimSpace(text) == "" {
		return domain.Event{}, errors.New("message is empty")
	}

	c, err := s.Store.Chat(ctx, chatID)
	if err != nil {
		return domain.Event{}, err
	}
	if _, ok := s.Providers[c.Agent.Provider]; !ok {
		return domain.Event{}, fmt.Errorf("chat %s: unknown provider %q", chatID, c.Agent.Provider)
	}

	e, err := s.append(ctx, chatID, s.Store.MemberID(), domain.UserMessage{Text: text})
	if err != nil {
		return domain.Event{}, err
	}
	s.kick(c.WorkspaceID)

	return e, nil
}

func (s *Service) Interrupt(chatID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cancel, ok := s.turns[chatID]
	if ok {
		cancel()
	}

	return ok
}

// Close interrupts running turns and waits for their checkpoints.
func (s *Service) Close() {
	s.mu.Lock()
	if s.stop != nil {
		s.stop()
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Service) kick(workspaceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.base == nil {
		s.base, s.stop = context.WithCancel(context.Background())
		s.workers = map[string]bool{}
		s.turns = map[string]context.CancelFunc{}
	}
	if s.base.Err() != nil {
		return
	}
	if _, running := s.workers[workspaceID]; running {
		s.workers[workspaceID] = true

		return
	}

	s.workers[workspaceID] = false
	s.wg.Add(1)
	go s.work(logging.WithWorkspace(s.base, workspaceID), workspaceID)
}

func (s *Service) work(ctx context.Context, workspaceID string) {
	defer s.wg.Done()
	for {
		ran, err := s.runNext(ctx, workspaceID)
		if err != nil {
			slog.ErrorContext(ctx, "chat turn failed", "err", err)
		}
		if (ran && err == nil) || s.again(workspaceID) {
			continue
		}

		return
	}
}

// again reports whether a message arrived since the worker last looked, and retires the worker if not.
func (s *Service) again(workspaceID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.workers[workspaceID] {
		s.workers[workspaceID] = false

		return true
	}
	delete(s.workers, workspaceID)

	return false
}

func (s *Service) runNext(ctx context.Context, workspaceID string) (bool, error) {
	if ctx.Err() != nil {
		return false, nil
	}

	w, err := s.Store.Workspace(ctx, workspaceID)
	if err != nil || w.Status != domain.WorkspaceActive {
		return false, err
	}

	c, queue, err := s.nextQueue(ctx, workspaceID)
	if err != nil || len(queue) == 0 {
		return false, err
	}

	return true, s.runTurn(ctx, w, c, queue)
}

// nextQueue picks the chat whose oldest queued message is the oldest in the workspace.
func (s *Service) nextQueue(ctx context.Context, workspaceID string) (domain.Chat, []domain.Event, error) {
	chats, err := s.Store.Chats(ctx, workspaceID)
	if err != nil {
		return domain.Chat{}, nil, err
	}

	var next domain.Chat
	var nextQueue []domain.Event
	for _, c := range chats {
		events, err := s.Store.Events(ctx, c.ID)
		if err != nil {
			return domain.Chat{}, nil, err
		}
		q, err := queued(events)
		if err != nil {
			return domain.Chat{}, nil, err
		}
		if len(q) > 0 && (nextQueue == nil || q[0].ID < nextQueue[0].ID) {
			next, nextQueue = c, q
		}
	}

	return next, nextQueue, nil
}

// queued returns the user messages no turn_started has consumed yet.
func queued(events []domain.Event) ([]domain.Event, error) {
	consumed := map[string]bool{}
	for _, e := range events {
		if e.Type != domain.EventTurnStarted {
			continue
		}
		var started domain.TurnStarted
		if err := json.Unmarshal(e.Payload, &started); err != nil {
			return nil, err
		}
		for _, id := range started.MessageIDs {
			consumed[id] = true
		}
	}

	var q []domain.Event
	for _, e := range events {
		if e.Type == domain.EventUserMessage && !consumed[e.ID] {
			q = append(q, e)
		}
	}

	return q, nil
}

func (s *Service) runTurn(ctx context.Context, w domain.Workspace, c domain.Chat, queue []domain.Event) error {
	turnID := domain.NewID()
	ctx = logging.WithTurn(logging.WithChat(ctx, c.ID), turnID)

	ids, prompts, err := decodeMessages(queue)
	if err != nil {
		return err
	}
	started := domain.TurnStarted{TurnID: turnID, MessageIDs: ids, Agent: c.Agent}
	if _, err := s.append(ctx, c.ID, domain.AuthorAgent, started); err != nil {
		return err
	}

	turnCtx, cancel := context.WithCancel(ctx)
	s.track(c.ID, cancel)
	defer s.untrack(c.ID)

	spec := sandbox.NewSpec(w, s.Repos.Dir(w.ProjectID))
	req := provider.Request{
		Spec:      spec,
		TurnID:    turnID,
		SessionID: c.ProviderSessionID,
		Prompts:   prompts,
		Model:     c.Agent.Model,
		Effort:    c.Agent.Effort,
	}
	res, runErr := s.run(turnCtx, c, req)

	persist := context.WithoutCancel(ctx)
	var sessionErr error
	if res.SessionID != "" && res.SessionID != c.ProviderSessionID {
		sessionErr = s.Store.SetChatProviderSessionID(persist, c.ID, res.SessionID)
	}

	interrupted := turnCtx.Err() != nil
	if interrupted {
		runErr = nil
	}

	commit, checkpointErr := s.checkpoint(persist, spec, prompts[0])
	if checkpointErr != nil {
		checkpointErr = fmt.Errorf("checkpoint: %w", checkpointErr)
	}

	ended := domain.TurnEnded{TurnID: turnID, DurationMS: res.DurationMS, Usage: res.Usage, CostUSD: res.CostUSD, Commit: commit}
	switch failure := errors.Join(runErr, checkpointErr); {
	case failure != nil:
		ended.Outcome, ended.Error = domain.OutcomeFailed, failure.Error()
	case interrupted:
		ended.Outcome = domain.OutcomeInterrupted
	default:
		ended.Outcome = domain.OutcomeCompleted
	}

	_, err = s.append(persist, c.ID, domain.AuthorAgent, ended)

	return errors.Join(sessionErr, err)
}

func (s *Service) run(ctx context.Context, c domain.Chat, req provider.Request) (provider.Result, error) {
	p, ok := s.Providers[c.Agent.Provider]
	if !ok {
		return provider.Result{}, fmt.Errorf("unknown provider %q", c.Agent.Provider)
	}

	persist := context.WithoutCancel(ctx)

	return p.Run(ctx, req, func(payload domain.Payload) error {
		_, err := s.append(persist, c.ID, domain.AuthorAgent, payload)
		return err
	})
}

func (s *Service) track(chatID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turns[chatID] = cancel
}

func (s *Service) untrack(chatID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turns[chatID]()
	delete(s.turns, chatID)
}

func decodeMessages(queue []domain.Event) (ids, texts []string, err error) {
	for _, e := range queue {
		var m domain.UserMessage
		if err := json.Unmarshal(e.Payload, &m); err != nil {
			return nil, nil, err
		}
		ids, texts = append(ids, e.ID), append(texts, m.Text)
	}

	return ids, texts, nil
}

func (s *Service) append(ctx context.Context, chatID, author string, p domain.Payload) (domain.Event, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return domain.Event{}, err
	}

	e := domain.Event{
		ID:        domain.NewID(),
		ChatID:    chatID,
		Author:    author,
		MachineID: s.Store.MachineID(),
		Type:      p.EventType(),
		Payload:   payload,
		CreatedAt: domain.Now(),
	}

	return e, s.Store.AppendEvent(ctx, e)
}

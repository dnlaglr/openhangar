package chat

import (
	"context"

	"github.com/dnlaglr/openhangar/backend/internal/engine/domain"
)

const subscriberBuffer = 256

type Subscription struct {
	Replay []domain.Event
	// Events is closed when the subscriber falls more than subscriberBuffer events behind, or on Close.
	Events <-chan domain.Event
	Close  func()
}

// Subscribe returns the chat events after afterID followed by a live feed with no gap or overlap between the two.
func (s *Service) Subscribe(ctx context.Context, chatID, afterID string) (Subscription, error) {
	if _, err := s.Store.Chat(ctx, chatID); err != nil {
		return Subscription{}, err
	}

	s.events.Lock()
	defer s.events.Unlock()
	replay, err := s.Store.EventsAfter(ctx, chatID, afterID)
	if err != nil {
		return Subscription{}, err
	}

	ch := make(chan domain.Event, subscriberBuffer)
	if s.subs == nil {
		s.subs = map[string]map[chan domain.Event]struct{}{}
	}
	if s.subs[chatID] == nil {
		s.subs[chatID] = map[chan domain.Event]struct{}{}
	}
	s.subs[chatID][ch] = struct{}{}

	return Subscription{Replay: replay, Events: ch, Close: func() { s.unsubscribe(chatID, ch) }}, nil
}

func (s *Service) unsubscribe(chatID string, ch chan domain.Event) {
	s.events.Lock()
	defer s.events.Unlock()
	s.drop(chatID, ch)
}

// publish must run with s.events held. A full subscriber is dropped rather than allowed to stall the turn.
func (s *Service) publish(e domain.Event) {
	for ch := range s.subs[e.ChatID] {
		select {
		case ch <- e:
		default:
			s.drop(e.ChatID, ch)
		}
	}
}

func (s *Service) drop(chatID string, ch chan domain.Event) {
	if _, ok := s.subs[chatID][ch]; !ok {
		return
	}
	delete(s.subs[chatID], ch)
	if len(s.subs[chatID]) == 0 {
		delete(s.subs, chatID)
	}
	close(ch)
}

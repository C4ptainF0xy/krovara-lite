package eventsfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/permissions"
)

const Channel = "user_events"

type Event struct {
	UserID uuid.UUID `json:"user_id"`
	Type   string    `json:"type"`
	Data   any       `json:"data,omitempty"`
}

type Service struct {
	pool    *pgxpool.Pool
	log     *slog.Logger
	clients map[uuid.UUID]map[chan Event]struct{}
	mu      sync.RWMutex
}

func NewService(pool *pgxpool.Pool, log *slog.Logger) *Service {
	return &Service{
		pool:    pool,
		log:     log,
		clients: make(map[uuid.UUID]map[chan Event]struct{}),
	}
}

func (s *Service) Routes(r chi.Router, uidFn permissions.UserIDFunc) {
	r.Get("/me/events", s.handleWS(uidFn))
}

func Emit(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, eventType string, data any) error {
	ev := Event{
		UserID: userID,
		Type:   eventType,
		Data:   data,
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}

	_, err = pool.Exec(ctx, "SELECT pg_notify($1, $2)", Channel, string(b))
	return err
}

func (s *Service) Listen(ctx context.Context) error {
	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for ctx.Err() == nil {
		err := s.listenOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.log.Warn("eventsfeed listener disconnected", "err", err, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
	return ctx.Err()
}

func (s *Service) listenOnce(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{Channel}.Sanitize()); err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}
	s.log.Info("eventsfeed listener online", "channel", Channel)

	for {
		n, err := conn.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var ev Event
		if err := json.Unmarshal([]byte(n.Payload), &ev); err != nil {
			continue
		}
		s.broadcast(ev)
	}
}

func (s *Service) broadcast(ev Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if ev.UserID == uuid.Nil {
		for _, chans := range s.clients {
			for ch := range chans {
				select {
				case ch <- ev:
				default:
				}
			}
		}
		return
	}

	chans, ok := s.clients[ev.UserID]
	if !ok {
		return
	}
	for ch := range chans {
		select {
		case ch <- ev:
		default:

		}
	}
}

func (s *Service) addClient(uid uuid.UUID, ch chan Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients[uid] == nil {
		s.clients[uid] = make(map[chan Event]struct{})
	}
	s.clients[uid][ch] = struct{}{}
}

func (s *Service) removeClient(uid uuid.UUID, ch chan Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if chans, ok := s.clients[uid]; ok {
		delete(chans, ch)
		if len(chans) == 0 {
			delete(s.clients, uid)
		}
	}
}

func (s *Service) handleWS(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			s.log.Error("websocket accept failed", "err", err)
			return
		}
		defer c.Close(websocket.StatusInternalError, "internal error")

		ch := make(chan Event, 16)
		s.addClient(uid, ch)
		defer s.removeClient(uid, ch)

		ctx := r.Context()
		ctx = c.CloseRead(ctx)

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-ch:
				data, _ := json.Marshal(ev)
				if err := c.Write(ctx, websocket.MessageText, data); err != nil {
					return
				}
			case <-ticker.C:
				if err := c.Write(ctx, websocket.MessageText, []byte(`{"type":"ping"}`)); err != nil {
					return
				}
			}
		}
	}
}

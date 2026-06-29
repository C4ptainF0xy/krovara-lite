package polls

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

type Service struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool), pool: pool}
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/channels/{channelID}/polls", func(rr chi.Router) {
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
			Post("/", s.handleCreate(userIDFn))
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
			Get("/", s.handleList(userIDFn))
	})
	r.Route("/polls/{pollID}", func(rr chi.Router) {
		rr.Use(s.attachPollChannel)
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
			Post("/vote", s.handleVote(userIDFn))
		rr.With(permissions.RequireChannel(resolver, userIDFn, permissions.ViewChannel)).
			Post("/close", s.handleClose(userIDFn, resolver))
	})
}

func (s *Service) attachPollChannel(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "pollID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid poll id")
			return
		}
		p, err := s.q.GetPoll(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "poll not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		rctx := chi.RouteContext(r.Context())
		rctx.URLParams.Add("channelID", uuid.UUID(p.ChannelID.Bytes).String())
		next.ServeHTTP(w, r)
	})
}

type createReq struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

func (s *Service) handleCreate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, err := uuid.Parse(chi.URLParam(r, "channelID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		actor := uidFn(r.Context())
		var req createReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		req.Question = strings.TrimSpace(req.Question)
		if req.Question == "" || len(req.Question) > 300 {
			writeError(w, http.StatusBadRequest, "question is required (max 300)")
			return
		}
		opts := make([]string, 0, len(req.Options))
		for _, o := range req.Options {
			o = strings.TrimSpace(o)
			if o != "" {
				opts = append(opts, o)
			}
		}
		if len(opts) < 2 || len(opts) > 10 {
			writeError(w, http.StatusBadRequest, "between 2 and 10 options required")
			return
		}

		ch, err := s.q.GetChannel(r.Context(), pgUUID(channelID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "channel lookup failed")
			return
		}

		var poll db.Poll
		var options []db.PollOption
		err = pgx.BeginFunc(r.Context(), s.pool, func(tx pgx.Tx) error {
			qtx := s.q.WithTx(tx)
			p, err := qtx.CreatePoll(r.Context(), db.CreatePollParams{
				SpaceID:   ch.SpaceID,
				ChannelID: pgUUID(channelID),
				Question:  req.Question,
				CreatedBy: pgUUID(actor),
			})
			if err != nil {
				return err
			}
			poll = p
			for i, label := range opts {
				o, err := qtx.CreatePollOption(r.Context(), db.CreatePollOptionParams{
					PollID: p.ID, Label: label, Position: int32(i),
				})
				if err != nil {
					return err
				}
				options = append(options, o)
			}
			return nil
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		writeJSON(w, http.StatusCreated, pollDTO(poll, options, nil, uuid.Nil))
	}
}

func (s *Service) handleList(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, err := uuid.Parse(chi.URLParam(r, "channelID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		actor := uidFn(r.Context())
		polls, err := s.q.ListChannelPolls(r.Context(), pgUUID(channelID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(polls))
		for _, p := range polls {
			options, _ := s.q.ListPollOptions(r.Context(), p.ID)
			results, _ := s.q.PollResults(r.Context(), p.ID)
			counts := map[string]int64{}
			for _, rrow := range results {
				counts[uuid.UUID(rrow.OptionID.Bytes).String()] = rrow.Votes
			}
			myOption := uuid.Nil
			if mv, err := s.q.GetMyVote(r.Context(), db.GetMyVoteParams{PollID: p.ID, UserID: pgUUID(actor)}); err == nil {
				myOption = uuid.UUID(mv.Bytes)
			}
			out = append(out, pollDTO(p, options, counts, myOption))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type voteReq struct {
	OptionID string `json:"option_id"`
}

func (s *Service) handleVote(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pollID, err := uuid.Parse(chi.URLParam(r, "pollID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid poll id")
			return
		}
		actor := uidFn(r.Context())
		var req voteReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		optionID, err := uuid.Parse(req.OptionID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid option_id")
			return
		}

		poll, err := s.q.GetPoll(r.Context(), pgUUID(pollID))
		if err != nil {
			writeError(w, http.StatusNotFound, "poll not found")
			return
		}
		if poll.Closed {
			writeError(w, http.StatusConflict, "poll is closed")
			return
		}
		ok, err := s.q.OptionInPoll(r.Context(), db.OptionInPollParams{ID: pgUUID(optionID), PollID: pgUUID(pollID)})
		if err != nil || !ok {
			writeError(w, http.StatusBadRequest, "option does not belong to this poll")
			return
		}
		if err := s.q.CastVote(r.Context(), db.CastVoteParams{
			PollID: pgUUID(pollID), OptionID: pgUUID(optionID), UserID: pgUUID(actor),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "vote failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleClose(uidFn permissions.UserIDFunc, resolver permissions.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pollID, err := uuid.Parse(chi.URLParam(r, "pollID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid poll id")
			return
		}
		actor := uidFn(r.Context())
		poll, err := s.q.GetPoll(r.Context(), pgUUID(pollID))
		if err != nil {
			writeError(w, http.StatusNotFound, "poll not found")
			return
		}

		if uuid.UUID(poll.CreatedBy.Bytes) != actor {
			mc, err := resolver.ResolveChannel(r.Context(), actor, uuid.UUID(poll.ChannelID.Bytes))
			if err != nil || !permissions.Compute(mc).Has(permissions.ManageMessages) {
				writeError(w, http.StatusForbidden, "only the creator or a moderator may close this poll")
				return
			}
		}
		updated, err := s.q.ClosePoll(r.Context(), pgUUID(pollID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "close failed")
			return
		}
		options, _ := s.q.ListPollOptions(r.Context(), updated.ID)
		writeJSON(w, http.StatusOK, pollDTO(updated, options, nil, uuid.Nil))
	}
}

func pollDTO(p db.Poll, options []db.PollOption, counts map[string]int64, myOption uuid.UUID) map[string]any {
	opts := make([]map[string]any, 0, len(options))
	for _, o := range options {
		oid := uuid.UUID(o.ID.Bytes).String()
		votes := int64(0)
		if counts != nil {
			votes = counts[oid]
		}
		opts = append(opts, map[string]any{"id": oid, "label": o.Label, "votes": votes})
	}
	out := map[string]any{
		"id":         uuid.UUID(p.ID.Bytes).String(),
		"channel_id": uuid.UUID(p.ChannelID.Bytes).String(),
		"question":   p.Question,
		"created_by": uuid.UUID(p.CreatedBy.Bytes).String(),
		"closed":     p.Closed,
		"created_at": p.CreatedAt.Time,
		"options":    opts,
		"my_option":  nil,
	}
	if myOption != uuid.Nil {
		out["my_option"] = myOption.String()
	}
	return out
}

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: true} }

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

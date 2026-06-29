package friends

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/eventsfeed"
	"github.com/krovara/krovara/internal/permissions"
)

type Service struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool), pool: pool}
}

func (s *Service) Routes(r chi.Router, userIDFn permissions.UserIDFunc) {

	reqLimit := auth.NewKeyedRateLimitMiddleware(20, time.Minute, func(req *http.Request) string {
		if uid := userIDFn(req.Context()); uid != uuid.Nil {
			return "fr:" + uid.String()
		}
		return ""
	})
	r.Get("/me/friends", s.handleListFriends(userIDFn))
	r.Get("/me/friends/requests", s.handleListRequests(userIDFn))
	r.With(reqLimit).Post("/me/friends", s.handleRequest(userIDFn))
	r.Post("/me/friends/{id}/accept", s.handleAccept(userIDFn))
	r.Delete("/me/friends/{id}", s.handleRemove(userIDFn))
	r.Get("/me/blocks", s.handleListBlocks(userIDFn))
	r.Post("/me/blocks", s.handleBlock(userIDFn))
	r.Delete("/me/blocks/{userID}", s.handleUnblock(userIDFn))
	r.Put("/me/friend-settings", s.handleSettings(userIDFn))
}

type handleReq struct {
	Handle string `json:"handle"`
}

func (s *Service) handleRequest(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		self := uidFn(r.Context())
		var req handleReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Handle == "" {
			writeError(w, http.StatusBadRequest, "handle required")
			return
		}
		target, err := s.q.GetUserByUsername(r.Context(), req.Handle)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no user with that handle")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		targetID := uuid.UUID(target.ID.Bytes)
		if targetID == self {
			writeError(w, http.StatusBadRequest, "cannot add yourself")
			return
		}

		if blocked, _ := s.q.IsBlockedEitherWay(r.Context(), db.IsBlockedEitherWayParams{
			BlockerID: pgUUID(self), BlockedID: pgUUID(targetID),
		}); blocked {
			writeError(w, http.StatusForbidden, "cannot send request")
			return
		}

		switch target.WhoCanAdd {
		case "nobody":
			writeError(w, http.StatusForbidden, "this user is not accepting requests")
			return
		case "friends_of_friends":

			mutual, _ := s.q.HasMutualFriend(r.Context(), db.HasMutualFriendParams{
				RequesterID: pgUUID(self), RequesterID_2: pgUUID(targetID),
			})
			if !mutual {
				writeError(w, http.StatusForbidden, "you must share a mutual friend")
				return
			}
		}

		if existing, err := s.q.GetFriendshipByPair(r.Context(), db.GetFriendshipByPairParams{
			RequesterID: pgUUID(self), AddresseeID: pgUUID(targetID),
		}); err == nil {
			if existing.Status == "accepted" {
				writeError(w, http.StatusConflict, "already friends")
				return
			}

			if uuid.UUID(existing.RequesterID.Bytes) == targetID {
				acc, err := s.q.AcceptFriendRequest(r.Context(), db.AcceptFriendRequestParams{
					ID: existing.ID, AddresseeID: pgUUID(self),
				})
				if err != nil {
					writeError(w, http.StatusInternalServerError, "accept failed")
					return
				}
				_ = eventsfeed.Emit(r.Context(), s.pool, targetID, "friend_accept", map[string]any{
					"id": uuid.UUID(acc.ID.Bytes).String(),
				})
				writeJSON(w, http.StatusOK, map[string]any{
					"id": uuid.UUID(acc.ID.Bytes).String(), "status": acc.Status,
				})
				return
			}
			writeError(w, http.StatusConflict, "request already pending")
			return
		}
		fr, err := s.q.CreateFriendRequest(r.Context(), db.CreateFriendRequestParams{
			RequesterID: pgUUID(self), AddresseeID: pgUUID(targetID),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "request failed")
			return
		}

		_ = eventsfeed.Emit(r.Context(), s.pool, targetID, "friend_request", map[string]any{
			"id":   uuid.UUID(fr.ID.Bytes).String(),
			"from": self.String(),
		})

		writeJSON(w, http.StatusCreated, map[string]any{
			"id": uuid.UUID(fr.ID.Bytes).String(), "status": fr.Status,
		})
	}
}

func (s *Service) handleAccept(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		self := uidFn(r.Context())
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}
		fr, err := s.q.AcceptFriendRequest(r.Context(), db.AcceptFriendRequestParams{
			ID: pgUUID(id), AddresseeID: pgUUID(self),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no such pending request")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "accept failed")
			return
		}

		_ = eventsfeed.Emit(r.Context(), s.pool, uuid.UUID(fr.RequesterID.Bytes), "friend_accept", map[string]any{
			"id": uuid.UUID(fr.ID.Bytes).String(),
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"id": uuid.UUID(fr.ID.Bytes).String(), "status": fr.Status,
		})
	}
}

func (s *Service) handleRemove(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		self := uidFn(r.Context())
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid id")
			return
		}

		existing, _ := s.q.GetFriendshipByID(r.Context(), pgUUID(id))

		if err := s.q.DeleteFriendship(r.Context(), db.DeleteFriendshipParams{
			ID: pgUUID(id), RequesterID: pgUUID(self),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "remove failed")
			return
		}

		if existing.ID.Valid {
			targetID := existing.RequesterID
			if uuid.UUID(targetID.Bytes) == self {
				targetID = existing.AddresseeID
			}
			_ = eventsfeed.Emit(r.Context(), s.pool, uuid.UUID(targetID.Bytes), "friend_remove", map[string]any{
				"id": uuid.UUID(existing.ID.Bytes).String(),
			})
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleListFriends(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.q.ListAcceptedFriends(r.Context(), pgUUID(uidFn(r.Context())))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, f := range rows {
			out = append(out, map[string]any{
				"id":         uuid.UUID(f.ID.Bytes).String(),
				"username":   f.Username,
				"avatar_key": f.AvatarKey,
				"since":      f.CreatedAt.Time,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleListRequests(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		self := pgUUID(uidFn(r.Context()))
		in, err := s.q.ListIncomingRequests(r.Context(), self)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out, err := s.q.ListOutgoingRequests(r.Context(), self)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"incoming": reqDTOsIn(in),
			"outgoing": reqDTOsOut(out),
		})
	}
}

func (s *Service) handleListBlocks(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := s.q.ListBlocks(r.Context(), pgUUID(uidFn(r.Context())))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, b := range rows {
			out = append(out, map[string]any{
				"id": uuid.UUID(b.ID.Bytes).String(), "username": b.Username, "avatar_key": b.AvatarKey,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleBlock(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		self := uidFn(r.Context())
		var req handleReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Handle == "" {
			writeError(w, http.StatusBadRequest, "handle required")
			return
		}
		target, err := s.q.GetUserByUsername(r.Context(), req.Handle)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no user with that handle")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		targetID := uuid.UUID(target.ID.Bytes)
		if targetID == self {
			writeError(w, http.StatusBadRequest, "cannot block yourself")
			return
		}

		_ = s.q.DeleteFriendshipByPair(r.Context(), db.DeleteFriendshipByPairParams{
			RequesterID: pgUUID(self), AddresseeID: pgUUID(targetID),
		})
		if err := s.q.CreateBlock(r.Context(), db.CreateBlockParams{
			BlockerID: pgUUID(self), BlockedID: pgUUID(targetID),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "block failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"blocked": uuid.UUID(target.ID.Bytes).String()})
	}
}

func (s *Service) handleUnblock(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		self := uidFn(r.Context())
		targetID, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if err := s.q.DeleteBlock(r.Context(), db.DeleteBlockParams{
			BlockerID: pgUUID(self), BlockedID: pgUUID(targetID),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "unblock failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type settingsReq struct {
	WhoCanAdd string `json:"who_can_add"`
}

func (s *Service) handleSettings(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req settingsReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		switch req.WhoCanAdd {
		case "everyone", "friends_of_friends", "nobody":
		default:
			writeError(w, http.StatusBadRequest, "invalid who_can_add")
			return
		}
		if err := s.q.SetWhoCanAdd(r.Context(), db.SetWhoCanAddParams{
			ID: pgUUID(uidFn(r.Context())), WhoCanAdd: req.WhoCanAdd,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"who_can_add": req.WhoCanAdd})
	}
}

func reqDTOsIn(rows []db.ListIncomingRequestsRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, f := range rows {
		out = append(out, map[string]any{
			"id":         uuid.UUID(f.ID.Bytes).String(),
			"user_id":    uuid.UUID(f.UserID.Bytes).String(),
			"username":   f.Username,
			"avatar_key": f.AvatarKey,
		})
	}
	return out
}

func reqDTOsOut(rows []db.ListOutgoingRequestsRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, f := range rows {
		out = append(out, map[string]any{
			"id":         uuid.UUID(f.ID.Bytes).String(),
			"user_id":    uuid.UUID(f.UserID.Bytes).String(),
			"username":   f.Username,
			"avatar_key": f.AvatarKey,
		})
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

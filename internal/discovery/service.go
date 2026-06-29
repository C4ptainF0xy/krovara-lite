package discovery

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

var categories = map[string]bool{
	"gaming": true, "tech": true, "art": true, "music": true,
	"education": true, "community": true, "other": true,
}

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, q: db.New(pool)}
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Get("/discover", s.handleExplore())
	r.Post("/discover/{spaceID}/join", s.handleOpenJoin(userIDFn))
	r.Route("/spaces/{spaceID}/listing", func(rr chi.Router) {
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ViewChannel)).
			Get("/", s.handleGet())
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageSpace)).
			Put("/", s.handleList())
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageSpace)).
			Delete("/", s.handleDelist())
	})
}

func (s *Service) handleExplore() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		category := r.URL.Query().Get("category")
		if category != "" && !categories[category] {
			writeError(w, http.StatusBadRequest, "invalid category")
			return
		}
		limit := queryInt(r, "limit", 50, 1, 100)
		rows, err := s.q.ExploreListings(r.Context(), db.ExploreListingsParams{
			Column1: category, Column2: r.URL.Query().Get("q"), Limit: limit,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "explore failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, l := range rows {
			tags := l.Tags
			if tags == nil {
				tags = []string{}
			}
			out = append(out, map[string]any{
				"space_id":     uuid.UUID(l.SpaceID.Bytes).String(),
				"name":         l.Name,
				"description":  l.Description,
				"icon_key":     l.IconKey,
				"banner_key":   l.BannerKey,
				"category":     l.Category,
				"member_count": l.MemberCount,
				"tags":         tags,
				"language":     l.Language,
				"vanity_slug":  l.VanitySlug,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleOpenJoin(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		actor := uidFn(r.Context())
		if actor == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}

		listing, err := s.q.GetListing(r.Context(), pgUUID(spaceID))
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && listing.DelistedAt.Valid) {
			writeError(w, http.StatusForbidden, "this space is not open to join")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		form, err := s.q.GetJoinForm(r.Context(), pgUUID(spaceID))
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "gate lookup failed")
			return
		}
		if err == nil && form.Enabled {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": "this space requires a join request", "requires_form": true,
			})
			return
		}

		if _, err := s.q.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
			SpaceID: pgUUID(spaceID), UserID: pgUUID(actor),
		}); err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"space_id": spaceID.String()})
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "member lookup failed")
			return
		}

		banned, err := s.q.IsUserBanned(r.Context(), db.IsUserBannedParams{
			SpaceID: pgUUID(spaceID), UserID: pgUUID(actor),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ban check failed")
			return
		}
		if banned {
			writeError(w, http.StatusForbidden, "banned from this space")
			return
		}

		var memberID pgtype.UUID
		err = pgx.BeginFunc(r.Context(), s.pool, func(tx pgx.Tx) error {
			qtx := s.q.WithTx(tx)
			mem, err := qtx.CreateMember(r.Context(), db.CreateMemberParams{
				SpaceID: pgUUID(spaceID), UserID: pgUUID(actor),
			})
			if err != nil {
				return err
			}
			memberID = mem.ID
			_, _ = qtx.CreateAuditLog(r.Context(), db.CreateAuditLogParams{
				SpaceID:  pgUUID(spaceID),
				ActorID:  pgUUID(actor),
				Action:   "space.join",
				TargetID: mem.ID,
			})
			return nil
		})

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeJSON(w, http.StatusOK, map[string]any{"space_id": spaceID.String()})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "join failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"space_id":  spaceID.String(),
			"member_id": uuid.UUID(memberID.Bytes).String(),
		})
	}
}

func (s *Service) handleGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		l, err := s.q.GetListing(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{"listed": false})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"listed":       !l.DelistedAt.Valid,
			"category":     l.Category,
			"member_count": l.MemberCount,
		})
	}
}

type listReq struct {
	Category string `json:"category"`
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req listReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Category == "" {
			req.Category = "other"
		}
		if !categories[req.Category] {
			writeError(w, http.StatusBadRequest, "invalid category")
			return
		}
		l, err := s.q.UpsertListing(r.Context(), db.UpsertListingParams{
			SpaceID: pgUUID(id), Category: req.Category,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"listed": true, "category": l.Category, "member_count": l.MemberCount,
		})
	}
}

func (s *Service) handleDelist() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		if err := s.q.DelistSpace(r.Context(), pgUUID(id)); err != nil {
			writeError(w, http.StatusInternalServerError, "delist failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func queryInt(r *http.Request, key string, def, min, max int32) int32 {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if int32(n) < min {
		return min
	}
	if int32(n) > max {
		return max
	}
	return int32(n)
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

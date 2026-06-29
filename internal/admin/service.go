package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

const deletedEmailSuffix = "@deleted.krovara.com"

type Service struct {
	q *db.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: db.New(pool)}
}

func (s *Service) Routes(r chi.Router, userIDFn permissions.UserIDFunc) {
	r.Route("/admin", func(rr chi.Router) {
		rr.Use(s.requireAdmin(userIDFn))
		rr.Get("/users", s.handleListUsers())
		rr.Get("/users/{userID}/signals", s.handleUserSignals())
		rr.Patch("/users/{userID}", s.handleSetDisabled(userIDFn))
		rr.Delete("/users/{userID}", s.handleDeleteUser(userIDFn))
		rr.Put("/users/{userID}/badges", s.handleSetBadges())
		rr.Put("/users/{userID}/admin", s.handleSetAdmin())
		rr.Post("/users/{userID}/global-ban", s.handleGlobalBan(userIDFn))
		rr.Get("/reports", s.handleListReports())
	})
}

var knownBadges = map[string]bool{
	"founder": true, "owner": true, "staff": true, "admin": true,
	"moderator": true, "trial_mod": true, "developer": true, "designer": true,
	"contributor": true, "bug_hunter": true, "supporter": true, "booster": true,
	"partner": true, "verified": true, "early": true, "vip": true, "bot": true,
}

type setBadgesReq struct {
	Badges []string `json:"badges"`
}

func (s *Service) handleSetBadges() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		var req setBadgesReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}

		seen := map[string]bool{}
		clean := make([]string, 0, len(req.Badges))
		for _, b := range req.Badges {
			if knownBadges[b] && !seen[b] {
				seen[b] = true
				clean = append(clean, b)
			}
		}
		if err := s.q.SetUserBadges(r.Context(), db.SetUserBadgesParams{
			ID: pgtype.UUID{Bytes: uid, Valid: true}, Badges: clean,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "set badges failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"badges": clean})
	}
}

type setAdminReq struct {
	IsAdmin bool `json:"is_admin"`
}

func (s *Service) handleSetAdmin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		var req setAdminReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}

		if err := s.q.SetUserAdmin(r.Context(), db.SetUserAdminParams{
			ID:      pgtype.UUID{Bytes: uid, Valid: true},
			IsAdmin: req.IsAdmin,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "set admin failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"is_admin": req.IsAdmin})
	}
}

func (s *Service) requireAdmin(userIDFn permissions.UserIDFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid := userIDFn(r.Context())
			if uid == uuid.Nil {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			u, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
			if err != nil || !u.IsAdmin {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Service) handleListUsers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := queryInt(r, "limit", 50, 1, 200)
		offset := queryInt(r, "offset", 0, 0, 1<<30)
		rows, err := s.q.ListUsers(r.Context(), db.ListUsersParams{Limit: limit, Offset: offset})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, u := range rows {

			if strings.HasSuffix(u.Email, deletedEmailSuffix) {
				continue
			}
			out = append(out, userDTO(u))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleUserSignals() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetID, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		u, err := s.q.GetUserByID(r.Context(), pgUUID(targetID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		siblings := 0
		if u.SignupIpHash != nil {
			total, err := s.q.CountAccountsBySignupIPHash(r.Context(), u.SignupIpHash)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "count failed")
				return
			}
			if total > 1 {
				siblings = int(total - 1)
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id":               targetID.String(),
			"signup_ip_known":       u.SignupIpHash != nil,
			"signup_ip_sibling_acc": siblings,
		})
	}
}

type setDisabledReq struct {
	Disabled *bool `json:"disabled"`
}

func (s *Service) handleSetDisabled(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetID, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		var req setDisabledReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Disabled == nil {
			writeError(w, http.StatusBadRequest, "disabled is required")
			return
		}
		if targetID == uidFn(r.Context()) {
			writeError(w, http.StatusBadRequest, "cannot change your own account")
			return
		}

		u, err := s.q.SetUserDisabled(r.Context(), db.SetUserDisabledParams{
			ID:       pgUUID(targetID),
			Disabled: *req.Disabled,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}

		if *req.Disabled {
			_ = s.q.DeleteUserSessions(r.Context(), pgUUID(targetID))
		}
		writeJSON(w, http.StatusOK, userDTO(u))
	}
}

func (s *Service) handleDeleteUser(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetID, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if targetID == uidFn(r.Context()) {
			writeError(w, http.StatusBadRequest, "cannot delete your own account")
			return
		}

		anonUsername := "deleted-" + targetID.String()[:8]
		anonEmail := targetID.String() + deletedEmailSuffix
		if err := s.q.SoftDeleteUser(r.Context(), db.SoftDeleteUserParams{
			ID:       pgUUID(targetID),
			Username: anonUsername,
			Email:    anonEmail,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		_ = s.q.DeleteUserSessions(r.Context(), pgUUID(targetID))
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleGlobalBan(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		targetID, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		if targetID == uidFn(r.Context()) {
			writeError(w, http.StatusBadRequest, "cannot ban your own account")
			return
		}
		u, err := s.q.GetUserByID(r.Context(), pgUUID(targetID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		actor := pgUUID(uidFn(r.Context()))
		_ = s.q.CreateBannedIdentifier(r.Context(), db.CreateBannedIdentifierParams{
			Kind: "email", Lower: u.Email, BannedBy: actor,
		})
		_ = s.q.CreateBannedIdentifier(r.Context(), db.CreateBannedIdentifierParams{
			Kind: "username", Lower: u.Username, BannedBy: actor,
		})
		anonUsername := "deleted-" + targetID.String()[:8]
		anonEmail := targetID.String() + deletedEmailSuffix
		if err := s.q.SoftDeleteUser(r.Context(), db.SoftDeleteUserParams{
			ID: pgUUID(targetID), Username: anonUsername, Email: anonEmail,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "ban failed")
			return
		}
		_ = s.q.DeleteUserSessions(r.Context(), pgUUID(targetID))
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleListReports() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := queryInt(r, "limit", 50, 1, 200)
		offset := queryInt(r, "offset", 0, 0, 1<<30)
		var status *string
		if st := r.URL.Query().Get("status"); st != "" {
			status = &st
		}
		rows, err := s.q.ListReports(r.Context(), db.ListReportsParams{
			Limit: limit, Offset: offset, Status: status,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, rep := range rows {
			out = append(out, reportDTO(rep))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func userDTO(u db.User) map[string]any {

	return map[string]any{
		"id":           uuid.UUID(u.ID.Bytes).String(),
		"username":     u.Username,
		"display_name": u.DisplayName,
		"status":       u.Status,
		"avatar_key":   u.AvatarKey,
		"is_admin":     u.IsAdmin,
		"disabled":     u.Disabled,
		"badges":       u.Badges,
		"created_at":   u.CreatedAt.Time,
	}
}

func reportDTO(rep db.Report) map[string]any {
	out := map[string]any{
		"id":          uuid.UUID(rep.ID.Bytes).String(),
		"reporter_id": uuid.UUID(rep.ReporterID.Bytes).String(),
		"target_type": rep.TargetType,
		"target_id":   uuid.UUID(rep.TargetID.Bytes).String(),
		"reason":      rep.Reason,
		"status":      rep.Status,
		"created_at":  rep.CreatedAt.Time,
	}
	if rep.SpaceID.Valid {
		out["space_id"] = uuid.UUID(rep.SpaceID.Bytes).String()
	}
	if rep.ChannelID.Valid {
		out["channel_id"] = uuid.UUID(rep.ChannelID.Bytes).String()
	}
	return out
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

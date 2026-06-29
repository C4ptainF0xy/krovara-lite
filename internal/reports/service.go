package reports

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

type Service struct {
	pool *pgxpool.Pool
	q    *db.Queries
	v    *validator.Validate
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{
		pool: pool,
		q:    db.New(pool),
		v:    validator.New(validator.WithRequiredStructEnabled()),
	}
}

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Post("/reports", s.handleCreate(userIDFn))
	r.Route("/spaces/{spaceID}/reports", func(rr chi.Router) {
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageMessages)).
			Get("/", s.handleListSpace())
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageMessages)).
			Patch("/{reportID}", s.handleResolve(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageMessages)).
			Post("/bulk", s.handleBulkResolve(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageMessages)).
			Get("/count", s.handlePendingCount())
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageMessages)).
			Post("/{reportID}/claim", s.handleClaim(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageMessages)).
			Get("/{reportID}/comments", s.handleListComments())
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageMessages)).
			Post("/{reportID}/comments", s.handleAddComment(userIDFn))
	})
}

func (s *Service) reportInSpace(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid space id")
		return uuid.Nil, false
	}
	reportID, err := uuid.Parse(chi.URLParam(r, "reportID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid report id")
		return uuid.Nil, false
	}
	existing, err := s.q.GetReport(r.Context(), pgUUID(reportID))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!existing.SpaceID.Valid || uuid.UUID(existing.SpaceID.Bytes) != spaceID)) {
		writeError(w, http.StatusNotFound, "report not found")
		return uuid.Nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "report lookup failed")
		return uuid.Nil, false
	}
	return reportID, true
}

func (s *Service) handleListComments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reportID, ok := s.reportInSpace(w, r)
		if !ok {
			return
		}
		rows, err := s.q.ListReportComments(r.Context(), pgUUID(reportID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, c := range rows {
			out = append(out, commentDTO(c))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type commentReq struct {
	Body string `json:"body" validate:"required,max=4000"`
}

func (s *Service) handleAddComment(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reportID, ok := s.reportInSpace(w, r)
		if !ok {
			return
		}
		var req commentReq
		if !s.decode(w, r, &req) {
			return
		}
		c, err := s.q.CreateReportComment(r.Context(), db.CreateReportCommentParams{
			ReportID: pgUUID(reportID),
			AuthorID: pgUUID(uidFn(r.Context())),
			Body:     req.Body,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "comment failed")
			return
		}
		writeJSON(w, http.StatusCreated, commentDTO(c))
	}
}

func commentDTO(c db.ReportComment) map[string]any {
	out := map[string]any{
		"id":         uuid.UUID(c.ID.Bytes).String(),
		"report_id":  uuid.UUID(c.ReportID.Bytes).String(),
		"body":       c.Body,
		"created_at": c.CreatedAt.Time,
	}
	if c.AuthorID.Valid {
		out["author_id"] = uuid.UUID(c.AuthorID.Bytes).String()
	}
	return out
}

type bulkResolveReq struct {
	IDs    []string `json:"ids"    validate:"required,min=1,max=200,dive,uuid"`
	Status string   `json:"status" validate:"required,oneof=resolved dismissed"`
}

func (s *Service) handleBulkResolve(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req bulkResolveReq
		if !s.decode(w, r, &req) {
			return
		}
		actor := uidFn(r.Context())
		resolved := 0
		for _, raw := range req.IDs {
			reportID, err := uuid.Parse(raw)
			if err != nil {
				continue
			}
			existing, err := s.q.GetReport(r.Context(), pgUUID(reportID))
			if err != nil || !existing.SpaceID.Valid || uuid.UUID(existing.SpaceID.Bytes) != spaceID {
				continue
			}
			if _, err := s.q.ResolveReport(r.Context(), db.ResolveReportParams{
				ID: pgUUID(reportID), Status: &req.Status, ResolvedBy: pgUUID(actor),
			}); err != nil {
				continue
			}
			resolved++
			meta, _ := json.Marshal(map[string]any{"status": req.Status, "bulk": true})
			_, _ = s.q.CreateAuditLog(r.Context(), db.CreateAuditLogParams{
				SpaceID: pgUUID(spaceID), ActorID: pgUUID(actor),
				Action: "report.resolve", TargetID: pgUUID(reportID), Metadata: meta,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"resolved": resolved})
	}
}

const maxContextBytes = 16 * 1024

type createReq struct {
	TargetType string          `json:"target_type" validate:"required,oneof=message user content channel space"`
	TargetID   string          `json:"target_id"   validate:"required,uuid"`
	Reason     string          `json:"reason"      validate:"required,max=1000"`
	Category   *string         `json:"category"    validate:"omitempty,oneof=spam harassment illegal nsfw other"`
	Context    json.RawMessage `json:"context"     validate:"omitempty"`
	SpaceID    *string         `json:"space_id"    validate:"omitempty,uuid"`
	ChannelID  *string         `json:"channel_id"  validate:"omitempty,uuid"`
}

func (s *Service) handleCreate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createReq
		if !s.decode(w, r, &req) {
			return
		}
		targetID, err := uuid.Parse(req.TargetID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid target_id")
			return
		}

		if len(req.Context) > maxContextBytes {
			writeError(w, http.StatusBadRequest, "context too large")
			return
		}

		channelID := optUUID(req.ChannelID)
		spaceID := optUUID(req.SpaceID)

		if channelID.Valid && !spaceID.Valid {
			ch, err := s.q.GetChannel(r.Context(), channelID)
			if err == nil {
				spaceID = ch.SpaceID
			} else if !errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusInternalServerError, "channel lookup failed")
				return
			}
		}

		rep, err := s.q.CreateReport(r.Context(), db.CreateReportParams{
			ReporterID: pgUUID(uidFn(r.Context())),
			TargetType: req.TargetType,
			TargetID:   pgUUID(targetID),
			Reason:     req.Reason,
			SpaceID:    spaceID,
			ChannelID:  channelID,
			Category:   req.Category,
			Context:    req.Context,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "report failed")
			return
		}
		writeJSON(w, http.StatusCreated, reportDTO(rep))
	}
}

func (s *Service) handleListSpace() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var status *string
		if s := r.URL.Query().Get("status"); s != "" {
			status = &s
		}
		var category *string
		if c := r.URL.Query().Get("category"); c != "" {
			category = &c
		}
		rows, err := s.q.ListSpaceReports(r.Context(), db.ListSpaceReportsParams{
			SpaceID:  pgUUID(spaceID),
			Status:   status,
			Category: category,
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

type resolveReq struct {
	Status string `json:"status" validate:"required,oneof=resolved dismissed"`
}

func (s *Service) handleResolve(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		reportID, err := uuid.Parse(chi.URLParam(r, "reportID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid report id")
			return
		}
		var req resolveReq
		if !s.decode(w, r, &req) {
			return
		}

		existing, err := s.q.GetReport(r.Context(), pgUUID(reportID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "report not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "report lookup failed")
			return
		}
		if !existing.SpaceID.Valid || uuid.UUID(existing.SpaceID.Bytes) != spaceID {
			writeError(w, http.StatusNotFound, "report not found")
			return
		}

		actor := uidFn(r.Context())
		rep, err := s.q.ResolveReport(r.Context(), db.ResolveReportParams{
			ID:         pgUUID(reportID),
			Status:     &req.Status,
			ResolvedBy: pgUUID(actor),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "resolve failed")
			return
		}
		meta, _ := json.Marshal(map[string]any{"status": req.Status})
		_, _ = s.q.CreateAuditLog(r.Context(), db.CreateAuditLogParams{
			SpaceID:  pgUUID(spaceID),
			ActorID:  pgUUID(actor),
			Action:   "report.resolve",
			TargetID: pgUUID(reportID),
			Metadata: meta,
		})
		writeJSON(w, http.StatusOK, reportDTO(rep))
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
	if rep.Category != nil {
		out["category"] = *rep.Category
	}
	if len(rep.Context) > 0 {
		out["context"] = json.RawMessage(rep.Context)
	}
	if rep.SpaceID.Valid {
		out["space_id"] = uuid.UUID(rep.SpaceID.Bytes).String()
	}
	if rep.ChannelID.Valid {
		out["channel_id"] = uuid.UUID(rep.ChannelID.Bytes).String()
	}
	if rep.ResolvedBy.Valid {
		out["resolved_by"] = uuid.UUID(rep.ResolvedBy.Bytes).String()
	}
	if rep.ResolvedAt.Valid {
		out["resolved_at"] = rep.ResolvedAt.Time
	}
	if rep.AssignedTo.Valid {
		out["assigned_to"] = uuid.UUID(rep.AssignedTo.Bytes).String()
	}
	return out
}

func (s *Service) handlePendingCount() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		n, err := s.q.CountPendingReports(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "count failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pending": n})
	}
}

func (s *Service) handleClaim(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		reportID, err := uuid.Parse(chi.URLParam(r, "reportID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid report id")
			return
		}

		existing, err := s.q.GetReport(r.Context(), pgUUID(reportID))
		if err != nil || !existing.SpaceID.Valid || uuid.UUID(existing.SpaceID.Bytes) != spaceID {
			writeError(w, http.StatusNotFound, "report not found")
			return
		}
		actor := uidFn(r.Context())
		rep, err := s.q.ClaimReport(r.Context(), db.ClaimReportParams{
			ID: pgUUID(reportID), AssignedTo: pgUUID(actor),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "claim failed")
			return
		}
		meta, _ := json.Marshal(map[string]any{"assigned_to": actor.String()})
		_, _ = s.q.CreateAuditLog(r.Context(), db.CreateAuditLogParams{
			SpaceID: pgUUID(spaceID), ActorID: pgUUID(actor),
			Action: "report.claim", TargetID: pgUUID(reportID), Metadata: meta,
		})
		writeJSON(w, http.StatusOK, reportDTO(rep))
	}
}

func (s *Service) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return false
	}
	if err := s.v.Struct(dst); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func optUUID(s *string) pgtype.UUID {
	if s == nil || *s == "" {
		return pgtype.UUID{}
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
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

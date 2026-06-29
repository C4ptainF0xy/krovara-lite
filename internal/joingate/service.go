package joingate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

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

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/spaces/{spaceID}/join-form", func(rr chi.Router) {
		rr.Get("/", s.handleGetForm(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageSpace)).
			Put("/", s.handlePutForm(userIDFn))
	})
	r.Route("/spaces/{spaceID}/join-requests", func(rr chi.Router) {
		rr.Post("/", s.handleSubmit(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageSpace)).
			Get("/", s.handleList())
	})
	r.Post("/join-requests/{requestID}/review", s.handleReview(userIDFn, resolver))
}

type question struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

type answer struct {
	QuestionID string `json:"question_id"`
	Answer     string `json:"answer"`
}

func (s *Service) handleGetForm(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, ok := parseSpace(w, r)
		if !ok {
			return
		}
		if uidFn(r.Context()) == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		form, err := s.q.GetJoinForm(r.Context(), pgUUID(spaceID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{
				"enabled": false, "questions": []question{}, "auto_role_id": nil, "min_karma": 0,
			})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, formDTO(form))
	}
}

type putFormReq struct {
	Enabled    bool       `json:"enabled"`
	Questions  []question `json:"questions"`
	AutoRoleID *string    `json:"auto_role_id"`
	MinKarma   int32      `json:"min_karma"`
}

func (s *Service) handlePutForm(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, ok := parseSpace(w, r)
		if !ok {
			return
		}
		var req putFormReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if len(req.Questions) > 20 {
			writeError(w, http.StatusBadRequest, "too many questions (max 20)")
			return
		}

		seen := map[string]bool{}
		for i := range req.Questions {
			req.Questions[i].Label = strings.TrimSpace(req.Questions[i].Label)
			if req.Questions[i].Label == "" {
				writeError(w, http.StatusBadRequest, "question label is required")
				return
			}
			if len(req.Questions[i].Label) > 200 {
				writeError(w, http.StatusBadRequest, "question label too long")
				return
			}
			id := strings.TrimSpace(req.Questions[i].ID)
			if id == "" {
				id = "q" + strconv.Itoa(i+1)
			}
			if seen[id] {
				writeError(w, http.StatusBadRequest, "duplicate question id")
				return
			}
			seen[id] = true
			req.Questions[i].ID = id
		}

		var autoRole pgtype.UUID
		if req.AutoRoleID != nil && *req.AutoRoleID != "" {
			rid, err := uuid.Parse(*req.AutoRoleID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid auto_role_id")
				return
			}

			role, err := s.q.GetRole(r.Context(), pgUUID(rid))
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && uuid.UUID(role.SpaceID.Bytes) != spaceID) {
				writeError(w, http.StatusBadRequest, "auto role not in this space")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "role lookup failed")
				return
			}
			if role.IsEveryone != nil && *role.IsEveryone {
				writeError(w, http.StatusBadRequest, "cannot auto-assign @everyone")
				return
			}
			autoRole = pgUUID(rid)
		}

		if req.MinKarma < 0 {
			writeError(w, http.StatusBadRequest, "min_karma must be >= 0")
			return
		}

		questionsJSON, _ := json.Marshal(req.Questions)
		form, err := s.q.UpsertJoinForm(r.Context(), db.UpsertJoinFormParams{
			SpaceID:    pgUUID(spaceID),
			Enabled:    req.Enabled,
			Questions:  questionsJSON,
			AutoRoleID: autoRole,
			MinKarma:   req.MinKarma,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "save failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), uidFn(r.Context()), "joinform.update", pgUUID(spaceID), nil)
		writeJSON(w, http.StatusOK, formDTO(form))
	}
}

type submitReq struct {
	Answers []answer `json:"answers"`
}

func (s *Service) handleSubmit(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, ok := parseSpace(w, r)
		if !ok {
			return
		}
		actor := uidFn(r.Context())
		if actor == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}

		form, err := s.q.GetJoinForm(r.Context(), pgUUID(spaceID))
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && !form.Enabled) {
			writeError(w, http.StatusForbidden, "this space is not accepting requests")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		if _, err := s.q.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
			SpaceID: pgUUID(spaceID), UserID: pgUUID(actor),
		}); err == nil {
			writeError(w, http.StatusConflict, "already a member")
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

		if form.MinKarma > 0 {
			total, err := s.q.SumUserKarma(r.Context(), pgUUID(actor))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "karma check failed")
				return
			}
			if total < int64(form.MinKarma) {
				writeError(w, http.StatusForbidden, "insufficient karma")
				return
			}
		}

		var req submitReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		normalized, msg, ok := validateAnswers(form.Questions, req.Answers)
		if !ok {
			writeError(w, http.StatusBadRequest, msg)
			return
		}

		answersJSON, _ := json.Marshal(normalized)
		jr, err := s.q.CreateJoinRequest(r.Context(), db.CreateJoinRequestParams{
			SpaceID: pgUUID(spaceID), UserID: pgUUID(actor), Answers: answersJSON,
		})
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeError(w, http.StatusConflict, "you already have a pending request")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "submit failed")
			return
		}
		writeJSON(w, http.StatusCreated, requestDTO(jr))
	}
}

func (s *Service) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, ok := parseSpace(w, r)
		if !ok {
			return
		}
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "pending"
		}
		if status != "pending" && status != "approved" && status != "rejected" {
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
		rows, err := s.q.ListJoinRequests(r.Context(), db.ListJoinRequestsParams{
			SpaceID: pgUUID(spaceID), Status: status, Limit: 100, Offset: 0,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			out = append(out, listRowDTO(row))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

type reviewReq struct {
	Action string `json:"action"`
}

func (s *Service) handleReview(uidFn permissions.UserIDFunc, resolver permissions.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reqID, err := uuid.Parse(chi.URLParam(r, "requestID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid request id")
			return
		}
		actor := uidFn(r.Context())
		if actor == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		var body reviewReq
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		var status string
		switch body.Action {
		case "approve":
			status = "approved"
		case "reject":
			status = "rejected"
		default:
			writeError(w, http.StatusBadRequest, "action must be approve or reject")
			return
		}

		jr, err := s.q.GetJoinRequest(r.Context(), pgUUID(reqID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "request not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		spaceID := uuid.UUID(jr.SpaceID.Bytes)

		mc, err := resolver.ResolveSpace(r.Context(), actor, spaceID)
		if err != nil || !permissions.Compute(mc).Has(permissions.ManageSpace) {
			writeError(w, http.StatusForbidden, "missing ManageSpace")
			return
		}

		var memberCreated bool
		err = pgx.BeginFunc(r.Context(), s.pool, func(tx pgx.Tx) error {
			qtx := s.q.WithTx(tx)
			reviewed, err := qtx.ReviewJoinRequest(r.Context(), db.ReviewJoinRequestParams{
				ID: pgUUID(reqID), Status: status, ReviewedBy: pgUUID(actor),
			})
			if errors.Is(err, pgx.ErrNoRows) {
				return errAlreadyReviewed
			}
			if err != nil {
				return err
			}
			if status != "approved" {
				return nil
			}

			if _, err := qtx.GetMemberByUser(r.Context(), db.GetMemberByUserParams{
				SpaceID: reviewed.SpaceID, UserID: reviewed.UserID,
			}); err == nil {
				return nil
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			mem, err := qtx.CreateMember(r.Context(), db.CreateMemberParams{
				SpaceID: reviewed.SpaceID, UserID: reviewed.UserID,
			})
			if err != nil {
				return err
			}
			memberCreated = true

			form, err := qtx.GetJoinForm(r.Context(), reviewed.SpaceID)
			if err == nil && form.AutoRoleID.Valid {
				if err := qtx.AssignMemberRole(r.Context(), db.AssignMemberRoleParams{
					MemberID: mem.ID, RoleID: form.AutoRoleID,
				}); err != nil {
					return err
				}
			}
			return nil
		})
		if errors.Is(err, errAlreadyReviewed) {
			writeError(w, http.StatusConflict, "request already reviewed")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "review failed")
			return
		}
		meta, _ := json.Marshal(map[string]any{"action": body.Action, "member_created": memberCreated})
		s.logAudit(r.Context(), jr.SpaceID, actor, "joinrequest.review", pgUUID(reqID), meta)

		if memberCreated {
			_ = eventsfeed.Emit(r.Context(), s.pool, uuid.Nil, "member_join", map[string]any{
				"space_id": uuid.UUID(jr.SpaceID.Bytes).String(),
				"user_id":  uuid.UUID(jr.UserID.Bytes).String(),
			})
		}

		s.notifyDecision(r.Context(), jr, actor, status, reqID)

		writeJSON(w, http.StatusOK, map[string]any{
			"id": reqID.String(), "status": status, "member_created": memberCreated,
		})
	}
}

var errAlreadyReviewed = errors.New("joingate: already reviewed")

func validateAnswers(questionsJSON []byte, answers []answer) ([]answer, string, bool) {
	var qs []question
	_ = json.Unmarshal(questionsJSON, &qs)
	given := map[string]string{}
	for _, a := range answers {
		given[a.QuestionID] = strings.TrimSpace(a.Answer)
	}
	normalized := make([]answer, 0, len(qs))
	for _, q := range qs {
		v := given[q.ID]
		if q.Required && v == "" {
			return nil, "missing answer for: " + q.Label, false
		}
		if len(v) > 2000 {
			return nil, "answer too long", false
		}
		if v != "" {
			normalized = append(normalized, answer{QuestionID: q.ID, Answer: v})
		}
	}
	return normalized, "", true
}

func (s *Service) logAudit(ctx context.Context, spaceID pgtype.UUID, actor uuid.UUID, action string, target pgtype.UUID, meta []byte) {
	_, _ = s.q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		SpaceID: spaceID, ActorID: pgUUID(actor), Action: action, TargetID: target, Metadata: meta,
	})
}

func (s *Service) notifyDecision(ctx context.Context, jr db.JoinRequest, reviewer uuid.UUID, status string, reqID uuid.UUID) {
	kind := "join_rejected"
	verb := "a refusé ta demande d’entrée"
	if status == "approved" {
		kind = "join_approved"
		verb = "a accepté ta demande d’entrée"
	}
	preview := verb
	if sp, err := s.q.GetSpace(ctx, jr.SpaceID); err == nil {
		preview = verb + " dans « " + sp.Name + " »"
	}
	_ = s.q.CreateInboxItem(ctx, db.CreateInboxItemParams{
		UserID:    jr.UserID,
		Kind:      kind,
		SpaceID:   jr.SpaceID,
		ChannelID: pgtype.UUID{},
		ArchiveID: "joinreq-" + reqID.String() + "-" + status,
		AuthorID:  pgUUID(reviewer),
		Preview:   &preview,
	})
}

func parseSpace(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid space id")
		return uuid.Nil, false
	}
	return id, true
}

func formDTO(f db.JoinForm) map[string]any {
	var qs []question
	_ = json.Unmarshal(f.Questions, &qs)
	if qs == nil {
		qs = []question{}
	}
	out := map[string]any{
		"enabled":      f.Enabled,
		"questions":    qs,
		"auto_role_id": nil,
		"min_karma":    f.MinKarma,
	}
	if f.AutoRoleID.Valid {
		out["auto_role_id"] = uuid.UUID(f.AutoRoleID.Bytes).String()
	}
	return out
}

func requestDTO(jr db.JoinRequest) map[string]any {
	var ans []answer
	_ = json.Unmarshal(jr.Answers, &ans)
	return map[string]any{
		"id":         uuid.UUID(jr.ID.Bytes).String(),
		"space_id":   uuid.UUID(jr.SpaceID.Bytes).String(),
		"user_id":    uuid.UUID(jr.UserID.Bytes).String(),
		"answers":    ans,
		"status":     jr.Status,
		"created_at": jr.CreatedAt.Time,
	}
}

func listRowDTO(row db.ListJoinRequestsRow) map[string]any {
	var ans []answer
	_ = json.Unmarshal(row.Answers, &ans)
	return map[string]any{
		"id":           uuid.UUID(row.ID.Bytes).String(),
		"user_id":      uuid.UUID(row.UserID.Bytes).String(),
		"username":     row.Username,
		"display_name": row.DisplayName,
		"avatar_key":   row.AvatarKey,
		"answers":      ans,
		"status":       row.Status,
		"created_at":   row.CreatedAt.Time,
	}
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

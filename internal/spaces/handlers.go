package spaces

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

func (s *Service) Routes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/spaces", func(sr chi.Router) {
		sr.Post("/", s.handleCreate(userIDFn))
		sr.Get("/", s.handleList(userIDFn))

		sr.Route("/{spaceID}", func(rr chi.Router) {
			rr.Get("/", s.handleGet(userIDFn))
			rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageSpace)).
				Patch("/", s.handleUpdate(userIDFn))
			rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageSpace)).
				Put("/vanity", s.handleSetVanity(userIDFn))

			rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageSpace)).
				Post("/transfer", s.handleTransfer(userIDFn))
			rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageSpace)).
				Delete("/", s.handleDelete(userIDFn))
		})
	})
}

type userIDFunc = permissions.UserIDFunc

func (s *Service) PublicRoutes(r chi.Router) {
	r.Get("/api/spaces/by-vanity/{slug}", s.handleGetByVanity())
}

type createReq struct {
	Name    string  `json:"name" validate:"required,min=1,max=64"`
	IconKey *string `json:"icon_key" validate:"omitempty,max=256"`
}

type updateReq struct {
	Name        *string  `json:"name" validate:"omitempty,min=1,max=64"`
	IconKey     *string  `json:"icon_key" validate:"omitempty,max=256"`
	Description *string  `json:"description" validate:"omitempty,max=2000"`
	Rules       *string  `json:"rules" validate:"omitempty,max=4000"`
	BannerKey   *string  `json:"banner_key" validate:"omitempty,max=256"`
	Tags        []string `json:"tags" validate:"omitempty,max=8,dive,min=1,max=24"`
	Language    *string  `json:"language" validate:"omitempty,max=8"`
}

type vanityReq struct {
	Slug *string `json:"slug" validate:"omitempty,max=32"`
}

type transferReq struct {
	NewOwnerID string `json:"new_owner_id" validate:"required,uuid4"`
}

type deleteReq struct {
	Password string `json:"password" validate:"required"`
}

func (s *Service) handleCreate(uidFn userIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		var req createReq
		if !s.decode(w, r, &req) {
			return
		}
		sp, err := s.CreateSpace(r.Context(), uid, req.Name, req.IconKey)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not create space")
			return
		}
		writeJSON(w, http.StatusCreated, spaceDTO(sp))
	}
}

func (s *Service) handleList(uidFn userIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		rows, err := s.q.ListUserSpaces(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, sp := range rows {
			out = append(out, spaceDTO(sp))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleGet(_ userIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		sp, err := s.GetSpace(r.Context(), id)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeError(w, http.StatusNotFound, "space not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, spaceDTO(sp))
	}
}

func (s *Service) handleUpdate(uidFn userIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req updateReq
		if !s.decode(w, r, &req) {
			return
		}
		sp, err := s.UpdateSpace(r.Context(), uidFn(r.Context()), id, SpaceSettings{
			Name:        req.Name,
			IconKey:     req.IconKey,
			Description: req.Description,
			Rules:       req.Rules,
			BannerKey:   req.BannerKey,
			Tags:        req.Tags,
			Language:    req.Language,
		})
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeError(w, http.StatusNotFound, "space not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		writeJSON(w, http.StatusOK, spaceDTO(sp))
	}
}

func (s *Service) handleSetVanity(uidFn userIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req vanityReq
		if !s.decode(w, r, &req) {
			return
		}
		var slug *string
		if req.Slug != nil && *req.Slug != "" {
			slug = req.Slug
		}
		sp, err := s.SetVanity(r.Context(), uidFn(r.Context()), id, slug)
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "space not found")
		case errors.Is(err, ErrVanityTaken):
			writeError(w, http.StatusConflict, "vanity already taken")
		case errors.Is(err, ErrVanityReserved):
			writeError(w, http.StatusBadRequest, "vanity is reserved")
		case err != nil:
			writeError(w, http.StatusBadRequest, "invalid vanity")
		default:
			writeJSON(w, http.StatusOK, spaceDTO(sp))
		}
	}
}

func (s *Service) handleGetByVanity() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sp, err := s.GetByVanity(r.Context(), chi.URLParam(r, "slug"))
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, spaceDTO(sp))
	}
}

func (s *Service) handleTransfer(uidFn userIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req transferReq
		if !s.decode(w, r, &req) {
			return
		}
		newOwner, _ := uuid.Parse(req.NewOwnerID)
		sp, err := s.TransferOwnership(r.Context(), uidFn(r.Context()), id, newOwner)
		switch {
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "space not found")
		case errors.Is(err, ErrNotOwner):
			writeError(w, http.StatusForbidden, "only the owner can transfer")
		case errors.Is(err, ErrTargetNotMember):
			writeError(w, http.StatusBadRequest, "target is not a member")
		case err != nil:
			writeError(w, http.StatusInternalServerError, "transfer failed")
		default:
			writeJSON(w, http.StatusOK, spaceDTO(sp))
		}
	}
}

func (s *Service) handleDelete(uidFn userIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req deleteReq
		if !s.decode(w, r, &req) {
			return
		}
		actor := uidFn(r.Context())
		ok, err := s.verifyStepUp(r.Context(), actor, req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "verification failed")
			return
		}
		if !ok {
			writeError(w, http.StatusForbidden, "incorrect password")
			return
		}
		if err := s.DeleteSpace(r.Context(), actor, id); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func spaceDTO(sp db.Space) map[string]any {
	tags := sp.Tags
	if tags == nil {
		tags = []string{}
	}
	return map[string]any{
		"id":          uuid.UUID(sp.ID.Bytes).String(),
		"owner_id":    uuid.UUID(sp.OwnerID.Bytes).String(),
		"name":        sp.Name,
		"icon_key":    sp.IconKey,
		"description": sp.Description,
		"rules":       sp.Rules,
		"banner_key":  sp.BannerKey,
		"tags":        tags,
		"language":    sp.Language,
		"vanity_slug": sp.VanitySlug,
		"created_at":  sp.CreatedAt.Time,
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

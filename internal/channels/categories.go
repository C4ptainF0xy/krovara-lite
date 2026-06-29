package channels

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

func (s *Service) CategoryRoutes(r chi.Router, resolver permissions.Resolver, userIDFn permissions.UserIDFunc) {
	r.Route("/spaces/{spaceID}/categories", func(rr chi.Router) {
		rr.Get("/", s.handleListCategories())
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageChannels)).
			Post("/", s.handleCreateCategory(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageChannels)).
			Patch("/{categoryID}", s.handleUpdateCategory(userIDFn))
		rr.With(permissions.RequireSpace(resolver, userIDFn, permissions.ManageChannels)).
			Delete("/{categoryID}", s.handleDeleteCategory(userIDFn))
	})
	r.With(permissions.RequireChannel(resolver, userIDFn, permissions.ManageChannels)).
		Patch("/channels/{channelID}/move", s.handleMoveChannel(userIDFn))
}

type createCategoryReq struct {
	Name     string `json:"name"     validate:"required,min=1,max=64"`
	Position *int32 `json:"position"`
}

type updateCategoryReq struct {
	Name     *string `json:"name"     validate:"omitempty,min=1,max=64"`
	Position *int32  `json:"position"`
}

type moveChannelReq struct {
	CategoryID *string `json:"category_id"`
	Position   int32   `json:"position"`
}

func (s *Service) handleListCategories() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		rows, err := s.q.ListSpaceCategories(r.Context(), pgUUID(spaceID))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, c := range rows {
			out = append(out, categoryDTO(c))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *Service) handleCreateCategory(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid space id")
			return
		}
		var req createCategoryReq
		if !s.decode(w, r, &req) {
			return
		}
		cat, err := s.q.CreateCategory(r.Context(), db.CreateCategoryParams{
			SpaceID:  pgUUID(spaceID),
			Name:     req.Name,
			Position: req.Position,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), uidFn(r.Context()), "category.create", cat.ID, nil)
		writeJSON(w, http.StatusCreated, categoryDTO(cat))
	}
}

func (s *Service) handleUpdateCategory(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, catID, ok := s.categoryInSpace(w, r)
		if !ok {
			return
		}
		var req updateCategoryReq
		if !s.decode(w, r, &req) {
			return
		}
		cat, err := s.q.UpdateCategory(r.Context(), db.UpdateCategoryParams{
			ID:       pgUUID(catID),
			Name:     req.Name,
			Position: req.Position,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "category not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), uidFn(r.Context()), "category.update", cat.ID, nil)
		writeJSON(w, http.StatusOK, categoryDTO(cat))
	}
}

func (s *Service) handleDeleteCategory(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		spaceID, catID, ok := s.categoryInSpace(w, r)
		if !ok {
			return
		}
		if err := s.q.DeleteCategory(r.Context(), pgUUID(catID)); err != nil {
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		s.logAudit(r.Context(), pgUUID(spaceID), uidFn(r.Context()), "category.delete", pgUUID(catID), nil)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleMoveChannel(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "channelID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid channel id")
			return
		}
		var req moveChannelReq
		if !s.decode(w, r, &req) {
			return
		}
		ch, err := s.q.GetChannel(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		var catID pgtype.UUID
		if req.CategoryID != nil && *req.CategoryID != "" {
			parsed, err := uuid.Parse(*req.CategoryID)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid category id")
				return
			}
			cat, err := s.q.GetCategory(r.Context(), pgUUID(parsed))
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusBadRequest, "category not found")
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "lookup failed")
				return
			}
			if cat.SpaceID != ch.SpaceID {
				writeError(w, http.StatusBadRequest, "category not in channel's space")
				return
			}
			catID = pgUUID(parsed)
		}
		updated, err := s.q.MoveChannel(r.Context(), db.MoveChannelParams{
			ID:         pgUUID(id),
			CategoryID: catID,
			Position:   &req.Position,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "move failed")
			return
		}
		s.logAudit(r.Context(), ch.SpaceID, uidFn(r.Context()), "channel.move", ch.ID, nil)
		writeJSON(w, http.StatusOK, channelDTO(updated))
	}
}

func (s *Service) categoryInSpace(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	spaceID, err := uuid.Parse(chi.URLParam(r, "spaceID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid space id")
		return uuid.Nil, uuid.Nil, false
	}
	catID, err := uuid.Parse(chi.URLParam(r, "categoryID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid category id")
		return uuid.Nil, uuid.Nil, false
	}
	cat, err := s.q.GetCategory(r.Context(), pgUUID(catID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "category not found")
		return uuid.Nil, uuid.Nil, false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return uuid.Nil, uuid.Nil, false
	}
	if uuid.UUID(cat.SpaceID.Bytes) != spaceID {
		writeError(w, http.StatusNotFound, "category not found")
		return uuid.Nil, uuid.Nil, false
	}
	return spaceID, catID, true
}

func categoryDTO(c db.Category) map[string]any {
	return map[string]any{
		"id":         uuid.UUID(c.ID.Bytes).String(),
		"space_id":   uuid.UUID(c.SpaceID.Bytes).String(),
		"name":       c.Name,
		"position":   c.Position,
		"created_at": c.CreatedAt.Time,
	}
}

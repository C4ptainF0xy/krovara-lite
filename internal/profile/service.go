package profile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/krovara/krovara/internal/auth"
	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/eventsfeed"
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

func (s *Service) Routes(r chi.Router, userIDFn permissions.UserIDFunc) {
	r.Get("/me", s.handleGet(userIDFn))
	r.Patch("/me", s.handleUpdate(userIDFn))
	r.Patch("/me/password", s.handlePassword(userIDFn))
	r.Get("/users/{userID}/profile", s.handlePublicProfile())
	r.Get("/users/{userID}/mutuals", s.handleMutuals(userIDFn))

	r.Get("/me/2fa/setup", s.handle2FASetup(userIDFn))
	r.Post("/me/2fa/enable", s.handle2FAEnable(userIDFn))
	r.Delete("/me/2fa", s.handle2FADisable(userIDFn))
	r.Delete("/me", s.handleDeleteAccount(userIDFn))
}

func (s *Service) handlePublicProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}
		u, err := s.q.GetUserByID(r.Context(), pgUUID(id))
		if err != nil {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeJSON(w, http.StatusOK, publicDTO(u))
	}
}

func (s *Service) handleMutuals(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		me := uidFn(r.Context())
		target, err := uuid.Parse(chi.URLParam(r, "userID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid user id")
			return
		}

		friends := []map[string]any{}
		if me != uuid.Nil && me != target {
			rows, _ := s.q.ListMutualFriends(r.Context(), db.ListMutualFriendsParams{
				RequesterID: pgUUID(me), RequesterID_2: pgUUID(target),
			})
			for _, f := range rows {
				friends = append(friends, map[string]any{
					"id":           uuid.UUID(f.ID.Bytes).String(),
					"username":     f.Username,
					"display_name": f.DisplayName,
					"avatar_key":   f.AvatarKey,
				})
			}
		}

		spaces := []map[string]any{}
		if me != uuid.Nil && me != target {
			rows, _ := s.q.ListMutualSpaces(r.Context(), db.ListMutualSpacesParams{
				UserID: pgUUID(me), UserID_2: pgUUID(target),
			})
			for _, sp := range rows {
				spaces = append(spaces, map[string]any{
					"id":       uuid.UUID(sp.ID.Bytes).String(),
					"name":     sp.Name,
					"icon_key": sp.IconKey,
				})
			}
		}

		groups := []map[string]any{}
		if me != uuid.Nil && me != target {
			rows, _ := s.q.ListMutualDMGroups(r.Context(), db.ListMutualDMGroupsParams{
				UserID: pgUUID(me), UserID_2: pgUUID(target),
			})
			for _, g := range rows {
				groups = append(groups, map[string]any{
					"id":       uuid.UUID(g.ID.Bytes).String(),
					"name":     g.Name,
					"icon_key": g.IconKey,
				})
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{"friends": friends, "spaces": spaces, "groups": groups})
	}
}

func (s *Service) handleGet(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		u, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		writeJSON(w, http.StatusOK, selfDTO(u))
	}
}

type profileLink struct {
	Label string `json:"label" validate:"max=48"`
	URL   string `json:"url"   validate:"omitempty,url,max=256"`
}

type updateReq struct {
	DisplayName *string        `json:"display_name" validate:"omitempty,max=64"`
	Status      *string        `json:"status"       validate:"omitempty,max=128"`
	AvatarKey   *string        `json:"avatar_key"   validate:"omitempty,max=256"`
	BannerKey   *string        `json:"banner_key"   validate:"omitempty,max=256"`
	Bio         *string        `json:"bio"          validate:"omitempty,max=512"`
	Pronouns    *string        `json:"pronouns"     validate:"omitempty,max=32"`
	Links       *[]profileLink `json:"links"        validate:"omitempty,max=5,dive"`
	AccentColor *string        `json:"accent_color" validate:"omitempty,hexcolor"`
}

func (s *Service) handleUpdate(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req updateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := s.v.Struct(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		var links []byte
		if req.Links != nil {
			links, _ = json.Marshal(*req.Links)
		}
		u, err := s.q.UpdateUserProfile(r.Context(), db.UpdateUserProfileParams{
			ID:          pgUUID(uid),
			DisplayName: req.DisplayName,
			Status:      req.Status,
			AvatarKey:   req.AvatarKey,
			BannerKey:   req.BannerKey,
			Bio:         req.Bio,
			Pronouns:    req.Pronouns,
			Links:       links,
			AccentColor: req.AccentColor,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}

		_ = eventsfeed.Emit(r.Context(), s.pool, uuid.Nil, "profile_update", map[string]any{
			"user_id": uid.String(),
		})

		writeJSON(w, http.StatusOK, selfDTO(u))
	}
}

type passwordReq struct {
	CurrentPassword string `json:"current_password" validate:"omitempty"`
	NewPassword     string `json:"new_password"     validate:"required,min=8,max=128"`
}

func (s *Service) handlePassword(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req passwordReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := s.v.Struct(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		u, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}
		if u.PasswordHash != nil && *u.PasswordHash != "" {
			ok, vErr := auth.VerifyPassword(req.CurrentPassword, *u.PasswordHash)
			if vErr != nil || !ok {
				writeError(w, http.StatusForbidden, "mot de passe actuel incorrect")
				return
			}
		}
		hash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "hash failed")
			return
		}
		if err := s.q.UpdateUserPassword(r.Context(), db.UpdateUserPasswordParams{
			ID:           pgUUID(uid),
			PasswordHash: &hash,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "update failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func selfDTO(u db.User) map[string]any {
	out := publicDTO(u)
	out["email"] = u.Email
	out["is_admin"] = u.IsAdmin
	out["tier"] = u.Tier
	out["email_verified"] = u.EmailVerified
	out["totp_enabled"] = u.TotpEnabled
	return out
}

func publicDTO(u db.User) map[string]any {
	links := json.RawMessage(u.Links)
	if len(links) == 0 {
		links = json.RawMessage("[]")
	}
	return map[string]any{
		"id":           uuid.UUID(u.ID.Bytes).String(),
		"username":     u.Username,
		"display_name": u.DisplayName,
		"status":       u.Status,
		"avatar_key":   u.AvatarKey,
		"banner_key":   u.BannerKey,
		"bio":          u.Bio,
		"pronouns":     u.Pronouns,
		"links":        links,
		"accent_color": u.AccentColor,
		"badges":       u.Badges,
		"created_at":   u.CreatedAt.Time,
	}
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

func (s *Service) handle2FASetup(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		u, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		if u.TotpEnabled {
			writeError(w, http.StatusBadRequest, "2FA already enabled")
			return
		}

		key, err := totp.Generate(totp.GenerateOpts{
			Issuer:      "Krovara",
			AccountName: u.Email,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not generate 2FA key")
			return
		}

		secret := key.Secret()
		_ = s.q.SetTOTP(r.Context(), db.SetTOTPParams{
			ID:          u.ID,
			TotpSecret:  &secret,
			TotpEnabled: false,
		})

		writeJSON(w, http.StatusOK, map[string]string{
			"secret": secret,
			"url":    key.URL(),
		})
	}
}

type enable2FAReq struct {
	Code string `json:"code" validate:"required"`
}

func (s *Service) handle2FAEnable(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		u, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
		if err != nil || u.TotpSecret == nil {
			writeError(w, http.StatusBadRequest, "setup 2FA first")
			return
		}

		var req enable2FAReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}

		req.Code = strings.TrimSpace(req.Code)

		valid, err := totp.ValidateCustom(req.Code, *u.TotpSecret, time.Now().UTC(), totp.ValidateOpts{
			Period:    30,
			Skew:      2,
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		})

		if err != nil || !valid {
			writeError(w, http.StatusBadRequest, "invalid code")
			return
		}

		backupCodes := make([]string, 10)
		for i := 0; i < 10; i++ {
			b := make([]byte, 4)
			rand.Read(b)
			backupCodes[i] = hex.EncodeToString(b)
		}
		bcJSON, _ := json.Marshal(backupCodes)

		err = s.q.SetTOTP(r.Context(), db.SetTOTPParams{
			ID:          u.ID,
			TotpSecret:  u.TotpSecret,
			TotpEnabled: true,
			BackupCodes: bcJSON,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to enable 2FA")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"backup_codes": backupCodes,
		})
	}
}

func (s *Service) handle2FADisable(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())

		err := s.q.DisableTOTP(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to disable 2FA")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Service) handleDeleteAccount(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		u, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lookup failed")
			return
		}

		anonUsername := "Deleted User " + uid.String()[:8]
		anonEmail := uid.String() + "@deleted.krovara.com"

		err = s.q.SoftDeleteUser(r.Context(), db.SoftDeleteUserParams{
			ID:       u.ID,
			Username: anonUsername,
			Email:    anonEmail,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not delete account")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

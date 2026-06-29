package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/email"
)

const emailCodeTTL = 30 * time.Minute

func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func (s *Service) sendVerificationCode(ctx context.Context, userID uuid.UUID, to string) {
	if s.Email == nil {
		return
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return
	}
	code := fmt.Sprintf("%06d", n.Int64())
	_ = s.Queries.UpsertEmailVerification(ctx, db.UpsertEmailVerificationParams{
		UserID:    pgtype.UUID{Bytes: userID, Valid: true},
		CodeHash:  hashCode(code),
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(emailCodeTTL), Valid: true},
	})
	if err := s.Email.Send(ctx, email.Message{
		To:      to,
		Subject: "Ton code de vérification Krovara",
		HTML:    verificationEmailHTML(code),
	}); err != nil {
		slog.Warn("email: verification code send failed", "to", to, "err", err)
	}
}

func verificationEmailHTML(code string) string {
	const (
		base    = "#0F0F14"
		surface = "#1A1A22"
		border  = "#2A2A33"
		primary = "#756E92"
		accent  = "#A79FCB"
		content = "#F2F1F6"
		muted   = "#9A98A6"
		font    = "'Onest','Helvetica Neue',Helvetica,Arial,-apple-system,'Segoe UI',Roboto,sans-serif"
	)
	return fmt.Sprintf(`<!doctype html><html><body style="margin:0;padding:0;background:%[1]s;font-family:%[8]s">
<table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:%[1]s;padding:40px 16px">
 <tr><td align="center">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:460px;background:%[2]s;border:1px solid %[3]s;border-radius:16px;overflow:hidden">
   <tr><td align="center" style="padding:36px 36px 8px">
     <img src="https://krovara.com/krovara.png" alt="Krovara" width="56" height="56" style="border-radius:14px;display:block">
   </td></tr>
   <tr><td align="center" style="padding:8px 36px 0">
     <h1 style="margin:0;font-size:22px;font-weight:700;color:%[5]s">Vérifie ton compte</h1>
     <p style="margin:10px 0 0;font-size:15px;line-height:1.5;color:%[6]s">
       Bienvenue sur Krovara&nbsp;! Saisis ce code pour activer ton compte.
     </p>
   </td></tr>
   <tr><td align="center" style="padding:28px 36px 8px">
     <div style="display:inline-block;padding:16px 28px;background:%[1]s;border:1px solid %[4]s;border-radius:12px;
                 font-size:34px;font-weight:700;letter-spacing:10px;color:%[7]s">%[9]s</div>
   </td></tr>
   <tr><td align="center" style="padding:8px 36px 36px">
     <p style="margin:0;font-size:13px;color:%[6]s">Ce code expire dans 30 minutes.</p>
     <p style="margin:14px 0 0;font-size:12px;color:%[6]s">Si tu n'es pas à l'origine de cette demande, ignore cet email.</p>
   </td></tr>
  </table>
  <p style="margin:20px 0 0;font-size:12px;color:%[6]s"><a href="https://krovara.com" style="color:%[6]s;text-decoration:none">krovara.com</a></p>
 </td></tr>
</table></body></html>`,
		base, surface, border, primary, content, muted, accent, font, code)
}

type verifyEmailReq struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

func (s *Service) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req verifyEmailReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Code = strings.TrimSpace(req.Code)

	u, err := s.Queries.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, "code invalide")
		return
	}
	if u.EmailVerified {
		writeJSON(w, http.StatusOK, map[string]any{"verified": true})
		return
	}
	v, err := s.Queries.GetEmailVerification(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "code invalide")
		return
	}
	if v.ExpiresAt.Time.Before(time.Now()) {
		writeError(w, http.StatusBadRequest, "code expiré")
		return
	}
	if v.CodeHash != hashCode(req.Code) {
		writeError(w, http.StatusBadRequest, "code invalide")
		return
	}
	_ = s.Queries.MarkEmailVerified(r.Context(), u.ID)
	_ = s.Queries.DeleteEmailVerification(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"verified": true})
}

type resendReq struct {
	Email string `json:"email"`
}

func (s *Service) handleResendVerification(w http.ResponseWriter, r *http.Request) {
	var req resendReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if u, err := s.Queries.GetUserByEmail(r.Context(), req.Email); err == nil && !u.EmailVerified {
		s.sendVerificationCode(r.Context(), uuid.UUID(u.ID.Bytes), req.Email)
	}
	w.WriteHeader(http.StatusNoContent)
}

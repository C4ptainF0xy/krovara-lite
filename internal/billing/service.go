package billing

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	stripe "github.com/stripe/stripe-go/v82"
	portalsession "github.com/stripe/stripe-go/v82/billingportal/session"
	checkoutsession "github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/krovara/krovara/internal/db"
	"github.com/krovara/krovara/internal/permissions"
)

type Config struct {
	SecretKey     string
	WebhookSecret string
	PriceID       string
	SuccessURL    string
	CancelURL     string
	ReturnURL     string
}

type Service struct {
	q   *db.Queries
	cfg Config
}

func New(pool *pgxpool.Pool, cfg Config) *Service {
	if cfg.SecretKey == "" {
		return nil
	}
	stripe.Key = cfg.SecretKey
	return &Service{q: db.New(pool), cfg: cfg}
}

func (s *Service) Routes(r chi.Router, userIDFn permissions.UserIDFunc) {
	r.Post("/billing/checkout", s.handleCheckout(userIDFn))
	r.Post("/billing/portal", s.handlePortal(userIDFn))
}

func (s *Service) PublicRoutes(r chi.Router) {
	r.Post("/api/billing/webhook", s.handleWebhook())
}

func (s *Service) handleCheckout(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		user, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "user lookup failed")
			return
		}
		if user.Tier == "plus" {
			writeError(w, http.StatusConflict, "already subscribed")
			return
		}

		params := &stripe.CheckoutSessionParams{
			Mode:              stripe.String(string(stripe.CheckoutSessionModeSubscription)),
			SuccessURL:        stripe.String(s.cfg.SuccessURL),
			CancelURL:         stripe.String(s.cfg.CancelURL),
			ClientReferenceID: stripe.String(uid.String()),
			LineItems: []*stripe.CheckoutSessionLineItemParams{
				{Price: stripe.String(s.cfg.PriceID), Quantity: stripe.Int64(1)},
			},
		}

		if user.StripeCustomerID != nil && *user.StripeCustomerID != "" {
			params.Customer = stripe.String(*user.StripeCustomerID)
		} else {
			params.CustomerEmail = stripe.String(user.Email)
		}

		sess, err := checkoutsession.New(params)
		if err != nil {
			slog.Error("stripe checkout create", "err", err, "user", uid)
			writeError(w, http.StatusBadGateway, "checkout unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": sess.URL})
	}
}

func (s *Service) handlePortal(uidFn permissions.UserIDFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		uid := uidFn(r.Context())
		if uid == uuid.Nil {
			writeError(w, http.StatusUnauthorized, "unauthenticated")
			return
		}
		user, err := s.q.GetUserByID(r.Context(), pgUUID(uid))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "user lookup failed")
			return
		}
		if user.StripeCustomerID == nil || *user.StripeCustomerID == "" {
			writeError(w, http.StatusConflict, "no billing account")
			return
		}
		sess, err := portalsession.New(&stripe.BillingPortalSessionParams{
			Customer:  stripe.String(*user.StripeCustomerID),
			ReturnURL: stripe.String(s.cfg.ReturnURL),
		})
		if err != nil {
			slog.Error("stripe portal create", "err", err, "user", uid)
			writeError(w, http.StatusBadGateway, "portal unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"url": sess.URL})
	}
}

type checkoutCompleted struct {
	Customer          string `json:"customer"`
	ClientReferenceID string `json:"client_reference_id"`
	Subscription      string `json:"subscription"`
}

type subscriptionObject struct {
	ID       string `json:"id"`
	Customer string `json:"customer"`
	Status   string `json:"status"`
	Items    struct {
		Data []struct {
			CurrentPeriodEnd int64 `json:"current_period_end"`
		} `json:"data"`
	} `json:"items"`
}

func tierForSubscription(eventType, status string) string {
	if eventType == "customer.subscription.deleted" {
		return "free"
	}
	switch status {
	case string(stripe.SubscriptionStatusActive), string(stripe.SubscriptionStatusTrialing):
		return "plus"
	default:
		return "free"
	}
}

func (s *Service) handleWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const maxBody = 1 << 20
		payload, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
		if err != nil {
			writeError(w, http.StatusBadRequest, "read failed")
			return
		}
		event, err := webhook.ConstructEvent(payload, r.Header.Get("Stripe-Signature"), s.cfg.WebhookSecret)
		if err != nil {

			writeError(w, http.StatusBadRequest, "invalid signature")
			return
		}

		switch event.Type {
		case "checkout.session.completed":
			var cc checkoutCompleted
			if err := json.Unmarshal(event.Data.Raw, &cc); err != nil {
				writeError(w, http.StatusBadRequest, "bad payload")
				return
			}

			if cc.Customer != "" && cc.ClientReferenceID != "" {
				if uid, err := uuid.Parse(cc.ClientReferenceID); err == nil {
					cust := cc.Customer
					if err := s.q.SetUserStripeCustomer(r.Context(), db.SetUserStripeCustomerParams{
						ID:               pgUUID(uid),
						StripeCustomerID: &cust,
					}); err != nil {
						slog.Error("billing bind customer", "err", err, "user", uid)
						writeError(w, http.StatusInternalServerError, "bind failed")
						return
					}
				}
			}

		case "customer.subscription.created",
			"customer.subscription.updated",
			"customer.subscription.deleted":
			var so subscriptionObject
			if err := json.Unmarshal(event.Data.Raw, &so); err != nil {
				writeError(w, http.StatusBadRequest, "bad payload")
				return
			}
			if so.Customer == "" {
				writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
				return
			}
			tier := tierForSubscription(string(event.Type), so.Status)
			var subID, status *string
			var periodEnd pgtype.Timestamptz
			if so.ID != "" {
				subID = &so.ID
			}
			if so.Status != "" {
				st := so.Status
				status = &st
			}
			if len(so.Items.Data) > 0 && so.Items.Data[0].CurrentPeriodEnd > 0 {
				periodEnd = pgtype.Timestamptz{Time: unixTime(so.Items.Data[0].CurrentPeriodEnd), Valid: true}
			}
			cust := so.Customer
			if err := s.q.SetUserSubscription(r.Context(), db.SetUserSubscriptionParams{
				StripeCustomerID:     &cust,
				StripeSubscriptionID: subID,
				SubscriptionStatus:   status,
				CurrentPeriodEnd:     periodEnd,
				Tier:                 tier,
			}); err != nil {
				slog.Error("billing apply subscription", "err", err, "customer", so.Customer)
				writeError(w, http.StatusInternalServerError, "apply failed")
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]bool{"received": true})
	}
}

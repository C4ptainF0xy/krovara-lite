package billing

import "testing"

func TestTierForSubscription(t *testing.T) {
	cases := []struct {
		name      string
		eventType string
		status    string
		want      string
	}{
		{"active grants plus", "customer.subscription.updated", "active", "plus"},
		{"trialing grants plus", "customer.subscription.created", "trialing", "plus"},
		{"past_due drops to free", "customer.subscription.updated", "past_due", "free"},
		{"canceled drops to free", "customer.subscription.updated", "canceled", "free"},
		{"unpaid drops to free", "customer.subscription.updated", "unpaid", "free"},
		{"incomplete drops to free", "customer.subscription.updated", "incomplete", "free"},
		{"deleted always free even if active", "customer.subscription.deleted", "active", "free"},
		{"empty status drops to free", "customer.subscription.updated", "", "free"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tierForSubscription(c.eventType, c.status); got != c.want {
				t.Fatalf("tierForSubscription(%q, %q) = %q, want %q", c.eventType, c.status, got, c.want)
			}
		})
	}
}

func TestNewNilWithoutKey(t *testing.T) {
	if New(nil, Config{}) != nil {
		t.Fatal("New with empty SecretKey should return nil")
	}
}

package webhooks_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/krovara/krovara/internal/webhooks"
)

func TestSignPayload_StableHMAC(t *testing.T) {
	body := []byte(`{"hello":"world"}`)
	got := webhooks.SignPayload([]byte("secret"), body)

	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))

	require.Equal(t, want, got)
}

func TestWorker_PostsAndSignsBody(t *testing.T) {
	var calls int32
	var gotSig, gotEvent, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		gotSig = r.Header.Get("X-Krovara-Signature")
		gotEvent = r.Header.Get("X-Krovara-Event")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := &webhooks.WebhookDeliverWorker{}
	args := webhooks.WebhookDeliverArgs{
		WebhookID: "abc",
		URL:       srv.URL,
		Secret:    "supersecret",
		Event:     webhooks.EventMessageCreate,
		Body:      []byte(`{"x":1}`),
	}
	err := w.Work(context.Background(), &river.Job[webhooks.WebhookDeliverArgs]{Args: args})
	require.NoError(t, err)
	require.EqualValues(t, 1, calls)
	require.Equal(t, webhooks.EventMessageCreate, gotEvent)
	require.Equal(t, `{"x":1}`, gotBody)
	require.Equal(t, "sha256="+webhooks.SignPayload([]byte("supersecret"), []byte(`{"x":1}`)), gotSig)
}

func TestWorker_NonSuccessReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	w := &webhooks.WebhookDeliverWorker{}
	err := w.Work(context.Background(), &river.Job[webhooks.WebhookDeliverArgs]{
		Args: webhooks.WebhookDeliverArgs{URL: srv.URL, Body: []byte(`{}`), Secret: "s"},
	})
	require.Error(t, err)
}

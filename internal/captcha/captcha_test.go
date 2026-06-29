package captcha

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTurnstile_NilWhenNoSecret(t *testing.T) {
	require.Nil(t, NewTurnstile(""))
	require.NotNil(t, NewTurnstile("s3cret"))
}

func TestTurnstile_Verify(t *testing.T) {
	var gotSecret, gotResponse string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotSecret = r.Form.Get("secret")
		gotResponse = r.Form.Get("response")
		w.Header().Set("Content-Type", "application/json")
		if r.Form.Get("response") == "good" {
			_, _ = w.Write([]byte(`{"success":true}`))
		} else {
			_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
		}
	}))
	defer srv.Close()

	v := &Turnstile{Secret: "sk", Endpoint: srv.URL}

	ok, err := v.Verify(context.Background(), "good", "1.2.3.4")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "sk", gotSecret)
	require.Equal(t, "good", gotResponse)

	ok, err = v.Verify(context.Background(), "bad", "")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestTurnstile_EmptyTokenShortCircuits(t *testing.T) {

	v := &Turnstile{Secret: "sk", Endpoint: "http://invalid.invalid"}
	ok, err := v.Verify(context.Background(), "", "")
	require.NoError(t, err)
	require.False(t, ok)
}

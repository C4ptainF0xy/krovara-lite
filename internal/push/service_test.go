package push_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/krovara/krovara/internal/push"
)

func TestWorker_PostsToTopicWithTitle(t *testing.T) {
	var gotPath, gotTitle, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTitle = r.Header.Get("Title")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := &push.PushNotifyWorker{BaseURL: srv.URL}
	err := w.Work(context.Background(), &river.Job[push.PushNotifyArgs]{
		Args: push.PushNotifyArgs{Topic: "krv-abc", Title: "hello", Body: "world"},
	})
	require.NoError(t, err)
	require.Equal(t, "/krv-abc", gotPath)
	require.Equal(t, "hello", gotTitle)
	require.Equal(t, "world", gotBody)
}

func TestWorker_TrailingSlashStripped(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := &push.PushNotifyWorker{BaseURL: srv.URL + "/"}
	err := w.Work(context.Background(), &river.Job[push.PushNotifyArgs]{
		Args: push.PushNotifyArgs{Topic: "t1", Body: "x"},
	})
	require.NoError(t, err)
	require.Equal(t, "/t1", gotPath, "url joining must collapse the trailing slash")
}

func TestWorker_NonSuccessReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	w := &push.PushNotifyWorker{BaseURL: srv.URL}
	err := w.Work(context.Background(), &river.Job[push.PushNotifyArgs]{
		Args: push.PushNotifyArgs{Topic: "t", Body: "x"},
	})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "503"))
}

func TestWorker_UnconfiguredReturnsError(t *testing.T) {
	w := &push.PushNotifyWorker{}
	err := w.Work(context.Background(), &river.Job[push.PushNotifyArgs]{
		Args: push.PushNotifyArgs{Topic: "t", Body: "x"},
	})
	require.Error(t, err)
}

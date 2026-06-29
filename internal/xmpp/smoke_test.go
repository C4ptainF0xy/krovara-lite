package xmpp_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestProsodyWebSocketSmoke(t *testing.T) {
	if os.Getenv("KROVARA_PROSODY_SMOKE") != "1" {
		t.Skip("set KROVARA_PROSODY_SMOKE=1 and run `make dev-up` first")
	}

	endpoint := os.Getenv("KROVARA_PROSODY_WS")
	if endpoint == "" {
		endpoint = "ws://localhost:5280/xmpp-websocket"
	}
	u, err := url.Parse(endpoint)
	require.NoError(t, err, "invalid KROVARA_PROSODY_WS")
	require.Contains(t, []string{"ws", "wss"}, u.Scheme)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{
		Subprotocols: []string{"xmpp"},
	})
	require.NoError(t, err, "dial failed — is `make dev-up` running?")
	defer conn.Close(websocket.StatusNormalClosure, "smoke done")

	openStanza := `<open xmlns="urn:ietf:params:xml:ns:xmpp-framing" to="krovara.local" version="1.0"/>`
	require.NoError(t, conn.Write(ctx, websocket.MessageText, []byte(openStanza)))

	mtype, data, err := conn.Read(ctx)
	require.NoError(t, err, "expected stream reply from Prosody")
	require.Equal(t, websocket.MessageText, mtype)
	require.True(t,
		strings.Contains(string(data), `<open`) ||
			strings.Contains(string(data), `<stream`),
		"unexpected stream reply: %s", string(data))
}

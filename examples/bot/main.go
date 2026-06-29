// Example Krovara bot: connects as an XMPP component (XEP-0114), listens
// for group-chat messages, and replies "pong" to anything starting with
// "!ping".
//
// Usage:
//
//	export BOT_JID=bot-abc123de.krovara.local
//	export BOT_SECRET=<from-POST-/api/spaces/:id/bots>
//	export BOT_HOST=localhost
//	export BOT_PORT=5347
//	go run ./examples/bot
//
// The bot uses mellium.im/xmpp because @xmpp/client-style libs don't expose
// a component dialer; mellium is the smallest Go option that does.
package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"mellium.im/xmpp"
	"mellium.im/xmpp/component"
	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/stanza"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	botJID, err := jid.Parse(must("BOT_JID"))
	if err != nil {
		log.Error("parse jid", "err", err)
		os.Exit(1)
	}
	host := envOr("BOT_HOST", "localhost")
	port := envOr("BOT_PORT", "5347")
	secret := must("BOT_SECRET")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	dial := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dial.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		log.Error("dial", "err", err)
		os.Exit(1)
	}

	session, err := component.NewSession(ctx, botJID, []byte(secret), conn)
	if err != nil {
		log.Error("component handshake", "err", err)
		os.Exit(1)
	}
	defer session.Close()
	log.Info("bot online", "jid", botJID.String())

	err = session.Serve(xmpp.HandlerFunc(func(t xmlstream.TokenReadEncoder, start *xml.StartElement) error {
		if start.Name.Local != "message" {
			return nil
		}
		msg := struct {
			stanza.Message
			Body string `xml:"body"`
		}{}
		if err := t.Decode(&msg); err != nil {
			return err
		}
		if !strings.HasPrefix(msg.Body, "!ping") {
			return nil
		}
		reply := stanza.Message{
			To:   msg.From,
			From: botJID,
			Type: msg.Type,
		}
		// Serialize a minimal <message><body>pong</body></message>.
		return reply.Wrap(t, func(enc xml.TokenWriter) error {
			return enc.EncodeToken(xml.CharData("pong"))
		})
	}))
	if err != nil && ctx.Err() == nil {
		log.Error("serve", "err", err)
	}
	_ = fmt.Sprintln // keep fmt for future debug use
}

func must(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "%s is required\n", key)
		os.Exit(2)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type Message struct {
	To      string
	Subject string
	HTML    string
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}

func New(apiKey, from string) Sender {
	if apiKey == "" {
		slog.Warn("email: no Resend API key — falling back to log sender (dev)")
		return LogSender{}
	}
	return &ResendSender{
		apiKey: apiKey,
		from:   from,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

type LogSender struct{}

func (LogSender) Send(_ context.Context, msg Message) error {
	slog.Info("email (log sender — not actually sent)",
		"to", msg.To, "subject", msg.Subject, "html", msg.HTML)
	return nil
}

type ResendSender struct {
	apiKey string
	from   string
	http   *http.Client
}

func (s *ResendSender) Send(ctx context.Context, msg Message) error {
	body, err := json.Marshal(map[string]any{
		"from":    s.from,
		"to":      []string{msg.To},
		"subject": msg.Subject,
		"html":    msg.HTML,
	})
	if err != nil {
		return fmt.Errorf("marshal email: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("resend status %d: %s", resp.StatusCode, snippet)
	}
	return nil
}

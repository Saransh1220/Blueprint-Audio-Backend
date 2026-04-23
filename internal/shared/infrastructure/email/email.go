package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	APIKey     string
	From       string
	ReplyTo    string
	Enabled    bool
	APIRoot    string
	HTTPClient *http.Client
}

type Message struct {
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html,omitempty"`
	Text    string   `json:"text,omitempty"`
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}

type noopSender struct{}

func (noopSender) Send(context.Context, Message) error { return nil }

type resendSender struct {
	apiKey     string
	from       string
	replyTo    string
	apiRoot    string
	httpClient *http.Client
}

func NewSender(cfg Config) Sender {
	if !cfg.Enabled || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.From) == "" {
		log.Printf("Email sender disabled. enabled=%t api_key_present=%t from_present=%t", cfg.Enabled, strings.TrimSpace(cfg.APIKey) != "", strings.TrimSpace(cfg.From) != "")
		return noopSender{}
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	apiRoot := strings.TrimRight(cfg.APIRoot, "/")
	if apiRoot == "" {
		apiRoot = "https://api.resend.com"
	}
	return &resendSender{
		apiKey:     cfg.APIKey,
		from:       cfg.From,
		replyTo:    cfg.ReplyTo,
		apiRoot:    apiRoot,
		httpClient: client,
	}
}

func (s *resendSender) Send(ctx context.Context, msg Message) error {
	log.Printf("Resend send start. from=%q to_count=%d subject=%q api_root=%q", s.from, len(msg.To), msg.Subject, s.apiRoot)
	payload := map[string]any{
		"from":    s.from,
		"to":      msg.To,
		"subject": msg.Subject,
	}
	if msg.HTML != "" {
		payload["html"] = msg.HTML
	}
	if msg.Text != "" {
		payload["text"] = msg.Text
	}
	if strings.TrimSpace(s.replyTo) != "" {
		payload["reply_to"] = s.replyTo
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal resend payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.apiRoot+"/emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		log.Printf("Resend send network error. from=%q to_count=%d subject=%q err=%v", s.from, len(msg.To), msg.Subject, err)
		return fmt.Errorf("send resend request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		log.Printf("Resend send failed. status=%d from=%q to_count=%d subject=%q body=%s", resp.StatusCode, s.from, len(msg.To), msg.Subject, strings.TrimSpace(string(respBody)))
		return fmt.Errorf("resend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	log.Printf("Resend send success. from=%q to_count=%d subject=%q", s.from, len(msg.To), msg.Subject)
	return nil
}

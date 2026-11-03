// Package dodo is the Dodo Payments implementation of the payment gateway
// strategy: hosted checkout session, then a signed webhook on completion.
package dodo

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
)

type Config struct {
	APIKey     string
	ProductID  string
	WebhookKey string
	APIURL     string
	// ReturnURL is where Dodo sends the buyer after checkout.
	ReturnURL  string
	HTTPClient *http.Client
}

type Gateway struct {
	cfg    Config
	client *http.Client
}

var (
	_ domain.Gateway          = (*Gateway)(nil)
	_ domain.WebhookConfirmer = (*Gateway)(nil)
)

func NewGateway(cfg Config) *Gateway {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Gateway{cfg: cfg, client: client}
}

func (g *Gateway) Name() string { return domain.ProviderDodo }

func (g *Gateway) CreateCheckout(ctx context.Context, req domain.CheckoutRequest) (domain.CheckoutSession, error) {
	apiKey := strings.TrimSpace(g.cfg.APIKey)
	productID := strings.TrimSpace(g.cfg.ProductID)
	if apiKey == "" || productID == "" {
		return domain.CheckoutSession{}, errors.New("dodo payments is not configured")
	}
	baseURL := strings.TrimRight(g.cfg.APIURL, "/")
	if baseURL == "" {
		baseURL = "https://test.dodopayments.com"
	}

	order := req.Order
	metadata := map[string]any{
		"order_id":     order.ID.String(),
		"spec_id":      order.SpecID.String(),
		"license_type": order.LicenseType,
		"spec_title":   req.SpecTitle,
		"license_name": req.LicenseName,
	}
	body := map[string]any{
		"product_cart": []map[string]any{{
			"product_id": productID,
			"quantity":   1,
			"amount":     order.Amount,
		}},
		"return_url":  g.cfg.ReturnURL,
		"metadata":    metadata,
		"custom_data": metadata,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return domain.CheckoutSession{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/checkouts", bytes.NewReader(payload))
	if err != nil {
		return domain.CheckoutSession{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	res, err := g.client.Do(httpReq)
	if err != nil {
		return domain.CheckoutSession{}, fmt.Errorf("dodo checkout creation failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return domain.CheckoutSession{}, fmt.Errorf("dodo checkout creation failed with status %d: %s", res.StatusCode, strings.TrimSpace(string(msg)))
	}
	var decoded struct {
		SessionID   string  `json:"session_id"`
		CheckoutURL *string `json:"checkout_url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return domain.CheckoutSession{}, err
	}
	if decoded.SessionID == "" || decoded.CheckoutURL == nil || *decoded.CheckoutURL == "" {
		return domain.CheckoutSession{}, errors.New("invalid dodo checkout response")
	}
	return domain.CheckoutSession{ProviderRef: decoded.SessionID, CheckoutURL: *decoded.CheckoutURL}, nil
}

func (g *Gateway) ParseWebhook(payload []byte, headers map[string]string) (*domain.PaymentConfirmation, error) {
	if err := g.verifySignature(payload, headers); err != nil {
		return nil, err
	}

	var event struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}
	if event.Type != "payment.succeeded" {
		return nil, nil
	}

	orderID, paymentID := extractOrderMetadata(event.Data)
	if orderID == uuid.Nil {
		return nil, errors.New("dodo webhook missing order_id")
	}
	if paymentID == "" {
		paymentID = headers["webhook-id"]
	}
	return &domain.PaymentConfirmation{
		OrderID:           orderID,
		ProviderPaymentID: paymentID,
		Signature:         headers["webhook-signature"],
	}, nil
}

func (g *Gateway) verifySignature(payload []byte, headers map[string]string) error {
	secret := strings.TrimSpace(g.cfg.WebhookKey)
	if secret == "" {
		return errors.New("dodo webhook secret is not configured")
	}
	if strings.HasPrefix(strings.ToLower(secret), "http://") || strings.HasPrefix(strings.ToLower(secret), "https://") {
		return errors.New("DODO_PAYMENTS_WEBHOOK_KEY must be the Dodo webhook signing secret, not the endpoint URL")
	}
	webhookID := headers["webhook-id"]
	timestamp := headers["webhook-timestamp"]
	signature := headers["webhook-signature"]
	if webhookID == "" || timestamp == "" || signature == "" {
		return errors.New("missing dodo webhook signature headers")
	}

	message := webhookID + "." + timestamp + "." + string(payload)
	secretKeys := [][]byte{[]byte(secret)}
	if encoded, ok := strings.CutPrefix(secret, "whsec_"); ok {
		secretKeys = append(secretKeys, []byte(encoded))
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		}
		if err == nil {
			secretKeys = append(secretKeys, decoded)
		}
	}

	for _, secretKey := range secretKeys {
		mac := hmac.New(sha256.New, secretKey)
		mac.Write([]byte(message))
		sum := mac.Sum(nil)
		if compareSignature(signature, base64.StdEncoding.EncodeToString(sum), hex.EncodeToString(sum)) {
			return nil
		}
	}
	return errors.New("invalid dodo webhook signature")
}

func compareSignature(received string, expected ...string) bool {
	parts := strings.FieldsFunc(received, func(r rune) bool {
		return r == ' ' || r == ','
	})
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(part, "v1="))
		if strings.EqualFold(part, "v1") || part == "" {
			continue
		}
		for _, candidate := range expected {
			if hmac.Equal([]byte(part), []byte(candidate)) {
				return true
			}
		}
	}
	for _, candidate := range expected {
		if hmac.Equal([]byte(strings.TrimSpace(received)), []byte(candidate)) {
			return true
		}
	}
	return false
}

func extractOrderMetadata(data map[string]any) (uuid.UUID, string) {
	paymentID := stringFromAny(data["payment_id"])
	if paymentID == "" {
		paymentID = stringFromAny(data["id"])
	}

	candidates := []any{data["metadata"], data["custom_data"], data["custom_fields"]}
	for _, candidate := range candidates {
		if meta, ok := candidate.(map[string]any); ok {
			if id, err := uuid.Parse(stringFromAny(meta["order_id"])); err == nil {
				return id, paymentID
			}
		}
	}
	if id, err := uuid.Parse(stringFromAny(data["order_id"])); err == nil {
		return id, paymentID
	}
	return uuid.Nil, paymentID
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", value)
}

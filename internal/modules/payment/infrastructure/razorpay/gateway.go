// Package razorpay is the Razorpay implementation of the payment gateway
// strategy: server-side order creation, then client-reported confirmation.
package razorpay

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	rzp "github.com/razorpay/razorpay-go"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
)

type Config struct {
	KeyID     string
	KeySecret string
	// BaseURL overrides the Razorpay API root. Tests point it at httptest.
	BaseURL string
}

type Gateway struct {
	client *rzp.Client
	secret string
}

var (
	_ domain.Gateway         = (*Gateway)(nil)
	_ domain.ClientConfirmer = (*Gateway)(nil)
)

func NewGateway(cfg Config) *Gateway {
	client := rzp.NewClient(cfg.KeyID, cfg.KeySecret)
	if cfg.BaseURL != "" {
		client.Request.BaseURL = cfg.BaseURL
	}
	return &Gateway{client: client, secret: cfg.KeySecret}
}

func (g *Gateway) Name() string { return domain.ProviderRazorpay }

func (g *Gateway) CreateCheckout(_ context.Context, req domain.CheckoutRequest) (domain.CheckoutSession, error) {
	order, err := g.client.Order.Create(map[string]interface{}{
		"amount":   req.Order.Amount,
		"currency": req.Order.Currency,
		"receipt":  formatReceipt(req.Order.ID),
	}, nil)
	if err != nil {
		return domain.CheckoutSession{}, fmt.Errorf("razorpay order creation failed: %w", err)
	}
	id, ok := order["id"].(string)
	if !ok || id == "" {
		return domain.CheckoutSession{}, errors.New("invalid razorpay order response")
	}
	return domain.CheckoutSession{ProviderRef: id}, nil
}

func (g *Gateway) ConfirmClientPayment(_ context.Context, order *domain.Order, paymentID, signature string) (*domain.PaymentConfirmation, error) {
	if order.ProviderCheckoutID == nil || *order.ProviderCheckoutID == "" {
		return nil, errors.New("invalid order state")
	}
	if !hmac.Equal([]byte(g.sign(*order.ProviderCheckoutID, paymentID)), []byte(signature)) {
		return nil, domain.ErrInvalidSignature
	}

	payment, err := g.client.Payment.Fetch(paymentID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch payment: %w", err)
	}
	status, _ := payment["status"].(string)
	if strings.ToLower(status) != "captured" {
		return nil, domain.ErrPaymentNotCaptured
	}

	c := &domain.PaymentConfirmation{
		OrderID:           order.ID,
		ProviderPaymentID: paymentID,
		Signature:         signature,
	}
	if method, ok := payment["method"].(string); ok {
		c.Method = &method
	}
	if email, ok := payment["email"].(string); ok {
		c.Email = &email
	}
	return c, nil
}

func (g *Gateway) sign(orderID, paymentID string) string {
	h := hmac.New(sha256.New, []byte(g.secret))
	h.Write([]byte(orderID + "|" + paymentID))
	return hex.EncodeToString(h.Sum(nil))
}

func formatReceipt(id uuid.UUID) string {
	return "order_" + strings.ReplaceAll(id.String(), "-", "")
}

package dodo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sign(key []byte, id, ts string, payload []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "." + string(payload)))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestGateway_Name(t *testing.T) {
	assert.Equal(t, domain.ProviderDodo, NewGateway(Config{}).Name())
}

func TestGateway_CreateCheckout(t *testing.T) {
	ctx := context.Background()
	order := &domain.Order{ID: uuid.New(), SpecID: uuid.New(), LicenseType: "Basic", Amount: 1000, Currency: "USD"}
	req := domain.CheckoutRequest{Order: order, SpecTitle: "Track", LicenseName: "Basic"}

	newGateway := func(handler http.HandlerFunc) (*Gateway, func()) {
		ts := httptest.NewServer(handler)
		return NewGateway(Config{APIKey: " key ", ProductID: "prod_1", APIURL: ts.URL + "/", ReturnURL: "http://localhost:4200/dashboard"}), ts.Close
	}

	t.Run("success", func(t *testing.T) {
		var gotAuth string
		var gotBody map[string]any
		g, closeFn := newGateway(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			assert.Equal(t, "/checkouts", r.URL.Path)
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_ = json.NewEncoder(w).Encode(map[string]any{"session_id": "cks_1", "checkout_url": "https://pay.example/cks_1"})
		})
		defer closeFn()

		session, err := g.CreateCheckout(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, "cks_1", session.ProviderRef)
		assert.Equal(t, "https://pay.example/cks_1", session.CheckoutURL)
		assert.Equal(t, "Bearer key", gotAuth)
		assert.Equal(t, "http://localhost:4200/dashboard", gotBody["return_url"])
		assert.Equal(t, gotBody["metadata"], gotBody["custom_data"])
		meta := gotBody["metadata"].(map[string]any)
		assert.Equal(t, order.ID.String(), meta["order_id"])
		assert.Equal(t, "Track", meta["spec_title"])
		cart := gotBody["product_cart"].([]any)[0].(map[string]any)
		assert.Equal(t, "prod_1", cart["product_id"])
		assert.EqualValues(t, 1000, cart["amount"])
	})

	t.Run("not configured", func(t *testing.T) {
		_, err := NewGateway(Config{}).CreateCheckout(ctx, req)
		assert.EqualError(t, err, "dodo payments is not configured")
	})

	t.Run("non-2xx status", func(t *testing.T) {
		g, closeFn := newGateway(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(" upstream broke "))
		})
		defer closeFn()
		_, err := g.CreateCheckout(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 502")
		assert.Contains(t, err.Error(), "upstream broke")
	})

	t.Run("malformed json", func(t *testing.T) {
		g, closeFn := newGateway(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{bad")) })
		defer closeFn()
		_, err := g.CreateCheckout(ctx, req)
		assert.Error(t, err)
	})

	t.Run("incomplete response", func(t *testing.T) {
		g, closeFn := newGateway(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"session_id": "cks_1"})
		})
		defer closeFn()
		_, err := g.CreateCheckout(ctx, req)
		assert.EqualError(t, err, "invalid dodo checkout response")
	})

	t.Run("connection failure", func(t *testing.T) {
		g, closeFn := newGateway(func(w http.ResponseWriter, r *http.Request) {})
		closeFn()
		_, err := g.CreateCheckout(ctx, req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dodo checkout creation failed")
	})
}

func TestGateway_VerifySignature(t *testing.T) {
	payload := []byte(`{"type":"x"}`)
	hdr := func(sig string) map[string]string {
		return map[string]string{"webhook-id": "id", "webhook-timestamp": "1", "webhook-signature": sig}
	}

	g := NewGateway(Config{})
	assert.EqualError(t, g.verifySignature(payload, hdr("x")), "dodo webhook secret is not configured")

	g = NewGateway(Config{WebhookKey: "HTTPS://example.com/hook"})
	assert.ErrorContains(t, g.verifySignature(payload, hdr("x")), "not the endpoint URL")

	g = NewGateway(Config{WebhookKey: "plain-secret"})
	assert.EqualError(t, g.verifySignature(payload, map[string]string{"webhook-id": "id"}), "missing dodo webhook signature headers")

	// plain secret, base64 signature with v1 prefix
	assert.NoError(t, g.verifySignature(payload, hdr("v1,"+sign([]byte("plain-secret"), "id", "1", payload))))
	// plain secret, hex signature, unprefixed
	mac := hmac.New(sha256.New, []byte("plain-secret"))
	mac.Write([]byte("id.1." + string(payload)))
	assert.NoError(t, g.verifySignature(payload, hdr(hex.EncodeToString(mac.Sum(nil)))))
	// wrong signature
	assert.EqualError(t, g.verifySignature(payload, hdr("v1,AAAA")), "invalid dodo webhook signature")

	// whsec with raw (unpadded) base64 key
	raw := []byte("rawkey-bytes!")
	g = NewGateway(Config{WebhookKey: "whsec_" + base64.RawStdEncoding.EncodeToString(raw)})
	assert.NoError(t, g.verifySignature(payload, hdr("v1,"+sign(raw, "id", "1", payload))))

	// whsec that is not valid base64: falls back to the literal remainder as key
	g = NewGateway(Config{WebhookKey: "whsec_***notbase64***"})
	assert.NoError(t, g.verifySignature(payload, hdr("v1,"+sign([]byte("***notbase64***"), "id", "1", payload))))
}

func TestCompareSignature(t *testing.T) {
	assert.True(t, compareSignature("v1,abc", "abc"))
	assert.True(t, compareSignature("v1,zzz v1,abc", "abc"), "space separated list")
	assert.True(t, compareSignature("v1=abc", "abc"))
	assert.True(t, compareSignature("abc", "other", "abc"))
	assert.True(t, compareSignature("  abc  ", "abc"), "trimmed whole-string fallback")
	assert.False(t, compareSignature("v1,nope", "abc"))
	assert.False(t, compareSignature("", "abc"))
	assert.False(t, compareSignature("v1", "v1x"))
}

func TestExtractOrderMetadata(t *testing.T) {
	oid := uuid.New()
	tests := []struct {
		name    string
		data    map[string]any
		wantID  uuid.UUID
		wantPay string
	}{
		{"metadata", map[string]any{"payment_id": "p1", "metadata": map[string]any{"order_id": oid.String()}}, oid, "p1"},
		{"custom_data and id fallback", map[string]any{"id": "p2", "custom_data": map[string]any{"order_id": oid.String()}}, oid, "p2"},
		{"custom_fields", map[string]any{"custom_fields": map[string]any{"order_id": oid.String()}}, oid, ""},
		{"top-level order_id", map[string]any{"order_id": oid.String(), "payment_id": 42}, oid, "42"},
		{"invalid metadata falls through", map[string]any{"metadata": map[string]any{"order_id": "nope"}, "order_id": oid.String()}, oid, ""},
		{"nothing", map[string]any{"metadata": "string"}, uuid.Nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, pay := extractOrderMetadata(tt.data)
			assert.Equal(t, tt.wantID, id)
			assert.Equal(t, tt.wantPay, pay)
		})
	}
}

func TestStringFromAny(t *testing.T) {
	assert.Equal(t, "", stringFromAny(nil))
	assert.Equal(t, "s", stringFromAny("s"))
	assert.Equal(t, "12", stringFromAny(12))
	assert.Equal(t, "true", stringFromAny(true))
}

func TestGateway_ParseWebhook(t *testing.T) {
	key := []byte("whk")
	g := NewGateway(Config{WebhookKey: string(key)})
	oid := uuid.New()

	signed := func(t *testing.T, event map[string]any) ([]byte, map[string]string) {
		payload, err := json.Marshal(event)
		require.NoError(t, err)
		return payload, map[string]string{
			"webhook-id": "evt_1", "webhook-timestamp": "10",
			"webhook-signature": "v1," + sign(key, "evt_1", "10", payload),
		}
	}
	succeeded := func(data map[string]any) map[string]any {
		if data == nil {
			data = map[string]any{}
		}
		data["metadata"] = map[string]any{"order_id": oid.String()}
		return map[string]any{"type": "payment.succeeded", "data": data}
	}

	t.Run("invalid signature", func(t *testing.T) {
		_, err := g.ParseWebhook([]byte(`{}`), map[string]string{"webhook-id": "a", "webhook-timestamp": "b", "webhook-signature": "c"})
		assert.EqualError(t, err, "invalid dodo webhook signature")
	})

	t.Run("invalid json with valid signature", func(t *testing.T) {
		payload := []byte(`{not json`)
		_, err := g.ParseWebhook(payload, map[string]string{
			"webhook-id": "evt_1", "webhook-timestamp": "10",
			"webhook-signature": "v1," + sign(key, "evt_1", "10", payload),
		})
		assert.Error(t, err)
	})

	t.Run("other event types are ignored", func(t *testing.T) {
		p, h := signed(t, map[string]any{"type": "payment.failed", "data": map[string]any{}})
		c, err := g.ParseWebhook(p, h)
		assert.NoError(t, err)
		assert.Nil(t, c)
	})

	t.Run("missing order id", func(t *testing.T) {
		p, h := signed(t, map[string]any{"type": "payment.succeeded", "data": map[string]any{}})
		_, err := g.ParseWebhook(p, h)
		assert.EqualError(t, err, "dodo webhook missing order_id")
	})

	t.Run("success maps to a provider-neutral confirmation", func(t *testing.T) {
		p, h := signed(t, succeeded(map[string]any{"payment_id": "pay_dodo_1"}))
		c, err := g.ParseWebhook(p, h)
		require.NoError(t, err)
		assert.Equal(t, oid, c.OrderID)
		assert.Equal(t, "pay_dodo_1", c.ProviderPaymentID)
		assert.Equal(t, h["webhook-signature"], c.Signature)
	})

	t.Run("payment id falls back to the webhook id", func(t *testing.T) {
		p, h := signed(t, succeeded(nil))
		c, err := g.ParseWebhook(p, h)
		require.NoError(t, err)
		assert.Equal(t, "evt_1", c.ProviderPaymentID)
	})

	t.Run("standard-webhooks whsec secret", func(t *testing.T) {
		secretBytes := []byte("dodo-test-webhook-secret")
		g := NewGateway(Config{WebhookKey: "whsec_" + base64.StdEncoding.EncodeToString(secretBytes)})
		payload, err := json.Marshal(succeeded(map[string]any{"payment_id": "pay_dodo_1"}))
		require.NoError(t, err)
		c, err := g.ParseWebhook(payload, map[string]string{
			"webhook-id": "evt_dodo_1", "webhook-timestamp": "1779091529",
			"webhook-signature": "v1," + sign(secretBytes, "evt_dodo_1", "1779091529", payload),
		})
		require.NoError(t, err)
		assert.Equal(t, "pay_dodo_1", c.ProviderPaymentID)
	})
}

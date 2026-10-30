package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/razorpay/razorpay-go"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func strp(s string) *string { return &s }

func TestNewPaymentService_WiresDependencies(t *testing.T) {
	t.Setenv("RAZORPAY_KEY_ID", "kid")
	t.Setenv("RAZORPAY_KEY_SECRET", "ksecret")
	cfg := DodoConfig{APIKey: "k", ProductID: "p", WebhookKey: "w", APIURL: "u"}
	svc := NewPaymentService(new(orderRepoMock), new(paymentRepoMock), new(licenseRepoMock), new(specFinderMock), new(userFinderMock), new(fileSvcMock), new(emailSenderMock), "http://app", cfg)
	ps, ok := svc.(*paymentService)
	require.True(t, ok)
	assert.Equal(t, "ksecret", ps.razorpaySecret)
	assert.NotNil(t, ps.razorpayClient)
	assert.Equal(t, "http://app", ps.appBaseURL)
	assert.Equal(t, cfg, ps.dodoConfig)
}

func specWithLicense(loID uuid.UUID, status catalogDomain.ProcessingStatus, currency string) *catalogDomain.Spec {
	return &catalogDomain.Spec{
		ID:               uuid.New(),
		Title:            "Track",
		ProcessingStatus: status,
		Licenses: []catalogDomain.LicenseOption{
			{ID: loID, LicenseType: catalogDomain.LicenseBasic, Name: "Basic", Price: 10, PriceCurrency: currency},
		},
	}
}

func TestPaymentService_CreateOrder_ValidationBranches(t *testing.T) {
	ctx := context.Background()
	s, _, _, _, sf, _, _, _ := newPaymentSvc()
	specID, loID := uuid.New(), uuid.New()

	sf.On("FindWithLicenses", ctx, specID).Return(specWithLicense(loID, catalogDomain.ProcessingStatusProcessing, "INR"), nil).Once()
	_, err := s.CreateOrder(ctx, uuid.New(), specID, loID, "INR")
	assert.EqualError(t, err, "Beat/Sample is not ready for purchase")

	sf.On("FindWithLicenses", ctx, specID).Return(specWithLicense(loID, catalogDomain.ProcessingStatusCompleted, "INR"), nil).Once()
	_, err = s.CreateOrder(ctx, uuid.New(), specID, uuid.New(), "INR")
	assert.EqualError(t, err, "license option not found")
}

func TestPaymentService_CreateOrder_RazorpayFailures(t *testing.T) {
	ctx := context.Background()
	loID := uuid.New()

	run := func(t *testing.T, handler http.HandlerFunc, repoErr error) error {
		s, or, _, _, sf, _, _, _ := newPaymentSvc()
		ts := httptest.NewServer(handler)
		defer ts.Close()
		s.razorpayClient = razorpay.NewClient("k", "s")
		s.razorpayClient.Request.BaseURL = ts.URL
		specID := uuid.New()
		sf.On("FindWithLicenses", ctx, specID).Return(specWithLicense(loID, "", ""), nil).Once()
		if repoErr != nil {
			or.On("Create", ctx, mock.AnythingOfType("*domain.Order")).Return(repoErr).Once()
		}
		_, err := s.CreateOrder(ctx, uuid.New(), specID, loID, "inr")
		return err
	}

	t.Run("razorpay API error", func(t *testing.T) {
		err := run(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"description":"nope"}}`))
		}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "razorpay order creation failed")
	})
	t.Run("empty id in response", func(t *testing.T) {
		err := run(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": ""})
		}, nil)
		assert.EqualError(t, err, "invalid razorpay order response")
	})
	t.Run("repo create fails", func(t *testing.T) {
		err := run(t, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "order_x"})
		}, errors.New("db down"))
		assert.EqualError(t, err, "db down")
	})
}

func TestPaymentService_CreateOrder_DodoCheckout(t *testing.T) {
	ctx := context.Background()
	loID := uuid.New()

	newSvc := func(handler http.HandlerFunc) (*paymentService, *orderRepoMock, *specFinderMock, uuid.UUID, func()) {
		s, or, _, _, sf, _, _, _ := newPaymentSvc()
		ts := httptest.NewServer(handler)
		s.dodoConfig = DodoConfig{APIKey: " key ", ProductID: "prod_1", APIURL: ts.URL + "/"}
		specID := uuid.New()
		sf.On("FindWithLicenses", ctx, specID).Return(specWithLicense(loID, catalogDomain.ProcessingStatusCompleted, "USD"), nil).Once()
		return s, or, sf, specID, ts.Close
	}

	t.Run("success", func(t *testing.T) {
		var gotAuth string
		var gotBody map[string]any
		s, or, _, specID, closeFn := newSvc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			assert.Equal(t, "/checkouts", r.URL.Path)
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_ = json.NewEncoder(w).Encode(map[string]any{"session_id": "cks_1", "checkout_url": "https://pay.example/cks_1"})
		})
		defer closeFn()
		or.On("Create", ctx, mock.AnythingOfType("*domain.Order")).Return(nil).Once()

		order, err := s.CreateOrder(ctx, uuid.New(), specID, loID, " usd ")
		require.NoError(t, err)
		assert.Equal(t, "dodo", order.Provider)
		assert.Equal(t, "USD", order.Currency)
		assert.Equal(t, 1000, order.Amount)
		require.NotNil(t, order.CheckoutURL)
		assert.Equal(t, "https://pay.example/cks_1", *order.CheckoutURL)
		require.NotNil(t, order.ProviderCheckoutID)
		assert.Equal(t, "cks_1", *order.ProviderCheckoutID)
		assert.Equal(t, "cks_1", order.Notes["dodo_session_id"])
		assert.Equal(t, "Bearer key", gotAuth)
		assert.Equal(t, "http://localhost:4200/dashboard", gotBody["return_url"])
		assert.Equal(t, gotBody["metadata"], gotBody["custom_data"])
	})

	t.Run("not configured", func(t *testing.T) {
		s, _, _, specID, closeFn := newSvc(func(w http.ResponseWriter, r *http.Request) {})
		defer closeFn()
		s.dodoConfig = DodoConfig{}
		_, err := s.CreateOrder(ctx, uuid.New(), specID, loID, "USD")
		assert.EqualError(t, err, "dodo payments is not configured")
	})

	t.Run("non-2xx status", func(t *testing.T) {
		s, _, _, specID, closeFn := newSvc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(" upstream broke "))
		})
		defer closeFn()
		_, err := s.CreateOrder(ctx, uuid.New(), specID, loID, "USD")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "status 502")
		assert.Contains(t, err.Error(), "upstream broke")
	})

	t.Run("malformed json", func(t *testing.T) {
		s, _, _, specID, closeFn := newSvc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{bad")) })
		defer closeFn()
		_, err := s.CreateOrder(ctx, uuid.New(), specID, loID, "USD")
		assert.Error(t, err)
	})

	t.Run("incomplete response", func(t *testing.T) {
		s, _, _, specID, closeFn := newSvc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"session_id": "cks_1"})
		})
		defer closeFn()
		_, err := s.CreateOrder(ctx, uuid.New(), specID, loID, "USD")
		assert.EqualError(t, err, "invalid dodo checkout response")
	})

	t.Run("connection failure", func(t *testing.T) {
		s, _, _, specID, closeFn := newSvc(func(w http.ResponseWriter, r *http.Request) {})
		closeFn() // server closed before request
		_, err := s.CreateOrder(ctx, uuid.New(), specID, loID, "USD")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dodo checkout creation failed")
	})
}

func pendingOrder(rzpID *string) *domain.Order {
	return &domain.Order{
		ID:              uuid.New(),
		UserID:          uuid.New(),
		SpecID:          uuid.New(),
		Status:          domain.OrderStatusPending,
		ExpiresAt:       time.Now().Add(time.Hour),
		RazorpayOrderID: rzpID,
		LicenseType:     "Basic",
		Amount:          1000,
		Currency:        "INR",
		Notes:           map[string]any{"license_option_id": uuid.NewString()},
	}
}

func TestPaymentService_VerifyPayment_LaterFailures(t *testing.T) {
	ctx := context.Background()
	paymentID := "pay_1"
	rzp := "order_1"

	setup := func(status string, fetchOK bool) (*paymentService, *orderRepoMock, *paymentRepoMock, *licenseRepoMock, *domain.Order, func()) {
		s, or, pr, lr, _, _, _, _ := newPaymentSvc()
		s.emailSender = nil // exercise nil-sender path in the goroutine
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !fetchOK {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"description":"bad"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": paymentID, "status": status})
		}))
		s.razorpayClient = razorpay.NewClient("k", "s")
		s.razorpayClient.Request.BaseURL = ts.URL
		order := pendingOrder(&rzp)
		or.On("GetByID", ctx, order.ID).Return(order, nil).Once()
		return s, or, pr, lr, order, ts.Close
	}

	t.Run("fetch error", func(t *testing.T) {
		s, _, _, _, order, closeFn := setup("", false)
		defer closeFn()
		_, err := s.VerifyPayment(ctx, order.ID, order.UserID, paymentID, s.generateSignature(rzp, paymentID))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to fetch payment")
	})
	t.Run("not captured and status update fails", func(t *testing.T) {
		s, or, _, _, order, closeFn := setup("authorized", true)
		defer closeFn()
		or.On("UpdateStatus", ctx, order.ID, domain.OrderStatusFailed).Return(errors.New("upd")).Once()
		_, err := s.VerifyPayment(ctx, order.ID, order.UserID, paymentID, s.generateSignature(rzp, paymentID))
		assert.ErrorContains(t, err, "payment not captured and status update failed")
	})
	t.Run("payment repo create fails", func(t *testing.T) {
		s, _, pr, _, order, closeFn := setup("CAPTURED", true)
		defer closeFn()
		pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Return(errors.New("pay db")).Once()
		_, err := s.VerifyPayment(ctx, order.ID, order.UserID, paymentID, s.generateSignature(rzp, paymentID))
		assert.EqualError(t, err, "pay db")
	})
	t.Run("mark paid fails", func(t *testing.T) {
		s, or, pr, _, order, closeFn := setup("captured", true)
		defer closeFn()
		pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Return(nil).Once()
		or.On("UpdateStatus", ctx, order.ID, domain.OrderStatusPaid).Return(errors.New("paid err")).Once()
		_, err := s.VerifyPayment(ctx, order.ID, order.UserID, paymentID, s.generateSignature(rzp, paymentID))
		assert.EqualError(t, err, "paid err")
	})
	t.Run("license creation fails", func(t *testing.T) {
		s, or, pr, lr, order, closeFn := setup("captured", true)
		defer closeFn()
		pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Return(nil).Once()
		or.On("UpdateStatus", ctx, order.ID, domain.OrderStatusPaid).Return(nil).Once()
		lr.On("Create", ctx, mock.AnythingOfType("*domain.License")).Return(errors.New("lic err")).Once()
		_, err := s.VerifyPayment(ctx, order.ID, order.UserID, paymentID, s.generateSignature(rzp, paymentID))
		assert.ErrorContains(t, err, "payment ok but license failed: lic err")
	})
	t.Run("order missing razorpay id", func(t *testing.T) {
		s, or, _, _, _, _, _, _ := newPaymentSvc()
		order := pendingOrder(nil)
		or.On("GetByID", ctx, order.ID).Return(order, nil).Once()
		_, err := s.VerifyPayment(ctx, order.ID, order.UserID, paymentID, "sig")
		assert.EqualError(t, err, "invalid order state")
	})
}

func TestPaymentService_VerifyPayment_StoresMethodEmailAndSendsReceipt(t *testing.T) {
	ctx := context.Background()
	s, or, pr, lr, _, _, uf, es := newPaymentSvc()
	rzp, paymentID := "order_9", "pay_9"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "captured", "method": "upi", "email": "pay@example.com"})
	}))
	defer ts.Close()
	s.razorpayClient = razorpay.NewClient("k", "s")
	s.razorpayClient.Request.BaseURL = ts.URL
	order := pendingOrder(&rzp)
	or.On("GetByID", ctx, order.ID).Return(order, nil).Once()
	var stored *domain.Payment
	pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Run(func(a mock.Arguments) { stored = a.Get(1).(*domain.Payment) }).Return(nil).Once()
	or.On("UpdateStatus", ctx, order.ID, domain.OrderStatusPaid).Return(nil).Once()
	lr.On("Create", ctx, mock.AnythingOfType("*domain.License")).Return(nil).Once()
	uf.On("FindByID", mock.Anything, order.UserID).Return(&authDomain.User{ID: order.UserID, Email: "acct@example.com", Name: "N"}, nil).Once()
	sent := make(chan string, 1)
	es.On("Send", mock.Anything, mock.AnythingOfType("email.Message")).Run(func(a mock.Arguments) {
		sent <- "ok"
	}).Return(nil).Once()

	_, err := s.VerifyPayment(ctx, order.ID, order.UserID, paymentID, s.generateSignature(rzp, paymentID))
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.NotNil(t, stored.Method)
	assert.Equal(t, "upi", *stored.Method)
	require.NotNil(t, stored.Email)
	assert.Equal(t, "pay@example.com", *stored.Email)
	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("receipt email was not sent")
	}
}

func signDodo(key []byte, id, ts string, payload []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + ts + "." + string(payload)))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestPaymentService_VerifyDodoSignature(t *testing.T) {
	s, _, _, _, _, _, _, _ := newPaymentSvc()
	payload := []byte(`{"type":"x"}`)
	hdr := func(sig string) map[string]string {
		return map[string]string{"webhook-id": "id", "webhook-timestamp": "1", "webhook-signature": sig}
	}

	s.dodoConfig = DodoConfig{}
	assert.EqualError(t, s.verifyDodoSignature(payload, hdr("x")), "dodo webhook secret is not configured")

	s.dodoConfig = DodoConfig{WebhookKey: "HTTPS://example.com/hook"}
	assert.ErrorContains(t, s.verifyDodoSignature(payload, hdr("x")), "not the endpoint URL")

	s.dodoConfig = DodoConfig{WebhookKey: "plain-secret"}
	assert.EqualError(t, s.verifyDodoSignature(payload, map[string]string{"webhook-id": "id"}), "missing dodo webhook signature headers")

	// plain secret, base64 signature with v1 prefix
	assert.NoError(t, s.verifyDodoSignature(payload, hdr("v1,"+signDodo([]byte("plain-secret"), "id", "1", payload))))
	// plain secret, hex signature, unprefixed
	mac := hmac.New(sha256.New, []byte("plain-secret"))
	mac.Write([]byte("id.1." + string(payload)))
	assert.NoError(t, s.verifyDodoSignature(payload, hdr(hex.EncodeToString(mac.Sum(nil)))))
	// wrong signature
	assert.EqualError(t, s.verifyDodoSignature(payload, hdr("v1,AAAA")), "invalid dodo webhook signature")

	// whsec with raw (unpadded) base64 key
	raw := []byte("rawkey-bytes!") // length not multiple of 3 -> padding exists in StdEncoding
	s.dodoConfig = DodoConfig{WebhookKey: "whsec_" + base64.RawStdEncoding.EncodeToString(raw)}
	assert.NoError(t, s.verifyDodoSignature(payload, hdr("v1,"+signDodo(raw, "id", "1", payload))))

	// whsec that is not valid base64: falls back to the literal remainder as key
	s.dodoConfig = DodoConfig{WebhookKey: "whsec_***notbase64***"}
	assert.NoError(t, s.verifyDodoSignature(payload, hdr("v1,"+signDodo([]byte("***notbase64***"), "id", "1", payload))))
}

func TestSubtleCompareSignature(t *testing.T) {
	assert.True(t, subtleCompareSignature("v1,abc", "abc"))
	assert.True(t, subtleCompareSignature("v1,zzz v1,abc", "abc"), "space separated list")
	assert.True(t, subtleCompareSignature("v1=abc", "abc"))
	assert.True(t, subtleCompareSignature("abc", "other", "abc"))
	assert.True(t, subtleCompareSignature("  abc  ", "abc"), "trimmed whole-string fallback")
	assert.False(t, subtleCompareSignature("v1,nope", "abc"))
	assert.False(t, subtleCompareSignature("", "abc"))
	assert.False(t, subtleCompareSignature("v1", "v1x"))
}

func TestExtractDodoOrderMetadata(t *testing.T) {
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
			id, pay := extractDodoOrderMetadata(tt.data)
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

func TestPaymentService_HandleDodoWebhook_Branches(t *testing.T) {
	ctx := context.Background()
	key := []byte("whk")
	build := func(t *testing.T, event map[string]any) ([]byte, map[string]string) {
		payload, err := json.Marshal(event)
		require.NoError(t, err)
		return payload, map[string]string{
			"webhook-id": "evt_1", "webhook-timestamp": "10",
			"webhook-signature": "v1," + signDodo(key, "evt_1", "10", payload),
		}
	}
	newSvc := func() (*paymentService, *orderRepoMock, *paymentRepoMock, *licenseRepoMock) {
		s, or, pr, lr, _, _, _, _ := newPaymentSvc()
		s.dodoConfig = DodoConfig{WebhookKey: string(key)}
		return s, or, pr, lr
	}
	oid := uuid.New()
	succeeded := func(extra map[string]any) map[string]any {
		data := map[string]any{"metadata": map[string]any{"order_id": oid.String()}}
		for k, v := range extra {
			data[k] = v
		}
		return map[string]any{"type": "payment.succeeded", "data": data}
	}
	order := func(status domain.OrderStatus, notes map[string]any) *domain.Order {
		return &domain.Order{ID: oid, UserID: uuid.New(), SpecID: uuid.New(), Status: status, Amount: 500, Currency: "USD", LicenseType: "Basic", Notes: notes}
	}

	t.Run("invalid signature", func(t *testing.T) {
		s, _, _, _ := newSvc()
		err := s.HandleDodoWebhook(ctx, []byte(`{}`), map[string]string{"webhook-id": "a", "webhook-timestamp": "b", "webhook-signature": "c"})
		assert.EqualError(t, err, "invalid dodo webhook signature")
	})
	t.Run("invalid json with valid signature", func(t *testing.T) {
		s, _, _, _ := newSvc()
		payload := []byte(`{not json`)
		err := s.HandleDodoWebhook(ctx, payload, map[string]string{
			"webhook-id": "evt_1", "webhook-timestamp": "10",
			"webhook-signature": "v1," + signDodo(key, "evt_1", "10", payload),
		})
		assert.Error(t, err)
	})
	t.Run("ignored event type", func(t *testing.T) {
		s, or, _, _ := newSvc()
		p, h := build(t, map[string]any{"type": "payment.failed", "data": map[string]any{}})
		assert.NoError(t, s.HandleDodoWebhook(ctx, p, h))
		or.AssertNotCalled(t, "GetByID")
	})
	t.Run("missing order id", func(t *testing.T) {
		s, _, _, _ := newSvc()
		p, h := build(t, map[string]any{"type": "payment.succeeded", "data": map[string]any{}})
		assert.EqualError(t, s.HandleDodoWebhook(ctx, p, h), "dodo webhook missing order_id")
	})
	t.Run("order not found", func(t *testing.T) {
		s, or, _, _ := newSvc()
		or.On("GetByID", ctx, oid).Return(nil, errors.New("nf")).Once()
		p, h := build(t, succeeded(nil))
		assert.EqualError(t, s.HandleDodoWebhook(ctx, p, h), "order not found")
	})
	t.Run("already paid is idempotent", func(t *testing.T) {
		s, or, pr, _ := newSvc()
		or.On("GetByID", ctx, oid).Return(order(domain.OrderStatusPaid, nil), nil).Once()
		p, h := build(t, succeeded(nil))
		assert.NoError(t, s.HandleDodoWebhook(ctx, p, h))
		pr.AssertNotCalled(t, "Create")
	})
	t.Run("not payable", func(t *testing.T) {
		s, or, _, _ := newSvc()
		or.On("GetByID", ctx, oid).Return(order(domain.OrderStatusFailed, nil), nil).Once()
		p, h := build(t, succeeded(nil))
		assert.EqualError(t, s.HandleDodoWebhook(ctx, p, h), "order is not payable")
	})
	t.Run("payment create fails", func(t *testing.T) {
		s, or, pr, _ := newSvc()
		or.On("GetByID", ctx, oid).Return(order(domain.OrderStatusPending, nil), nil).Once()
		pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Return(errors.New("pay err")).Once()
		p, h := build(t, succeeded(nil))
		assert.EqualError(t, s.HandleDodoWebhook(ctx, p, h), "pay err")
	})
	t.Run("update status fails", func(t *testing.T) {
		s, or, pr, _ := newSvc()
		or.On("GetByID", ctx, oid).Return(order(domain.OrderStatusPending, nil), nil).Once()
		pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Return(nil).Once()
		or.On("UpdateStatus", ctx, oid, domain.OrderStatusPaid).Return(errors.New("upd")).Once()
		p, h := build(t, succeeded(nil))
		assert.EqualError(t, s.HandleDodoWebhook(ctx, p, h), "upd")
	})
	t.Run("license issue fails", func(t *testing.T) {
		s, or, pr, _ := newSvc()
		or.On("GetByID", ctx, oid).Return(order(domain.OrderStatusPending, map[string]any{}), nil).Once()
		pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Return(nil).Once()
		or.On("UpdateStatus", ctx, oid, domain.OrderStatusPaid).Return(nil).Once()
		p, h := build(t, succeeded(nil))
		assert.EqualError(t, s.HandleDodoWebhook(ctx, p, h), "license_option_id missing")
	})
	t.Run("payment id falls back to webhook id", func(t *testing.T) {
		s, or, pr, lr := newSvc()
		or.On("GetByID", ctx, oid).Return(order(domain.OrderStatusPending, map[string]any{"license_option_id": uuid.NewString()}), nil).Once()
		var got *domain.Payment
		pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Run(func(a mock.Arguments) { got = a.Get(1).(*domain.Payment) }).Return(nil).Once()
		or.On("UpdateStatus", ctx, oid, domain.OrderStatusPaid).Return(nil).Once()
		lr.On("Create", ctx, mock.AnythingOfType("*domain.License")).Return(nil).Once()
		p, h := build(t, succeeded(nil))
		require.NoError(t, s.HandleDodoWebhook(ctx, p, h))
		require.NotNil(t, got)
		assert.Equal(t, "evt_1", got.RazorpayPaymentID)
		assert.Equal(t, h["webhook-signature"], got.RazorpaySignature)
		assert.NotNil(t, got.CapturedAt)
	})
}

func TestPaymentService_GetOrder(t *testing.T) {
	ctx := context.Background()
	s, or, _, _, _, _, _, _ := newPaymentSvc()
	id := uuid.New()
	want := &domain.Order{ID: id}
	or.On("GetByID", ctx, id).Return(want, nil).Once()
	got, err := s.GetOrder(ctx, id, uuid.Nil)
	require.NoError(t, err)
	assert.Same(t, want, got)

	or.On("GetByID", ctx, id).Return(nil, errors.New("nf")).Once()
	_, err = s.GetOrder(ctx, id, uuid.Nil)
	assert.EqualError(t, err, "nf")
}

func TestPaymentService_GetUserLicenses_SignsImages(t *testing.T) {
	ctx := context.Background()
	s, _, _, lr, _, fs, _, _ := newPaymentSvc()
	uid := uuid.New()
	img1, img2, img3, empty := "https://cdn/a.png", "https://cdn/b.png", "https://cdn/c.png", ""
	lr.On("ListByUser", ctx, uid, 5, 10, "q", "Basic").Return([]domain.License{
		{SpecImage: &img1}, {SpecImage: &img2}, {SpecImage: &img3}, {SpecImage: &empty}, {},
	}, 12, nil).Once()
	fs.On("GetKeyFromUrl", img1).Return("a", nil)
	fs.On("GetPresignedURL", ctx, "a", time.Hour).Return("signed-a", nil)
	fs.On("GetKeyFromUrl", img2).Return("", errors.New("bad url"))
	fs.On("GetKeyFromUrl", img3).Return("c", nil)
	fs.On("GetPresignedURL", ctx, "c", time.Hour).Return("", errors.New("sign fail"))

	got, total, err := s.GetUserLicenses(ctx, uid, 3, "q", "Basic")
	require.NoError(t, err)
	assert.Equal(t, 12, total)
	assert.Equal(t, "signed-a", *got[0].SpecImage)
	assert.Equal(t, img2, *got[1].SpecImage, "unchanged when key extraction fails")
	assert.Equal(t, img3, *got[2].SpecImage, "unchanged when signing fails")
	assert.Equal(t, "", *got[3].SpecImage)
	assert.Nil(t, got[4].SpecImage)
}

func TestPaymentService_GetUserOrders_NegativePageClampsOffset(t *testing.T) {
	ctx := context.Background()
	s, or, _, _, _, _, _, _ := newPaymentSvc()
	uid := uuid.New()
	or.On("ListByUser", ctx, uid, 20, 0).Return([]domain.Order{}, nil).Once()
	_, err := s.GetUserOrders(ctx, uid, -5)
	assert.NoError(t, err)
	or.AssertExpectations(t)
}

func TestPaymentService_GetLicenseDownloads_ByLicenseType(t *testing.T) {
	ctx := context.Background()
	uid := uuid.New()
	wav, stems := "https://cdn/wav", "https://cdn/stems"
	emptyStr := ""

	mkSvc := func(licType string, spec *catalogDomain.Spec) (*paymentService, *licenseRepoMock, *fileSvcMock, uuid.UUID) {
		s, _, _, lr, sf, fs, _, _ := newPaymentSvc()
		lic := &domain.License{ID: uuid.New(), UserID: uid, SpecID: spec.ID, LicenseType: licType, IsActive: true}
		lr.On("GetByID", ctx, lic.ID).Return(lic, nil).Once()
		sf.On("FindByIDIncludingDeleted", ctx, spec.ID).Return(spec, nil).Once()
		lr.On("IncrementDownloads", ctx, lic.ID).Return(errors.New("ignored")).Once()
		return s, lr, fs, lic.ID
	}
	fullSpec := func() *catalogDomain.Spec {
		return &catalogDomain.Spec{ID: uuid.New(), Title: "T", PreviewUrl: "https://cdn/preview", WavUrl: &wav, StemsUrl: &stems}
	}

	t.Run("Basic only mp3", func(t *testing.T) {
		s, _, fs, id := mkSvc("Basic", fullSpec())
		fs.On("GetKeyFromUrl", "https://cdn/preview").Return("p", nil)
		fs.On("GetPresignedURL", ctx, "p", time.Hour).Return("signed-p", nil)
		res, err := s.GetLicenseDownloads(ctx, id, uid)
		require.NoError(t, err)
		assert.Equal(t, "signed-p", *res.MP3URL)
		assert.Nil(t, res.WAVURL)
		assert.Nil(t, res.StemsURL)
		assert.Equal(t, 3600, res.ExpiresIn)
		assert.Equal(t, "T", res.SpecTitle)
	})
	t.Run("Premium mp3 and wav, key error falls back to raw url", func(t *testing.T) {
		s, _, fs, id := mkSvc("Premium", fullSpec())
		fs.On("GetKeyFromUrl", "https://cdn/preview").Return("p", nil)
		fs.On("GetPresignedURL", ctx, "p", time.Hour).Return("signed-p", nil)
		fs.On("GetKeyFromUrl", wav).Return("", errors.New("bad"))
		res, err := s.GetLicenseDownloads(ctx, id, uid)
		require.NoError(t, err)
		assert.Equal(t, "signed-p", *res.MP3URL)
		assert.Equal(t, wav, *res.WAVURL)
		assert.Nil(t, res.StemsURL)
	})
	for _, lt := range []string{"Trackout", "Unlimited"} {
		t.Run(lt+" everything, sign error falls back", func(t *testing.T) {
			s, _, fs, id := mkSvc(lt, fullSpec())
			fs.On("GetKeyFromUrl", mock.Anything).Return("k", nil)
			fs.On("GetPresignedURL", ctx, "k", time.Hour).Return("", errors.New("sign"))
			res, err := s.GetLicenseDownloads(ctx, id, uid)
			require.NoError(t, err)
			assert.Equal(t, "https://cdn/preview", *res.MP3URL)
			assert.Equal(t, wav, *res.WAVURL)
			assert.Equal(t, stems, *res.StemsURL)
		})
	}
	t.Run("empty urls yield nil", func(t *testing.T) {
		spec := &catalogDomain.Spec{ID: uuid.New(), WavUrl: &emptyStr, StemsUrl: &emptyStr}
		s, _, _, id := mkSvc("Unlimited", spec)
		res, err := s.GetLicenseDownloads(ctx, id, uid)
		require.NoError(t, err)
		assert.Nil(t, res.MP3URL)
		assert.Nil(t, res.WAVURL)
		assert.Nil(t, res.StemsURL)
	})
	t.Run("unknown type yields no urls", func(t *testing.T) {
		s, _, _, id := mkSvc("Exclusive", fullSpec())
		res, err := s.GetLicenseDownloads(ctx, id, uid)
		require.NoError(t, err)
		assert.Nil(t, res.MP3URL)
		assert.Equal(t, "Exclusive", res.LicenseType)
	})
}

func TestPaymentService_GetProducerOrders_MapsAndClampsOffset(t *testing.T) {
	ctx := context.Background()
	s, or, _, _, _, _, _, _ := newPaymentSvc()
	pid := uuid.New()
	rz := "order_z"
	or.On("ListByProducer", ctx, pid, 10, 0).Return([]domain.OrderWithBuyer{{
		Order:     domain.Order{ID: uuid.New(), Amount: 1550, Currency: "USD", Status: domain.OrderStatusPaid, LicenseType: "Basic", RazorpayOrderID: &rz},
		BuyerName: "B", BuyerEmail: "b@x.com", SpecTitle: "S",
	}}, 1, nil).Once()
	res, err := s.GetProducerOrders(ctx, pid, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Offset)
	assert.Equal(t, 1, res.Total)
	require.Len(t, res.Orders, 1)
	assert.Equal(t, 15.5, res.Orders[0].Amount)
	assert.Equal(t, "B", res.Orders[0].BuyerName)
	assert.Equal(t, &rz, res.Orders[0].RazorpayOrderID)
}

func TestPaymentService_SendReceiptEmail_Branches(t *testing.T) {
	ctx := context.Background()
	order := &domain.Order{ID: uuid.New(), UserID: uuid.New(), Amount: 2500, Currency: "usd", LicenseType: "Basic", Notes: map[string]any{}}
	license := &domain.License{ID: uuid.New()}

	t.Run("nil sender or finder is a no-op", func(t *testing.T) {
		s, _, _, _, _, _, _, _ := newPaymentSvc()
		s.emailSender = nil
		assert.NoError(t, s.sendReceiptEmail(ctx, order, &domain.Payment{}, license))
		s2, _, _, _, _, _, _, _ := newPaymentSvc()
		s2.userFinder = nil
		assert.NoError(t, s2.sendReceiptEmail(ctx, order, &domain.Payment{}, license))
	})
	t.Run("user lookup error", func(t *testing.T) {
		s, _, _, _, _, _, uf, _ := newPaymentSvc()
		uf.On("FindByID", ctx, order.UserID).Return(nil, errors.New("no user")).Once()
		assert.EqualError(t, s.sendReceiptEmail(ctx, order, &domain.Payment{}, license), "no user")
	})
	t.Run("uses account email and default title", func(t *testing.T) {
		s, _, _, _, _, _, uf, es := newPaymentSvc()
		uf.On("FindByID", ctx, order.UserID).Return(&authDomain.User{Email: "acct@example.com", Name: "Acct"}, nil).Once()
		es.On("Send", ctx, mock.MatchedBy(func(m interface{}) bool { return true })).Return(errors.New("smtp")).Once()
		err := s.sendReceiptEmail(ctx, order, &domain.Payment{Email: strp("  ")}, license)
		assert.EqualError(t, err, "smtp")
		es.AssertNumberOfCalls(t, "Send", 1)
	})
	t.Run("payment email overrides", func(t *testing.T) {
		s, _, _, _, _, _, uf, es := newPaymentSvc()
		o := *order
		o.Notes = map[string]any{"spec_title": "Midnight"}
		uf.On("FindByID", ctx, o.UserID).Return(&authDomain.User{Email: "acct@example.com", Name: "Acct"}, nil).Once()
		es.On("Send", ctx, mock.Anything).Return(nil).Once()
		assert.NoError(t, s.sendReceiptEmail(ctx, &o, &domain.Payment{Email: strp("pay@example.com")}, license))
		msg := es.Calls[0].Arguments.Get(1)
		assert.Contains(t, toString(msg), "pay@example.com")
		assert.Contains(t, toString(msg), "Midnight")
	})
}

func toString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestFormatMoney(t *testing.T) {
	assert.Equal(t, "INR 12.34", formatMoney(1234, "inr"))
	assert.Equal(t, "USD 5.00", formatMoney(500, "usd"))
	assert.Equal(t, "EUR 0.99", formatMoney(99, "Eur"))
}

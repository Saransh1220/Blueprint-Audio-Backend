package application

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/razorpay/razorpay-go"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type orderRepoMock struct{ mock.Mock }

func (m *orderRepoMock) Create(ctx context.Context, order *domain.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}
func (m *orderRepoMock) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}
func (m *orderRepoMock) GetByRazorpayID(ctx context.Context, razorpayOrderID string) (*domain.Order, error) {
	args := m.Called(ctx, razorpayOrderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}
func (m *orderRepoMock) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}
func (m *orderRepoMock) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Order, error) {
	args := m.Called(ctx, userID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Order), args.Error(1)
}
func (m *orderRepoMock) ListByProducer(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]domain.OrderWithBuyer, int, error) {
	args := m.Called(ctx, producerID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.OrderWithBuyer), args.Int(1), args.Error(2)
}

type paymentRepoMock struct{ mock.Mock }

func (m *paymentRepoMock) Create(ctx context.Context, payment *domain.Payment) error {
	args := m.Called(ctx, payment)
	return args.Error(0)
}
func (m *paymentRepoMock) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}
func (m *paymentRepoMock) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}
func (m *paymentRepoMock) GetByRazorpayID(ctx context.Context, razorpayPaymentID string) (*domain.Payment, error) {
	args := m.Called(ctx, razorpayPaymentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}

type licenseRepoMock struct{ mock.Mock }

func (m *licenseRepoMock) Create(ctx context.Context, license *domain.License) error {
	args := m.Called(ctx, license)
	return args.Error(0)
}
func (m *licenseRepoMock) GetByID(ctx context.Context, id uuid.UUID) (*domain.License, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.License), args.Error(1)
}
func (m *licenseRepoMock) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.License, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.License), args.Error(1)
}
func (m *licenseRepoMock) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int, search, licenseType string) ([]domain.License, int, error) {
	args := m.Called(ctx, userID, limit, offset, search, licenseType)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.License), args.Int(1), args.Error(2)
}
func (m *licenseRepoMock) IncrementDownloads(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}
func (m *licenseRepoMock) Revoke(ctx context.Context, id uuid.UUID, reason string) error {
	args := m.Called(ctx, id, reason)
	return args.Error(0)
}

type specFinderMock struct{ mock.Mock }

func (m *specFinderMock) FindByID(ctx context.Context, id uuid.UUID) (*catalogDomain.Spec, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*catalogDomain.Spec), args.Error(1)
}
func (m *specFinderMock) FindByIDIncludingDeleted(ctx context.Context, id uuid.UUID) (*catalogDomain.Spec, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*catalogDomain.Spec), args.Error(1)
}
func (m *specFinderMock) FindWithLicenses(ctx context.Context, id uuid.UUID) (*catalogDomain.Spec, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*catalogDomain.Spec), args.Error(1)
}
func (m *specFinderMock) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}
func (m *specFinderMock) GetLicenseByID(ctx context.Context, licenseID uuid.UUID) (*catalogDomain.LicenseOption, error) {
	args := m.Called(ctx, licenseID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*catalogDomain.LicenseOption), args.Error(1)
}

type fileSvcMock struct{ mock.Mock }

func (m *fileSvcMock) GetKeyFromUrl(fileURL string) (string, error) {
	args := m.Called(fileURL)
	return args.String(0), args.Error(1)
}
func (m *fileSvcMock) GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	args := m.Called(ctx, key, expiration)
	return args.String(0), args.Error(1)
}

func newPaymentSvc() (*paymentService, *orderRepoMock, *paymentRepoMock, *licenseRepoMock, *specFinderMock, *fileSvcMock) {
	or := new(orderRepoMock)
	pr := new(paymentRepoMock)
	lr := new(licenseRepoMock)
	sf := new(specFinderMock)
	fs := new(fileSvcMock)
	return &paymentService{
		orderRepo:      or,
		paymentRepo:    pr,
		licenseRepo:    lr,
		specFinder:     sf,
		fileService:    fs,
		razorpaySecret: "key-secret",
	}, or, pr, lr, sf, fs
}

func TestPaymentService_GenerateSignature(t *testing.T) {
	s, _, _, _, _, _ := newPaymentSvc()
	sig := s.generateSignature("order_1", "pay_1")
	assert.NotEmpty(t, sig)
	assert.Equal(t, sig, s.generateSignature("order_1", "pay_1"))
}

func TestPaymentService_CreateOrder_Errors(t *testing.T) {
	s, _, _, _, sf, _ := newPaymentSvc()
	ctx := context.Background()
	userID := uuid.New()
	specID := uuid.New()
	licenseID := uuid.New()

	sf.On("FindWithLicenses", ctx, specID).Return(nil, errors.New("not found")).Once()
	_, err := s.CreateOrder(ctx, userID, specID, licenseID)
	assert.EqualError(t, err, "Beat/Sample not found")

	spec := &catalogDomain.Spec{ID: specID, Title: "Track", Licenses: []catalogDomain.LicenseOption{}}
	sf.On("FindWithLicenses", ctx, specID).Return(spec, nil).Once()
	_, err = s.CreateOrder(ctx, userID, specID, licenseID)
	assert.EqualError(t, err, "license option not found")
}

func TestPaymentService_VerifyPayment_EarlyFailures(t *testing.T) {
	s, or, _, _, _, _ := newPaymentSvc()
	ctx := context.Background()
	orderID := uuid.New()

	or.On("GetByID", ctx, orderID).Return(nil, errors.New("missing")).Once()
	_, err := s.VerifyPayment(ctx, orderID, "pay_1", "sig")
	assert.EqualError(t, err, "order not found")

	paid := &domain.Order{ID: orderID, Status: domain.OrderStatusPaid}
	or.On("GetByID", ctx, orderID).Return(paid, nil).Once()
	_, err = s.VerifyPayment(ctx, orderID, "pay_1", "sig")
	assert.EqualError(t, err, "order already processed")

	expired := &domain.Order{
		ID:              orderID,
		Status:          domain.OrderStatusPending,
		ExpiresAt:       time.Now().Add(-time.Hour),
		RazorpayOrderID: ptr("order_1"),
	}
	or.On("GetByID", ctx, orderID).Return(expired, nil).Once()
	or.On("UpdateStatus", ctx, orderID, domain.OrderStatusFailed).Return(nil).Once()
	_, err = s.VerifyPayment(ctx, orderID, "pay_1", "sig")
	assert.EqualError(t, err, "order expired")

	pending := &domain.Order{
		ID:              orderID,
		Status:          domain.OrderStatusPending,
		ExpiresAt:       time.Now().Add(time.Hour),
		RazorpayOrderID: ptr("order_1"),
	}
	or.On("GetByID", ctx, orderID).Return(pending, nil).Once()
	or.On("UpdateStatus", ctx, orderID, domain.OrderStatusFailed).Return(nil).Once()
	_, err = s.VerifyPayment(ctx, orderID, "pay_1", "wrong")
	assert.EqualError(t, err, "invalid signature")
}

func TestPaymentService_GetUserOrdersAndProducerOrders(t *testing.T) {
	s, or, _, _, _, _ := newPaymentSvc()
	ctx := context.Background()
	userID := uuid.New()
	producerID := uuid.New()

	or.On("ListByUser", ctx, userID, 20, 0).Return([]domain.Order{}, nil).Once()
	orders, err := s.GetUserOrders(ctx, userID, -1)
	assert.NoError(t, err)
	assert.NotNil(t, orders)

	or.On("ListByProducer", ctx, producerID, 50, 0).Return([]domain.OrderWithBuyer{}, 0, nil).Once()
	resp, err := s.GetProducerOrders(ctx, producerID, 0)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 50, resp.Limit)
}

func TestPaymentService_GetUserLicensesAndDownloads(t *testing.T) {
	s, _, _, lr, sf, fs := newPaymentSvc()
	ctx := context.Background()
	userID := uuid.New()
	specID := uuid.New()
	licenseID := uuid.New()
	wav := "http://bucket/wav.wav"
	stems := "http://bucket/stems.zip"

	licenses := []domain.License{
		{ID: licenseID, UserID: userID, SpecID: specID, LicenseType: "Premium", IsActive: true, SpecImage: ptr("http://bucket/img.jpg")},
	}
	lr.On("ListByUser", ctx, userID, 5, 0, "", "").Return(licenses, 1, nil).Once()
	fs.On("GetKeyFromUrl", "http://bucket/img.jpg").Return("img.jpg", nil).Once()
	fs.On("GetPresignedURL", ctx, "img.jpg", mock.Anything).Return("signed-img", nil).Once()
	out, total, err := s.GetUserLicenses(ctx, userID, 0, "", "")
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.NotNil(t, out[0].SpecImage)

	lic := &domain.License{ID: licenseID, UserID: userID, SpecID: specID, LicenseType: "Unlimited", IsActive: true}
	spec := &catalogDomain.Spec{ID: specID, Title: "Track", PreviewUrl: "http://bucket/prev.mp3", WavUrl: &wav, StemsUrl: &stems}
	lr.On("GetByID", ctx, licenseID).Return(lic, nil).Once()
	sf.On("FindByIDIncludingDeleted", ctx, specID).Return(spec, nil).Once()
	fs.On("GetKeyFromUrl", "http://bucket/prev.mp3").Return("prev.mp3", nil).Once()
	fs.On("GetPresignedURL", ctx, "prev.mp3", mock.Anything).Return("signed-prev", nil).Once()
	fs.On("GetKeyFromUrl", wav).Return("wav.wav", nil).Once()
	fs.On("GetPresignedURL", ctx, "wav.wav", mock.Anything).Return("signed-wav", nil).Once()
	fs.On("GetKeyFromUrl", stems).Return("stems.zip", nil).Once()
	fs.On("GetPresignedURL", ctx, "stems.zip", mock.Anything).Return("signed-stems", nil).Once()
	lr.On("IncrementDownloads", ctx, licenseID).Return(nil).Once()
	dl, err := s.GetLicenseDownloads(ctx, licenseID, userID)
	assert.NoError(t, err)
	assert.NotNil(t, dl)
	assert.NotNil(t, dl.MP3URL)
	assert.NotNil(t, dl.WAVURL)
	assert.NotNil(t, dl.StemsURL)
}

func TestPaymentService_GetLicenseDownloads_Errors(t *testing.T) {
	s, _, _, lr, sf, _ := newPaymentSvc()
	ctx := context.Background()
	licenseID := uuid.New()
	userID := uuid.New()
	specID := uuid.New()

	lr.On("GetByID", ctx, licenseID).Return(nil, errors.New("missing")).Once()
	_, err := s.GetLicenseDownloads(ctx, licenseID, userID)
	assert.EqualError(t, err, "license not found")

	lr.On("GetByID", ctx, licenseID).Return(&domain.License{ID: licenseID, UserID: uuid.New(), SpecID: specID, IsActive: true}, nil).Once()
	_, err = s.GetLicenseDownloads(ctx, licenseID, userID)
	assert.EqualError(t, err, "unauthorized: you do not own this license")

	lr.On("GetByID", ctx, licenseID).Return(&domain.License{ID: licenseID, UserID: userID, SpecID: specID, IsActive: false}, nil).Once()
	_, err = s.GetLicenseDownloads(ctx, licenseID, userID)
	assert.EqualError(t, err, "license is not active")

	lr.On("GetByID", ctx, licenseID).Return(&domain.License{ID: licenseID, UserID: userID, SpecID: specID, IsActive: true, IsRevoked: true}, nil).Once()
	_, err = s.GetLicenseDownloads(ctx, licenseID, userID)
	assert.EqualError(t, err, "license has been revoked")

	lr.On("GetByID", ctx, licenseID).Return(&domain.License{ID: licenseID, UserID: userID, SpecID: specID, IsActive: true}, nil).Once()
	sf.On("FindByIDIncludingDeleted", ctx, specID).Return(nil, errors.New("missing")).Once()
	_, err = s.GetLicenseDownloads(ctx, licenseID, userID)
	assert.EqualError(t, err, "spec not found")
}

func TestPaymentService_IssueLicense(t *testing.T) {
	s, _, _, lr, sf, _ := newPaymentSvc()
	ctx := context.Background()
	specID := uuid.New()
	loID := uuid.New()

	order := &domain.Order{
		ID:          uuid.New(),
		UserID:      uuid.New(),
		SpecID:      specID,
		LicenseType: "Premium",
		Amount:      1200,
		Notes:       map[string]any{"license_option_id": loID.String()},
	}

	spec := &catalogDomain.Spec{
		ID: specID,
		Licenses: []catalogDomain.LicenseOption{
			{ID: loID, LicenseType: catalogDomain.LicenseType("Premium"), Name: "Premium"},
		},
	}
	sf.On("FindWithLicenses", ctx, specID).Return(spec, nil).Once()
	lr.On("Create", ctx, mock.AnythingOfType("*domain.License")).Return(nil).Once()
	lic, err := s.issueLicense(ctx, order)
	assert.NoError(t, err)
	assert.Equal(t, loID, lic.LicenseOptionID)
}

func TestPaymentService_IssueLicense_InvalidLicenseOptionID(t *testing.T) {
	s, _, _, _, _, _ := newPaymentSvc()
	ctx := context.Background()
	order := &domain.Order{
		ID:          uuid.New(),
		UserID:      uuid.New(),
		SpecID:      uuid.New(),
		LicenseType: "Premium",
		Amount:      1200,
		Notes:       map[string]any{"license_option_id": "not-a-uuid"},
	}
	_, err := s.issueLicense(ctx, order)
	assert.EqualError(t, err, "invalid license_option_id")
}

func ptr(s string) *string { return &s }

func TestPaymentService_VerifyPayment_StatusUpdateFailureBranches(t *testing.T) {
	s, or, _, _, _, _ := newPaymentSvc()
	ctx := context.Background()
	orderID := uuid.New()

	expired := &domain.Order{ID: orderID, Status: domain.OrderStatusPending, ExpiresAt: time.Now().Add(-time.Hour), RazorpayOrderID: ptr("order_1")}
	or.On("GetByID", ctx, orderID).Return(expired, nil).Once()
	or.On("UpdateStatus", ctx, orderID, domain.OrderStatusFailed).Return(errors.New("upd")).Once()
	_, err := s.VerifyPayment(ctx, orderID, "pay", "sig")
	assert.EqualError(t, err, "order expired and status update failed: upd")

	pending := &domain.Order{ID: orderID, Status: domain.OrderStatusPending, ExpiresAt: time.Now().Add(time.Hour), RazorpayOrderID: nil}
	or.On("GetByID", ctx, orderID).Return(pending, nil).Once()
	_, err = s.VerifyPayment(ctx, orderID, "pay", "sig")
	assert.EqualError(t, err, "invalid order state")

	pending = &domain.Order{ID: orderID, Status: domain.OrderStatusPending, ExpiresAt: time.Now().Add(time.Hour), RazorpayOrderID: ptr("order_1")}
	or.On("GetByID", ctx, orderID).Return(pending, nil).Once()
	or.On("UpdateStatus", ctx, orderID, domain.OrderStatusFailed).Return(errors.New("upd2")).Once()
	_, err = s.VerifyPayment(ctx, orderID, "pay", "wrong")
	assert.EqualError(t, err, "invalid signature and status update failed: upd2")
}

func TestPaymentService_GetUserLicensesAndProducerOrders_ErrorsAndFallbacks(t *testing.T) {
	s, or, _, lr, _, fs := newPaymentSvc()
	ctx := context.Background()
	userID := uuid.New()
	producerID := uuid.New()

	lr.On("ListByUser", ctx, userID, 5, 0, "q", "Basic").Return(nil, 0, errors.New("db")).Once()
	_, _, err := s.GetUserLicenses(ctx, userID, 1, "q", "Basic")
	assert.EqualError(t, err, "db")

	img := "http://bucket/img.jpg"
	licenses := []domain.License{{ID: uuid.New(), UserID: userID, SpecImage: &img}}
	lr.On("ListByUser", ctx, userID, 5, 0, "", "").Return(licenses, 1, nil).Once()
	fs.On("GetKeyFromUrl", img).Return("", errors.New("bad key")).Once()
	out, total, err := s.GetUserLicenses(ctx, userID, 1, "", "")
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, img, *out[0].SpecImage)

	or.On("ListByProducer", ctx, producerID, 50, 0).Return(nil, 0, errors.New("repo")).Once()
	_, err = s.GetProducerOrders(ctx, producerID, 1)
	assert.EqualError(t, err, "repo")
}

func TestPaymentService_IssueLicense_MissingOptionID(t *testing.T) {
	s, _, _, _, _, _ := newPaymentSvc()
	_, err := s.issueLicense(context.Background(), &domain.Order{Notes: map[string]any{}})
	assert.EqualError(t, err, "license_option_id missing")
}

func TestPaymentService_CreateOrder_SuccessWithLocalRazorpay(t *testing.T) {
	s, or, _, _, sf, _ := newPaymentSvc()
	ctx := context.Background()
	userID := uuid.New()
	specID := uuid.New()
	loID := uuid.New()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/orders" && r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "order_local_1"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	s.razorpayClient = razorpay.NewClient("key", "secret")
	s.razorpayClient.Request.BaseURL = ts.URL

	spec := &catalogDomain.Spec{
		ID:    specID,
		Title: "Track",
		Licenses: []catalogDomain.LicenseOption{
			{ID: loID, LicenseType: catalogDomain.LicenseBasic, Name: "Basic", Price: 99},
		},
	}
	sf.On("FindWithLicenses", ctx, specID).Return(spec, nil).Once()
	or.On("Create", ctx, mock.AnythingOfType("*domain.Order")).Return(nil).Once()

	order, err := s.CreateOrder(ctx, userID, specID, loID)
	assert.NoError(t, err)
	assert.NotNil(t, order)
	assert.Equal(t, "Basic", order.LicenseType)
	assert.Equal(t, 9900, order.Amount)
}

func TestPaymentService_VerifyPayment_SuccessAndNotCaptured(t *testing.T) {
	s, or, pr, lr, _, _ := newPaymentSvc()
	ctx := context.Background()
	orderID := uuid.New()
	loID := uuid.New()
	rzpOrderID := "order_local_2"
	paymentID := "pay_local_1"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/payments/"+paymentID && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     paymentID,
				"status": "captured",
				"method": "card",
				"email":  "buyer@example.com",
			})
			return
		}
		if r.URL.Path == "/v1/payments/pay_not_captured" && r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "pay_not_captured",
				"status": "failed",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	s.razorpayClient = razorpay.NewClient("key", "secret")
	s.razorpayClient.Request.BaseURL = ts.URL

	order := &domain.Order{
		ID:              orderID,
		UserID:          uuid.New(),
		SpecID:          uuid.New(),
		Status:          domain.OrderStatusPending,
		ExpiresAt:       time.Now().Add(time.Hour),
		RazorpayOrderID: &rzpOrderID,
		LicenseType:     "Basic",
		Amount:          1000,
		Currency:        "INR",
		Notes:           map[string]any{"license_option_id": loID.String()},
	}

	signature := s.generateSignature(rzpOrderID, paymentID)
	or.On("GetByID", ctx, orderID).Return(order, nil).Twice()
	pr.On("Create", ctx, mock.AnythingOfType("*domain.Payment")).Return(nil).Once()
	or.On("UpdateStatus", ctx, orderID, domain.OrderStatusPaid).Return(nil).Once()
	lr.On("Create", ctx, mock.AnythingOfType("*domain.License")).Return(nil).Once()

	license, err := s.VerifyPayment(ctx, orderID, paymentID, signature)
	assert.NoError(t, err)
	assert.NotNil(t, license)

	or.On("UpdateStatus", ctx, orderID, domain.OrderStatusFailed).Return(nil).Once()
	_, err = s.VerifyPayment(ctx, orderID, "pay_not_captured", s.generateSignature(rzpOrderID, "pay_not_captured"))
	assert.EqualError(t, err, "payment not captured")
}

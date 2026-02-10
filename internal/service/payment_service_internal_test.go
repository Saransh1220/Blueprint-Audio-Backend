package service

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
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

type specRepoMock struct{ mock.Mock }

func (m *specRepoMock) Create(ctx context.Context, spec *domain.Spec) error {
	args := m.Called(ctx, spec)
	return args.Error(0)
}
func (m *specRepoMock) GetByID(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Spec), args.Error(1)
}
func (m *specRepoMock) GetByIDSystem(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Spec), args.Error(1)
}
func (m *specRepoMock) List(ctx context.Context, filter domain.SpecFilter) ([]domain.Spec, int, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Spec), args.Int(1), args.Error(2)
}
func (m *specRepoMock) Update(ctx context.Context, spec *domain.Spec) error {
	args := m.Called(ctx, spec)
	return args.Error(0)
}
func (m *specRepoMock) Delete(ctx context.Context, id uuid.UUID, producerID uuid.UUID) error {
	args := m.Called(ctx, id, producerID)
	return args.Error(0)
}
func (m *specRepoMock) ListByUserID(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]domain.Spec, int, error) {
	args := m.Called(ctx, producerID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Spec), args.Int(1), args.Error(2)
}

type fileSvcMock struct{ mock.Mock }

func (m *fileSvcMock) Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, string, error) {
	args := m.Called(ctx, file, header, folder)
	return args.String(0), args.String(1), args.Error(2)
}
func (m *fileSvcMock) UploadWithKey(ctx context.Context, file io.Reader, key string, contentType string) (string, error) {
	args := m.Called(ctx, file, key, contentType)
	return args.String(0), args.Error(1)
}
func (m *fileSvcMock) GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	args := m.Called(ctx, key, expiration)
	return args.String(0), args.Error(1)
}
func (m *fileSvcMock) GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error) {
	args := m.Called(ctx, key, filename, expiration)
	return args.String(0), args.Error(1)
}
func (m *fileSvcMock) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}
func (m *fileSvcMock) GetKeyFromUrl(fileURL string) (string, error) {
	args := m.Called(fileURL)
	return args.String(0), args.Error(1)
}

func TestPaymentService_GenerateSignature(t *testing.T) {
	s := &paymentService{razorpaySecret: "key-secret"}
	sig := s.generateSignature("order_1", "pay_1")
	assert.NotEmpty(t, sig)
	assert.Equal(t, sig, s.generateSignature("order_1", "pay_1"))
}

func TestNewPaymentService(t *testing.T) {
	t.Setenv("RAZORPAY_KEY_ID", "key_id")
	t.Setenv("RAZORPAY_KEY_SECRET", "key_secret")

	or := new(orderRepoMock)
	pr := new(paymentRepoMock)
	lr := new(licenseRepoMock)
	sr := new(specRepoMock)
	fs := new(fileSvcMock)

	svc := NewPaymentService(or, pr, lr, sr, fs)
	assert.NotNil(t, svc)
}

func TestPaymentService_GetUserOrders(t *testing.T) {
	or := new(orderRepoMock)
	s := &paymentService{orderRepo: or}
	userID := uuid.New()
	or.On("ListByUser", mock.Anything, userID, 20, 0).Return([]domain.Order{}, nil)

	orders, err := s.GetUserOrders(context.Background(), userID, -2)
	assert.NoError(t, err)
	assert.Len(t, orders, 0)
}

func TestPaymentService_GetUserLicensesAndDownloads(t *testing.T) {
	ctx := context.Background()
	lr := new(licenseRepoMock)
	sr := new(specRepoMock)
	fs := new(fileSvcMock)
	s := &paymentService{
		licenseRepo: lr,
		specRepo:    sr,
		fileService: fs,
	}

	userID := uuid.New()
	licenseID := uuid.New()
	specID := uuid.New()
	image := "http://storage/bucket/image.jpg"
	licenses := []domain.License{{ID: licenseID, UserID: userID, SpecImage: &image}}

	lr.On("ListByUser", ctx, userID, 5, 0, "abc", "Basic").Return(licenses, 1, nil).Once()
	fs.On("GetKeyFromUrl", image).Return("image.jpg", nil).Once()
	fs.On("GetPresignedURL", ctx, "image.jpg", time.Hour).Return("signed-image", nil).Once()

	got, total, err := s.GetUserLicenses(ctx, userID, 1, "abc", "Basic")
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, "signed-image", *got[0].SpecImage)

	license := &domain.License{ID: licenseID, UserID: userID, SpecID: specID, LicenseType: "Unlimited", IsActive: true}
	wav := "http://storage/bucket/track.wav"
	stems := "http://storage/bucket/stems.zip"
	spec := &domain.Spec{ID: specID, Title: "Track", PreviewUrl: "http://storage/bucket/preview.mp3", WavUrl: &wav, StemsUrl: &stems}

	lr.On("GetByID", ctx, licenseID).Return(license, nil)
	sr.On("GetByIDSystem", ctx, specID).Return(spec, nil)
	fs.On("GetKeyFromUrl", spec.PreviewUrl).Return("preview.mp3", nil)
	fs.On("GetPresignedURL", ctx, "preview.mp3", time.Hour).Return("signed-preview", nil)
	fs.On("GetKeyFromUrl", wav).Return("track.wav", nil)
	fs.On("GetPresignedURL", ctx, "track.wav", time.Hour).Return("signed-wav", nil)
	fs.On("GetKeyFromUrl", stems).Return("stems.zip", nil)
	fs.On("GetPresignedURL", ctx, "stems.zip", time.Hour).Return("signed-stems", nil)
	lr.On("IncrementDownloads", ctx, licenseID).Return(nil)

	downloads, err := s.GetLicenseDownloads(ctx, licenseID, userID)
	assert.NoError(t, err)
	assert.Equal(t, dto.LicenseDownloadsResponse{
		LicenseID:   licenseID.String(),
		LicenseType: "Unlimited",
		SpecTitle:   "Track",
		ExpiresIn:   3600,
		MP3URL:      ptr("signed-preview"),
		WAVURL:      ptr("signed-wav"),
		StemsURL:    ptr("signed-stems"),
	}, *downloads)
}

func TestPaymentService_GetLicenseDownloadsErrors(t *testing.T) {
	ctx := context.Background()
	lr := new(licenseRepoMock)
	s := &paymentService{licenseRepo: lr}
	licenseID := uuid.New()
	userID := uuid.New()

	lr.On("GetByID", ctx, licenseID).Return(nil, errors.New("sql: no rows"))
	_, err := s.GetLicenseDownloads(ctx, licenseID, userID)
	assert.EqualError(t, err, "license not found")
}

func TestPaymentService_VerifyPaymentEarlyFailures(t *testing.T) {
	ctx := context.Background()
	or := new(orderRepoMock)
	s := &paymentService{orderRepo: or, razorpaySecret: "secret"}
	orderID := uuid.New()

	or.On("GetByID", ctx, orderID).Return(nil, errors.New("sql")).Once()
	_, err := s.VerifyPayment(ctx, orderID, "pay", "sig")
	assert.EqualError(t, err, "order not found")

	or2 := new(orderRepoMock)
	s = &paymentService{orderRepo: or2, razorpaySecret: "secret"}
	or2.On("GetByID", ctx, orderID).Return(&domain.Order{
		ID:     orderID,
		Status: domain.OrderStatusPaid,
	}, nil).Once()
	_, err = s.VerifyPayment(ctx, orderID, "pay", "sig")
	assert.EqualError(t, err, "order already processed")

	or3 := new(orderRepoMock)
	s = &paymentService{orderRepo: or3, razorpaySecret: "secret"}
	or3.On("GetByID", ctx, orderID).Return(&domain.Order{
		ID:              orderID,
		Status:          domain.OrderStatusPending,
		ExpiresAt:       time.Now().Add(-time.Minute),
		RazorpayOrderID: ptr("order_1"),
	}, nil).Once()
	or3.On("UpdateStatus", ctx, orderID, domain.OrderStatusFailed).Return(nil).Once()
	_, err = s.VerifyPayment(ctx, orderID, "pay", "sig")
	assert.EqualError(t, err, "order expired")

	or4 := new(orderRepoMock)
	s = &paymentService{orderRepo: or4, razorpaySecret: "secret"}
	or4.On("GetByID", ctx, orderID).Return(&domain.Order{
		ID:              orderID,
		Status:          domain.OrderStatusPending,
		ExpiresAt:       time.Now().Add(time.Minute),
		RazorpayOrderID: ptr("order_1"),
	}, nil).Once()
	or4.On("UpdateStatus", ctx, orderID, domain.OrderStatusFailed).Return(nil).Once()
	_, err = s.VerifyPayment(ctx, orderID, "pay", "bad-signature")
	assert.EqualError(t, err, "invalid signature")
}

func TestPaymentService_IssueLicenseMissingOption(t *testing.T) {
	lr := new(licenseRepoMock)
	s := &paymentService{licenseRepo: lr}

	_, err := s.issueLicense(context.Background(), &domain.Order{
		ID:    uuid.New(),
		Notes: map[string]any{},
	})
	assert.EqualError(t, err, "license_option_id missing")
}

func TestPaymentService_CreateOrderAndOtherBranches(t *testing.T) {
	ctx := context.Background()
	specRepo := new(specRepoMock)
	s := &paymentService{specRepo: specRepo}
	userID := uuid.New()
	specID := uuid.New()
	licenseID := uuid.New()

	specRepo.On("GetByID", ctx, specID).Return(nil, errors.New("db")).Once()
	_, err := s.CreateOrder(ctx, userID, specID, licenseID)
	assert.EqualError(t, err, "Beat/Sample not found")

	specRepo.On("GetByID", ctx, specID).Return(&domain.Spec{ID: specID, Title: "Track"}, nil).Once()
	_, err = s.CreateOrder(ctx, userID, specID, licenseID)
	assert.EqualError(t, err, "license option not found")

	or := new(orderRepoMock)
	s.orderRepo = or
	orderID := uuid.New()
	or.On("GetByID", ctx, orderID).Return(nil, errors.New("db")).Once()
	_, err = s.GetOrder(ctx, orderID)
	assert.Error(t, err)
}

func TestPaymentService_GetLicenseDownloadsForbiddenPaths(t *testing.T) {
	ctx := context.Background()
	lr := new(licenseRepoMock)
	sr := new(specRepoMock)
	s := &paymentService{licenseRepo: lr, specRepo: sr, fileService: new(fileSvcMock)}

	licenseID := uuid.New()
	ownerID := uuid.New()
	otherID := uuid.New()
	specID := uuid.New()

	lr.On("GetByID", ctx, licenseID).Return(&domain.License{ID: licenseID, UserID: ownerID, IsActive: true, SpecID: specID}, nil).Once()
	_, err := s.GetLicenseDownloads(ctx, licenseID, otherID)
	assert.EqualError(t, err, "unauthorized: you do not own this license")

	lr.On("GetByID", ctx, licenseID).Return(&domain.License{ID: licenseID, UserID: ownerID, IsActive: false, SpecID: specID}, nil).Once()
	_, err = s.GetLicenseDownloads(ctx, licenseID, ownerID)
	assert.EqualError(t, err, "license is not active")

	lr.On("GetByID", ctx, licenseID).Return(&domain.License{ID: licenseID, UserID: ownerID, IsActive: true, IsRevoked: true, SpecID: specID}, nil).Once()
	_, err = s.GetLicenseDownloads(ctx, licenseID, ownerID)
	assert.EqualError(t, err, "license has been revoked")

	lr.On("GetByID", ctx, licenseID).Return(&domain.License{ID: licenseID, UserID: ownerID, IsActive: true, SpecID: specID, LicenseType: "Basic"}, nil).Once()
	sr.On("GetByIDSystem", ctx, specID).Return(nil, errors.New("db")).Once()
	_, err = s.GetLicenseDownloads(ctx, licenseID, ownerID)
	assert.EqualError(t, err, "spec not found")
}

func TestPaymentService_GetLicenseDownloadsByLicenseType(t *testing.T) {
	ctx := context.Background()
	lr := new(licenseRepoMock)
	sr := new(specRepoMock)
	fs := new(fileSvcMock)
	s := &paymentService{licenseRepo: lr, specRepo: sr, fileService: fs}

	licenseID := uuid.New()
	userID := uuid.New()
	specID := uuid.New()
	wav := "wav-url"
	stems := "stems-url"
	spec := &domain.Spec{ID: specID, Title: "Track", PreviewUrl: "preview-url", WavUrl: &wav, StemsUrl: &stems}
	sr.On("GetByIDSystem", ctx, specID).Return(spec, nil).Maybe()

	cases := []string{"Basic", "Premium", "Trackout"}
	for _, lt := range cases {
		lr.On("GetByID", ctx, licenseID).Return(&domain.License{ID: licenseID, UserID: userID, SpecID: specID, IsActive: true, LicenseType: lt}, nil).Once()
		fs.On("GetKeyFromUrl", "preview-url").Return("preview", nil).Maybe()
		fs.On("GetPresignedURL", ctx, "preview", time.Hour).Return("p", nil).Maybe()
		fs.On("GetKeyFromUrl", wav).Return("wav", nil).Maybe()
		fs.On("GetPresignedURL", ctx, "wav", time.Hour).Return("w", nil).Maybe()
		fs.On("GetKeyFromUrl", stems).Return("stems", nil).Maybe()
		fs.On("GetPresignedURL", ctx, "stems", time.Hour).Return("s", nil).Maybe()
		lr.On("IncrementDownloads", ctx, licenseID).Return(nil).Once()
		out, err := s.GetLicenseDownloads(ctx, licenseID, userID)
		assert.NoError(t, err)
		assert.Equal(t, lt, out.LicenseType)
	}
}

func TestPaymentService_GetUserLicensesBranches(t *testing.T) {
	ctx := context.Background()
	lr := new(licenseRepoMock)
	fs := new(fileSvcMock)
	s := &paymentService{licenseRepo: lr, fileService: fs}
	userID := uuid.New()
	image := "img-url"

	lr.On("ListByUser", ctx, userID, 5, 0, "", "").Return(nil, 0, errors.New("db")).Once()
	_, _, err := s.GetUserLicenses(ctx, userID, 1, "", "")
	assert.EqualError(t, err, "db")

	lr.On("ListByUser", ctx, userID, 5, 0, "", "").Return([]domain.License{{SpecImage: &image}}, 1, nil).Once()
	fs.On("GetKeyFromUrl", image).Return("", errors.New("bad")).Once()
	licenses, total, err := s.GetUserLicenses(ctx, userID, 1, "", "")
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, image, *licenses[0].SpecImage)
}

func TestPaymentService_IssueLicenseSuccess(t *testing.T) {
	ctx := context.Background()
	lr := new(licenseRepoMock)
	s := &paymentService{licenseRepo: lr}
	orderID := uuid.New()
	userID := uuid.New()
	specID := uuid.New()
	optID := uuid.New()

	order := &domain.Order{
		ID:          orderID,
		UserID:      userID,
		SpecID:      specID,
		LicenseType: "Basic",
		Amount:      1000,
		Notes: map[string]any{
			"license_option_id": optID.String(),
		},
	}
	lr.On("Create", ctx, mock.AnythingOfType("*domain.License")).Return(nil).Once()

	license, err := s.issueLicense(ctx, order)
	assert.NoError(t, err)
	assert.Equal(t, orderID, license.OrderID)
	assert.Equal(t, userID, license.UserID)
	assert.Equal(t, specID, license.SpecID)
	assert.Equal(t, optID, license.LicenseOptionID)
	assert.True(t, strings.HasPrefix(license.LicenseKey, "LIC-"))
}

func TestPaymentService_GetLicenseDownloadsFallbackURL(t *testing.T) {
	ctx := context.Background()
	lr := new(licenseRepoMock)
	sr := new(specRepoMock)
	fs := new(fileSvcMock)
	s := &paymentService{licenseRepo: lr, specRepo: sr, fileService: fs}

	licenseID := uuid.New()
	userID := uuid.New()
	specID := uuid.New()
	preview := "preview-url"
	license := &domain.License{ID: licenseID, UserID: userID, SpecID: specID, IsActive: true, LicenseType: "Basic"}

	lr.On("GetByID", ctx, licenseID).Return(license, nil).Once()
	sr.On("GetByIDSystem", ctx, specID).Return(&domain.Spec{ID: specID, Title: "Track", PreviewUrl: preview}, nil).Once()
	fs.On("GetKeyFromUrl", preview).Return("preview.mp3", nil).Once()
	fs.On("GetPresignedURL", ctx, "preview.mp3", time.Hour).Return("", errors.New("presign failed")).Once()
	lr.On("IncrementDownloads", ctx, licenseID).Return(nil).Once()

	out, err := s.GetLicenseDownloads(ctx, licenseID, userID)
	assert.NoError(t, err)
	if assert.NotNil(t, out.MP3URL) {
		assert.Equal(t, preview, *out.MP3URL)
	}
}

func ptr(s string) *string { return &s }

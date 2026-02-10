package service_test

import (
	"context"
	"io"
	"mime/multipart"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/stretchr/testify/mock"
)

type mockSpecRepository struct {
	mock.Mock
}

func (m *mockSpecRepository) Create(ctx context.Context, spec *domain.Spec) error {
	args := m.Called(ctx, spec)
	return args.Error(0)
}

func (m *mockSpecRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Spec), args.Error(1)
}

func (m *mockSpecRepository) GetByIDSystem(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Spec), args.Error(1)
}

func (m *mockSpecRepository) List(ctx context.Context, filter domain.SpecFilter) ([]domain.Spec, int, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Spec), args.Int(1), args.Error(2)
}

func (m *mockSpecRepository) Update(ctx context.Context, spec *domain.Spec) error {
	args := m.Called(ctx, spec)
	return args.Error(0)
}

func (m *mockSpecRepository) Delete(ctx context.Context, id uuid.UUID, producerID uuid.UUID) error {
	args := m.Called(ctx, id, producerID)
	return args.Error(0)
}

func (m *mockSpecRepository) ListByUserID(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]domain.Spec, int, error) {
	args := m.Called(ctx, producerID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Spec), args.Int(1), args.Error(2)
}

type mockAnalyticsRepository struct {
	mock.Mock
}

func (m *mockAnalyticsRepository) GetSpecAnalytics(ctx context.Context, specID uuid.UUID) (*domain.SpecAnalytics, error) {
	args := m.Called(ctx, specID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.SpecAnalytics), args.Error(1)
}

func (m *mockAnalyticsRepository) IncrementPlayCount(ctx context.Context, specID uuid.UUID) error {
	args := m.Called(ctx, specID)
	return args.Error(0)
}

func (m *mockAnalyticsRepository) IncrementFreeDownloadCount(ctx context.Context, specID uuid.UUID) error {
	args := m.Called(ctx, specID)
	return args.Error(0)
}

func (m *mockAnalyticsRepository) AddFavorite(ctx context.Context, userID, specID uuid.UUID) error {
	args := m.Called(ctx, userID, specID)
	return args.Error(0)
}

func (m *mockAnalyticsRepository) RemoveFavorite(ctx context.Context, userID, specID uuid.UUID) error {
	args := m.Called(ctx, userID, specID)
	return args.Error(0)
}

func (m *mockAnalyticsRepository) IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	args := m.Called(ctx, userID, specID)
	return args.Bool(0), args.Error(1)
}

func (m *mockAnalyticsRepository) GetLicensePurchaseCounts(ctx context.Context, specID uuid.UUID) (map[string]int, error) {
	args := m.Called(ctx, specID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]int), args.Error(1)
}

func (m *mockAnalyticsRepository) GetTotalPlays(ctx context.Context, producerID uuid.UUID) (int, error) {
	args := m.Called(ctx, producerID)
	return args.Int(0), args.Error(1)
}

func (m *mockAnalyticsRepository) GetTotalFavorites(ctx context.Context, producerID uuid.UUID) (int, error) {
	args := m.Called(ctx, producerID)
	return args.Int(0), args.Error(1)
}

func (m *mockAnalyticsRepository) GetTotalDownloads(ctx context.Context, producerID uuid.UUID) (int, error) {
	args := m.Called(ctx, producerID)
	return args.Int(0), args.Error(1)
}

func (m *mockAnalyticsRepository) GetTotalRevenue(ctx context.Context, producerID uuid.UUID) (float64, error) {
	args := m.Called(ctx, producerID)
	return args.Get(0).(float64), args.Error(1)
}

func (m *mockAnalyticsRepository) GetRevenueByLicenseGlobal(ctx context.Context, producerID uuid.UUID) (map[string]float64, error) {
	args := m.Called(ctx, producerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string]float64), args.Error(1)
}

func (m *mockAnalyticsRepository) GetPlaysByDay(ctx context.Context, producerID uuid.UUID, days int) ([]domain.DailyStat, error) {
	args := m.Called(ctx, producerID, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.DailyStat), args.Error(1)
}

func (m *mockAnalyticsRepository) GetDownloadsByDay(ctx context.Context, producerID uuid.UUID, days int) ([]domain.DailyStat, error) {
	args := m.Called(ctx, producerID, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.DailyStat), args.Error(1)
}

func (m *mockAnalyticsRepository) GetRevenueByDay(ctx context.Context, producerID uuid.UUID, days int) ([]domain.DailyRevenueStat, error) {
	args := m.Called(ctx, producerID, days)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.DailyRevenueStat), args.Error(1)
}

func (m *mockAnalyticsRepository) GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int) ([]domain.TopSpecStat, error) {
	args := m.Called(ctx, producerID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.TopSpecStat), args.Error(1)
}

type mockOrderRepository struct {
	mock.Mock
}

func (m *mockOrderRepository) Create(ctx context.Context, order *domain.Order) error {
	args := m.Called(ctx, order)
	return args.Error(0)
}

func (m *mockOrderRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *mockOrderRepository) GetByRazorpayID(ctx context.Context, razorpayOrderID string) (*domain.Order, error) {
	args := m.Called(ctx, razorpayOrderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}

func (m *mockOrderRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error {
	args := m.Called(ctx, id, status)
	return args.Error(0)
}

func (m *mockOrderRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.Order, error) {
	args := m.Called(ctx, userID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Order), args.Error(1)
}

type mockPaymentRepository struct {
	mock.Mock
}

func (m *mockPaymentRepository) Create(ctx context.Context, payment *domain.Payment) error {
	args := m.Called(ctx, payment)
	return args.Error(0)
}

func (m *mockPaymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}

func (m *mockPaymentRepository) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}

func (m *mockPaymentRepository) GetByRazorpayID(ctx context.Context, razorpayPaymentID string) (*domain.Payment, error) {
	args := m.Called(ctx, razorpayPaymentID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Payment), args.Error(1)
}

type mockLicenseRepository struct {
	mock.Mock
}

func (m *mockLicenseRepository) Create(ctx context.Context, license *domain.License) error {
	args := m.Called(ctx, license)
	return args.Error(0)
}

func (m *mockLicenseRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.License, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.License), args.Error(1)
}

func (m *mockLicenseRepository) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.License, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.License), args.Error(1)
}

func (m *mockLicenseRepository) ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int, search, licenseType string) ([]domain.License, int, error) {
	args := m.Called(ctx, userID, limit, offset, search, licenseType)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.License), args.Int(1), args.Error(2)
}

func (m *mockLicenseRepository) IncrementDownloads(ctx context.Context, id uuid.UUID) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockLicenseRepository) Revoke(ctx context.Context, id uuid.UUID, reason string) error {
	args := m.Called(ctx, id, reason)
	return args.Error(0)
}

type mockFileService struct {
	mock.Mock
}

func (m *mockFileService) Upload(ctx context.Context, file multipart.File, header *multipart.FileHeader, folder string) (string, string, error) {
	args := m.Called(ctx, file, header, folder)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *mockFileService) UploadWithKey(ctx context.Context, file io.Reader, key string, contentType string) (string, error) {
	args := m.Called(ctx, file, key, contentType)
	return args.String(0), args.Error(1)
}

func (m *mockFileService) GetPresignedURL(ctx context.Context, key string, expiration time.Duration) (string, error) {
	args := m.Called(ctx, key, expiration)
	return args.String(0), args.Error(1)
}

func (m *mockFileService) GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error) {
	args := m.Called(ctx, key, filename, expiration)
	return args.String(0), args.Error(1)
}

func (m *mockFileService) Delete(ctx context.Context, key string) error {
	args := m.Called(ctx, key)
	return args.Error(0)
}

func (m *mockFileService) GetKeyFromUrl(fileURL string) (string, error) {
	args := m.Called(fileURL)
	return args.String(0), args.Error(1)
}

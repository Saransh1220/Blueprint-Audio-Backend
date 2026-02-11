package handler_test

import (
	"context"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/mock"
)

type mockUserService struct{ mock.Mock }

func (m *mockUserService) UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) error {
	args := m.Called(ctx, userID, req)
	return args.Error(0)
}
func (m *mockUserService) GetPublicProfile(ctx context.Context, userID uuid.UUID) (*dto.PublicUserResponse, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.PublicUserResponse), args.Error(1)
}

type mockPaymentService struct{ mock.Mock }

func (m *mockPaymentService) CreateOrder(ctx context.Context, userID, specID, licenseOptionID uuid.UUID) (*domain.Order, error) {
	args := m.Called(ctx, userID, specID, licenseOptionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}
func (m *mockPaymentService) GetOrder(ctx context.Context, orderID uuid.UUID) (*domain.Order, error) {
	args := m.Called(ctx, orderID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Order), args.Error(1)
}
func (m *mockPaymentService) VerifyPayment(ctx context.Context, orderID uuid.UUID, razorpayPaymentID, razorpaySignature string) (*domain.License, error) {
	args := m.Called(ctx, orderID, razorpayPaymentID, razorpaySignature)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.License), args.Error(1)
}
func (m *mockPaymentService) GetUserOrders(ctx context.Context, userID uuid.UUID, page int) ([]domain.Order, error) {
	args := m.Called(ctx, userID, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]domain.Order), args.Error(1)
}
func (m *mockPaymentService) GetUserLicenses(ctx context.Context, userID uuid.UUID, page int, search, licenseType string) ([]domain.License, int, error) {
	args := m.Called(ctx, userID, page, search, licenseType)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.License), args.Int(1), args.Error(2)
}
func (m *mockPaymentService) GetLicenseDownloads(ctx context.Context, licenseID, userID uuid.UUID) (*dto.LicenseDownloadsResponse, error) {
	args := m.Called(ctx, licenseID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.LicenseDownloadsResponse), args.Error(1)
}
func (m *mockPaymentService) GetProducerOrders(ctx context.Context, producerID uuid.UUID, page int) (*dto.ProducerOrderResponse, error) {
	args := m.Called(ctx, producerID, page)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.ProducerOrderResponse), args.Error(1)
}

type mockAnalyticsService struct{ mock.Mock }

func (m *mockAnalyticsService) TrackPlay(ctx context.Context, specID uuid.UUID) error {
	args := m.Called(ctx, specID)
	return args.Error(0)
}
func (m *mockAnalyticsService) TrackFreeDownload(ctx context.Context, specID uuid.UUID) error {
	args := m.Called(ctx, specID)
	return args.Error(0)
}
func (m *mockAnalyticsService) ToggleFavorite(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	args := m.Called(ctx, userID, specID)
	return args.Bool(0), args.Error(1)
}
func (m *mockAnalyticsService) IsFavorited(ctx context.Context, userID, specID uuid.UUID) (bool, error) {
	args := m.Called(ctx, userID, specID)
	return args.Bool(0), args.Error(1)
}
func (m *mockAnalyticsService) GetPublicAnalytics(ctx context.Context, specID uuid.UUID, userID *uuid.UUID) (*service.PublicAnalytics, error) {
	args := m.Called(ctx, specID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*service.PublicAnalytics), args.Error(1)
}
func (m *mockAnalyticsService) GetProducerAnalytics(ctx context.Context, specID, producerID uuid.UUID) (*service.ProducerAnalytics, error) {
	args := m.Called(ctx, specID, producerID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*service.ProducerAnalytics), args.Error(1)
}
func (m *mockAnalyticsService) GetStatsOverview(ctx context.Context, producerID uuid.UUID, days int, sortBy string) (*dto.AnalyticsOverviewResponse, error) {
	args := m.Called(ctx, producerID, days, sortBy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.AnalyticsOverviewResponse), args.Error(1)
}
func (m *mockAnalyticsService) GetTopSpecs(ctx context.Context, producerID uuid.UUID, limit int, sortBy string) ([]dto.TopSpecStat, error) {
	args := m.Called(ctx, producerID, limit, sortBy)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]dto.TopSpecStat), args.Error(1)
}

type mockSpecRepo struct{ mock.Mock }

func (m *mockSpecRepo) Create(ctx context.Context, spec *domain.Spec) error {
	args := m.Called(ctx, spec)
	return args.Error(0)
}
func (m *mockSpecRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Spec), args.Error(1)
}
func (m *mockSpecRepo) GetByIDSystem(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Spec), args.Error(1)
}
func (m *mockSpecRepo) List(ctx context.Context, filter domain.SpecFilter) ([]domain.Spec, int, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Spec), args.Int(1), args.Error(2)
}
func (m *mockSpecRepo) Update(ctx context.Context, spec *domain.Spec) error {
	args := m.Called(ctx, spec)
	return args.Error(0)
}
func (m *mockSpecRepo) Delete(ctx context.Context, id uuid.UUID, producerID uuid.UUID) error {
	args := m.Called(ctx, id, producerID)
	return args.Error(0)
}
func (m *mockSpecRepo) ListByUserID(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]domain.Spec, int, error) {
	args := m.Called(ctx, producerID, limit, offset)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Spec), args.Int(1), args.Error(2)
}

type mockSpecService struct{ mock.Mock }

func (m *mockSpecService) CreateSpec(ctx context.Context, spec *domain.Spec) error {
	args := m.Called(ctx, spec)
	return args.Error(0)
}
func (m *mockSpecService) GetSpec(ctx context.Context, id uuid.UUID) (*domain.Spec, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Spec), args.Error(1)
}
func (m *mockSpecService) ListSpecs(ctx context.Context, filter domain.SpecFilter) ([]domain.Spec, int, error) {
	args := m.Called(ctx, filter)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Spec), args.Int(1), args.Error(2)
}
func (m *mockSpecService) UpdateSpec(ctx context.Context, spec *domain.Spec, producerID uuid.UUID) error {
	args := m.Called(ctx, spec, producerID)
	return args.Error(0)
}
func (m *mockSpecService) DeleteSpec(ctx context.Context, id uuid.UUID, producerID uuid.UUID) error {
	args := m.Called(ctx, id, producerID)
	return args.Error(0)
}
func (m *mockSpecService) GetUserSpecs(ctx context.Context, producerID uuid.UUID, page int) ([]domain.Spec, int, error) {
	args := m.Called(ctx, producerID, page)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]domain.Spec), args.Int(1), args.Error(2)
}

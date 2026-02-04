package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/razorpay/razorpay-go"
	"github.com/saransh1220/blueprint-audio/internal/domain"
)

type PaymentService interface {
	CreateOrder(ctx context.Context, userID, specID, licenseOptionID uuid.UUID) (*domain.Order, error)
	GetOrder(ctx context.Context, orderID uuid.UUID) (*domain.Order, error)
	VerifyPayment(ctx context.Context, orderID uuid.UUID, razorpayPaymentID, razorpaySignature string) (*domain.License, error)
	GetUserOrders(ctx context.Context, userID uuid.UUID, page int) ([]domain.Order, error)
	GetUserLicenses(ctx context.Context, userID uuid.UUID, page int) ([]domain.License, error)
}

type paymentService struct {
	orderRepo      domain.OrderRepository
	paymentRepo    domain.PaymentRepository
	licenseRepo    domain.LicenseRepository
	specRepo       domain.SpecRepository
	razorpayClient *razorpay.Client
	razorpaySecret string
}

func NewPaymentService(
	orderRepo domain.OrderRepository,
	paymentRepo domain.PaymentRepository,
	licenseRepo domain.LicenseRepository,
	specRepo domain.SpecRepository,
) PaymentService {
	client := razorpay.NewClient(
		os.Getenv("RAZORPAY_KEY_ID"),
		os.Getenv("RAZORPAY_KEY_SECRET"),
	)
	return &paymentService{
		orderRepo:      orderRepo,
		paymentRepo:    paymentRepo,
		licenseRepo:    licenseRepo,
		specRepo:       specRepo,
		razorpayClient: client,
		razorpaySecret: os.Getenv("RAZORPAY_KEY_SECRET"),
	}
}

func (s *paymentService) CreateOrder(ctx context.Context, userID, specID, licenseOptionID uuid.UUID) (*domain.Order, error) {
	spec, err := s.specRepo.GetByID(ctx, specID)
	if err != nil {
		return nil, errors.New("Beat/Sample not found")
	}

	var licenseOption *domain.LicenseOption
	for _, lo := range spec.Licenses {
		if lo.ID == licenseOptionID {
			licenseOption = &lo
			break
		}
	}

	if licenseOption == nil {
		return nil, errors.New("license option not found")
	}

	amountInPaise := int(licenseOption.Price * 100)

	razorpayOrderData := map[string]interface{}{
		"amount":   amountInPaise,
		"currency": "INR",
		"receipt":  fmt.Sprintf("order_%s", uuid.New().String()[:8]),
	}

	razorpayOrder, err := s.razorpayClient.Order.Create(razorpayOrderData, nil)
	if err != nil {
		return nil, fmt.Errorf("razorpay order creation failed: %w", err)
	}

	razorpayOrderID := razorpayOrder["id"].(string)
	order := &domain.Order{
		UserID:          userID,
		SpecID:          specID,
		LicenseType:     string(licenseOption.LicenseType),
		Amount:          amountInPaise,
		Currency:        "INR",
		RazorpayOrderID: &razorpayOrderID,
		Status:          domain.OrderStatusPending,
		Notes: map[string]any{
			"license_option_id": licenseOptionID.String(),
			"spec_title":        spec.Title,
			"license_name":      licenseOption.Name,
		},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	if err := s.orderRepo.Create(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *paymentService) VerifyPayment(ctx context.Context, orderID uuid.UUID, razorpayPaymentID, razorpaySignature string) (*domain.License, error) {
	// 1. Get order
	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return nil, errors.New("order not found")
	}

	// 2. Validate order state
	if order.Status != domain.OrderStatusPending {
		return nil, errors.New("order already processed")
	}
	if time.Now().After(order.ExpiresAt) {
		s.orderRepo.UpdateStatus(ctx, orderID, domain.OrderStatusFailed)
		return nil, errors.New("order expired")
	}

	// 3. CRITICAL: Verify signature
	expectedSignature := s.generateSignature(*order.RazorpayOrderID, razorpayPaymentID)
	if expectedSignature != razorpaySignature {
		s.orderRepo.UpdateStatus(ctx, orderID, domain.OrderStatusFailed)
		return nil, errors.New("invalid signature")
	}

	// 4. Fetch payment details from Razorpay
	razorpayPayment, err := s.razorpayClient.Payment.Fetch(razorpayPaymentID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch payment: %w", err)
	}

	// 5. Save payment record
	now := time.Now()
	payment := &domain.Payment{
		OrderID:           orderID,
		RazorpayPaymentID: razorpayPaymentID,
		RazorpaySignature: razorpaySignature,
		Amount:            order.Amount,
		Currency:          order.Currency,
		Status:            domain.PaymentStatusCaptured,
		CapturedAt:        &now,
	}

	// Extract optional fields
	if method, ok := razorpayPayment["method"].(string); ok {
		payment.Method = &method
	}
	if email, ok := razorpayPayment["email"].(string); ok {
		payment.Email = &email
	}

	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		return nil, err
	}

	// 6. Update order status
	if err := s.orderRepo.UpdateStatus(ctx, orderID, domain.OrderStatusPaid); err != nil {
		return nil, err
	}

	// 7. Issue license
	license, err := s.issueLicense(ctx, order)
	if err != nil {
		return nil, fmt.Errorf("payment ok but license failed: %w", err)
	}

	return license, nil
}

func (s *paymentService) GetOrder(ctx context.Context, orderID uuid.UUID) (*domain.Order, error) {
	return s.orderRepo.GetByID(ctx, orderID)
}

func (s *paymentService) GetUserOrders(ctx context.Context, userID uuid.UUID, page int) ([]domain.Order, error) {
	limit := 20
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	return s.orderRepo.ListByUser(ctx, userID, limit, offset)
}

func (s *paymentService) GetUserLicenses(ctx context.Context, userID uuid.UUID, page int) ([]domain.License, error) {
	limit := 20
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	return s.licenseRepo.ListByUser(ctx, userID, limit, offset)
}

// generateSignature - HMAC SHA256 for Razorpay verification
func (s *paymentService) generateSignature(orderID, paymentID string) string {
	message := orderID + "|" + paymentID
	h := hmac.New(sha256.New, []byte(s.razorpaySecret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

// issueLicense creates license after successful payment
func (s *paymentService) issueLicense(ctx context.Context, order *domain.Order) (*domain.License, error) {
	licenseOptionIDStr, ok := order.Notes["license_option_id"].(string)
	if !ok {
		return nil, errors.New("license_option_id missing")
	}
	licenseOptionID, _ := uuid.Parse(licenseOptionIDStr)

	license := &domain.License{
		OrderID:         order.ID,
		UserID:          order.UserID,
		SpecID:          order.SpecID,
		LicenseOptionID: licenseOptionID,
		LicenseType:     order.LicenseType,
		PurchasePrice:   order.Amount,
		LicenseKey:      fmt.Sprintf("LIC-%s", uuid.New().String()),
		IsActive:        true,
		IsRevoked:       false,
		DownloadsCount:  0,
	}

	return license, s.licenseRepo.Create(ctx, license)
}

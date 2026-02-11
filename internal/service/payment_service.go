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
	"github.com/saransh1220/blueprint-audio/internal/dto"
)

type PaymentService interface {
	CreateOrder(ctx context.Context, userID, specID, licenseOptionID uuid.UUID) (*domain.Order, error)
	GetOrder(ctx context.Context, orderID uuid.UUID) (*domain.Order, error)
	VerifyPayment(ctx context.Context, orderID uuid.UUID, razorpayPaymentID, razorpaySignature string) (*domain.License, error)
	GetUserOrders(ctx context.Context, userID uuid.UUID, page int) ([]domain.Order, error)
	GetUserLicenses(ctx context.Context, userID uuid.UUID, page int, search, licenseType string) ([]domain.License, int, error)
	GetLicenseDownloads(ctx context.Context, licenseID, userID uuid.UUID) (*dto.LicenseDownloadsResponse, error)
	GetProducerOrders(ctx context.Context, producerID uuid.UUID, page int) (*dto.ProducerOrderResponse, error)
}

type paymentService struct {
	orderRepo      domain.OrderRepository
	paymentRepo    domain.PaymentRepository
	licenseRepo    domain.LicenseRepository
	specRepo       domain.SpecRepository
	fileService    FileService
	razorpayClient *razorpay.Client
	razorpaySecret string
}

func NewPaymentService(
	orderRepo domain.OrderRepository,
	paymentRepo domain.PaymentRepository,
	licenseRepo domain.LicenseRepository,
	specRepo domain.SpecRepository,
	fileService FileService,
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
		fileService:    fileService,
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

func (s *paymentService) GetUserLicenses(ctx context.Context, userID uuid.UUID, page int, search, licenseType string) ([]domain.License, int, error) {
	limit := 5 // Per user request for testing
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	licenses, total, err := s.licenseRepo.ListByUser(ctx, userID, limit, offset, search, licenseType)
	if err != nil {
		return nil, 0, err
	}

	// Sign SpecImage URLs
	for i := range licenses {
		if licenses[i].SpecImage != nil && *licenses[i].SpecImage != "" {
			key, err := s.fileService.GetKeyFromUrl(*licenses[i].SpecImage)
			if err == nil {
				signedURL, err := s.fileService.GetPresignedURL(ctx, key, 1*time.Hour)
				if err == nil {
					licenses[i].SpecImage = &signedURL
				}
			}
		}
	}

	return licenses, total, nil
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

// GetLicenseDownloads generates download URLs for a purchased license
func (s *paymentService) GetLicenseDownloads(ctx context.Context, licenseID, userID uuid.UUID) (*dto.LicenseDownloadsResponse, error) {
	// 1. Fetch the license
	license, err := s.licenseRepo.GetByID(ctx, licenseID)
	if err != nil {
		return nil, errors.New("license not found")
	}

	// 2. SECURITY: Verify ownership
	if license.UserID != userID {
		return nil, errors.New("unauthorized: you do not own this license")
	}

	// 3. Check if license is active
	if !license.IsActive {
		return nil, errors.New("license is not active")
	}
	if license.IsRevoked {
		return nil, errors.New("license has been revoked")
	}

	// 4. Fetch the spec to get file URLs
	spec, err := s.specRepo.GetByIDSystem(ctx, license.SpecID)
	if err != nil {
		return nil, errors.New("spec not found")
	}

	// 5. Build response
	response := &dto.LicenseDownloadsResponse{
		LicenseID:   license.ID.String(),
		LicenseType: license.LicenseType,
		SpecTitle:   spec.Title,
		ExpiresIn:   3600, // 1 hour
	}

	// Helper to get presigned URL from stored URL
	getSignedURL := func(fileURL string) *string {
		if fileURL == "" {
			return nil
		}
		key, err := s.fileService.GetKeyFromUrl(fileURL)
		if err != nil {
			return &fileURL // Fallback
		}
		signedURL, err := s.fileService.GetPresignedURL(ctx, key, 1*time.Hour)
		if err != nil {
			return &fileURL
		}
		return &signedURL
	}

	// 6. Generate URLs based on license type
	switch license.LicenseType {
	case "Basic":
		if spec.PreviewUrl != "" {
			response.MP3URL = getSignedURL(spec.PreviewUrl)
		}

	case "Premium":
		if spec.PreviewUrl != "" {
			response.MP3URL = getSignedURL(spec.PreviewUrl)
		}
		if spec.WavUrl != nil && *spec.WavUrl != "" {
			response.WAVURL = getSignedURL(*spec.WavUrl)
		}

	case "Trackout", "Unlimited":
		if spec.PreviewUrl != "" {
			response.MP3URL = getSignedURL(spec.PreviewUrl)
		}
		if spec.WavUrl != nil && *spec.WavUrl != "" {
			response.WAVURL = getSignedURL(*spec.WavUrl)
		}
		if spec.StemsUrl != nil && *spec.StemsUrl != "" {
			response.StemsURL = getSignedURL(*spec.StemsUrl)
		}
	}

	// 7. Track download analytics
	_ = s.licenseRepo.IncrementDownloads(ctx, licenseID)

	return response, nil
}

func (s *paymentService) GetProducerOrders(ctx context.Context, producerID uuid.UUID, page int) (*dto.ProducerOrderResponse, error) {
	limit := 50
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	orders, total, err := s.orderRepo.ListByProducer(ctx, producerID, limit, offset)
	if err != nil {
		return nil, err
	}

	orderDtos := make([]dto.ProducerOrderDto, len(orders))
	for i, o := range orders {
		orderDtos[i] = dto.ProducerOrderDto{
			ID:              o.ID,
			Amount:          float64(o.Amount) / 100.0, // Convert paise to rupees
			Currency:        o.Currency,
			Status:          o.Status,
			CreatedAt:       o.CreatedAt,
			LicenseType:     o.LicenseType,
			BuyerName:       o.BuyerName,
			BuyerEmail:      o.BuyerEmail,
			SpecTitle:       o.SpecTitle,
			RazorpayOrderID: o.RazorpayOrderID,
		}
	}

	return &dto.ProducerOrderResponse{
		Orders: orderDtos,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

package application

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/razorpay/razorpay-go"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	sharedemail "github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/email"
)

func formatRazorpayReceipt(id uuid.UUID) string {
	return "order_" + strings.ReplaceAll(id.String(), "-", "")
}

type FileService interface {
	GetKeyFromUrl(url string) (string, error)
	GetPresignedURL(ctx context.Context, key string, expiresIn time.Duration) (string, error)
}

type PaymentService interface {
	CreateOrder(ctx context.Context, userID, specID, licenseOptionID uuid.UUID) (*domain.Order, error)
	GetOrder(ctx context.Context, orderID uuid.UUID) (*domain.Order, error)
	VerifyPayment(ctx context.Context, orderID uuid.UUID, razorpayPaymentID, razorpaySignature string) (*domain.License, error)
	GetUserOrders(ctx context.Context, userID uuid.UUID, page int) ([]domain.Order, error)
	GetUserLicenses(ctx context.Context, userID uuid.UUID, page int, search, licenseType string) ([]domain.License, int, error)
	GetLicenseDownloads(ctx context.Context, licenseID, userID uuid.UUID) (*LicenseDownloadsResponse, error)
	GetProducerOrders(ctx context.Context, producerID uuid.UUID, page, limit int) (*ProducerOrderResponse, error)
}

type paymentService struct {
	orderRepo      domain.OrderRepository
	paymentRepo    domain.PaymentRepository
	licenseRepo    domain.LicenseRepository
	specFinder     catalogDomain.SpecFinder
	userFinder     authDomain.UserFinder
	fileService    FileService
	razorpayClient *razorpay.Client
	razorpaySecret string
	emailSender    sharedemail.Sender
	appBaseURL     string
}

func NewPaymentService(
	orderRepo domain.OrderRepository,
	paymentRepo domain.PaymentRepository,
	licenseRepo domain.LicenseRepository,
	specFinder catalogDomain.SpecFinder,
	userFinder authDomain.UserFinder,
	fileService FileService,
	emailSender sharedemail.Sender,
	appBaseURL string,
) PaymentService {
	client := razorpay.NewClient(
		os.Getenv("RAZORPAY_KEY_ID"),
		os.Getenv("RAZORPAY_KEY_SECRET"),
	)
	return &paymentService{
		orderRepo:      orderRepo,
		paymentRepo:    paymentRepo,
		licenseRepo:    licenseRepo,
		specFinder:     specFinder,
		userFinder:     userFinder,
		fileService:    fileService,
		razorpayClient: client,
		razorpaySecret: os.Getenv("RAZORPAY_KEY_SECRET"),
		emailSender:    emailSender,
		appBaseURL:     appBaseURL,
	}
}

func (s *paymentService) CreateOrder(ctx context.Context, userID, specID, licenseOptionID uuid.UUID) (*domain.Order, error) {
	spec, err := s.specFinder.FindWithLicenses(ctx, specID)
	if err != nil {
		return nil, errors.New("Beat/Sample not found")
	}

	var licenseOption *catalogDomain.LicenseOption
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

	receiptID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate uuid: %w", err)
	}

	razorpayOrderData := map[string]interface{}{
		"amount":   amountInPaise,
		"currency": "INR",
		"receipt":  formatRazorpayReceipt(receiptID),
	}

	razorpayOrder, err := s.razorpayClient.Order.Create(razorpayOrderData, nil)
	if err != nil {
		return nil, fmt.Errorf("razorpay order creation failed: %w", err)
	}

	razorpayOrderID, ok := razorpayOrder["id"].(string)
	if !ok || razorpayOrderID == "" {
		return nil, errors.New("invalid razorpay order response")
	}
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
	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return nil, errors.New("order not found")
	}

	if order.Status != domain.OrderStatusPending {
		return nil, errors.New("order already processed")
	}
	if time.Now().After(order.ExpiresAt) {
		if updateErr := s.orderRepo.UpdateStatus(ctx, orderID, domain.OrderStatusFailed); updateErr != nil {
			return nil, fmt.Errorf("order expired and status update failed: %w", updateErr)
		}
		return nil, errors.New("order expired")
	}

	if order.RazorpayOrderID == nil || *order.RazorpayOrderID == "" {
		return nil, errors.New("invalid order state")
	}
	expectedSignature := s.generateSignature(*order.RazorpayOrderID, razorpayPaymentID)
	if expectedSignature != razorpaySignature {
		if updateErr := s.orderRepo.UpdateStatus(ctx, orderID, domain.OrderStatusFailed); updateErr != nil {
			return nil, fmt.Errorf("invalid signature and status update failed: %w", updateErr)
		}
		return nil, errors.New("invalid signature")
	}

	razorpayPayment, err := s.razorpayClient.Payment.Fetch(razorpayPaymentID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch payment: %w", err)
	}
	paymentStatus, _ := razorpayPayment["status"].(string)
	if strings.ToLower(paymentStatus) != "captured" {
		if updateErr := s.orderRepo.UpdateStatus(ctx, orderID, domain.OrderStatusFailed); updateErr != nil {
			return nil, fmt.Errorf("payment not captured and status update failed: %w", updateErr)
		}
		return nil, errors.New("payment not captured")
	}

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

	if method, ok := razorpayPayment["method"].(string); ok {
		payment.Method = &method
	}
	if email, ok := razorpayPayment["email"].(string); ok {
		payment.Email = &email
	}

	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		return nil, err
	}

	if err := s.orderRepo.UpdateStatus(ctx, orderID, domain.OrderStatusPaid); err != nil {
		return nil, err
	}

	license, err := s.issueLicense(ctx, order)
	if err != nil {
		return nil, fmt.Errorf("payment ok but license failed: %w", err)
	}

	if err := s.sendReceiptEmail(ctx, order, payment, license); err != nil {
		log.Printf("PaymentService.VerifyPayment receipt email failed. order_id=%s err=%v", order.ID, err)
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
	limit := 5
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	licenses, total, err := s.licenseRepo.ListByUser(ctx, userID, limit, offset, search, licenseType)
	if err != nil {
		return nil, 0, err
	}

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

func (s *paymentService) generateSignature(orderID, paymentID string) string {
	message := orderID + "|" + paymentID
	h := hmac.New(sha256.New, []byte(s.razorpaySecret))
	h.Write([]byte(message))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *paymentService) issueLicense(ctx context.Context, order *domain.Order) (*domain.License, error) {
	licenseOptionIDStr, ok := order.Notes["license_option_id"].(string)
	if !ok {
		return nil, errors.New("license_option_id missing")
	}
	licenseOptionID, err := uuid.Parse(licenseOptionIDStr)
	if err != nil {
		return nil, errors.New("invalid license_option_id")
	}

	licenseKeyID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate uuid: %w", err)
	}

	license := &domain.License{
		OrderID:         order.ID,
		UserID:          order.UserID,
		SpecID:          order.SpecID,
		LicenseOptionID: licenseOptionID,
		LicenseType:     order.LicenseType,
		PurchasePrice:   order.Amount,
		LicenseKey:      fmt.Sprintf("LIC-%s", licenseKeyID.String()),
		IsActive:        true,
		IsRevoked:       false,
		DownloadsCount:  0,
		IssuedAt:        time.Now(),
	}

	return license, s.licenseRepo.Create(ctx, license)
}

func (s *paymentService) GetLicenseDownloads(ctx context.Context, licenseID, userID uuid.UUID) (*LicenseDownloadsResponse, error) {
	license, err := s.licenseRepo.GetByID(ctx, licenseID)
	if err != nil {
		return nil, errors.New("license not found")
	}

	if license.UserID != userID {
		return nil, errors.New("unauthorized: you do not own this license")
	}
	if !license.IsActive {
		return nil, errors.New("license is not active")
	}
	if license.IsRevoked {
		return nil, errors.New("license has been revoked")
	}

	spec, err := s.specFinder.FindByIDIncludingDeleted(ctx, license.SpecID)
	if err != nil {
		return nil, errors.New("spec not found")
	}

	response := &LicenseDownloadsResponse{
		LicenseID:   license.ID.String(),
		LicenseType: license.LicenseType,
		SpecTitle:   spec.Title,
		ExpiresIn:   3600,
	}

	getSignedURL := func(fileURL string) *string {
		if fileURL == "" {
			return nil
		}
		key, err := s.fileService.GetKeyFromUrl(fileURL)
		if err != nil {
			return &fileURL
		}
		signedURL, err := s.fileService.GetPresignedURL(ctx, key, 1*time.Hour)
		if err != nil {
			return &fileURL
		}
		return &signedURL
	}

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

	_ = s.licenseRepo.IncrementDownloads(ctx, licenseID)
	return response, nil
}

func (s *paymentService) GetProducerOrders(ctx context.Context, producerID uuid.UUID, page, limit int) (*ProducerOrderResponse, error) {
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}

	orders, total, err := s.orderRepo.ListByProducer(ctx, producerID, limit, offset)
	if err != nil {
		return nil, err
	}

	orderDtos := make([]ProducerOrderDto, len(orders))
	for i, o := range orders {
		orderDtos[i] = ProducerOrderDto{
			ID:              o.ID,
			Amount:          float64(o.Amount) / 100.0,
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

	return &ProducerOrderResponse{
		Orders: orderDtos,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (s *paymentService) sendReceiptEmail(ctx context.Context, order *domain.Order, payment *domain.Payment, license *domain.License) error {
	if s.emailSender == nil || s.userFinder == nil {
		return nil
	}
	user, err := s.userFinder.FindByID(ctx, order.UserID)
	if err != nil {
		return err
	}

	buyerEmail := user.Email
	if payment.Email != nil && strings.TrimSpace(*payment.Email) != "" {
		buyerEmail = *payment.Email
	}
	specTitle, _ := order.Notes["spec_title"].(string)
	if specTitle == "" {
		specTitle = "Blueprint purchase"
	}

	return s.emailSender.Send(ctx, sharedemail.BuildPaymentReceiptEmail(sharedemail.ReceiptData{
		BuyerName:     user.Name,
		BuyerEmail:    buyerEmail,
		SpecTitle:     specTitle,
		LicenseType:   order.LicenseType,
		AmountDisplay: formatMoney(order.Amount, order.Currency),
		OrderID:       order.ID.String(),
		PaymentID:     payment.RazorpayPaymentID,
		LicenseID:     license.ID.String(),
	}, s.appBaseURL))
}

func formatMoney(amount int, currency string) string {
	switch strings.ToUpper(currency) {
	case "INR":
		return fmt.Sprintf("INR %.2f", float64(amount)/100.0)
	default:
		return fmt.Sprintf("%s %.2f", strings.ToUpper(currency), float64(amount)/100.0)
	}
}

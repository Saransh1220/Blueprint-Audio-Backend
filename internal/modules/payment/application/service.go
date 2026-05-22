package application

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/razorpay/razorpay-go"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	sharedemail "github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/email"
	sharedmoney "github.com/saransh1220/blueprint-audio/internal/shared/money"
)

func formatRazorpayReceipt(id uuid.UUID) string {
	return "order_" + strings.ReplaceAll(id.String(), "-", "")
}

type FileService interface {
	GetKeyFromUrl(url string) (string, error)
	GetPresignedURL(ctx context.Context, key string, expiresIn time.Duration) (string, error)
}

type PaymentService interface {
	CreateOrder(ctx context.Context, userID, specID, licenseOptionID uuid.UUID, currency string) (*domain.Order, error)
	GetOrder(ctx context.Context, orderID uuid.UUID) (*domain.Order, error)
	VerifyPayment(ctx context.Context, orderID uuid.UUID, razorpayPaymentID, razorpaySignature string) (*domain.License, error)
	HandleDodoWebhook(ctx context.Context, payload []byte, headers map[string]string) error
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
	dodoConfig     DodoConfig
}

type DodoConfig struct {
	APIKey     string
	ProductID  string
	WebhookKey string
	APIURL     string
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
	dodoConfig DodoConfig,
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
		dodoConfig:     dodoConfig,
	}
}

func (s *paymentService) CreateOrder(ctx context.Context, userID, specID, licenseOptionID uuid.UUID, currency string) (*domain.Order, error) {
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

	requestedCurrency := strings.ToUpper(strings.TrimSpace(currency))
	if requestedCurrency != sharedmoney.CurrencyUSD {
		requestedCurrency = sharedmoney.CurrencyINR
	}

	// Resolve the stored currency for this license — fall back to INR for legacy records
	storedCurrency := strings.ToUpper(strings.TrimSpace(licenseOption.PriceCurrency))
	if storedCurrency != sharedmoney.CurrencyINR && storedCurrency != sharedmoney.CurrencyUSD {
		storedCurrency = sharedmoney.CurrencyINR
	}

	displayMoney := sharedmoney.DisplayPrice(licenseOption.Price, storedCurrency, requestedCurrency)

	receiptID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate uuid: %w", err)
	}

	order := &domain.Order{
		ID:          receiptID,
		UserID:      userID,
		SpecID:      specID,
		LicenseType: string(licenseOption.LicenseType),
		Amount:      displayMoney.AmountMinor,
		Currency:    requestedCurrency,
		Status:      domain.OrderStatusPending,
		Notes: map[string]any{
			"license_option_id": licenseOptionID.String(),
			"spec_title":        spec.Title,
			"license_name":      licenseOption.Name,
			"display_currency":  requestedCurrency,
		},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	if requestedCurrency == sharedmoney.CurrencyUSD {
		order.Provider = "dodo"
		checkoutURL, sessionID, err := s.createDodoCheckout(ctx, order, spec.Title, licenseOption.Name)
		if err != nil {
			return nil, err
		}
		order.CheckoutURL = &checkoutURL
		order.ProviderCheckoutID = &sessionID
		order.Notes["dodo_session_id"] = sessionID
	} else {
		order.Provider = "razorpay"
		razorpayOrderData := map[string]interface{}{
			"amount":   displayMoney.AmountMinor,
			"currency": sharedmoney.CurrencyINR,
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
		order.RazorpayOrderID = &razorpayOrderID
	}
	if err := s.orderRepo.Create(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *paymentService) createDodoCheckout(ctx context.Context, order *domain.Order, specTitle, licenseName string) (string, string, error) {
	apiKey := strings.TrimSpace(s.dodoConfig.APIKey)
	productID := strings.TrimSpace(s.dodoConfig.ProductID)
	if apiKey == "" || productID == "" {
		return "", "", errors.New("dodo payments is not configured")
	}
	baseURL := strings.TrimRight(s.dodoConfig.APIURL, "/")
	if baseURL == "" {
		baseURL = "https://test.dodopayments.com"
	}

	body := map[string]any{
		"product_cart": []map[string]any{{
			"product_id": productID,
			"quantity":   1,
			"amount":     order.Amount,
		}},
		"return_url": s.appBaseURL + "/dashboard",
		"metadata": map[string]any{
			"order_id":     order.ID.String(),
			"spec_id":      order.SpecID.String(),
			"license_type": order.LicenseType,
			"spec_title":   specTitle,
			"license_name": licenseName,
		},
	}
	body["custom_data"] = body["metadata"]

	payload, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/checkouts", bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("dodo checkout creation failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return "", "", fmt.Errorf("dodo checkout creation failed with status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded struct {
		SessionID   string  `json:"session_id"`
		CheckoutURL *string `json:"checkout_url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&decoded); err != nil {
		return "", "", err
	}
	if decoded.SessionID == "" || decoded.CheckoutURL == nil || *decoded.CheckoutURL == "" {
		return "", "", errors.New("invalid dodo checkout response")
	}
	return *decoded.CheckoutURL, decoded.SessionID, nil
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

	go func() {
		emailCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.sendReceiptEmail(emailCtx, order, payment, license); err != nil {
			log.Printf("PaymentService.VerifyPayment receipt email failed. order_id=%s err=%v", order.ID, err)
		}
	}()

	return license, nil
}

func (s *paymentService) HandleDodoWebhook(ctx context.Context, payload []byte, headers map[string]string) error {
	if err := s.verifyDodoSignature(payload, headers); err != nil {
		return err
	}

	var event struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}
	if event.Type != "payment.succeeded" {
		return nil
	}

	orderID, paymentID := extractDodoOrderMetadata(event.Data)
	if orderID == uuid.Nil {
		return errors.New("dodo webhook missing order_id")
	}
	if paymentID == "" {
		paymentID = headers["webhook-id"]
	}

	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return errors.New("order not found")
	}
	if order.Status == domain.OrderStatusPaid {
		return nil
	}
	if order.Status != domain.OrderStatusPending {
		return errors.New("order is not payable")
	}

	payment := &domain.Payment{
		OrderID:           order.ID,
		RazorpayPaymentID: paymentID,
		RazorpaySignature: headers["webhook-signature"],
		Amount:            order.Amount,
		Currency:          order.Currency,
		Status:            domain.PaymentStatusCaptured,
	}
	now := time.Now()
	payment.CapturedAt = &now
	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		return err
	}
	if err := s.orderRepo.UpdateStatus(ctx, order.ID, domain.OrderStatusPaid); err != nil {
		return err
	}
	_, err = s.issueLicense(ctx, order)
	return err
}

func (s *paymentService) verifyDodoSignature(payload []byte, headers map[string]string) error {
	secret := strings.TrimSpace(s.dodoConfig.WebhookKey)
	if secret == "" {
		return errors.New("dodo webhook secret is not configured")
	}
	if strings.HasPrefix(strings.ToLower(secret), "http://") || strings.HasPrefix(strings.ToLower(secret), "https://") {
		return errors.New("DODO_PAYMENTS_WEBHOOK_KEY must be the Dodo webhook signing secret, not the endpoint URL")
	}
	webhookID := headers["webhook-id"]
	timestamp := headers["webhook-timestamp"]
	signature := headers["webhook-signature"]
	if webhookID == "" || timestamp == "" || signature == "" {
		return errors.New("missing dodo webhook signature headers")
	}

	message := webhookID + "." + timestamp + "." + string(payload)
	secretKeys := [][]byte{[]byte(secret)}
	if encoded, ok := strings.CutPrefix(secret, "whsec_"); ok {
		secretKeys = append(secretKeys, []byte(encoded))
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(encoded)
		}
		if err == nil {
			secretKeys = append(secretKeys, decoded)
		}
	}

	for _, secretKey := range secretKeys {
		mac := hmac.New(sha256.New, secretKey)
		mac.Write([]byte(message))
		sum := mac.Sum(nil)
		if subtleCompareSignature(signature, base64.StdEncoding.EncodeToString(sum), hex.EncodeToString(sum)) {
			return nil
		}
	}
	return errors.New("invalid dodo webhook signature")
}

func subtleCompareSignature(received string, expected ...string) bool {
	parts := strings.FieldsFunc(received, func(r rune) bool {
		return r == ' ' || r == ','
	})
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimPrefix(part, "v1="))
		if strings.EqualFold(part, "v1") || part == "" {
			continue
		}
		for _, candidate := range expected {
			if hmac.Equal([]byte(part), []byte(candidate)) {
				return true
			}
		}
	}
	for _, candidate := range expected {
		if hmac.Equal([]byte(strings.TrimSpace(received)), []byte(candidate)) {
			return true
		}
	}
	return false
}

func extractDodoOrderMetadata(data map[string]any) (uuid.UUID, string) {
	paymentID := stringFromAny(data["payment_id"])
	if paymentID == "" {
		paymentID = stringFromAny(data["id"])
	}

	candidates := []any{data["metadata"], data["custom_data"], data["custom_fields"]}
	for _, candidate := range candidates {
		if meta, ok := candidate.(map[string]any); ok {
			if id, err := uuid.Parse(stringFromAny(meta["order_id"])); err == nil {
				return id, paymentID
			}
		}
	}
	if id, err := uuid.Parse(stringFromAny(data["order_id"])); err == nil {
		return id, paymentID
	}
	return uuid.Nil, paymentID
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", value)
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
		Currency:        order.Currency,
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

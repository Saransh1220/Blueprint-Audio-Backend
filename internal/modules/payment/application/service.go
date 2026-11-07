package application

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	sharedemail "github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/email"
	sharedmoney "github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type FileService interface {
	GetKeyFromUrl(url string) (string, error)
	GetPresignedURL(ctx context.Context, key string, expiresIn time.Duration) (string, error)
}

type PaymentService interface {
	CreateOrder(ctx context.Context, userID, specID, licenseOptionID uuid.UUID, currency string) (*domain.Order, error)
	CreateOrderWithOffer(ctx context.Context, input CreateOrderInput) (*domain.Order, error)
	GetOrder(ctx context.Context, orderID, requesterID uuid.UUID) (*domain.Order, error)
	// VerifyPayment confirms a payment the browser reported back, for gateways
	// that work that way (Razorpay).
	VerifyPayment(ctx context.Context, orderID, userID uuid.UUID, providerPaymentID, signature string) (*domain.License, error)
	// HandleWebhook confirms a payment the gateway reported to us, for gateways
	// that work that way (Dodo).
	HandleWebhook(ctx context.Context, provider string, payload []byte, headers map[string]string) error
	GetUserOrders(ctx context.Context, userID uuid.UUID, page int) ([]domain.Order, error)
	GetUserLicenses(ctx context.Context, userID uuid.UUID, page int, search, licenseType string) ([]domain.License, int, error)
	GetLicenseDownloads(ctx context.Context, licenseID, userID uuid.UUID) (*LicenseDownloadsResponse, error)
	GetProducerOrders(ctx context.Context, producerID uuid.UUID, page, limit int) (*ProducerOrderResponse, error)

}

type paymentService struct {
	orderRepo   domain.OrderRepository
	paymentRepo domain.PaymentRepository
	licenseRepo domain.LicenseRepository
	specFinder  catalogDomain.SpecFinder
	userFinder  authDomain.UserFinder
	fileService FileService
	emailSender sharedemail.Sender
	appBaseURL  string
	gateways    GatewayResolver
	snapshotter domain.ReportingRateSnapshotter
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
	gateways GatewayResolver,
) PaymentService {
	return &paymentService{
		orderRepo:   orderRepo,
		paymentRepo: paymentRepo,
		licenseRepo: licenseRepo,
		specFinder:  specFinder,
		userFinder:  userFinder,
		fileService: fileService,
		emailSender: emailSender,
		appBaseURL:  appBaseURL,
		gateways:    gateways,
	}
}

func (s *paymentService) SetReportingRateSnapshotter(snapshotter domain.ReportingRateSnapshotter) {
	s.snapshotter = snapshotter
}

func (s *paymentService) CreateOrder(ctx context.Context, userID, specID, licenseOptionID uuid.UUID, currency string) (*domain.Order, error) {
	spec, err := s.specFinder.FindWithLicenses(ctx, specID)
	if err != nil {
		return nil, errors.New("Beat/Sample not found")
	}
	if spec.ProcessingStatus != "" && spec.ProcessingStatus != catalogDomain.ProcessingStatusCompleted {
		return nil, errors.New("Beat/Sample is not ready for purchase")
	}
	var lo *catalogDomain.LicenseOption
	for _, l := range spec.Licenses {
		if l.ID == licenseOptionID {
			lo = &l
			break
		}
	}
	if lo == nil {
		return nil, errors.New("license option not found")
	}
	offer := sharedmoney.SelectPriceOffer(lo.Prices, lo.PriceCurrency, currency)
	if !offer.IsPurchasable || offer.PriceID == nil {
		return nil, sharedmoney.ErrPriceUnavailable
	}
	return s.createOrderFromSpec(ctx, spec, lo, offer, CreateOrderInput{
		UserID:              userID,
		SpecID:              specID,
		LicenseOptionID:     licenseOptionID,
		Currency:            currency,
		ExpectedPriceID:     offer.PriceID,
		ExpectedRevision:    &offer.Revision,
		ExpectedAmountMinor: &offer.AmountMinor,
		ExpectedCurrency:    &offer.Currency,
	})
}

func (s *paymentService) CreateOrderWithOffer(ctx context.Context, input CreateOrderInput) (*domain.Order, error) {
	spec, err := s.specFinder.FindWithLicenses(ctx, input.SpecID)
	if err != nil {
		return nil, errors.New("Beat/Sample not found")
	}
	if spec.ProcessingStatus != "" && spec.ProcessingStatus != catalogDomain.ProcessingStatusCompleted {
		return nil, errors.New("Beat/Sample is not ready for purchase")
	}

	var licenseOption *catalogDomain.LicenseOption
	for _, lo := range spec.Licenses {
		if lo.ID == input.LicenseOptionID {
			licenseOption = &lo
			break
		}
	}

	if licenseOption == nil {
		return nil, errors.New("license option not found")
	}

	requestedCurrency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if requestedCurrency == "" {
		requestedCurrency = sharedmoney.CurrencyINR
	}

	// Validate currency
	if _, ok := sharedmoney.LookupCurrency(requestedCurrency); !ok {
		return nil, sharedmoney.ErrInvalidCurrency
	}

	// Exact stored price selection
	offer := sharedmoney.SelectPriceOffer(licenseOption.Prices, licenseOption.PriceCurrency, requestedCurrency)
	if !offer.IsPurchasable || offer.Selection == sharedmoney.SelectionUnavailable {
		return nil, sharedmoney.ErrPriceUnavailable
	}

	// Verify expected evidence is completely provided
	if input.ExpectedPriceID == nil || input.ExpectedRevision == nil || input.ExpectedAmountMinor == nil || input.ExpectedCurrency == nil {
		return nil, &PriceMismatchError{
			Message:      "missing required price evidence: expected_price_id, expected_revision, expected_amount_minor, and expected_currency are required",
			CurrentOffer: offer,
		}
	}

	// Verify currency matches current offer (guards against unconfirmed fallback)
	expectedCurr := strings.ToUpper(strings.TrimSpace(*input.ExpectedCurrency))
	if expectedCurr != offer.Currency {
		return nil, &PriceMismatchError{
			Message:      fmt.Sprintf("currency mismatch or unconfirmed fallback: expected %s, current offer is %s", expectedCurr, offer.Currency),
			CurrentOffer: offer,
		}
	}

	if offer.PriceID == nil || *input.ExpectedPriceID != *offer.PriceID {
		return nil, &PriceMismatchError{
			Message:      fmt.Sprintf("price changed: expected %s, current %v", input.ExpectedPriceID, offer.PriceID),
			CurrentOffer: offer,
		}
	}
	if *input.ExpectedRevision != offer.Revision {
		return nil, &PriceMismatchError{
			Message:      fmt.Sprintf("price revision changed: expected %d, current %d", *input.ExpectedRevision, offer.Revision),
			CurrentOffer: offer,
		}
	}
	if *input.ExpectedAmountMinor != offer.AmountMinor {
		return nil, &PriceMismatchError{
			Message:      fmt.Sprintf("price amount changed: expected %d, current %d", *input.ExpectedAmountMinor, offer.AmountMinor),
			CurrentOffer: offer,
		}
	}

	return s.createOrderFromSpec(ctx, spec, licenseOption, offer, input)
}

func (s *paymentService) createOrderFromSpec(ctx context.Context, spec *catalogDomain.Spec, licenseOption *catalogDomain.LicenseOption, offer sharedmoney.SelectedPriceOffer, input CreateOrderInput) (*domain.Order, error) {

	receiptID, err := uuid.NewV7()
	if err != nil {
		return nil, fmt.Errorf("failed to generate uuid: %w", err)
	}

	priceIDStr := ""
	if offer.PriceID != nil {
		priceIDStr = offer.PriceID.String()
	}

	order := &domain.Order{
		ID:          receiptID,
		UserID:      input.UserID,
		SpecID:      input.SpecID,
		LicenseType: string(licenseOption.LicenseType),
		Amount:      offer.AmountMinor,
		Currency:    offer.Currency,
		Status:      domain.OrderStatusPending,
		Notes: map[string]any{
			"license_option_id": input.LicenseOptionID.String(),
			"spec_title":        spec.Title,
			"license_name":      licenseOption.Name,
			"display_currency":  offer.Currency,
			"pricing_snapshot": map[string]any{
				"price_id":     priceIDStr,
				"revision":     offer.Revision,
				"currency":     offer.Currency,
				"amount_minor": offer.AmountMinor,
				"amount_major": offer.AmountMajor,
				"selection":    string(offer.Selection),
				"validated_at": time.Now().UTC().Format(time.RFC3339),
			},
		},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	if s.snapshotter != nil {
		snapshot, snapErr := s.snapshotter.Snapshot(ctx, order.Currency)
		if snapErr != nil {
			log.Printf("[PaymentService] Warning: FX snapshot failed for currency %s: %v", order.Currency, snapErr)
		} else if snapshot != nil {
			order.ReportCurrency = &snapshot.ReportCurrency
			order.ReportFXRate = snapshot.ReportFXRate
			order.ReportFXRateID = snapshot.ReportFXRateID
			order.ReportFXAsOf = snapshot.ReportFXAsOf
			order.ReportFXFetchedAt = snapshot.ReportFXFetchedAt
		}
	}

	gateway, err := s.gateways.ForCurrency(order.Currency)
	if err != nil {
		return nil, err
	}
	session, err := gateway.CreateCheckout(ctx, domain.CheckoutRequest{
		Order:       order,
		SpecTitle:   spec.Title,
		LicenseName: licenseOption.Name,
	})
	if err != nil {
		return nil, err
	}
	order.Provider = gateway.Name()
	order.ProviderCheckoutID = &session.ProviderRef
	if session.CheckoutURL != "" {
		order.CheckoutURL = &session.CheckoutURL
	}

	if err := s.orderRepo.Create(ctx, order); err != nil {
		return nil, err
	}
	order.FillClientFields()
	return order, nil
}

// VerifyPayment confirms a payment that the browser reported back.
func (s *paymentService) VerifyPayment(ctx context.Context, orderID, userID uuid.UUID, providerPaymentID, signature string) (*domain.License, error) {
	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return nil, errors.New("order not found")
	}
	// Only the buyer may verify payment for an order; otherwise a bad signature
	// from a stranger would mark the buyer's order as failed.
	if order.UserID != userID {
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

	gateway, err := s.gateways.ByName(order.Provider)
	if err != nil {
		return nil, err
	}
	confirmer, ok := gateway.(domain.ClientConfirmer)
	if !ok {
		return nil, fmt.Errorf("%s payments are confirmed by webhook, not by the client", gateway.Name())
	}

	confirmation, err := confirmer.ConfirmClientPayment(ctx, order, providerPaymentID, signature)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidSignature), errors.Is(err, domain.ErrPaymentNotCaptured):
			if updateErr := s.orderRepo.UpdateStatus(ctx, orderID, domain.OrderStatusFailed); updateErr != nil {
				return nil, fmt.Errorf("%w and status update failed: %v", err, updateErr)
			}
		}
		return nil, err
	}

	return s.finalizePayment(ctx, order, confirmation)
}

// HandleWebhook confirms a payment that the gateway reported to us.
func (s *paymentService) HandleWebhook(ctx context.Context, provider string, payload []byte, headers map[string]string) error {
	gateway, err := s.gateways.ByName(provider)
	if err != nil {
		return err
	}
	confirmer, ok := gateway.(domain.WebhookConfirmer)
	if !ok {
		return fmt.Errorf("%s does not use webhooks", gateway.Name())
	}

	confirmation, err := confirmer.ParseWebhook(payload, headers)
	if err != nil {
		return err
	}
	if confirmation == nil {
		return nil // valid event we do not act on
	}

	order, err := s.orderRepo.GetByID(ctx, confirmation.OrderID)
	if err != nil {
		return errors.New("order not found")
	}
	if order.Provider != gateway.Name() {
		return errors.New("order not found")
	}
	if order.Status == domain.OrderStatusPaid {
		return nil // webhooks are delivered at least once
	}
	if order.Status != domain.OrderStatusPending {
		return errors.New("order is not payable")
	}

	_, err = s.finalizePayment(ctx, order, confirmation)
	return err
}

// finalizePayment is the one place a successful payment turns into a paid
// order, a license and a receipt, whichever gateway it came from.
func (s *paymentService) finalizePayment(ctx context.Context, order *domain.Order, c *domain.PaymentConfirmation) (*domain.License, error) {
	now := time.Now()
	payment := &domain.Payment{
		OrderID:           order.ID,
		Provider:          order.Provider,
		ProviderPaymentID: c.ProviderPaymentID,
		ProviderSignature: c.Signature,
		Amount:            order.Amount,
		Currency:          order.Currency,
		Status:            domain.PaymentStatusCaptured,
		Method:            c.Method,
		Email:             c.Email,
		CapturedAt:        &now,
	}
	if err := s.paymentRepo.Create(ctx, payment); err != nil {
		return nil, err
	}
	if err := s.orderRepo.UpdateStatus(ctx, order.ID, domain.OrderStatusPaid); err != nil {
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
			log.Printf("PaymentService.finalizePayment receipt email failed. order_id=%s err=%v", order.ID, err)
		}
	}()

	return license, nil
}

// GetOrder returns an order only to its buyer or to the producer who owns the
// ordered spec. Anyone else gets the same "not found" error as a missing order.
func (s *paymentService) GetOrder(ctx context.Context, orderID, requesterID uuid.UUID) (*domain.Order, error) {
	order, err := s.orderRepo.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.UserID == requesterID {
		order.FillClientFields()
		return order, nil
	}
	if spec, specErr := s.specFinder.FindByIDIncludingDeleted(ctx, order.SpecID); specErr == nil && spec != nil && spec.ProducerID == requesterID {
		order.FillClientFields()
		return order, nil
	}
	return nil, errors.New("order not found")
}

func (s *paymentService) GetUserOrders(ctx context.Context, userID uuid.UUID, page int) ([]domain.Order, error) {
	limit := 20
	offset := (page - 1) * limit
	if offset < 0 {
		offset = 0
	}
	orders, err := s.orderRepo.ListByUser(ctx, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].FillClientFields()
	}
	return orders, nil
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
		o.FillClientFields()
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
			Provider:        o.Provider,
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
		PaymentID:     payment.ProviderPaymentID,
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

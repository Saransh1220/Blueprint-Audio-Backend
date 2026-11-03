package payment

import (
	"github.com/jmoiron/sqlx"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/infrastructure/dodo"
	persistence "github.com/saransh1220/blueprint-audio/internal/modules/payment/infrastructure/persistence/postgres"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/infrastructure/razorpay"
	paymentHttp "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
	sharedemail "github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/email"
	sharedmoney "github.com/saransh1220/blueprint-audio/internal/shared/money"
)

// GatewayConfig holds the credentials for every payment gateway.
type GatewayConfig struct {
	Razorpay razorpay.Config
	Dodo     dodo.Config
}

// Module represents the Payment module
type Module struct {
	service application.PaymentService
	handler *paymentHttp.PaymentHandler
}

// NewModule creates and initializes the Payment module
func NewModule(
	db *sqlx.DB,
	specFinder catalogDomain.SpecFinder,
	userFinder authDomain.UserFinder,
	fileService application.FileService,
	emailSender sharedemail.Sender,
	appBaseURL string,
	gateways GatewayConfig,
) *Module {
	orderRepo := persistence.NewOrderRepository(db)
	paymentRepo := persistence.NewPaymentRepository(db)
	licenseRepo := persistence.NewLicenseRepository(db)

	if gateways.Dodo.ReturnURL == "" {
		gateways.Dodo.ReturnURL = appBaseURL + "/dashboard"
	}
	// Which gateway charges which currency is decided here, in one place.
	registry, err := application.NewGatewayRegistry(
		map[string]string{
			sharedmoney.CurrencyINR: domain.ProviderRazorpay,
			sharedmoney.CurrencyUSD: domain.ProviderDodo,
		},
		razorpay.NewGateway(gateways.Razorpay),
		dodo.NewGateway(gateways.Dodo),
	)
	if err != nil {
		panic(err) // a wiring mistake, caught at startup
	}

	service := application.NewPaymentService(orderRepo, paymentRepo, licenseRepo, specFinder, userFinder, fileService, emailSender, appBaseURL, registry)
	handler := paymentHttp.NewPaymentHandler(service)

	return &Module{
		service: service,
		handler: handler,
	}
}

// HTTPHandler returns the HTTP handler
func (m *Module) HTTPHandler() *paymentHttp.PaymentHandler {
	return m.handler
}

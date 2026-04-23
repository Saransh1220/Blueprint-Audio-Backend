package payment

import (
	"github.com/jmoiron/sqlx"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/application"
	persistence "github.com/saransh1220/blueprint-audio/internal/modules/payment/infrastructure/persistence/postgres"
	paymentHttp "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
	sharedemail "github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/email"
)

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
) *Module {
	orderRepo := persistence.NewOrderRepository(db)
	paymentRepo := persistence.NewPaymentRepository(db)
	licenseRepo := persistence.NewLicenseRepository(db)

	service := application.NewPaymentService(orderRepo, paymentRepo, licenseRepo, specFinder, userFinder, fileService, emailSender, appBaseURL)
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

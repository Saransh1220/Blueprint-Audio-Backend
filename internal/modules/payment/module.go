package payment

import (
	"github.com/jmoiron/sqlx"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/application"
	persistence "github.com/saransh1220/blueprint-audio/internal/modules/payment/infrastructure/persistence/postgres"
	paymentHttp "github.com/saransh1220/blueprint-audio/internal/modules/payment/interfaces/http"
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
	fileService application.FileService,
) *Module {
	orderRepo := persistence.NewOrderRepository(db)
	paymentRepo := persistence.NewPaymentRepository(db)
	licenseRepo := persistence.NewLicenseRepository(db)

	service := application.NewPaymentService(orderRepo, paymentRepo, licenseRepo, specFinder, fileService)
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

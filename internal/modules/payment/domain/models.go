package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	OrderStatusPending    OrderStatus = "pending"
	OrderStatusProcessing OrderStatus = "processing"
	OrderStatusPaid       OrderStatus = "paid"
	OrderStatusFailed     OrderStatus = "failed"
	OrderStatusCancelled  OrderStatus = "cancelled"
	OrderStatusRefunded   OrderStatus = "refunded"
)

type PaymentStatus string

const (
	PaymentStatusPending  PaymentStatus = "pending"
	PaymentStatusCaptured PaymentStatus = "captured"
	PaymentStatusFailed   PaymentStatus = "failed"
	PaymentStatusRefunded PaymentStatus = "refunded"
)

type Order struct {
	ID          uuid.UUID `json:"id" db:"id"`
	UserID      uuid.UUID `json:"user_id" db:"user_id"`
	SpecID      uuid.UUID `json:"spec_id" db:"spec_id"`
	LicenseType string    `json:"license_type" db:"license_type"`
	Amount      int       `json:"amount" db:"amount"`
	Currency    string    `json:"currency" db:"currency"`
	// RazorpayOrderID is derived from ProviderCheckoutID for Razorpay orders so
	// the browser checkout keeps working. It is not stored.
	RazorpayOrderID    *string        `json:"razorpay_order_id,omitempty" db:"-"`
	Provider           string         `json:"provider" db:"provider"`
	ProviderCheckoutID *string        `json:"provider_checkout_id,omitempty" db:"provider_checkout_id"`
	ProviderPaymentID  *string        `json:"provider_payment_id,omitempty" db:"provider_payment_id"`
	CheckoutURL        *string        `json:"checkout_url,omitempty" db:"-"`
	Status             OrderStatus    `json:"status" db:"status"`
	Notes              map[string]any `json:"notes,omitempty" db:"notes"`
	ReportCurrency     *string        `json:"report_currency,omitempty" db:"report_currency"`
	ReportFXRate       *string        `json:"report_fx_rate,omitempty" db:"report_fx_rate"`
	ReportFXAsOf       *time.Time     `json:"report_fx_as_of,omitempty" db:"report_fx_as_of"`
	ReportFXFetchedAt  *time.Time     `json:"report_fx_fetched_at,omitempty" db:"report_fx_fetched_at"`
	ReportFXRateID     *int64         `json:"report_fx_rate_id,omitempty" db:"report_fx_rate_id"`
	CreatedAt          time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at" db:"updated_at"`
	ExpiresAt          time.Time      `json:"expires_at" db:"expires_at"`
}

type OrderFXSnapshot struct {
	ReportCurrency    string
	ReportFXRate      *string
	ReportFXRateID    *int64
	ReportFXAsOf      *time.Time
	ReportFXFetchedAt *time.Time
}

type ReportingRateSnapshotter interface {
	Snapshot(ctx context.Context, orderCurrency string) (*OrderFXSnapshot, error)
}

// FillClientFields sets the derived fields the frontend expects.
func (o *Order) FillClientFields() {
	o.RazorpayOrderID = nil
	if o.Provider == ProviderRazorpay && o.ProviderCheckoutID != nil && *o.ProviderCheckoutID != "" {
		id := *o.ProviderCheckoutID
		o.RazorpayOrderID = &id
	}
}

type OrderWithBuyer struct {
	Order
	BuyerName  string `json:"buyer_name" db:"buyer_name"`
	BuyerEmail string `json:"buyer_email" db:"buyer_email"`
	SpecTitle  string `json:"spec_title" db:"spec_title"`
}

type Payment struct {
	ID                uuid.UUID     `json:"id" db:"id"`
	OrderID           uuid.UUID     `json:"order_id" db:"order_id"`
	Provider          string        `json:"provider" db:"provider"`
	ProviderPaymentID string        `json:"provider_payment_id" db:"provider_payment_id"`
	ProviderSignature string        `json:"provider_signature" db:"provider_signature"`
	Amount            int           `json:"amount" db:"amount"`
	Currency          string        `json:"currency" db:"currency"`
	Status            PaymentStatus `json:"status" db:"status"`
	Method            *string       `json:"method,omitempty" db:"method"`
	Bank              *string       `json:"bank,omitempty" db:"bank"`
	Wallet            *string       `json:"wallet,omitempty" db:"wallet"`
	VPA               *string       `json:"vpa,omitempty" db:"vpa"`
	CardNetwork       *string       `json:"card_network,omitempty" db:"card_network"`
	CardLast4         *string       `json:"card_last4,omitempty" db:"card_last4"`
	Email             *string       `json:"email,omitempty" db:"email"`
	Contact           *string       `json:"contact,omitempty" db:"contact"`
	ErrorCode         *string       `json:"error_code,omitempty" db:"error_code"`
	ErrorDescription  *string       `json:"error_description,omitempty" db:"error_description"`
	CapturedAt        *time.Time    `json:"captured_at,omitempty" db:"captured_at"`
	CreatedAt         time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at" db:"updated_at"`
}

type License struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	OrderID          uuid.UUID  `json:"order_id" db:"order_id"`
	UserID           uuid.UUID  `json:"user_id" db:"user_id"`
	SpecID           uuid.UUID  `json:"spec_id" db:"spec_id"`
	LicenseOptionID  uuid.UUID  `json:"license_option_id" db:"license_option_id"`
	LicenseType      string     `json:"license_type" db:"license_type"`
	PurchasePrice    int        `json:"purchase_price" db:"purchase_price"`
	Currency         string     `json:"currency" db:"currency"`
	LicenseKey       string     `json:"license_key" db:"license_key"`
	IsActive         bool       `json:"is_active" db:"is_active"`
	IsRevoked        bool       `json:"is_revoked" db:"is_revoked"`
	RevokedReason    *string    `json:"revoked_reason,omitempty" db:"revoked_reason"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	DownloadsCount   int        `json:"downloads_count" db:"downloads_count"`
	LastDownloadedAt *time.Time `json:"last_downloaded_at,omitempty" db:"last_downloaded_at"`
	IssuedAt         time.Time  `json:"issued_at" db:"issued_at"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`

	// Joined fields
	SpecTitle string  `json:"spec_title" db:"spec_title"`
	SpecImage *string `json:"spec_image" db:"spec_image"`
}

// Repositories

type OrderRepository interface {
	Create(ctx context.Context, order *Order) error
	GetByID(ctx context.Context, id uuid.UUID) (*Order, error)
	GetByProviderCheckoutID(ctx context.Context, provider, checkoutID string) (*Order, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status OrderStatus) error
	MarkPaidIfPayable(ctx context.Context, id uuid.UUID, providerPaymentID string) (bool, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Order, error)
	ListByProducer(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]OrderWithBuyer, int, error)
}

type PaymentRepository interface {
	Create(ctx context.Context, payment *Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*Payment, error)
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Payment, error)
	GetByProviderPaymentID(ctx context.Context, provider, providerPaymentID string) (*Payment, error)
}

type LicenseRepository interface {
	Create(ctx context.Context, license *License) error
	GetByID(ctx context.Context, id uuid.UUID) (*License, error)
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*License, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int, search, licenseType string) ([]License, int, error)
	IncrementDownloads(ctx context.Context, id uuid.UUID) error
	Revoke(ctx context.Context, id uuid.UUID, reason string) error
}

type PaymentTx interface {
	OrderRepo() OrderRepository
	PaymentRepo() PaymentRepository
	LicenseRepo() LicenseRepository
}

type TxRunner interface {
	WithTx(ctx context.Context, fn func(tx PaymentTx) error) error
}

package application

import (
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/payment/domain"
)

type LicenseDownloadsResponse struct {
	LicenseID   string  `json:"license_id"`
	LicenseType string  `json:"license_type"`
	SpecTitle   string  `json:"spec_title"`
	MP3URL      *string `json:"mp3_url,omitempty"`
	WAVURL      *string `json:"wav_url,omitempty"`
	StemsURL    *string `json:"stems_url,omitempty"`
	ExpiresIn   int     `json:"expires_in"` // Standardize on seconds
}

type ProducerOrderDto struct {
	ID              uuid.UUID          `json:"id"`
	Amount          float64            `json:"amount"`
	Currency        string             `json:"currency"`
	Status          domain.OrderStatus `json:"status"`
	CreatedAt       time.Time          `json:"created_at"`
	LicenseType     string             `json:"license_type"`
	BuyerName       string             `json:"buyer_name"`
	BuyerEmail      string             `json:"buyer_email"`
	SpecTitle       string             `json:"spec_title"`
	RazorpayOrderID *string            `json:"razorpay_order_id,omitempty"`
}

type ProducerOrderResponse struct {
	Orders []ProducerOrderDto `json:"orders"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

package domain

import "errors"

var (
	ErrOrderNotFound              = errors.New("order not found")
	ErrPaymentNotFound            = errors.New("payment not found")
	ErrLicenseNotFound            = errors.New("license not found")
	ErrInvalidOrderStatus         = errors.New("invalid order status")
	ErrInvalidPaymentStatus       = errors.New("invalid payment status")
	ErrLicenseRevoked             = errors.New("license has been revoked")
	ErrLicenseInactive            = errors.New("license is inactive")
	ErrInvalidSignature           = errors.New("invalid signature")
	ErrTimestampToleranceExceeded = errors.New("timestamp tolerance exceeded")
	ErrMalformedPayload           = errors.New("malformed payload")
	ErrPaymentNotCaptured         = errors.New("payment not captured")
	ErrAmountMismatch             = errors.New("payment amount or currency mismatch")
	ErrProviderMismatch           = errors.New("provider mismatch")
	ErrOrderNotPayable            = errors.New("order is not payable")
	ErrProviderCompletesByWebhook = errors.New("provider completes by webhook")
	ErrMissingWebhookSecret       = errors.New("webhook secret is not configured")
	ErrMissingPaymentID           = errors.New("missing payment id")
)

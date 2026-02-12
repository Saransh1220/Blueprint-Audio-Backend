package domain

import "errors"

var (
	ErrOrderNotFound        = errors.New("order not found")
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrLicenseNotFound      = errors.New("license not found")
	ErrInvalidOrderStatus   = errors.New("invalid order status")
	ErrInvalidPaymentStatus = errors.New("invalid payment status")
	ErrLicenseRevoked       = errors.New("license has been revoked")
	ErrLicenseInactive      = errors.New("license is inactive")
)

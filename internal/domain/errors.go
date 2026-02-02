package domain

import "errors"

var (
	ErrUserAlreadyExists  = errors.New("user with this email already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrSpecNotFound       = errors.New("spec not found")
	ErrUnauthorized       = errors.New("unauthorized action")
)

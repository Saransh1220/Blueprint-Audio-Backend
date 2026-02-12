package domain

import "errors"

var (
	ErrSpecNotFound    = errors.New("spec not found")
	ErrSpecSoftDeleted = errors.New("spec soft deleted")
	ErrUnauthorized    = errors.New("unauthorized action")
	ErrLicenseNotFound = errors.New("license not found")
	ErrInvalidCategory = errors.New("invalid category")
	ErrInvalidLicense  = errors.New("invalid license type")
)

package domain

import "errors"

var (
	ErrSpecNotFound    = errors.New("spec not found")
	ErrSpecSoftDeleted = errors.New("spec soft deleted")
	ErrSpecProcessing  = errors.New("spec cannot be changed while processing")
	ErrUnauthorized    = errors.New("unauthorized action")
	ErrLicenseNotFound = errors.New("license not found")
	ErrInvalidCategory = errors.New("invalid category")
	ErrInvalidLicense  = errors.New("invalid license type")
	ErrInvalidCurrency = errors.New("invalid currency")
	ErrInvalidUpload   = errors.New("invalid upload request")
	ErrUploadNotFound  = errors.New("upload session not found")
	ErrUploadForbidden = errors.New("upload session does not belong to producer")
	ErrUploadExpired   = errors.New("upload session expired")
	ErrUploadState     = errors.New("upload session is not in the required state")
	ErrNoProcessingJob = errors.New("no processing job available")
)

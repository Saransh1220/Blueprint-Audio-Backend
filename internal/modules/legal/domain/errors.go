package domain

import "errors"

var (
	ErrDocumentNotFound   = errors.New("legal document not found")
	ErrVersionNotFound    = errors.New("document version not found")
	ErrInvalidHash        = errors.New("content hash mismatch")
	ErrAlreadyPublished   = errors.New("document version already published")
	ErrInvalidState       = errors.New("invalid document state")
	ErrEmptySections      = errors.New("document must contain at least one section")
	ErrDuplicateVersion   = errors.New("display version already exists for this document")
)

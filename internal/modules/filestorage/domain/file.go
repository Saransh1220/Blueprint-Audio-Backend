package domain

import "time"

// File represents file metadata
type File struct {
	Key         string
	URL         string
	ContentType string
	Size        int64
}

// PresignedUpload contains everything a client needs to upload one object
// directly to the configured object store.
type PresignedUpload struct {
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// ObjectInfo contains server-verified metadata for an object.
type ObjectInfo struct {
	Key         string `json:"key"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	ETag        string `json:"etag"`
}

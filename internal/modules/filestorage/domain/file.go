package domain

// File represents file metadata
type File struct {
	Key         string
	URL         string
	ContentType string
	Size        int64
}

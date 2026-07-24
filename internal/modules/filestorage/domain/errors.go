package domain

import "errors"

// ErrDirectUploadUnsupported indicates that the selected storage backend
// cannot support browser-to-object-storage uploads.
var ErrDirectUploadUnsupported = errors.New("direct uploads are not supported by this storage backend")

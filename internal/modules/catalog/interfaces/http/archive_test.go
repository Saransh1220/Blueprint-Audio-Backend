package http

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetectStemsArchive(t *testing.T) {
	tests := []struct {
		name        string
		header      []byte
		extension   string
		contentType string
		valid       bool
	}{
		{name: "zip", header: []byte{'P', 'K', 0x03, 0x04}, extension: ".zip", contentType: "application/zip", valid: true},
		{name: "rar4", header: []byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0x00}, extension: ".rar", contentType: "application/vnd.rar", valid: true},
		{name: "rar5", header: []byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0x01, 0x00}, extension: ".rar", contentType: "application/vnd.rar", valid: true},
		{name: "invalid", header: []byte("not an archive"), valid: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extension, contentType, valid := detectStemsArchive(test.header)
			require.Equal(t, test.valid, valid)
			require.Equal(t, test.extension, extension)
			require.Equal(t, test.contentType, contentType)
		})
	}
}

func TestValidateUploadedFilesAcceptsRARStems(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stems.rar")
	require.NoError(t, os.WriteFile(path, []byte{'R', 'a', 'r', '!', 0x1a, 0x07, 0x01, 0x00}, 0o600))

	require.NoError(t, validateUploadedFiles(map[string]string{"stems": path}))
}

func TestValidateUploadedFilesRejectsUnknownStemsArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stems.rar")
	require.NoError(t, os.WriteFile(path, []byte("not an archive"), 0o600))

	err := validateUploadedFiles(map[string]string{"stems": path})
	require.EqualError(t, err, "stems must be a ZIP or RAR archive")
}

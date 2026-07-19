package http

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeValidationFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func jpegForValidation(t *testing.T, width, height int) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "image.jpg")
	file, err := os.Create(path)
	require.NoError(t, err)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 100, G: 50, B: 200, A: 255})
		}
	}
	require.NoError(t, jpeg.Encode(file, img, nil))
	require.NoError(t, file.Close())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func TestValidateUploadedFilesAllSupportedTypes(t *testing.T) {
	files := map[string]string{
		"image":   writeValidationFile(t, "cover.jpg", jpegForValidation(t, 4, 4)),
		"preview": writeValidationFile(t, "preview.mp3", []byte("ID3valid-enough-for-signature-check")),
		"wav":     writeValidationFile(t, "audio.wav", []byte("RIFF\x04\x00\x00\x00WAVEdata")),
		"stems":   writeValidationFile(t, "stems.zip", []byte{'P', 'K', 0x05, 0x06}),
	}
	require.NoError(t, validateUploadedFiles(files))
}

func TestValidateUploadedFilesFailureBranches(t *testing.T) {
	tests := []struct {
		name string
		key  string
		path func(*testing.T) string
		want string
	}{
		{"missing", "image", func(t *testing.T) string { return filepath.Join(t.TempDir(), "missing") }, "invalid uploaded file"},
		{"empty", "preview", func(t *testing.T) string { return writeValidationFile(t, "empty.mp3", nil) }, "preview file exceeds its size limit"},
		{"invalid image", "image", func(t *testing.T) string { return writeValidationFile(t, "bad.jpg", []byte("not-image")) }, "cover image must be a valid image"},
		{"non square image", "image", func(t *testing.T) string { return writeValidationFile(t, "wide.jpg", jpegForValidation(t, 8, 4)) }, "cover image must be square"},
		{"invalid preview", "preview", func(t *testing.T) string { return writeValidationFile(t, "bad.mp3", []byte("BAD")) }, "preview must be an MP3 file"},
		{"short preview", "preview", func(t *testing.T) string { return writeValidationFile(t, "short.mp3", []byte{0xff}) }, "preview must be an MP3 file"},
		{"invalid wav", "wav", func(t *testing.T) string { return writeValidationFile(t, "bad.wav", []byte("not a wave file")) }, "wav must be a WAV file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.EqualError(t, validateUploadedFiles(map[string]string{tt.key: tt.path(t)}), tt.want)
		})
	}
}

func TestCleanupTempFilesAndAdditionalZipSignatures(t *testing.T) {
	first := writeValidationFile(t, "first.tmp", []byte("x"))
	second := writeValidationFile(t, "second.tmp", []byte("x"))
	cleanupTempFiles(map[string]string{"first": first, "second": second})
	_, err := os.Stat(first)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(second)
	require.ErrorIs(t, err, os.ErrNotExist)

	for _, signature := range [][]byte{{'P', 'K', 0x05, 0x06}, {'P', 'K', 0x07, 0x08}} {
		extension, contentType, ok := detectStemsArchive(signature)
		require.True(t, ok)
		require.Equal(t, ".zip", extension)
		require.Equal(t, "application/zip", contentType)
	}
}

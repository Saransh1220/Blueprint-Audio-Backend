package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	catalogHTTP "github.com/saransh1220/blueprint-audio/internal/modules/catalog/interfaces/http"
	notificationDomain "github.com/saransh1220/blueprint-audio/internal/modules/notification/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func catalogSquareJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 80, B: 30, A: 255})
		}
	}
	var data bytes.Buffer
	require.NoError(t, jpeg.Encode(&data, img, nil))
	return data.Bytes()
}

func sampleCreateRequest(t *testing.T, producerID uuid.UUID) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	metadata, err := json.Marshal(map[string]any{
		"title": "Sample Pack", "category": "sample", "price": 10,
	})
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("metadata", string(metadata)))
	imagePart, err := writer.CreateFormFile("image", "cover.jpg")
	require.NoError(t, err)
	_, err = imagePart.Write(catalogSquareJPEG(t))
	require.NoError(t, err)
	previewPart, err := writer.CreateFormFile("preview", "preview.mp3")
	require.NoError(t, err)
	_, err = previewPart.Write([]byte("ID3valid-preview"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/specs", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyUserId, producerID))
}

func TestSpecHandlerCreateSampleCompletesAsyncProcessing(t *testing.T) {
	specs := new(mockSpecService)
	files := new(mockFileService)
	analytics := new(mockAnalyticsService)
	notifications := new(mockNotificationService)
	handler := catalogHTTP.NewSpecHandler(specs, files, analytics, notifications, nil)
	producerID := uuid.New()
	done := make(chan struct{})

	specs.On("CreateSpec", mock.Anything, mock.MatchedBy(func(spec *domain.Spec) bool {
		return spec.ID != uuid.Nil && spec.ProducerID == producerID && spec.ProcessingStatus == domain.ProcessingStatusProcessing
	})).Return(nil).Once()
	files.On("UploadWithKey", mock.Anything, mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "images/") && strings.HasSuffix(key, ".jpg")
	}), "image/jpeg").Return("https://storage/cover.jpg", nil).Once()
	files.On("UploadWithKey", mock.Anything, mock.Anything, mock.MatchedBy(func(key string) bool {
		return strings.HasPrefix(key, "audio/previews/") && strings.HasSuffix(key, ".mp3")
	}), "audio/mpeg").Return("https://storage/preview.mp3", nil).Once()
	specs.On("UpdateFilesAndStatus", mock.Anything, mock.AnythingOfType("uuid.UUID"), mock.MatchedBy(func(updated map[string]*string) bool {
		return updated["image_url"] != nil && updated["preview_url"] != nil
	}), domain.ProcessingStatusCompleted).Return(nil).Once()
	notifications.On("Create", mock.Anything, producerID, "Upload Complete", mock.AnythingOfType("string"), notificationDomain.NotificationTypeSuccess).
		Run(func(mock.Arguments) { close(done) }).Return(nil).Once()

	response := httptest.NewRecorder()
	handler.Create(response, sampleCreateRequest(t, producerID))
	assert.Equal(t, http.StatusAccepted, response.Code)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for asynchronous processing")
	}
	specs.AssertExpectations(t)
	files.AssertExpectations(t)
	notifications.AssertExpectations(t)
}

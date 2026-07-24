package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	filestorageDomain "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	"github.com/stretchr/testify/require"
)

type stubSpecUploadService struct {
	initiate     func(context.Context, uuid.UUID) (*application.InitiateSpecUploadResult, error)
	saveMetadata func(context.Context, uuid.UUID, uuid.UUID, domain.Spec) error
	prepareFile  func(context.Context, uuid.UUID, uuid.UUID, application.UploadFileCommand) (*application.UploadInstruction, error)
	confirmFile  func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.SpecUploadAsset, error)
	complete     func(context.Context, uuid.UUID, uuid.UUID) (*domain.Spec, error)
	status       func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadStatus, error)
}

func (s stubSpecUploadService) Initiate(
	ctx context.Context,
	producerID uuid.UUID,
) (*application.InitiateSpecUploadResult, error) {
	return s.initiate(ctx, producerID)
}

func (s stubSpecUploadService) SaveMetadata(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
	spec domain.Spec,
) error {
	return s.saveMetadata(ctx, uploadID, producerID, spec)
}

func (s stubSpecUploadService) PrepareFile(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
	file application.UploadFileCommand,
) (*application.UploadInstruction, error) {
	return s.prepareFile(ctx, uploadID, producerID, file)
}

func (s stubSpecUploadService) ConfirmFile(
	ctx context.Context,
	uploadID, producerID, assetID uuid.UUID,
) (*domain.SpecUploadAsset, error) {
	return s.confirmFile(ctx, uploadID, producerID, assetID)
}

func (s stubSpecUploadService) Complete(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
) (*domain.Spec, error) {
	return s.complete(ctx, uploadID, producerID)
}

func (s stubSpecUploadService) Status(
	ctx context.Context,
	uploadID, producerID uuid.UUID,
) (*domain.SpecUploadStatus, error) {
	return s.status(ctx, uploadID, producerID)
}

func authenticatedUploadRequest(method, target string, body []byte, producerID uuid.UUID) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	return request.WithContext(context.WithValue(
		request.Context(), middleware.ContextKeyUserId, producerID,
	))
}

func TestSpecUploadHandlerInitiateUsesStrictJSON(t *testing.T) {
	handler := NewSpecUploadHandler(stubSpecUploadService{
		initiate: func(
			context.Context,
			uuid.UUID,
		) (*application.InitiateSpecUploadResult, error) {
			t.Fatal("service must not be called for an unknown JSON field")
			return nil, nil
		},
	})
	request := authenticatedUploadRequest(
		http.MethodPost,
		"/spec-uploads",
		[]byte(`{"unexpected":true}`),
		uuid.New(),
	)
	response := httptest.NewRecorder()

	handler.Initiate(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestSpecUploadHandlerSaveMetadata(t *testing.T) {
	uploadID := uuid.New()
	producerID := uuid.New()
	handler := NewSpecUploadHandler(stubSpecUploadService{
		saveMetadata: func(
			_ context.Context,
			actualUploadID, actualProducerID uuid.UUID,
			spec domain.Spec,
		) error {
			require.Equal(t, uploadID, actualUploadID)
			require.Equal(t, producerID, actualProducerID)
			require.Equal(t, "Night Drive", spec.Title)
			require.Equal(t, domain.CategoryBeat, spec.Category)
			return nil
		},
	})
	request := authenticatedUploadRequest(
		http.MethodPut,
		"/spec-uploads/"+uploadID.String()+"/metadata",
		[]byte(`{"title":"Night Drive","category":"beat"}`),
		producerID,
	)
	request.SetPathValue("id", uploadID.String())
	response := httptest.NewRecorder()

	handler.SaveMetadata(response, request)

	require.Equal(t, http.StatusNoContent, response.Code)
}

func TestSpecUploadHandlerPrepareFile(t *testing.T) {
	uploadID := uuid.New()
	assetID := uuid.New()
	producerID := uuid.New()
	handler := NewSpecUploadHandler(stubSpecUploadService{
		prepareFile: func(
			_ context.Context,
			actualUploadID, actualProducerID uuid.UUID,
			file application.UploadFileCommand,
		) (*application.UploadInstruction, error) {
			require.Equal(t, uploadID, actualUploadID)
			require.Equal(t, producerID, actualProducerID)
			require.Equal(t, domain.UploadAssetWAV, file.Kind)
			require.Equal(t, int64(4096), file.SizeBytes)
			return &application.UploadInstruction{
				AssetID:   assetID,
				Kind:      domain.UploadAssetWAV,
				ObjectKey: "incoming/master.wav",
				Upload: filestorageDomain.PresignedUpload{
					URL:     "https://storage.example/upload",
					Method:  http.MethodPut,
					Headers: map[string]string{"Content-Type": "audio/wav"},
				},
			}, nil
		},
	})
	request := authenticatedUploadRequest(
		http.MethodPost,
		"/spec-uploads/"+uploadID.String()+"/files",
		[]byte(`{
			"kind":"wav",
			"file_name":"master.wav",
			"content_type":"audio/wav",
			"size_bytes":4096
		}`),
		producerID,
	)
	request.SetPathValue("id", uploadID.String())
	response := httptest.NewRecorder()

	handler.PrepareFile(response, request)

	require.Equal(t, http.StatusCreated, response.Code)
	require.Contains(t, response.Body.String(), assetID.String())
	require.Contains(t, response.Body.String(), `"method":"PUT"`)
}

func TestSpecUploadHandlerConfirmFile(t *testing.T) {
	uploadID := uuid.New()
	assetID := uuid.New()
	producerID := uuid.New()
	actualSize := int64(4096)
	actualContentType := "audio/wav"
	etag := "verified-etag"
	handler := NewSpecUploadHandler(stubSpecUploadService{
		confirmFile: func(
			_ context.Context,
			actualUploadID, actualProducerID, actualAssetID uuid.UUID,
		) (*domain.SpecUploadAsset, error) {
			require.Equal(t, uploadID, actualUploadID)
			require.Equal(t, producerID, actualProducerID)
			require.Equal(t, assetID, actualAssetID)
			return &domain.SpecUploadAsset{
				ID:                  assetID,
				Kind:                domain.UploadAssetWAV,
				FileName:            "master.wav",
				ExpectedSize:        actualSize,
				ActualSize:          &actualSize,
				DeclaredContentType: actualContentType,
				ActualContentType:   &actualContentType,
				ETag:                &etag,
			}, nil
		},
	})
	request := authenticatedUploadRequest(
		http.MethodPost,
		"/spec-uploads/"+uploadID.String()+"/files/"+assetID.String()+"/complete",
		[]byte(`{}`),
		producerID,
	)
	request.SetPathValue("id", uploadID.String())
	request.SetPathValue("assetID", assetID.String())
	response := httptest.NewRecorder()

	handler.ConfirmFile(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), assetID.String())
	require.Contains(t, response.Body.String(), `"size_bytes":4096`)
}

func TestSpecUploadHandlerErrorMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
	}{
		{name: "invalid", err: domain.ErrInvalidUpload, code: http.StatusBadRequest},
		{name: "not found", err: domain.ErrUploadNotFound, code: http.StatusNotFound},
		{name: "forbidden", err: domain.ErrUploadForbidden, code: http.StatusForbidden},
		{name: "expired", err: domain.ErrUploadExpired, code: http.StatusConflict},
		{name: "state", err: domain.ErrUploadState, code: http.StatusConflict},
		{
			name: "unsupported storage",
			err:  filestorageDomain.ErrDirectUploadUnsupported,
			code: http.StatusNotImplemented,
		},
		{name: "internal", err: errors.New("database unavailable"), code: http.StatusInternalServerError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := NewSpecUploadHandler(stubSpecUploadService{
				initiate: func(
					context.Context,
					uuid.UUID,
				) (*application.InitiateSpecUploadResult, error) {
					return nil, test.err
				},
			})
			request := authenticatedUploadRequest(
				http.MethodPost,
				"/spec-uploads",
				[]byte(`{}`),
				uuid.New(),
			)
			response := httptest.NewRecorder()

			handler.Initiate(response, request)

			require.Equal(t, test.code, response.Code)
		})
	}
}

func TestSpecUploadHandlerStatusResponse(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	uploadID := uuid.New()
	specID := uuid.New()
	producerID := uuid.New()
	handler := NewSpecUploadHandler(stubSpecUploadService{
		status: func(
			_ context.Context,
			actualUploadID, actualProducerID uuid.UUID,
		) (*domain.SpecUploadStatus, error) {
			require.Equal(t, uploadID, actualUploadID)
			require.Equal(t, producerID, actualProducerID)
			return &domain.SpecUploadStatus{
				UploadID:         uploadID,
				SpecID:           specID,
				ProducerID:       producerID,
				Status:           domain.UploadStatusProcessing,
				ProcessingStatus: domain.ProcessingStatusProcessing,
				CreatedAt:        now,
				UpdatedAt:        now,
				ExpiresAt:        now.Add(time.Hour),
			}, nil
		},
	})
	request := authenticatedUploadRequest(
		http.MethodGet, "/spec-uploads/"+uploadID.String(), nil, producerID,
	)
	request.SetPathValue("id", uploadID.String())
	response := httptest.NewRecorder()

	handler.Status(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"status":"processing"`)
	require.Contains(t, response.Body.String(), specID.String())
}

package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/require"
)

func uploadReq(method string, body string, producerID uuid.UUID, pathValues map[string]string) *http.Request {
	var b []byte
	if body != "" {
		b = []byte(body)
	}
	r := authenticatedUploadRequest(method, "/spec-uploads", b, producerID)
	for k, v := range pathValues {
		r.SetPathValue(k, v)
	}
	return r
}

func unauthUploadReq(method string, pathValues map[string]string) *http.Request {
	r := httptest.NewRequest(method, "/spec-uploads", strings.NewReader("{}"))
	for k, v := range pathValues {
		r.SetPathValue(k, v)
	}
	return r
}

func TestSpecUploadHandler_Unauthorized(t *testing.T) {
	h := NewSpecUploadHandler(stubSpecUploadService{})
	id := uuid.New().String()
	pv := map[string]string{"id": id, "assetID": uuid.New().String()}
	calls := map[string]func(http.ResponseWriter, *http.Request){
		"initiate": h.Initiate, "save": h.SaveMetadata, "prepare": h.PrepareFile,
		"confirm": h.ConfirmFile, "complete": h.Complete, "status": h.Status,
	}
	for name, fn := range calls {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			fn(w, unauthUploadReq(http.MethodPost, pv))
			require.Equal(t, http.StatusUnauthorized, w.Code)
			require.Contains(t, w.Body.String(), "unauthorized")
		})
	}
}

func TestSpecUploadHandler_InvalidUploadID(t *testing.T) {
	h := NewSpecUploadHandler(stubSpecUploadService{})
	pv := map[string]string{"id": "not-a-uuid", "assetID": uuid.New().String()}
	calls := map[string]func(http.ResponseWriter, *http.Request){
		"save": h.SaveMetadata, "prepare": h.PrepareFile, "confirm": h.ConfirmFile,
		"complete": h.Complete, "status": h.Status,
	}
	for name, fn := range calls {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			fn(w, uploadReq(http.MethodPost, "{}", uuid.New(), pv))
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), "invalid upload id")
		})
	}
}

func TestSpecUploadHandler_ConfirmFileInvalidAssetID(t *testing.T) {
	h := NewSpecUploadHandler(stubSpecUploadService{})
	w := httptest.NewRecorder()
	h.ConfirmFile(w, uploadReq(http.MethodPost, "", uuid.New(),
		map[string]string{"id": uuid.New().String(), "assetID": "bad"}))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "invalid upload asset id")
}

func TestSpecUploadHandler_BadBodies(t *testing.T) {
	h := NewSpecUploadHandler(stubSpecUploadService{})
	pv := map[string]string{"id": uuid.New().String()}

	t.Run("save metadata malformed", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.SaveMetadata(w, uploadReq(http.MethodPut, "{bad", uuid.New(), pv))
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Contains(t, w.Body.String(), "invalid beat metadata")
	})
	t.Run("save metadata unknown field", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.SaveMetadata(w, uploadReq(http.MethodPut, `{"nope":1}`, uuid.New(), pv))
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("save metadata trailing value", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.SaveMetadata(w, uploadReq(http.MethodPut, `{"title":"a"}{"title":"b"}`, uuid.New(), pv))
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("prepare file malformed", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.PrepareFile(w, uploadReq(http.MethodPost, "[", uuid.New(), pv))
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Contains(t, w.Body.String(), "invalid upload file")
	})
	t.Run("initiate malformed", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.Initiate(w, uploadReq(http.MethodPost, "{bad", uuid.New(), nil))
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Contains(t, w.Body.String(), "empty object")
	})
	t.Run("initiate trailing value", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.Initiate(w, uploadReq(http.MethodPost, `{}{}`, uuid.New(), nil))
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("complete unknown field", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.Complete(w, uploadReq(http.MethodPost, `{"x":1}`, uuid.New(), pv))
		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Contains(t, w.Body.String(), "empty object")
	})
}

func TestSpecUploadHandler_InitiateSuccess(t *testing.T) {
	uploadID, specID, producerID := uuid.New(), uuid.New(), uuid.New()
	h := NewSpecUploadHandler(stubSpecUploadService{
		initiate: func(_ context.Context, p uuid.UUID) (*application.InitiateSpecUploadResult, error) {
			require.Equal(t, producerID, p)
			return &application.InitiateSpecUploadResult{
				UploadID: uploadID, SpecID: specID, ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	})
	w := httptest.NewRecorder()
	// empty body exercises the optional-empty path
	h.Initiate(w, uploadReq(http.MethodPost, "", producerID, nil))
	require.Equal(t, http.StatusCreated, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	require.Contains(t, w.Body.String(), uploadID.String())
	require.Contains(t, w.Body.String(), specID.String())
}

func TestSpecUploadHandler_Complete(t *testing.T) {
	uploadID, specID, producerID := uuid.New(), uuid.New(), uuid.New()
	pv := map[string]string{"id": uploadID.String()}

	t.Run("success", func(t *testing.T) {
		h := NewSpecUploadHandler(stubSpecUploadService{
			complete: func(_ context.Context, u, p uuid.UUID) (*domain.Spec, error) {
				require.Equal(t, uploadID, u)
				require.Equal(t, producerID, p)
				return &domain.Spec{ID: specID, Title: "Night Drive"}, nil
			},
		})
		w := httptest.NewRecorder()
		h.Complete(w, uploadReq(http.MethodPost, `{}`, producerID, pv))
		require.Equal(t, http.StatusAccepted, w.Code)
		require.Equal(t, "/specs/"+specID.String(), w.Header().Get("Location"))
		require.Contains(t, w.Body.String(), "Night Drive")
	})
	t.Run("service error", func(t *testing.T) {
		h := NewSpecUploadHandler(stubSpecUploadService{
			complete: func(context.Context, uuid.UUID, uuid.UUID) (*domain.Spec, error) {
				return nil, domain.ErrUploadState
			},
		})
		w := httptest.NewRecorder()
		h.Complete(w, uploadReq(http.MethodPost, ``, producerID, pv))
		require.Equal(t, http.StatusConflict, w.Code)
	})
}

func TestSpecUploadHandler_ServiceErrors(t *testing.T) {
	uploadID, assetID, producerID := uuid.New(), uuid.New(), uuid.New()
	pv := map[string]string{"id": uploadID.String(), "assetID": assetID.String()}
	boom := errors.New("boom")
	h := NewSpecUploadHandler(stubSpecUploadService{
		saveMetadata: func(context.Context, uuid.UUID, uuid.UUID, domain.Spec) error { return domain.ErrUploadNotFound },
		prepareFile: func(context.Context, uuid.UUID, uuid.UUID, application.UploadFileCommand) (*application.UploadInstruction, error) {
			return nil, domain.ErrUploadForbidden
		},
		confirmFile: func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*domain.SpecUploadAsset, error) {
			return nil, domain.ErrInvalidUpload
		},
		status: func(context.Context, uuid.UUID, uuid.UUID) (*domain.SpecUploadStatus, error) { return nil, boom },
	})

	w := httptest.NewRecorder()
	h.SaveMetadata(w, uploadReq(http.MethodPut, `{"title":"x"}`, producerID, pv))
	require.Equal(t, http.StatusNotFound, w.Code)
	require.Contains(t, w.Body.String(), "upload session not found")

	w = httptest.NewRecorder()
	h.PrepareFile(w, uploadReq(http.MethodPost, `{"kind":"wav"}`, producerID, pv))
	require.Equal(t, http.StatusForbidden, w.Code)

	w = httptest.NewRecorder()
	h.ConfirmFile(w, uploadReq(http.MethodPost, ``, producerID, pv))
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "invalid upload request")

	w = httptest.NewRecorder()
	h.Status(w, uploadReq(http.MethodGet, ``, producerID, pv))
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.Contains(t, w.Body.String(), "internal server error")
}

func TestCreateSpecMetadataRequest_ToSpecMapsNested(t *testing.T) {
	req := CreateSpecMetadataRequest{
		Title: "T", Category: domain.CategoryBeat, BPM: 140, Price: 9.5, PriceCurrency: "USD",
		Tags:   []string{"a"},
		Genres: []CreateGenreRequest{{Name: "Trap", Slug: "trap"}},
		Licenses: []CreateLicenseRequest{{
			Type: domain.LicenseType("basic"), Name: "Basic", Price: 5, PriceCurrency: "USD",
			Features: []string{"mp3"}, FileTypes: []string{"mp3"},
		}},
	}
	spec := req.toSpec()
	require.Equal(t, "T", spec.Title)
	require.Equal(t, 140, spec.BPM)
	require.Len(t, spec.Genres, 1)
	require.Equal(t, "trap", spec.Genres[0].Slug)
	require.Len(t, spec.Licenses, 1)
	require.Equal(t, "Basic", spec.Licenses[0].Name)
	require.Equal(t, 5.0, spec.Licenses[0].Price)
	require.Len(t, spec.Licenses[0].FileTypes, 1)
}

type failingWriter struct{ header http.Header }

func (f *failingWriter) Header() http.Header         { return f.header }
func (f *failingWriter) Write([]byte) (int, error)   { return 0, errors.New("write failed") }
func (f *failingWriter) WriteHeader(statusCode int) {}

func TestWriteJSON_EncodeFailureDoesNotPanic(t *testing.T) {
	w := &failingWriter{header: http.Header{}}
	require.NotPanics(t, func() { writeJSON(w, http.StatusOK, map[string]string{"a": "b"}) })
	require.Equal(t, "application/json", w.header.Get("Content-Type"))
}

package http

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	filestorageDomain "github.com/saransh1220/blueprint-audio/internal/modules/filestorage/domain"
	"github.com/saransh1220/blueprint-audio/internal/shared/money"
)

type SpecUploadHandler struct {
	service application.SpecUploadService
}

func NewSpecUploadHandler(service application.SpecUploadService) *SpecUploadHandler {
	return &SpecUploadHandler{service: service}
}

func (h *SpecUploadHandler) Initiate(w http.ResponseWriter, r *http.Request) {
	producerID, ok := authenticatedProducer(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var body struct{}
		if err := decodeOptionalEmptyJSON(r.Body, &body); err != nil {
			http.Error(w, "request body must be an empty object", http.StatusBadRequest)
			return
		}
	}
	result, err := h.service.Initiate(r.Context(), producerID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCreateUploadResponse(result))
}

func (h *SpecUploadHandler) SaveMetadata(w http.ResponseWriter, r *http.Request) {
	producerID, ok := authenticatedProducer(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	uploadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid upload id", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var request CreateSpecMetadataRequest
	if err := decodeStrictJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid beat metadata", http.StatusBadRequest)
		return
	}
	if err := h.service.SaveMetadata(
		r.Context(), uploadID, producerID, request.toSpec(),
	); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SpecUploadHandler) PrepareFile(w http.ResponseWriter, r *http.Request) {
	producerID, ok := authenticatedProducer(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	uploadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid upload id", http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var request CreateUploadFileRequest
	if err := decodeStrictJSON(r.Body, &request); err != nil {
		http.Error(w, "invalid upload file", http.StatusBadRequest)
		return
	}
	instruction, err := h.service.PrepareFile(
		r.Context(), uploadID, producerID, request.toCommand(),
	)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toPresignedUploadResponse(instruction))
}

func (h *SpecUploadHandler) ConfirmFile(w http.ResponseWriter, r *http.Request) {
	producerID, ok := authenticatedProducer(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	uploadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid upload id", http.StatusBadRequest)
		return
	}
	assetID, err := uuid.Parse(r.PathValue("assetID"))
	if err != nil {
		http.Error(w, "invalid upload asset id", http.StatusBadRequest)
		return
	}
	asset, err := h.service.ConfirmFile(r.Context(), uploadID, producerID, assetID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toConfirmedUploadFileResponse(asset))
}

func (h *SpecUploadHandler) Complete(w http.ResponseWriter, r *http.Request) {
	producerID, ok := authenticatedProducer(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	uploadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid upload id", http.StatusBadRequest)
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		var body struct{}
		if err := decodeOptionalEmptyJSON(r.Body, &body); err != nil {
			http.Error(w, "request body must be an empty object", http.StatusBadRequest)
			return
		}
	}
	spec, err := h.service.Complete(r.Context(), uploadID, producerID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	w.Header().Set("Location", "/specs/"+spec.ID.String())
	writeJSON(w, http.StatusAccepted, ToSpecResponseForCurrency(spec, money.ResolveCurrencyFromRequest(r)))
}

func (h *SpecUploadHandler) Status(w http.ResponseWriter, r *http.Request) {
	producerID, ok := authenticatedProducer(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	uploadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid upload id", http.StatusBadRequest)
		return
	}
	status, err := h.service.Status(r.Context(), uploadID, producerID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toUploadStatusResponse(status))
}

func (h *SpecUploadHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidUpload):
		log.Printf("[SpecUploadHandler] invalid upload request: %v", err)
		http.Error(w, "invalid upload request", http.StatusBadRequest)
	case errors.Is(err, domain.ErrUploadNotFound):
		http.Error(w, "upload session not found", http.StatusNotFound)
	case errors.Is(err, domain.ErrUploadForbidden):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, domain.ErrUploadExpired), errors.Is(err, domain.ErrUploadState):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, filestorageDomain.ErrDirectUploadUnsupported):
		http.Error(w, "direct uploads are not supported by this storage backend", http.StatusNotImplemented)
	default:
		log.Printf("[SpecUploadHandler] request failed: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func authenticatedProducer(r *http.Request) (uuid.UUID, bool) {
	id, ok := r.Context().Value(middleware.ContextKeyUserId).(uuid.UUID)
	return id, ok && id != uuid.Nil
}

func decodeStrictJSON(reader io.Reader, destination any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func decodeOptionalEmptyJSON(reader io.Reader, destination any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Printf("encode JSON response: %v", err)
	}
}

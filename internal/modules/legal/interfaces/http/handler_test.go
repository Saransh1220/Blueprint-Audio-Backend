package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/gateway/middleware"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/application"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockLegalService struct {
	view         *application.DocumentView
	versions     []domain.DocumentVersion
	version      *domain.DocumentVersion
	requirements []domain.Requirement
	audits       []domain.PublicationAudit
	acceptedKey  string
	acceptedVer  uuid.UUID
	err          error
}

func (m *mockLegalService) GetDocument(ctx context.Context, key, locale string) (*application.DocumentView, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.view, nil
}
func (m *mockLegalService) GetDocumentVersions(ctx context.Context, key string) ([]domain.DocumentVersion, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.versions, nil
}
func (m *mockLegalService) GetDocumentVersion(ctx context.Context, key, displayVersion string) (*domain.DocumentVersion, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.version, nil
}
func (m *mockLegalService) RecordAcceptance(ctx context.Context, userID uuid.UUID, key string, versionID uuid.UUID, hash, action, contextInfo string) error {
	m.acceptedKey = key
	m.acceptedVer = versionID
	return m.err
}
func (m *mockLegalService) GetRequirements(ctx context.Context, userID uuid.UUID) ([]domain.Requirement, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.requirements, nil
}
func (m *mockLegalService) CreateDraft(ctx context.Context, authorID uuid.UUID, version *domain.DocumentVersion) error {
	return m.err
}
func (m *mockLegalService) PublishVersion(ctx context.Context, actorID uuid.UUID, versionID uuid.UUID, notes string) error {
	return m.err
}
func (m *mockLegalService) GetAudits(ctx context.Context, limit int) ([]domain.PublicationAudit, error) {
	return m.audits, m.err
}

func TestLegalHandler_GetDocument(t *testing.T) {
	svc := &mockLegalService{
		view: &application.DocumentView{
			Document: domain.Document{Key: "terms", Title: "Terms of Use"},
			CurrentVersion: domain.DocumentVersion{
				DisplayVersion: "1.0.0",
				Title:          "Terms of Use",
			},
		},
	}
	handler := NewLegalHandler(svc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/legal/documents/{key}", handler.GetDocument)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/legal/documents/terms", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp application.DocumentView
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "terms", resp.Document.Key)
	assert.Equal(t, "1.0.0", resp.CurrentVersion.DisplayVersion)
}

func TestLegalHandler_RecordAcceptance(t *testing.T) {
	verID := uuid.New()
	userID := uuid.New()
	svc := &mockLegalService{}
	handler := NewLegalHandler(svc)

	bodyBytes, _ := json.Marshal(RecordAcceptanceBody{
		DocumentKey: "terms",
		VersionID:   verID,
		ContentHash: "hash123",
		Action:      "register",
		Context:     "form",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legal/acceptances", bytes.NewReader(bodyBytes))
	ctx := context.WithValue(req.Context(), middleware.ContextKeyUserId, userID)
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	handler.RecordAcceptance(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "terms", svc.acceptedKey)
	assert.Equal(t, verID, svc.acceptedVer)
}

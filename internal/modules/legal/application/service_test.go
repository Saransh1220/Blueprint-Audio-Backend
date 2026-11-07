package application

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDocRepo struct {
	doc          *domain.Document
	effective    *domain.DocumentVersion
	upcoming     *domain.DocumentVersion
	versions     map[string]*domain.DocumentVersion
	versionList  []domain.DocumentVersion
	audits       []domain.PublicationAudit
	draftCreated *domain.DocumentVersion
	publishedID  uuid.UUID
}

func (m *mockDocRepo) GetDocument(ctx context.Context, key, locale string) (*domain.Document, error) {
	if m.doc == nil {
		return nil, domain.ErrDocumentNotFound
	}
	return m.doc, nil
}
func (m *mockDocRepo) GetEffectiveVersion(ctx context.Context, key, locale string) (*domain.DocumentVersion, error) {
	if m.effective == nil {
		return nil, domain.ErrVersionNotFound
	}
	return m.effective, nil
}
func (m *mockDocRepo) GetUpcomingVersion(ctx context.Context, key, locale string) (*domain.DocumentVersion, error) {
	if m.upcoming == nil {
		return nil, domain.ErrVersionNotFound
	}
	return m.upcoming, nil
}
func (m *mockDocRepo) GetVersionByDisplay(ctx context.Context, key, displayVersion string) (*domain.DocumentVersion, error) {
	v, ok := m.versions[displayVersion]
	if !ok {
		return nil, domain.ErrVersionNotFound
	}
	return v, nil
}
func (m *mockDocRepo) GetVersionByID(ctx context.Context, id uuid.UUID) (*domain.DocumentVersion, error) {
	for _, v := range m.versions {
		if v.ID == id {
			return v, nil
		}
	}
	if m.effective != nil && m.effective.ID == id {
		return m.effective, nil
	}
	return nil, domain.ErrVersionNotFound
}
func (m *mockDocRepo) ListPublishedVersions(ctx context.Context, key string) ([]domain.DocumentVersion, error) {
	return m.versionList, nil
}
func (m *mockDocRepo) CreateDraft(ctx context.Context, version *domain.DocumentVersion) error {
	m.draftCreated = version
	return nil
}
func (m *mockDocRepo) PublishVersion(ctx context.Context, id uuid.UUID) error {
	m.publishedID = id
	return nil
}
func (m *mockDocRepo) RecordAudit(ctx context.Context, audit *domain.PublicationAudit) error {
	m.audits = append(m.audits, *audit)
	return nil
}
func (m *mockDocRepo) ListAudits(ctx context.Context, limit int) ([]domain.PublicationAudit, error) {
	return m.audits, nil
}

type mockAcceptanceRepo struct {
	acceptances map[string]*domain.Acceptance
}

func (m *mockAcceptanceRepo) RecordAcceptance(ctx context.Context, a *domain.Acceptance) error {
	m.acceptances[a.DocumentKey] = a
	return nil
}
func (m *mockAcceptanceRepo) GetLatestAcceptance(ctx context.Context, userID uuid.UUID, documentKey string) (*domain.Acceptance, error) {
	return m.acceptances[documentKey], nil
}
func (m *mockAcceptanceRepo) HasAcceptedVersion(ctx context.Context, userID, versionID uuid.UUID) (bool, error) {
	for _, a := range m.acceptances {
		if a.VersionID == versionID {
			return true, nil
		}
	}
	return false, nil
}

func TestLegalService_GetDocument(t *testing.T) {
	docRepo := &mockDocRepo{
		doc: &domain.Document{Key: "terms", Title: "Terms of Use", Locale: "en"},
		effective: &domain.DocumentVersion{
			ID:             uuid.New(),
			DocumentKey:    "terms",
			DisplayVersion: "1.0.0",
			Title:          "Terms of Use",
			Status:         domain.StatusPublished,
		},
		upcoming: &domain.DocumentVersion{
			ID:             uuid.New(),
			DocumentKey:    "terms",
			DisplayVersion: "1.1.0",
			Title:          "Terms of Use (Upcoming)",
			Status:         domain.StatusPublished,
		},
	}
	acceptanceRepo := &mockAcceptanceRepo{acceptances: make(map[string]*domain.Acceptance)}
	service := NewLegalService(docRepo, acceptanceRepo)

	view, err := service.GetDocument(context.Background(), "terms", "en")
	require.NoError(t, err)
	assert.Equal(t, "terms", view.Document.Key)
	assert.Equal(t, "1.0.0", view.CurrentVersion.DisplayVersion)
	require.NotNil(t, view.UpcomingVersion)
	assert.Equal(t, "1.1.0", view.UpcomingVersion.DisplayVersion)
}

func TestLegalService_RecordAcceptance_ValidAndMismatchedHash(t *testing.T) {
	verID := uuid.New()
	correctHash := "correct-hash-12345"
	docRepo := &mockDocRepo{
		effective: &domain.DocumentVersion{
			ID:          verID,
			DocumentKey: "terms",
			ContentHash: correctHash,
		},
		versions: map[string]*domain.DocumentVersion{},
	}
	acceptanceRepo := &mockAcceptanceRepo{acceptances: make(map[string]*domain.Acceptance)}
	service := NewLegalService(docRepo, acceptanceRepo)

	userID := uuid.New()
	// Test wrong hash rejected
	err := service.RecordAcceptance(context.Background(), userID, "terms", verID, "wrong-hash", "register", "signup_form")
	assert.ErrorIs(t, err, domain.ErrInvalidHash)

	// Test correct hash accepted
	err = service.RecordAcceptance(context.Background(), userID, "terms", verID, correctHash, "register", "signup_form")
	assert.NoError(t, err)
	assert.Equal(t, verID, acceptanceRepo.acceptances["terms"].VersionID)
}

func TestLegalService_GetRequirements(t *testing.T) {
	verTerms := uuid.New()
	now := time.Now().UTC()
	docRepo := &mockDocRepo{
		effective: &domain.DocumentVersion{
			ID:             verTerms,
			DocumentKey:    "terms",
			DisplayVersion: "1.0.0",
			Title:          "Terms of Use",
		},
	}
	acceptanceRepo := &mockAcceptanceRepo{
		acceptances: map[string]*domain.Acceptance{
			"terms": {
				ID:          uuid.New(),
				DocumentKey: "terms",
				VersionID:   verTerms,
				CreatedAt:   now,
			},
		},
	}
	service := NewLegalService(docRepo, acceptanceRepo)

	reqs, err := service.GetRequirements(context.Background(), uuid.New())
	require.NoError(t, err)
	assert.NotEmpty(t, reqs)
	// Terms should be accepted
	var termsReq *domain.Requirement
	for _, r := range reqs {
		if r.DocumentKey == "terms" {
			termsReq = &r
			break
		}
	}
	require.NotNil(t, termsReq)
	assert.True(t, termsReq.Accepted)
}

func TestLegalService_DraftPublishAndAudits(t *testing.T) {
	docRepo := &mockDocRepo{
		versions: make(map[string]*domain.DocumentVersion),
	}
	acceptanceRepo := &mockAcceptanceRepo{
		acceptances: make(map[string]*domain.Acceptance),
	}
	svc := NewLegalService(docRepo, acceptanceRepo)
	ctx := context.Background()
	authorID := uuid.New()

	t.Run("CreateDraft validates empty sections and creates draft", func(t *testing.T) {
		ver := &domain.DocumentVersion{
			DocumentKey:    "terms",
			DisplayVersion: "1.0",
			Sections:       nil,
		}
		err := svc.CreateDraft(ctx, authorID, ver)
		assert.ErrorIs(t, err, domain.ErrEmptySections)

		ver.Sections = []domain.Section{{ID: "s1", Title: "T", Content: "C"}}
		err = svc.CreateDraft(ctx, authorID, ver)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusDraft, ver.Status)
		assert.NotEmpty(t, ver.ContentHash)
		assert.NotEmpty(t, docRepo.audits)
	})

	t.Run("PublishVersion checks published status and records audit", func(t *testing.T) {
		verID := uuid.New()
		docRepo.versions["3.0"] = &domain.DocumentVersion{
			ID:             verID,
			DocumentKey:    "terms",
			DisplayVersion: "3.0",
			Status:         domain.StatusPublished,
		}
		err := svc.PublishVersion(ctx, authorID, verID, "publish notes")
		assert.ErrorIs(t, err, domain.ErrAlreadyPublished)

		docRepo.versions["3.0"].Status = domain.StatusDraft
		err = svc.PublishVersion(ctx, authorID, verID, "publish notes")
		require.NoError(t, err)
		assert.Equal(t, verID, docRepo.publishedID)
	})

	t.Run("GetAudits limits", func(t *testing.T) {
		audits, err := svc.GetAudits(ctx, 0)
		require.NoError(t, err)
		assert.NotNil(t, audits)

		audits, err = svc.GetAudits(ctx, 200)
		require.NoError(t, err)
		assert.NotNil(t, audits)
	})
}

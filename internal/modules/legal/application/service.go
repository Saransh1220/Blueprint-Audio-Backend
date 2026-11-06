package application

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal/domain"
)

type DocumentView struct {
	Document        domain.Document         `json:"document"`
	CurrentVersion  domain.DocumentVersion  `json:"current_version"`
	UpcomingVersion *domain.DocumentVersion `json:"upcoming_version,omitempty"`
}

type LegalService interface {
	GetDocument(ctx context.Context, key, locale string) (*DocumentView, error)
	GetDocumentVersions(ctx context.Context, key string) ([]domain.DocumentVersion, error)
	GetDocumentVersion(ctx context.Context, key, displayVersion string) (*domain.DocumentVersion, error)
	RecordAcceptance(ctx context.Context, userID uuid.UUID, key string, versionID uuid.UUID, hash, action, contextInfo string) error
	GetRequirements(ctx context.Context, userID uuid.UUID) ([]domain.Requirement, error)
	CreateDraft(ctx context.Context, authorID uuid.UUID, version *domain.DocumentVersion) error
	PublishVersion(ctx context.Context, actorID uuid.UUID, versionID uuid.UUID, notes string) error
	GetAudits(ctx context.Context, limit int) ([]domain.PublicationAudit, error)
}

type legalService struct {
	docRepo        domain.DocumentRepository
	acceptanceRepo domain.AcceptanceRepository
}

func NewLegalService(docRepo domain.DocumentRepository, acceptanceRepo domain.AcceptanceRepository) LegalService {
	return &legalService{
		docRepo:        docRepo,
		acceptanceRepo: acceptanceRepo,
	}
}

func (s *legalService) GetDocument(ctx context.Context, key, locale string) (*DocumentView, error) {
	if locale == "" {
		locale = "en"
	}
	doc, err := s.docRepo.GetDocument(ctx, key, locale)
	if err != nil {
		return nil, err
	}

	effectiveVer, err := s.docRepo.GetEffectiveVersion(ctx, key, locale)
	if err != nil {
		return nil, err
	}

	upcomingVer, _ := s.docRepo.GetUpcomingVersion(ctx, key, locale)

	return &DocumentView{
		Document:        *doc,
		CurrentVersion:  *effectiveVer,
		UpcomingVersion: upcomingVer,
	}, nil
}

func (s *legalService) GetDocumentVersions(ctx context.Context, key string) ([]domain.DocumentVersion, error) {
	return s.docRepo.ListPublishedVersions(ctx, key)
}

func (s *legalService) GetDocumentVersion(ctx context.Context, key, displayVersion string) (*domain.DocumentVersion, error) {
	return s.docRepo.GetVersionByDisplay(ctx, key, displayVersion)
}

func (s *legalService) RecordAcceptance(ctx context.Context, userID uuid.UUID, key string, versionID uuid.UUID, hash, action, contextInfo string) error {
	ver, err := s.docRepo.GetVersionByID(ctx, versionID)
	if err != nil {
		return err
	}
	if ver.DocumentKey != key {
		return domain.ErrInvalidState
	}

	if hash != "" && hash != ver.ContentHash {
		return domain.ErrInvalidHash
	}

	acceptance := &domain.Acceptance{
		ID:          uuid.New(),
		UserID:      userID,
		DocumentKey: key,
		VersionID:   versionID,
		ContentHash: ver.ContentHash,
		Action:      action,
		Context:     contextInfo,
		CreatedAt:   time.Now().UTC(),
	}

	return s.acceptanceRepo.RecordAcceptance(ctx, acceptance)
}

func (s *legalService) GetRequirements(ctx context.Context, userID uuid.UUID) ([]domain.Requirement, error) {
	requiredKeys := []string{"terms", "privacy"}
	var requirements []domain.Requirement

	for _, key := range requiredKeys {
		ver, err := s.docRepo.GetEffectiveVersion(ctx, key, "en")
		if err != nil {
			continue
		}

		latestAcceptance, err := s.acceptanceRepo.GetLatestAcceptance(ctx, userID, key)
		var lastAcceptedAt *time.Time
		accepted := false

		if err == nil && latestAcceptance != nil {
			lastAcceptedAt = &latestAcceptance.CreatedAt
			if latestAcceptance.VersionID == ver.ID {
				accepted = true
			}
		}

		requirements = append(requirements, domain.Requirement{
			DocumentKey:    key,
			Title:          ver.Title,
			CurrentVersion: *ver,
			Accepted:       accepted,
			LastAcceptedAt: lastAcceptedAt,
		})
	}

	return requirements, nil
}

func (s *legalService) CreateDraft(ctx context.Context, authorID uuid.UUID, version *domain.DocumentVersion) error {
	if len(version.Sections) == 0 {
		return domain.ErrEmptySections
	}

	version.ID = uuid.New()
	version.CreatedBy = &authorID
	version.CreatedAt = time.Now().UTC()
	version.Status = domain.StatusDraft
	version.ContentHash = domain.ComputeContentHash(version.Title, version.Summary, version.Sections)

	if err := s.docRepo.CreateDraft(ctx, version); err != nil {
		return err
	}

	audit := &domain.PublicationAudit{
		ID:          uuid.New(),
		DocumentKey: version.DocumentKey,
		VersionID:   version.ID,
		Action:      "create_draft",
		ActorID:     &authorID,
		Notes:       fmt.Sprintf("Created draft version %s", version.DisplayVersion),
		CreatedAt:   time.Now().UTC(),
	}
	_ = s.docRepo.RecordAudit(ctx, audit)

	return nil
}

func (s *legalService) PublishVersion(ctx context.Context, actorID uuid.UUID, versionID uuid.UUID, notes string) error {
	ver, err := s.docRepo.GetVersionByID(ctx, versionID)
	if err != nil {
		return err
	}
	if ver.Status == domain.StatusPublished {
		return domain.ErrAlreadyPublished
	}

	if err := s.docRepo.PublishVersion(ctx, versionID); err != nil {
		return err
	}

	audit := &domain.PublicationAudit{
		ID:          uuid.New(),
		DocumentKey: ver.DocumentKey,
		VersionID:   versionID,
		Action:      "publish",
		ActorID:     &actorID,
		Notes:       notes,
		CreatedAt:   time.Now().UTC(),
	}
	_ = s.docRepo.RecordAudit(ctx, audit)

	return nil
}

func (s *legalService) GetAudits(ctx context.Context, limit int) ([]domain.PublicationAudit, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.docRepo.ListAudits(ctx, limit)
}

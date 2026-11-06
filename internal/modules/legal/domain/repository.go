package domain

import (
	"context"

	"github.com/google/uuid"
)

type DocumentRepository interface {
	GetDocument(ctx context.Context, key, locale string) (*Document, error)
	GetEffectiveVersion(ctx context.Context, key, locale string) (*DocumentVersion, error)
	GetUpcomingVersion(ctx context.Context, key, locale string) (*DocumentVersion, error)
	GetVersionByDisplay(ctx context.Context, key, displayVersion string) (*DocumentVersion, error)
	GetVersionByID(ctx context.Context, id uuid.UUID) (*DocumentVersion, error)
	ListPublishedVersions(ctx context.Context, key string) ([]DocumentVersion, error)
	CreateDraft(ctx context.Context, version *DocumentVersion) error
	PublishVersion(ctx context.Context, id uuid.UUID) error
	RecordAudit(ctx context.Context, audit *PublicationAudit) error
	ListAudits(ctx context.Context, limit int) ([]PublicationAudit, error)
}

type AcceptanceRepository interface {
	RecordAcceptance(ctx context.Context, acceptance *Acceptance) error
	GetLatestAcceptance(ctx context.Context, userID uuid.UUID, documentKey string) (*Acceptance, error)
	HasAcceptedVersion(ctx context.Context, userID, versionID uuid.UUID) (bool, error)
}

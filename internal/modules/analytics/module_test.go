package analytics_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/analytics"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type specRepoStub struct{}

func (s *specRepoStub) Create(ctx context.Context, spec *catalogDomain.Spec) error { return nil }
func (s *specRepoStub) GetByID(ctx context.Context, id uuid.UUID) (*catalogDomain.Spec, error) {
	return &catalogDomain.Spec{ID: id}, nil
}
func (s *specRepoStub) GetByIDSystem(ctx context.Context, id uuid.UUID) (*catalogDomain.Spec, error) {
	return &catalogDomain.Spec{ID: id}, nil
}
func (s *specRepoStub) List(ctx context.Context, filter catalogDomain.SpecFilter) ([]catalogDomain.Spec, int, error) {
	return nil, 0, nil
}
func (s *specRepoStub) Update(ctx context.Context, spec *catalogDomain.Spec) error { return nil }
func (s *specRepoStub) Delete(ctx context.Context, id uuid.UUID, producerID uuid.UUID) error {
	return nil
}
func (s *specRepoStub) ListByUserID(ctx context.Context, producerID uuid.UUID, limit, offset int) ([]catalogDomain.Spec, int, error) {
	return nil, 0, nil
}

type fileSvcStub struct{}

func (f *fileSvcStub) GetPresignedDownloadURL(ctx context.Context, key string, filename string, expiration time.Duration) (string, error) {
	return "signed", nil
}
func (f *fileSvcStub) GetKeyFromUrl(url string) (string, error) { return "k", nil }

func TestNewModule(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer sqlDB.Close()

	db := sqlx.NewDb(sqlDB, "sqlmock")
	m := analytics.NewModule(db, &specRepoStub{}, &fileSvcStub{})
	assert.NotNil(t, m)
	assert.NotNil(t, m.AnalyticsService)
	assert.NotNil(t, m.AnalyticsHandler)
}


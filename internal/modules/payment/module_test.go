package payment

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	catalogDomain "github.com/saransh1220/blueprint-audio/internal/modules/catalog/domain"
	sharedemail "github.com/saransh1220/blueprint-audio/internal/shared/infrastructure/email"
	"github.com/stretchr/testify/require"
)

type nilSpecFinder struct{}

func (nilSpecFinder) FindByID(_ context.Context, _ uuid.UUID) (*catalogDomain.Spec, error) {
	return nil, nil
}
func (nilSpecFinder) FindByIDIncludingDeleted(_ context.Context, _ uuid.UUID) (*catalogDomain.Spec, error) {
	return nil, nil
}
func (nilSpecFinder) FindWithLicenses(_ context.Context, _ uuid.UUID) (*catalogDomain.Spec, error) {
	return &catalogDomain.Spec{}, nil
}
func (nilSpecFinder) Exists(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil }
func (nilSpecFinder) GetLicenseByID(_ context.Context, _ uuid.UUID) (*catalogDomain.LicenseOption, error) {
	return nil, nil
}

type nilFileService struct{}

func (nilFileService) GetKeyFromUrl(_ string) (string, error) { return "", nil }
func (nilFileService) GetPresignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", nil
}

type nilUserFinder struct{}

func (nilUserFinder) FindByID(_ context.Context, id uuid.UUID) (*authDomain.User, error) {
	return &authDomain.User{ID: id}, nil
}
func (nilUserFinder) Exists(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil }

func TestModuleAccessors(t *testing.T) {
	m := NewModule(&sqlx.DB{}, nilSpecFinder{}, nilUserFinder{}, nilFileService{}, sharedemail.NewSender(sharedemail.Config{}), "http://localhost:4200")
	require.NotNil(t, m)
	require.NotNil(t, m.HTTPHandler())
}

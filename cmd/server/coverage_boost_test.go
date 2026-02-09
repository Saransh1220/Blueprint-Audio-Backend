package main

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/saransh1220/blueprint-audio/internal/mocks"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

func TestCoverageBoost_AuthAndDTO(t *testing.T) {
	ctx := context.Background()
	repo := new(mocks.MockUserRepository)
	svc := service.NewAuthService(repo, "secret", time.Hour)

	repo.On("CreateUser", ctx, mock.AnythingOfType("*domain.User")).Return(nil).Once()
	user, err := svc.RegisterUser(ctx, service.RegisterUserReq{
		Email:       "boost@example.com",
		Password:    "password123",
		Name:        "Boost User",
		DisplayName: "Boost",
		Role:        "artist",
	})
	assert.NoError(t, err)
	assert.NotNil(t, user)

	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	assert.NoError(t, err)
	repo.On("GetUserByEmail", ctx, "boost@example.com").Return(&domain.User{
		ID:           uuid.New(),
		Email:        "boost@example.com",
		PasswordHash: string(hash),
		Role:         domain.RoleArtist,
	}, nil).Once()
	token, err := svc.LoginUser(ctx, service.LoginUserReq{Email: "boost@example.com", Password: "password123"})
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	spec := &domain.Spec{
		ID:             uuid.New(),
		ProducerID:     uuid.New(),
		ProducerName:   "Producer",
		Title:          "Track",
		Category:       domain.CategoryBeat,
		Type:           "WAV",
		BPM:            120,
		Key:            "C",
		ImageUrl:       "img",
		PreviewUrl:     "preview",
		BasePrice:      10,
		Duration:       120,
		FreeMp3Enabled: true,
		Tags:           pq.StringArray{"trap"},
		Licenses: []domain.LicenseOption{
			{ID: uuid.New(), SpecID: uuid.New(), LicenseType: domain.LicenseBasic, Name: "Basic", Price: 10},
		},
		Genres: []domain.Genre{
			{ID: uuid.New(), Name: "Hip Hop", Slug: "hip-hop"},
		},
		Analytics: &domain.SpecAnalytics{
			PlayCount:         5,
			FavoriteCount:     2,
			FreeDownloadCount: 1,
			IsFavorited:       true,
		},
	}
	resp := dto.ToSpecResponse(spec)
	assert.Equal(t, "Producer", resp.ProducerName)
	assert.NotNil(t, resp.Analytics)
	assert.Len(t, resp.Licenses, 1)
	assert.Len(t, resp.Genres, 1)
}


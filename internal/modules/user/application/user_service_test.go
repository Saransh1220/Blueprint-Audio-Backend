package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/user/application"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockUserRepo struct{ mock.Mock }

func (m *mockUserRepo) Create(ctx context.Context, user *authDomain.User) error { return nil }
func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*authDomain.User, error) {
	return nil, nil
}
func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*authDomain.User, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*authDomain.User), args.Error(1)
}
func (m *mockUserRepo) UpdateProfile(ctx context.Context, id uuid.UUID, bio *string, avatarUrl *string, displayName *string, instagramURL, twitterURL, youtubeURL, spotifyURL *string) error {
	args := m.Called(ctx, id, bio, avatarUrl, displayName, instagramURL, twitterURL, youtubeURL, spotifyURL)
	return args.Error(0)
}

func TestUserService_UpdateProfile(t *testing.T) {
	ctx := context.Background()
	repo := new(mockUserRepo)
	svc := application.NewUserService(repo)
	id := uuid.New()
	bio := "hello"
	req := application.UpdateProfileRequest{Bio: &bio}

	repo.On("UpdateProfile", ctx, id, req.Bio, req.AvatarURL, req.DisplayName, req.InstagramURL, req.TwitterURL, req.YoutubeURL, req.SpotifyURL).Return(nil).Once()
	err := svc.UpdateProfile(ctx, id, req)
	assert.NoError(t, err)
}

func TestUserService_GetPublicProfile(t *testing.T) {
	ctx := context.Background()
	repo := new(mockUserRepo)
	svc := application.NewUserService(repo)
	id := uuid.New()
	now := time.Now().UTC()
	display := "Producer Alias"
	user := &authDomain.User{ID: id, Name: "N", DisplayName: &display, Role: authDomain.RoleProducer, CreatedAt: now}

	repo.On("GetByID", ctx, id).Return(user, nil).Once()
	profile, err := svc.GetPublicProfile(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, id.String(), profile.ID)
	assert.Equal(t, "producer", profile.Role)
	assert.NotNil(t, profile.DisplayName)
	assert.Equal(t, "Producer Alias", *profile.DisplayName)

	id2 := uuid.New()
	repo.On("GetByID", ctx, id2).Return(nil, nil).Once()
	_, err = svc.GetPublicProfile(ctx, id2)
	assert.EqualError(t, err, "user not found")

	id3 := uuid.New()
	repo.On("GetByID", ctx, id3).Return(nil, errors.New("db")).Once()
	_, err = svc.GetPublicProfile(ctx, id3)
	assert.EqualError(t, err, "db")
}


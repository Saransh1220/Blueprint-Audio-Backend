package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
	"github.com/saransh1220/blueprint-audio/internal/mocks"
	"github.com/saransh1220/blueprint-audio/internal/service"
	"github.com/stretchr/testify/assert"
)

func TestUserService_UpdateProfile(t *testing.T) {
	ctx := context.Background()
	repo := new(mocks.MockUserRepository)
	svc := service.NewUserService(repo)
	id := uuid.New()
	bio := "hello"
	req := dto.UpdateProfileRequest{Bio: &bio}

	repo.On("UpdateProfile", ctx, id, req.Bio, req.AvatarURL, req.InstagramURL, req.TwitterURL, req.YoutubeURL, req.SpotifyURL).Return(nil)
	err := svc.UpdateProfile(ctx, id, req)
	assert.NoError(t, err)
}

func TestUserService_GetPublicProfile(t *testing.T) {
	ctx := context.Background()
	repo := new(mocks.MockUserRepository)
	svc := service.NewUserService(repo)
	id := uuid.New()
	now := time.Now().UTC()
	user := &domain.User{ID: id, Name: "N", Role: domain.RoleProducer, CreatedAt: now}

	repo.On("GetUserById", ctx, id).Return(user, nil)
	profile, err := svc.GetPublicProfile(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, id.String(), profile.ID)
	assert.Equal(t, "producer", profile.Role)

	id2 := uuid.New()
	repo.On("GetUserById", ctx, id2).Return(nil, nil)
	_, err = svc.GetPublicProfile(ctx, id2)
	assert.EqualError(t, err, "user not found")

	id3 := uuid.New()
	repo.On("GetUserById", ctx, id3).Return(nil, errors.New("db"))
	_, err = svc.GetPublicProfile(ctx, id3)
	assert.EqualError(t, err, "db")
}

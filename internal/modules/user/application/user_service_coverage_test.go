package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/user/application"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func stringPointer(value string) *string { return &value }

func TestUserServiceUpdateProfileValidationAndNormalization(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()

	invalid := []struct {
		name string
		req  application.UpdateProfileRequest
		want string
	}{
		{"display name", application.UpdateProfileRequest{DisplayName: stringPointer(strings.Repeat("x", 51))}, "display name must be at most 50 characters"},
		{"bio", application.UpdateProfileRequest{Bio: stringPointer(strings.Repeat("x", 501))}, "bio must be at most 500 characters"},
		{"long social URL", application.UpdateProfileRequest{InstagramURL: stringPointer("https://example.com/" + strings.Repeat("x", 240))}, "social url is too long"},
		{"malformed social URL", application.UpdateProfileRequest{TwitterURL: stringPointer("not-a-url")}, "social urls must be valid http(s) urls"},
		{"unsupported scheme", application.UpdateProfileRequest{YoutubeURL: stringPointer("ftp://example.com/file")}, "social urls must be valid http(s) urls"},
	}

	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(mockUserRepo)
			err := application.NewUserService(repo).UpdateProfile(ctx, userID, tt.req)
			require.EqualError(t, err, tt.want)
			repo.AssertNotCalled(t, "UpdateProfile", mock.Anything)
		})
	}

	repo := new(mockUserRepo)
	currency := " inr "
	social := "https://example.com/profile"
	repo.On("UpdateProfile", ctx, userID, (*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil), (*string)(nil), &social, stringPointer("INR")).Return(nil).Once()
	err := application.NewUserService(repo).UpdateProfile(ctx, userID, application.UpdateProfileRequest{
		SpotifyURL:    &social,
		StoreCurrency: &currency,
	})
	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestUserServicePublicProfileMapsAllFields(t *testing.T) {
	ctx := context.Background()
	userID := uuid.New()
	repo := new(mockUserRepo)
	display := "Display"
	bio := "Bio"
	avatar := "avatar"
	banner := "banner"
	instagram := "instagram"
	twitter := "twitter"
	youtube := "youtube"
	spotify := "spotify"
	createdAt := time.Date(2026, 7, 19, 10, 11, 12, 0, time.UTC)
	repo.On("GetByID", ctx, userID).Return(&authDomain.User{
		ID: userID, Name: "Name", DisplayName: &display, Role: authDomain.RoleArtist,
		Bio: &bio, AvatarUrl: &avatar, BannerURL: &banner, InstagramURL: &instagram,
		TwitterURL: &twitter, YoutubeURL: &youtube, SpotifyURL: &spotify,
		StoreCurrency: authDomain.CurrencyINR, CreatedAt: createdAt,
	}, nil).Once()

	profile, err := application.NewUserService(repo).GetPublicProfile(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, "Name", profile.Name)
	assert.Equal(t, "INR", profile.StoreCurrency)
	assert.Equal(t, createdAt.Format(time.RFC3339), profile.CreatedAt)
	assert.Equal(t, &banner, profile.BannerURL)
	assert.Equal(t, &spotify, profile.SpotifyURL)
}

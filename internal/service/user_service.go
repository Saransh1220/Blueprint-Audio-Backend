package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/dto"
)

type UserService interface {
	UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) error
	GetPublicProfile(ctx context.Context, userID uuid.UUID) (*dto.PublicUserResponse, error)
}

type userService struct {
	repo domain.UserRepository
}

func NewUserService(repo domain.UserRepository) UserService {
	return &userService{repo: repo}
}

// UpdateProfile updates a user's profile information
func (s *userService) UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) error {
	return s.repo.UpdateProfile(ctx, userID, req.Bio, req.AvatarURL, req.DisplayName, req.InstagramURL, req.TwitterURL, req.YoutubeURL, req.SpotifyURL)
}

// GetPublicProfile retrieves a user's public profile information
func (s *userService) GetPublicProfile(ctx context.Context, userID uuid.UUID) (*dto.PublicUserResponse, error) {
	user, err := s.repo.GetUserById(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	return &dto.PublicUserResponse{
		ID:           user.ID.String(),
		Name:         user.Name,
		DisplayName:  user.DisplayName,
		Role:         string(user.Role),
		Bio:          user.Bio,
		AvatarURL:    user.AvatarUrl,
		InstagramURL: user.InstagramURL,
		TwitterURL:   user.TwitterURL,
		YoutubeURL:   user.YoutubeURL,
		SpotifyURL:   user.SpotifyURL,
		CreatedAt:    user.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

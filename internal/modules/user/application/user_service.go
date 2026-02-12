package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	authDomain "github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
)

type UserService struct {
	repo authDomain.UserRepository
}

func NewUserService(repo authDomain.UserRepository) *UserService {
	return &UserService{repo: repo}
}

// UpdateProfile updates a user's profile information
func (s *UserService) UpdateProfile(ctx context.Context, userID uuid.UUID, req UpdateProfileRequest) error {
	return s.repo.UpdateProfile(
		ctx,
		userID,
		req.Bio,
		req.AvatarURL,
		req.DisplayName,
		req.InstagramURL,
		req.TwitterURL,
		req.YoutubeURL,
		req.SpotifyURL,
	)
}

// GetPublicProfile retrieves a user's public profile information
func (s *UserService) GetPublicProfile(ctx context.Context, userID uuid.UUID) (*PublicUserResponse, error) {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	return &PublicUserResponse{
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

package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type UserRole string

const (
	RoleArtist   UserRole = "artist"
	RoleProducer UserRole = "producer"
)

// User represents a user in the system
type User struct {
	ID           uuid.UUID `json:"id" db:"id"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"`
	Name         string    `json:"name" db:"name"`
	DisplayName  *string   `json:"display_name" db:"display_name"`
	Role         UserRole  `json:"role" db:"role"`
	Bio          *string   `json:"bio" db:"bio"`
	AvatarUrl    *string   `json:"avatar_url" db:"avatar_url"`
	InstagramURL *string   `json:"instagram_url" db:"instagram_url"`
	TwitterURL   *string   `json:"twitter_url" db:"twitter_url"`
	YoutubeURL   *string   `json:"youtube_url" db:"youtube_url"`
	SpotifyURL   *string   `json:"spotify_url" db:"spotify_url"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type UserRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserById(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, bio *string, avatarUrl *string, displayName *string, instagramURL, twitterURL, youtubeURL, spotifyURL *string) error
}

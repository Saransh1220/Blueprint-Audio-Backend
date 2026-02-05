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
	Role         UserRole  `json:"role" db:"role"`
	Bio          *string   `json:"bio,omitempty" db:"bio"`
	InstagramURL *string   `json:"instagram_url,omitempty" db:"instagram_url"`
	TwitterURL   *string   `json:"twitter_url,omitempty" db:"twitter_url"`
	YoutubeURL   *string   `json:"youtube_url,omitempty" db:"youtube_url"`
	SpotifyURL   *string   `json:"spotify_url,omitempty" db:"spotify_url"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type UserRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserById(ctx context.Context, id uuid.UUID) (*User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, bio *string, instagramURL, twitterURL, youtubeURL, spotifyURL *string) error
}

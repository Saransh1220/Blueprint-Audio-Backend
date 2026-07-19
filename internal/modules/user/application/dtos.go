package application

// UpdateProfileRequest represents the request body for updating a user's profile
type UpdateProfileRequest struct {
	Bio           *string `json:"bio,omitempty"`
	AvatarURL     *string `json:"avatar_url,omitempty"`
	BannerURL     *string `json:"banner_url,omitempty"`
	DisplayName   *string `json:"display_name,omitempty"`
	InstagramURL  *string `json:"instagram_url,omitempty"`
	TwitterURL    *string `json:"twitter_url,omitempty"`
	YoutubeURL    *string `json:"youtube_url,omitempty"`
	SpotifyURL    *string `json:"spotify_url,omitempty"`
	StoreCurrency *string `json:"store_currency,omitempty"`
}

// PublicUserResponse represents a user's public profile information
type PublicUserResponse struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	DisplayName   *string `json:"display_name,omitempty"`
	Role          string  `json:"role"`
	Bio           *string `json:"bio,omitempty"`
	AvatarURL     *string `json:"avatar_url,omitempty"`
	BannerURL     *string `json:"banner_url,omitempty"`
	InstagramURL  *string `json:"instagram_url,omitempty"`
	TwitterURL    *string `json:"twitter_url,omitempty"`
	YoutubeURL    *string `json:"youtube_url,omitempty"`
	SpotifyURL    *string `json:"spotify_url,omitempty"`
	StoreCurrency string  `json:"store_currency"`
	CreatedAt     string  `json:"created_at"`
}

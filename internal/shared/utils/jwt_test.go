package utils

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidateToken_Success(t *testing.T) {
	userID := uuid.New()
	email := "test@example.com"
	role := "user"
	secret := "test-secret"

	// Generate token
	token, err := GenerateToken(userID, email, role, secret, 1*time.Hour)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// Validate token
	claims, err := ValidateToken(token, secret)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, email, claims.Email)
	assert.Equal(t, role, claims.Role)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	userID := uuid.New()
	token, err := GenerateToken(userID, "test@example.com", "user", "secret1", 1*time.Hour)
	require.NoError(t, err)

	// Try to validate with wrong secret
	_, err = ValidateToken(token, "secret2")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidToken, err)
}

func TestValidateToken_Malformed(t *testing.T) {
	_, err := ValidateToken("malformed-token", "secret")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidToken, err)
}

func TestValidateToken_Empty(t *testing.T) {
	_, err := ValidateToken("", "secret")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidToken, err)
}

func TestGenerateToken_DifferentRoles(t *testing.T) {
	roles := []string{"user", "artist", "admin"}
	userID := uuid.New()
	secret := "test-secret"

	for _, role := range roles {
		token, err := GenerateToken(userID, "test@example.com", role, secret, 1*time.Hour)
		require.NoError(t, err)

		claims, err := ValidateToken(token, secret)
		require.NoError(t, err)
		assert.Equal(t, role, claims.Role)
	}
}

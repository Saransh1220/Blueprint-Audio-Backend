package utils_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidateToken_Success(t *testing.T) {
	userID := uuid.New()
	token, err := utils.GenerateToken("secret", time.Hour, userID, "producer")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := utils.ValidateToken(token, "secret")
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, "producer", claims.Role)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	token, err := utils.GenerateToken("secret", time.Hour, uuid.New(), "artist")
	require.NoError(t, err)

	_, err = utils.ValidateToken(token, "wrong-secret")
	require.Error(t, err)
}

func TestValidateToken_Malformed(t *testing.T) {
	_, err := utils.ValidateToken("not-a-token", "secret")
	require.Error(t, err)
}

func TestValidateToken_InvalidSigningMethod(t *testing.T) {
	claims := utils.CustomClaims{
		UserID: uuid.New(),
		Role:   "artist",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStr, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = utils.ValidateToken(tokenStr, "secret")
	require.Error(t, err)
}

package jwt

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidateToken(t *testing.T) {
	secret := "secret"
	uid := uuid.New()
	tok, err := GenerateToken(secret, time.Hour, uid, "producer")
	require.NoError(t, err)

	claims, err := ValidateToken(tok, secret)
	require.NoError(t, err)
	require.Equal(t, uid, claims.UserID)
	require.Equal(t, "producer", claims.Role)

	_, err = ValidateToken(tok, "wrong")
	require.Error(t, err)

	_, err = ValidateToken("not-a-token", secret)
	require.Error(t, err)
}

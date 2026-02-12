package jwt

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type CustomClaims struct {
	UserID uuid.UUID `json:"user_id"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken creates a JWT token string signed with the provided secret.
// The token includes custom claims for the user's UUID and role, and standard
// registered claims for expiration, issued at, and not before times.
// The duration parameter specifies how long the token is valid.
// Returns the signed JWT token string or an error if signing fails.
func GenerateToken(secret string, duration time.Duration, userID uuid.UUID, role string) (string, error) {
	claims := CustomClaims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken validates a JWT token string using the provided secret key.
// It parses the token with custom claims and verifies the HMAC signing method.
// Returns the validated CustomClaims if the token is valid, or an error if validation fails.
// Errors can include invalid signature, malformed token, or parsing errors.
func ValidateToken(tokenStr string, secret string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrTokenMalformed
}

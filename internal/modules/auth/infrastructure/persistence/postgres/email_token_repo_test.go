package postgres_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
)

func digest(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func TestPgEmailActionTokenRepository_Create(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewEmailActionTokenRepository(db)
	ctx := context.Background()
	token := &domain.EmailActionToken{
		UserID:  uuid.New(),
		Email:   "test@example.com",
		Purpose: domain.TokenPurposeVerifyEmail,
		Code:    "123456",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}

	mock.ExpectExec("INSERT INTO email_action_tokens").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(ctx, token)
	assert.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, token.ID)
	assert.False(t, token.CreatedAt.IsZero())
	assert.False(t, token.UpdatedAt.IsZero())
	
	expectedDigest := digest("123456")
	assert.Equal(t, expectedDigest, token.CodeDigest)
}

func TestPgEmailActionTokenRepository_Consume_Success(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewEmailActionTokenRepository(db)
	ctx := context.Background()

	email := "test@example.com"
	purpose := domain.TokenPurposeVerifyEmail
	code := "123456"
	id := uuid.New()
	userID := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "user_id", "email", "purpose", "code_digest", "expires_at", "consumed_at", "created_at", "updated_at"}).
		AddRow(id, userID, email, purpose, digest(code), time.Now().Add(time.Hour), time.Now(), time.Now(), time.Now())

	mock.ExpectQuery("UPDATE email_action_tokens SET consumed_at = NOW\\(\\), updated_at = NOW\\(\\)").
		WithArgs(email, purpose, digest(code)).
		WillReturnRows(rows)

	token, err := repo.Consume(ctx, email, purpose, code)
	assert.NoError(t, err)
	assert.NotNil(t, token)
	assert.Equal(t, id, token.ID)
}

func TestPgEmailActionTokenRepository_Consume_InvalidOrExpired(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewEmailActionTokenRepository(db)
	ctx := context.Background()

	mock.ExpectQuery("UPDATE email_action_tokens").
		WillReturnError(sql.ErrNoRows)

	token, err := repo.Consume(ctx, "test@example.com", domain.TokenPurposeVerifyEmail, "wrong")
	assert.Equal(t, domain.ErrInvalidOrExpiredCode, err)
	assert.Nil(t, token)
}

func TestPgEmailActionTokenRepository_Consume_DBError(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewEmailActionTokenRepository(db)
	ctx := context.Background()

	mock.ExpectQuery("UPDATE email_action_tokens").
		WillReturnError(errors.New("db error"))

	token, err := repo.Consume(ctx, "test@example.com", domain.TokenPurposeVerifyEmail, "123456")
	assert.EqualError(t, err, "db error")
	assert.Nil(t, token)
}

func TestPgEmailActionTokenRepository_InvalidateActive(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewEmailActionTokenRepository(db)
	ctx := context.Background()
	userID := uuid.New()

	mock.ExpectExec("UPDATE email_action_tokens SET consumed_at = NOW\\(\\), updated_at = NOW\\(\\)").
		WithArgs(userID, domain.TokenPurposeVerifyEmail).
		WillReturnResult(sqlmock.NewResult(0, 2))

	err := repo.InvalidateActive(ctx, userID, domain.TokenPurposeVerifyEmail)
	assert.NoError(t, err)
}

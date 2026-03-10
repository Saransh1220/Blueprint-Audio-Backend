package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPgSessionRepository_Create_GeneratesUUIDv7(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()

	repo := postgres.NewSessionRepository(db)
	session := &domain.UserSession{
		UserID:       uuid.New(),
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}

	mock.ExpectExec("INSERT INTO user_sessions").WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(context.Background(), session)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, session.ID)
	assert.Equal(t, uuid.Version(7), session.ID.Version())
}

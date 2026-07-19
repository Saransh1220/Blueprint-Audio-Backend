package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/require"
)

func TestPgSessionRepositoryMissingAndErrorBranches(t *testing.T) {
	db, mockDB, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewSessionRepository(db)
	ctx := context.Background()

	mockDB.ExpectQuery("SELECT \\* FROM user_sessions").WithArgs(sqlmock.AnyArg()).WillReturnError(sql.ErrNoRows)
	session, err := repo.GetByToken(ctx, "missing")
	require.NoError(t, err)
	require.Nil(t, session)

	mockDB.ExpectQuery("SELECT \\* FROM user_sessions").WithArgs(sqlmock.AnyArg()).WillReturnError(errors.New("read failed"))
	session, err = repo.GetByToken(ctx, "error")
	require.EqualError(t, err, "read failed")
	require.Nil(t, session)

	userID := uuid.New()
	mockDB.ExpectExec("UPDATE user_sessions SET is_revoked = true").
		WithArgs(sqlmock.AnyArg(), userID).WillReturnResult(sqlmock.NewResult(0, 2))
	require.NoError(t, repo.RevokeAllForUser(ctx, userID))

	mockDB.ExpectExec("UPDATE user_sessions SET is_revoked = true").
		WithArgs(sqlmock.AnyArg(), userID).WillReturnError(errors.New("write failed"))
	require.EqualError(t, repo.RevokeAllForUser(ctx, userID), "write failed")
	require.NoError(t, mockDB.ExpectationsWereMet())
}

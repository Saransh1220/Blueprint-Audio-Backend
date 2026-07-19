package postgres_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/require"
)

func TestPgUserRepositoryAccountMutations(t *testing.T) {
	db, mockDB, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()
	userID := uuid.New()

	tests := []struct {
		name    string
		pattern string
		args    []driver.Value
		call    func() error
	}{
		{"mark verified", "UPDATE users SET email_verified", []driver.Value{userID}, func() error { return repo.MarkEmailVerified(ctx, userID) }},
		{"password", "UPDATE users SET password_hash", []driver.Value{"hash", userID}, func() error { return repo.UpdatePassword(ctx, userID, "hash") }},
		{"system role", "UPDATE users SET system_role", []driver.Value{domain.SystemRoleSuperAdmin, userID}, func() error { return repo.UpdateSystemRole(ctx, userID, domain.SystemRoleSuperAdmin) }},
		{"status", "UPDATE users SET status", []driver.Value{domain.UserStatusSuspended, userID}, func() error { return repo.UpdateStatus(ctx, userID, domain.UserStatusSuspended) }},
	}

	for _, tt := range tests {
		t.Run(tt.name+" success", func(t *testing.T) {
			expectation := mockDB.ExpectExec(tt.pattern)
			expectation.WithArgs(tt.args...).WillReturnResult(sqlmock.NewResult(0, 1))
			require.NoError(t, tt.call())
		})
		t.Run(tt.name+" missing", func(t *testing.T) {
			expectation := mockDB.ExpectExec(tt.pattern)
			expectation.WithArgs(tt.args...).WillReturnResult(sqlmock.NewResult(0, 0))
			require.ErrorIs(t, tt.call(), domain.ErrUserNotFound)
		})
		t.Run(tt.name+" database error", func(t *testing.T) {
			expectation := mockDB.ExpectExec(tt.pattern)
			expectation.WithArgs(tt.args...).WillReturnError(errors.New("database unavailable"))
			require.EqualError(t, tt.call(), "database unavailable")
		})
	}

	mockDB.ExpectQuery("SELECT COUNT\\(\\*\\) FROM users WHERE system_role").WithArgs(domain.SystemRoleSuperAdmin).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	count, err := repo.CountBySystemRole(ctx, domain.SystemRoleSuperAdmin)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestPgUserRepositoryBootstrapSuperAdmin(t *testing.T) {
	db, mockDB, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	require.NoError(t, repo.BootstrapSuperAdmin(ctx, "  "))

	mockDB.ExpectExec("UPDATE users SET system_role").WithArgs(domain.SystemRoleSuperAdmin, "admin@example.com").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, repo.BootstrapSuperAdmin(ctx, " Admin@Example.COM "))

	mockDB.ExpectExec("UPDATE users SET system_role").WithArgs(domain.SystemRoleSuperAdmin, "missing@example.com").
		WillReturnResult(sqlmock.NewResult(0, 0))
	require.EqualError(t, repo.BootstrapSuperAdmin(ctx, "missing@example.com"), "no user found for email missing@example.com")

	mockDB.ExpectExec("UPDATE users SET system_role").WithArgs(domain.SystemRoleSuperAdmin, "error@example.com").
		WillReturnError(errors.New("write failed"))
	require.EqualError(t, repo.BootstrapSuperAdmin(ctx, "error@example.com"), "write failed")
	require.NoError(t, mockDB.ExpectationsWereMet())
}

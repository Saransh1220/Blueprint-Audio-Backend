package postgres_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockDB(t *testing.T) (*sqlx.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	return sqlx.NewDb(sqlDB, "sqlmock"), mock, func() { _ = sqlDB.Close() }
}

func TestPgUserRepository_CreateAndGets(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	u := &domain.User{ID: uuid.New(), Email: "a@a.com", PasswordHash: "hash", Name: "A", Role: domain.RoleArtist}
	mock.ExpectExec("INSERT INTO users").WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, repo.Create(ctx, u))

	mock.ExpectExec("INSERT INTO users").WillReturnError(&pq.Error{Code: "23505"})
	err := repo.Create(ctx, u)
	assert.ErrorIs(t, err, domain.ErrUserAlreadyExists)

	rows := sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "role"}).AddRow(u.ID, u.Email, u.PasswordHash, u.Name, u.Role)
	mock.ExpectQuery(`SELECT \* FROM users WHERE email = \$1`).WithArgs(u.Email).WillReturnRows(rows)
	got, err := repo.GetByEmail(ctx, u.Email)
	require.NoError(t, err)
	require.NotNil(t, got)

	mock.ExpectQuery(`SELECT \* FROM users WHERE email = \$1`).WithArgs("missing@x.com").WillReturnError(sql.ErrNoRows)
	got, err = repo.GetByEmail(ctx, "missing@x.com")
	require.NoError(t, err)
	assert.Nil(t, got)

	idRows := sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "role"}).AddRow(u.ID, u.Email, u.PasswordHash, u.Name, u.Role)
	mock.ExpectQuery(`SELECT \* FROM users WHERE id = \$1`).WithArgs(u.ID).WillReturnRows(idRows)
	got, err = repo.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	missingID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM users WHERE id = \$1`).WithArgs(missingID).WillReturnError(sql.ErrNoRows)
	got, err = repo.GetByID(ctx, missingID)
	require.ErrorIs(t, err, domain.ErrUserNotFound)
	assert.Nil(t, got)

	findID := uuid.New()
	mock.ExpectQuery(`SELECT \* FROM users WHERE id = \$1`).WithArgs(findID).WillReturnError(assert.AnError)
	_, err = repo.FindByID(ctx, findID)
	require.Error(t, err)
}

func TestPgUserRepository_ExistsAndUpdateProfile(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()
	id := uuid.New()

	mock.ExpectQuery(`SELECT EXISTS\(SELECT 1 FROM users WHERE id = \$1\)`).WithArgs(id).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	exists, err := repo.Exists(ctx, id)
	require.NoError(t, err)
	assert.True(t, exists)

	err = repo.UpdateProfile(ctx, id, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)

	bio := "bio"
	display := "display"
	avatar := "avatar"
	instagram := "ig"
	twitter := "tw"
	youtube := "yt"
	spotify := "sp"

	mock.ExpectExec("UPDATE users SET").WithArgs(&bio, &display, &avatar, &instagram, &twitter, &youtube, &spotify, sqlmock.AnyArg(), id).WillReturnResult(sqlmock.NewResult(0, 1))
	err = repo.UpdateProfile(ctx, id, &bio, &avatar, &display, &instagram, &twitter, &youtube, &spotify)
	require.NoError(t, err)

	mock.ExpectExec("UPDATE users SET").WillReturnError(assert.AnError)
	err = repo.UpdateProfile(ctx, id, &bio, nil, nil, nil, nil, nil, nil)
	require.Error(t, err)
}

package repository_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/saransh1220/blueprint-audio/internal/domain"
	"github.com/saransh1220/blueprint-audio/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPGUserRepository_CreateUser(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewUserRepository(db)

	user := &domain.User{ID: uuid.New(), Email: "a@a.com", PasswordHash: "hash", Name: "A", Role: domain.RoleArtist}

	mock.ExpectExec("INSERT INTO users").
		WillReturnResult(sqlmock.NewResult(1, 1))
	err := repo.CreateUser(context.Background(), user)
	require.NoError(t, err)

	mock.ExpectExec("INSERT INTO users").
		WillReturnError(&pq.Error{Code: "23505"})
	err = repo.CreateUser(context.Background(), user)
	assert.ErrorIs(t, err, domain.ErrUserAlreadyExists)
}

func TestPGUserRepository_GetUserByEmailAndID(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewUserRepository(db)
	ctx := context.Background()
	id := uuid.New()

	rows := sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "role"}).
		AddRow(id, "a@a.com", "hash", "A", "artist")
	mock.ExpectQuery("SELECT \\* FROM users WHERE email = \\$1").WithArgs("a@a.com").WillReturnRows(rows)
	u, err := repo.GetUserByEmail(ctx, "a@a.com")
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "a@a.com", u.Email)

	mock.ExpectQuery("SELECT \\* FROM users WHERE email = \\$1").WithArgs("none@a.com").WillReturnError(sql.ErrNoRows)
	u, err = repo.GetUserByEmail(ctx, "none@a.com")
	require.NoError(t, err)
	assert.Nil(t, u)

	rows = sqlmock.NewRows([]string{"id", "email", "password_hash", "name", "role"}).
		AddRow(id, "a@a.com", "hash", "A", "artist")
	mock.ExpectQuery("SELECT \\* FROM users WHERE id = \\$1").WithArgs(id).WillReturnRows(rows)
	u, err = repo.GetUserById(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, id, u.ID)
}

func TestPGUserRepository_UpdateProfile(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := repository.NewUserRepository(db)
	ctx := context.Background()
	id := uuid.New()
	bio := "hello"

	err := repo.UpdateProfile(ctx, id, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)

	mock.ExpectExec("UPDATE users SET").
		WithArgs(&bio, sqlmock.AnyArg(), id).
		WillReturnResult(sqlmock.NewResult(0, 1))
	err = repo.UpdateProfile(ctx, id, &bio, nil, nil, nil, nil, nil)
	require.NoError(t, err)

	avatar := "a.jpg"
	instagram := "https://instagram.com/x"
	twitter := "https://x.com/x"
	youtube := "https://youtube.com/x"
	spotify := "https://spotify.com/x"
	mock.ExpectExec("UPDATE users SET").
		WithArgs(&bio, &avatar, &instagram, &twitter, &youtube, &spotify, sqlmock.AnyArg(), id).
		WillReturnResult(sqlmock.NewResult(0, 1))
	err = repo.UpdateProfile(ctx, id, &bio, &avatar, &instagram, &twitter, &youtube, &spotify)
	require.NoError(t, err)
}

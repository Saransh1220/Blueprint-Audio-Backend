package postgres_test

import (
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/domain"
	"github.com/saransh1220/blueprint-audio/internal/modules/auth/infrastructure/persistence/postgres"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPublicHandleRequiresUniqueActiveUsername(t *testing.T) {
	db, mock, cleanup := newMockDB(t)
	defer cleanup()
	repo := postgres.NewUserRepository(db)
	id := uuid.New()
	for _, count := range []int{1, 0, 2} {
		rows := sqlmock.NewRows([]string{"id", "name"})
		for i := 0; i < count; i++ {
			rows.AddRow(id, "Producer.Handle")
		}
		mock.ExpectQuery(`SELECT \* FROM users WHERE lower\(name\) = lower\(\$1\) AND status = 'active' LIMIT 2`).WithArgs("Producer.Handle").WillReturnRows(rows)
		user, err := repo.GetByHandle(context.Background(), "Producer.Handle")
		if count == 1 {
			require.NoError(t, err)
			require.Equal(t, id, user.ID)
		} else {
			require.ErrorIs(t, err, domain.ErrUserNotFound)
			require.Nil(t, user)
		}
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

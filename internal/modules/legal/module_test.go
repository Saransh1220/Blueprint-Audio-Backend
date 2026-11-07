package legal_test

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/saransh1220/blueprint-audio/internal/modules/legal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewModule(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)
	defer sqlDB.Close()
	db := sqlx.NewDb(sqlDB, "sqlmock")

	m := legal.NewModule(db)
	assert.NotNil(t, m)
	assert.NotNil(t, m.Service())
	assert.NotNil(t, m.HTTPHandler())
}

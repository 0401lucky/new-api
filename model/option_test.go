package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestWithOptionKeyQuotesReservedColumnAcrossDialects(t *testing.T) {
	sqliteDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := sqliteDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	tests := []struct {
		name      string
		dialector gorm.Dialector
		quotedKey string
	}{
		{
			name:      "sqlite",
			dialector: sqlite.Open(":memory:"),
			quotedKey: "`key`",
		},
		{
			name: "mysql",
			dialector: mysql.New(mysql.Config{
				Conn:                      sqlDB,
				SkipInitializeWithVersion: true,
			}),
			quotedKey: "`key`",
		},
		{
			name: "postgresql",
			dialector: postgres.New(postgres.Config{
				Conn: sqlDB,
			}),
			quotedKey: `"key"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(tt.dialector, &gorm.Config{
				DryRun:               true,
				DisableAutomaticPing: true,
			})
			require.NoError(t, err)

			find := db.Scopes(WithOptionKey("migration-marker")).First(&Option{})
			require.NoError(t, find.Error)
			assert.Contains(t, find.Statement.SQL.String(), tt.quotedKey)

			remove := db.Scopes(WithOptionKey([]string{"old-a", "old-b"})).Delete(&Option{})
			require.NoError(t, remove.Error)
			assert.Contains(t, remove.Statement.SQL.String(), tt.quotedKey+" IN")
		})
	}
}

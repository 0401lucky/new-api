package model

import (
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type forkSystemInstanceSchema struct {
	ID       int64  `gorm:"primaryKey"`
	NodeName string `gorm:"type:varchar(191);uniqueIndex;not null"`
	Info     string `gorm:"type:text"`
}

type releasedSystemInstanceSchema struct {
	NodeName string `gorm:"type:varchar(128);primaryKey"`
	Info     string `gorm:"type:text"`
}

func TestSystemInstanceSchemaCompatibilityDatabaseMatrix(t *testing.T) {
	for _, kind := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(kind, func(t *testing.T) {
			var driver gorm.Dialector
			switch kind {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			connection, err := db.DB()
			require.NoError(t, err)
			connection.SetMaxOpenConns(1)
			t.Cleanup(func() { assert.NoError(t, connection.Close()) })
			for _, layout := range []string{"fresh", "fork", "released"} {
				t.Run(layout, func(t *testing.T) {
					const tableName = "test_system_instance_compatibility"
					require.NoError(t, db.Migrator().DropTable(tableName))
					t.Cleanup(func() { assert.NoError(t, db.Migrator().DropTable(tableName)) })
					table := db.Table(tableName)
					if layout == "fork" {
						require.NoError(t, table.AutoMigrate(&forkSystemInstanceSchema{}))
					} else if layout == "released" {
						require.NoError(t, table.AutoMigrate(&releasedSystemInstanceSchema{}))
					}
					if layout != "fresh" {
						require.NoError(t, table.Create(map[string]any{"node_name": "existing-node", "info": `{"seq":1}`}).Error)
					}
					for range 2 {
						require.NoError(t, table.AutoMigrate(&SystemInstance{}))
					}
					previous := DB
					DB = db.Session(&gorm.Session{NewDB: true}).Table(tableName)
					defer func() { DB = previous }()
					if layout != "fresh" {
						rows, err := ListSystemInstances()
						require.NoError(t, err)
						require.Len(t, rows, 1)
						assert.JSONEq(t, `{"seq":1}`, rows[0].Info)
					}
					require.NoError(t, UpsertSystemInstance("existing-node", map[string]any{"seq": 2}, 100, 200))
					require.NoError(t, UpsertSystemInstance("existing-node", map[string]any{"seq": 3}, 100, 300))
					rows, err := ListSystemInstances()
					require.NoError(t, err)
					require.Len(t, rows, 1)
					assert.Equal(t, "existing-node", rows[0].NodeName)
					assert.JSONEq(t, `{"seq":3}`, rows[0].Info)
					assert.EqualValues(t, 300, rows[0].LastSeenAt)
					if layout == "fork" {
						var id int64
						require.NoError(t, table.Select("id").Where("node_name = ?", "existing-node").Scan(&id).Error)
						assert.Positive(t, id, "existing generated IDs remain intact")
					}
				})
			}
		})
	}
}

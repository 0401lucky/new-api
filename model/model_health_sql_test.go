package model

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// The pre-fix schema is preserved verbatim to exercise rolling upgrades.
type legacyModelHealthSlice5m struct {
	SliceStartTs             int64     `json:"slice_start_ts" gorm:"primaryKey;autoIncrement:false;index:idx_model_health_slice_start;index:idx_model_health_slice_model,priority:1;comment:slice start unix seconds, aligned to 300s"`
	ModelName                string    `json:"model_name" gorm:"size:128;primaryKey;autoIncrement:false;default:'';index:idx_model_health_slice_model,priority:2;comment:model name used in consume/error logs"`
	TotalRequests            int64     `json:"total_requests" gorm:"not null;default:0;comment:events observed in this slice for this model"`
	ErrorRequests            int64     `json:"error_requests" gorm:"not null;default:0;comment:events considered failure in this slice for this model"`
	SuccessQualifiedRequests int64     `json:"success_qualified_requests" gorm:"not null;default:0;comment:successful requests meeting threshold"`
	SuccessTokens            int64     `json:"success_tokens" gorm:"not null;default:0;comment:total prompt and completion tokens from successful requests"`
	HasSuccessQualified      bool      `json:"has_success_qualified" gorm:"not null;default:false;comment:1 if any qualified success in slice"`
	MaxResponseBytes         int       `json:"max_response_bytes" gorm:"not null;default:0;comment:max response bytes observed in slice"`
	MaxCompletionTokens      int       `json:"max_completion_tokens" gorm:"not null;default:0;comment:max completion tokens observed in slice"`
	MaxAssistantChars        int       `json:"max_assistant_chars" gorm:"not null;default:0;comment:max assistant content char length observed in slice"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func (legacyModelHealthSlice5m) TableName() string { return "model_health_slice_5m" }

func TestModelHealthDatabaseCompatibility(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_HEALTH_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_HEALTH_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_HEALTH_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_HEALTH_POSTGRES_DSN is not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			var version string
			query := "SELECT version()"
			if dialect == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("%s version: %s", dialect, version)

			for _, upgrade := range []bool{false, true} {
				name := "fresh"
				if upgrade {
					name = "upgrade"
				}
				t.Run(name, func(t *testing.T) {
					require.NoError(t, db.Migrator().DropTable(&ModelHealthSlice5m{}, &legacyModelHealthSlice5m{}))
					if upgrade {
						require.NoError(t, db.AutoMigrate(&legacyModelHealthSlice5m{}))
						require.NoError(t, db.Create(&legacyModelHealthSlice5m{
							SliceStartTs: 3600, ModelName: "sample", TotalRequests: 22,
							SuccessQualifiedRequests: 19, SuccessTokens: 409007,
						}).Error)
					}
					// A restart must preserve the legacy table and current counters.
					for range 2 {
						require.NoError(t, db.AutoMigrate(&ModelHealthSlice5m{}))
					}
					rows, err := GetAllModelsHealthHourlyStats(db, 3600, 7200)
					require.NoError(t, err)
					assert.Empty(t, rows, "legacy attempts must not become final outcomes")
					for _, event := range []*ModelHealthEvent{
						{ModelName: "sample", CreatedAt: 3601, ResponseBytes: 2048, CompletionTokens: 8, SuccessTokens: 42},
						{ModelName: "sample", CreatedAt: 3602, CompletionTokens: 1, SuccessTokens: 11},
						{ModelName: "sample", CreatedAt: 3603, IsError: true, SuccessTokens: 999},
					} {
						require.NoError(t, UpsertModelHealthSlice5m(context.Background(), db, event))
					}
					for range 2 {
						require.NoError(t, db.AutoMigrate(&ModelHealthSlice5m{}))
					}
					rows, err = GetAllModelsHealthHourlyStats(db, 3600, 7200)
					require.NoError(t, err)
					require.Len(t, rows, 1)
					assert.Equal(t, int64(3), rows[0].TotalRequests)
					assert.Equal(t, int64(1), rows[0].ErrorRequests)
					assert.Equal(t, int64(2), rows[0].SuccessRequests)
					assert.Equal(t, int64(1), rows[0].QualifiedSuccessRequests)
					assert.Equal(t, int64(53), rows[0].SuccessTokens)
					assert.InDelta(t, 2.0/3, rows[0].SuccessRate, 0.00001)
					totals, err := GetAllModelsHealthTotals(db, 3600, 7200)
					require.NoError(t, err)
					require.Len(t, totals, 1)
					assert.Equal(t, int64(2), totals[0].SuccessRequests)
					var slice ModelHealthSlice5m
					require.NoError(t, db.First(&slice).Error)
					assert.Equal(t, 2048, slice.MaxResponseBytes)
					assert.Equal(t, 8, slice.MaxCompletionTokens)
					assert.True(t, slice.HasSuccessQualified)
					assert.True(t, db.Migrator().HasIndex(&ModelHealthSlice5m{}, "idx_model_health_request_start"))
					assert.True(t, db.Migrator().HasIndex(&ModelHealthSlice5m{}, "idx_model_health_request_model"))
					assert.Error(t, db.Create(&ModelHealthSlice5m{SliceStartTs: 3600, ModelName: "sample"}).Error, "the model/slice key remains unique")
					require.NoError(t, db.Create(&ModelHealthSlice5m{SliceStartTs: 3600, ModelName: "threshold", TotalRequests: 1000000, ErrorRequests: 50001}).Error)
					boundary, err := GetModelHealthHourlyStats(db, "threshold", 3600, 7200)
					require.NoError(t, err)
					require.Len(t, boundary, 1)
					assert.Less(t, boundary[0].SuccessRate, 0.95, "SQL rounding must not turn a degraded model green")
					assert.InDelta(t, 0.949999, boundary[0].SuccessRate, 1e-12)
					if upgrade {
						var old legacyModelHealthSlice5m
						require.NoError(t, db.First(&old).Error)
						assert.Equal(t, int64(22), old.TotalRequests)
						assert.Equal(t, int64(19), old.SuccessQualifiedRequests)
						assert.Equal(t, int64(409007), old.SuccessTokens)
						assert.True(t, db.Migrator().HasIndex(&legacyModelHealthSlice5m{}, "idx_model_health_slice_model"))
					}
				})
			}
			require.NoError(t, db.Migrator().DropTable(&ModelHealthSlice5m{}, &legacyModelHealthSlice5m{}))
		})
	}
}

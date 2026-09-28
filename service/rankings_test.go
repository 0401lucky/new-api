package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRankingTodayStartsAtLocalMidnight(t *testing.T) {
	config, err := rankingConfig("today")
	require.NoError(t, err)
	for _, zone := range []string{"Asia/Shanghai", "America/New_York", "Asia/Kathmandu"} {
		t.Run(zone, func(t *testing.T) {
			location, err := time.LoadLocation(zone)
			require.NoError(t, err)
			now := time.Date(2026, 3, 8, 15, 42, 37, 0, location)
			start, end := rankingTimeRange(config, now)
			assert.Equal(t, time.Date(2026, 3, 8, 0, 0, 0, 0, location).Unix(), start)
			assert.Equal(t, now.Unix(), end)
		})
	}
}

func TestRankingTodayPreviousWindowAndRollingPeriods(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	config, err := rankingConfig("today")
	require.NoError(t, err)
	for _, now := range []time.Time{
		time.Date(2026, 3, 8, 15, 0, 0, 0, location),
		time.Date(2026, 11, 1, 15, 0, 0, 0, location),
		time.Date(2026, 9, 28, 0, 0, 0, 0, location),
	} {
		start, end := previousRankingTimeRange(config, now)
		yesterday := now.AddDate(0, 0, -1)
		assert.Equal(t, time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, location).Unix(), start)
		assert.Equal(t, yesterday.Unix(), end)
	}
	for _, period := range []string{"week", "month", "year"} {
		config, err := rankingConfig(period)
		require.NoError(t, err)
		now := time.Date(2026, 9, 28, 15, 0, 0, 0, location)
		start, end := rankingTimeRange(config, now)
		assert.Equal(t, int64(config.duration/time.Second), end-start)
		previousStart, previousEnd := previousRankingTimeRange(config, now)
		assert.Equal(t, start-1, previousEnd)
		assert.Equal(t, start-int64(config.duration/time.Second), previousStart)
	}
	for _, zone := range []string{"Local", "Not/AZone", "../../etc/passwd"} {
		_, err := GetRankingsSnapshot("today", zone)
		require.Error(t, err)
	}
}

func TestRankingTodayDatabaseBoundaries(t *testing.T) {
	for _, dialect := range []common.DatabaseType{common.DatabaseTypeSQLite, common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(dialect), func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case common.DatabaseTypeSQLite:
				driver = sqlite.Open(filepath.Join(t.TempDir(), "rankings.db"))
			case common.DatabaseTypeMySQL:
				dsn := os.Getenv("TEST_RANKINGS_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_RANKINGS_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case common.DatabaseTypePostgreSQL:
				dsn := os.Getenv("TEST_RANKINGS_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_RANKINGS_POSTGRES_DSN is not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			originalDB, originalType := model.DB, common.MainDatabaseType()
			model.DB = db
			common.SetMainDatabaseType(dialect)
			t.Cleanup(func() { model.DB = originalDB; common.SetMainDatabaseType(originalType) })
			require.False(t, db.Migrator().HasTable(&model.QuotaData{}), "use a dedicated empty test database")
			require.NoError(t, db.Migrator().CreateTable(&model.QuotaData{}))
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&model.QuotaData{})) })
			var version string
			versionQuery := "select version()"
			if dialect == common.DatabaseTypeSQLite {
				versionQuery = "select sqlite_version()"
			}
			require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
			t.Logf("%s version: %s", dialect, version)
			location, err := time.LoadLocation("Asia/Shanghai")
			require.NoError(t, err)
			now := time.Date(2026, 9, 28, 15, 42, 37, 0, location)
			config, err := rankingConfig("today")
			require.NoError(t, err)
			start, end := rankingTimeRange(config, now)
			rows := []model.QuotaData{
				{ModelName: "yesterday", CreatedAt: start - 3600, TokenUsed: 1000},
				{ModelName: "alpha", CreatedAt: start, TokenUsed: 11},
				{ModelName: "alpha", CreatedAt: start + 3600, TokenUsed: 7},
				{ModelName: "beta", CreatedAt: end - end%3600, TokenUsed: 5},
				{ModelName: "future", CreatedAt: end - end%3600 + 3600, TokenUsed: 2000},
			}
			require.NoError(t, db.Create(&rows).Error)
			totals, err := model.GetRankingQuotaTotals(start, end)
			require.NoError(t, err)
			assert.Equal(t, []model.RankingQuotaTotal{{ModelName: "alpha", TotalTokens: 18}, {ModelName: "beta", TotalTokens: 5}}, totals)
			buckets, err := model.GetRankingQuotaBuckets(start, end, config.bucketSize)
			require.NoError(t, err)
			assert.Equal(t, []model.RankingQuotaBucket{
				{ModelName: "alpha", Bucket: start, Tokens: 11},
				{ModelName: "alpha", Bucket: start + 3600, Tokens: 7},
				{ModelName: "beta", Bucket: end - end%3600, Tokens: 5},
			}, buckets)
		})
	}
}

package model

import (
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// openLogQueryMatrixDB 为指定方言打开一个隔离的日志库连接：SQLite 用独立内存库，
// MySQL 用表前缀，PostgreSQL 用 schema（其索引名是 schema 级的，前缀无法隔离）。
func openLogQueryMatrixDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()

	if dialect == "sqlite" {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		return db
	}

	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dialect == "postgres" {
		dsn = os.Getenv("TEST_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skipf("%s is not configured", dialect)
	}

	t.Setenv("LOG_QUERY_MATRIX_DSN", dsn)
	prefix := "lqm_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "") + "_"
	db, _, err := chooseDB("LOG_QUERY_MATRIX_DSN", false)
	require.NoError(t, err)

	if dialect == "postgres" {
		require.NoError(t, db.Exec("CREATE SCHEMA ?", clause.Table{Name: prefix}).Error)
		parsed, err := url.Parse(dsn)
		require.NoError(t, err)
		query := parsed.Query()
		query.Set("search_path", prefix)
		parsed.RawQuery = query.Encode()
		// 这里改用 os.Setenv：同一个 key 不能重复 t.Setenv，上面的 t.Setenv 清理时仍会恢复原值。
		require.NoError(t, os.Setenv("LOG_QUERY_MATRIX_DSN", parsed.String()))
		db, _, err = chooseDB("LOG_QUERY_MATRIX_DSN", false)
		require.NoError(t, err)
	}

	db.Config.NamingStrategy = schema.NamingStrategy{TablePrefix: prefix}
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	t.Cleanup(func() {
		assert.NoError(t, db.Migrator().DropTable(&Log{}))
		if dialect == "postgres" {
			assert.NoError(t, db.Exec("DROP SCHEMA ? CASCADE", clause.Table{Name: prefix}).Error)
		}
		sqlDB, err := db.DB()
		require.NoError(t, err)
		assert.NoError(t, sqlDB.Close())
	})
	return db
}

// TestChannelModelUsageStatsDatabaseMatrix 在真实 SQLite / MySQL / PostgreSQL 上验证渠道模型调用
// 统计的聚合语义：只统计消费日志、按渠道与时间范围过滤、按调用次数排序、limit 生效。
func TestChannelModelUsageStatsDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openLogQueryMatrixDB(t, dialect)

			previousDB, previousLogDB := DB, LOG_DB
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(dbTypeOf(dialect), dbTypeOf(dialect))
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
				initCol()
			})

			require.NoError(t, db.AutoMigrate(&Log{}))
			require.NoError(t, db.Create(&[]Log{
				{ChannelId: 1, ModelName: "gpt-a", Type: LogTypeConsume, CreatedAt: 1000},
				{ChannelId: 1, ModelName: "gpt-a", Type: LogTypeConsume, CreatedAt: 1500},
				{ChannelId: 1, ModelName: "gpt-b", Type: LogTypeConsume, CreatedAt: 1500},
				{ChannelId: 1, ModelName: "gpt-c", Type: LogTypeError, CreatedAt: 1500},
				{ChannelId: 1, ModelName: "gpt-d", Type: LogTypeConsume, CreatedAt: 3000},
				{ChannelId: 2, ModelName: "gpt-a", Type: LogTypeConsume, CreatedAt: 1500},
			}).Error)

			stats, err := GetChannelModelUsageStats(1, 0, 0, 0)
			require.NoError(t, err)
			// 失败的调用（错误日志）不计入，其他渠道的调用也不计入。
			assert.Equal(t, map[string]int64{"gpt-a": 2, "gpt-b": 1, "gpt-d": 1}, requestCountsByModel(stats))
			require.NotEmpty(t, stats)
			assert.Equal(t, "gpt-a", stats[0].ModelName, "调用次数最多的模型排在最前")

			stats, err = GetChannelModelUsageStats(1, 1200, 2000, 0)
			require.NoError(t, err)
			assert.Equal(t, map[string]int64{"gpt-a": 1, "gpt-b": 1}, requestCountsByModel(stats))

			stats, err = GetChannelModelUsageStats(2, 0, 0, 0)
			require.NoError(t, err)
			assert.Equal(t, map[string]int64{"gpt-a": 1}, requestCountsByModel(stats))

			stats, err = GetChannelModelUsageStats(1, 0, 0, 1)
			require.NoError(t, err)
			require.Len(t, stats, 1)
			assert.Equal(t, "gpt-a", stats[0].ModelName)
			assert.Equal(t, int64(2), stats[0].RequestCount)
		})
	}
}

func requestCountsByModel(stats []ChannelModelUsageStat) map[string]int64 {
	counts := make(map[string]int64, len(stats))
	for _, stat := range stats {
		counts[stat.ModelName] = stat.RequestCount
	}
	return counts
}

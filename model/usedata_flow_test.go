package model

import (
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Snapshot of QuotaData from v1.0.0-rc.45, before cache statistics were added.
type releasedQuotaData struct {
	Id        int
	UserID    int    `gorm:"index"`
	Username  string `gorm:"index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName string `gorm:"index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt int64  `gorm:"bigint;index:idx_qdt_created_at,priority:2"`
	UseGroup  string `gorm:"index;size:64;default:''"`
	TokenID   int    `gorm:"index;default:0"`
	ChannelID int    `gorm:"index;default:0"`
	NodeName  string `gorm:"index;size:64;default:''"`
	TokenUsed int    `gorm:"default:0"`
	Count     int    `gorm:"default:0"`
	Quota     int    `gorm:"default:0"`
}

func (releasedQuotaData) TableName() string { return "quota_data" }

func TestQuotaDataCacheStatisticsDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_CACHE_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_CACHE_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_CACHE_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_CACHE_POSTGRES_DSN is not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousType := DB, common.MainDatabaseType()
			DB = db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			initCol()
			t.Cleanup(func() {
				DB = previousDB
				common.SetMainDatabaseType(previousType)
				initCol()
				require.NoError(t, sqlDB.Close())
			})
			var version string
			versionSQL := "SELECT version()"
			if dialect == "sqlite" {
				versionSQL = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
			t.Logf("cache statistics database: %s %s", dialect, version)
			for _, upgrade := range []bool{false, true} {
				name := "fresh"
				if upgrade {
					name = "upgrade_rc45"
				}
				t.Run(name, func(t *testing.T) {
					require.NoError(t, db.Migrator().DropTable(&QuotaData{}))
					if upgrade {
						require.NoError(t, db.AutoMigrate(&releasedQuotaData{}))
						require.NoError(t, db.Create(&releasedQuotaData{Id: 7, UserID: 2, Username: "legacy", ModelName: "legacy-model", CreatedAt: 3600, Count: 3, Quota: 90, TokenUsed: 120}).Error)
					}
					for range 2 {
						require.NoError(t, db.AutoMigrate(&QuotaData{}))
					}
					if upgrade {
						var old QuotaData
						require.NoError(t, db.First(&old, 7).Error)
						assert.Equal(t, 3, old.Count)
						assert.Equal(t, 90, old.Quota)
						assert.Equal(t, 120, old.TokenUsed)
						assert.Zero(t, old.CacheReadTokens)
						assert.Zero(t, old.CacheCreationTokens)
					}
					CacheQuotaDataLock.Lock()
					previousCache := CacheQuotaData
					CacheQuotaData = make(map[string]*QuotaData)
					CacheQuotaDataLock.Unlock()
					t.Cleanup(func() {
						CacheQuotaDataLock.Lock()
						CacheQuotaData = previousCache
						CacheQuotaDataLock.Unlock()
					})
					for range 2 {
						LogQuotaData(QuotaDataLogParams{UserID: 3, Username: "cache-owner", ModelName: "cached-model", CreatedAt: 7201, Quota: 11, TokenUsed: 120, CacheReadTokens: 80, CacheCreationTokens: 20, UseGroup: "vip", TokenID: 5, ChannelID: 9, NodeName: "primary"})
						SaveQuotaDataCache()
					}
					for range 2 {
						require.NoError(t, db.AutoMigrate(&QuotaData{}))
					}
					rows, err := GetQuotaDataByUserId(3, 7200, 10800)
					require.NoError(t, err)
					require.Len(t, rows, 1)
					assert.Equal(t, 2, rows[0].Count)
					assert.Equal(t, 22, rows[0].Quota)
					assert.Equal(t, 240, rows[0].TokenUsed)
					assert.EqualValues(t, 160, rows[0].CacheReadTokens)
					assert.EqualValues(t, 40, rows[0].CacheCreationTokens)
					grouped, err := GetQuotaDataGroupByUser(7200, 10800)
					require.NoError(t, err)
					require.Len(t, grouped, 1)
					assert.Equal(t, 3, grouped[0].UserID)
					assert.EqualValues(t, 160, grouped[0].CacheReadTokens)
					assert.True(t, db.Migrator().HasIndex(&QuotaData{}, "idx_qdt_model_user_name"))
					assert.True(t, db.Migrator().HasIndex(&QuotaData{}, "idx_qdt_created_at"))
					require.NoError(t, db.Migrator().DropTable(&QuotaData{}))
				})
			}
		})
	}
}

func seedFlowQuotaData(t *testing.T, quotaData QuotaData) {
	t.Helper()
	require.NoError(t, DB.Create(&quotaData).Error)
}

func seedFlowLookupData(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Create(&Channel{Id: 1, Name: "east"}).Error)
	require.NoError(t, DB.Create(&Channel{Id: 2, Name: "west"}).Error)
	require.NoError(t, DB.Create(&Token{Id: 11, UserId: 1, Key: "sk-primary", Name: "primary"}).Error)
	require.NoError(t, DB.Create(&Token{Id: 22, UserId: 2, Key: "sk-backup", Name: "backup"}).Error)
	require.NoError(t, DB.Delete(&Token{Id: 11}).Error)
}

func TestGetFlowQuotaDataUsesQuotaDataRoleSpecificDimensions(t *testing.T) {
	truncateTables(t)
	seedFlowLookupData(t)

	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		NodeName:  "node-a",
		TokenID:   11,
		UseGroup:  "vip",
		ModelName: "gpt-a",
		ChannelID: 1,
		CreatedAt: 1000,
		Count:     2,
		Quota:     100,
		TokenUsed: 40,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		NodeName:  "node-a",
		TokenID:   11,
		UseGroup:  "vip",
		ModelName: "gpt-a",
		ChannelID: 1,
		CreatedAt: 1100,
		Count:     1,
		Quota:     50,
		TokenUsed: 20,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		NodeName:  "node-a",
		TokenID:   11,
		UseGroup:  "vip",
		ModelName: "gpt-a",
		ChannelID: 2,
		CreatedAt: 1200,
		Count:     1,
		Quota:     25,
		TokenUsed: 10,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    2,
		Username:  "bob",
		NodeName:  "node-b",
		TokenID:   22,
		UseGroup:  "default",
		ModelName: "gpt-b",
		ChannelID: 1,
		CreatedAt: 1300,
		Count:     3,
		Quota:     70,
		TokenUsed: 30,
	})
	seedFlowQuotaData(t, QuotaData{
		UserID:    1,
		Username:  "alice",
		ModelName: "legacy",
		CreatedAt: 1400,
		Count:     99,
		Quota:     999,
		TokenUsed: 999,
	})

	rootRows, err := GetFlowQuotaData(900, 2000, "", 0, common.RoleRootUser)
	require.NoError(t, err)
	require.Len(t, rootRows, 3)
	// Token 11 was soft-deleted, so its name is intentionally left empty for the
	// frontend to render a localized "deleted (id)" label instead.
	require.Equal(t, FlowQuotaData{
		UserID:      1,
		Username:    "alice",
		NodeName:    "node-a",
		TokenID:     11,
		TokenName:   "",
		UseGroup:    "vip",
		ChannelID:   1,
		ChannelName: "east",
		ModelName:   "gpt-a",
		TokenUsed:   60,
		Count:       3,
		Quota:       150,
	}, *rootRows[0])
	// A token that still exists resolves to its current name.
	require.Equal(t, 22, rootRows[1].TokenID)
	require.Equal(t, "backup", rootRows[1].TokenName)

	adminRows, err := GetFlowQuotaData(900, 2000, "alice", 0, common.RoleAdminUser)
	require.NoError(t, err)
	require.Len(t, adminRows, 2)
	require.Equal(t, 0, adminRows[0].TokenID)
	require.Empty(t, adminRows[0].TokenName)
	require.Empty(t, adminRows[0].NodeName)
	require.Equal(t, "alice", adminRows[0].Username)
	require.Equal(t, "vip", adminRows[0].UseGroup)
	require.Equal(t, "east", adminRows[0].ChannelName)
	require.Equal(t, 150, adminRows[0].Quota)

	selfRows, err := GetFlowQuotaData(900, 2000, "", 1, common.RoleCommonUser)
	require.NoError(t, err)
	require.Len(t, selfRows, 1)
	require.Empty(t, selfRows[0].Username)
	require.Equal(t, 0, selfRows[0].ChannelID)
	require.Empty(t, selfRows[0].ChannelName)
	require.Empty(t, selfRows[0].TokenName)
	require.Equal(t, "vip", selfRows[0].UseGroup)
	require.Equal(t, 175, selfRows[0].Quota)
}

func TestLogQuotaDataSplitsRowsByUseGroupTokenChannelAndNode(t *testing.T) {
	truncateTables(t)
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()

	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3661,
		UseGroup:  "vip",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     100,
		TokenUsed: 40,
	})
	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3700,
		UseGroup:  "vip",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     50,
		TokenUsed: 20,
	})
	LogQuotaData(QuotaDataLogParams{
		UserID:    1,
		Username:  "alice",
		ModelName: "gpt-a",
		CreatedAt: 3700,
		UseGroup:  "default",
		TokenID:   11,
		ChannelID: 1,
		NodeName:  "node-a",
		Quota:     25,
		TokenUsed: 10,
	})

	SaveQuotaDataCache()

	var rows []QuotaData
	require.NoError(t, DB.Order("quota DESC").Find(&rows).Error)
	require.Len(t, rows, 2)
	require.Equal(t, int64(3600), rows[0].CreatedAt)
	require.Equal(t, "vip", rows[0].UseGroup)
	require.Equal(t, 11, rows[0].TokenID)
	require.Equal(t, 1, rows[0].ChannelID)
	require.Equal(t, "node-a", rows[0].NodeName)
	require.Equal(t, 2, rows[0].Count)
	require.Equal(t, 150, rows[0].Quota)
	require.Equal(t, 60, rows[0].TokenUsed)
	require.Equal(t, "default", rows[1].UseGroup)
	require.Equal(t, 25, rows[1].Quota)
}

func TestSaveQuotaDataCacheDoesNotBlockNewLogsDuringDatabaseWrite(t *testing.T) {
	truncateTables(t)
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()

	LogQuotaData(QuotaDataLogParams{
		UserID: 1, Username: "alice", ModelName: "gpt-a", CreatedAt: 3600,
		UseGroup: "vip", TokenID: 11, ChannelID: 1, NodeName: "node-a", Quota: 100, TokenUsed: 40,
	})

	const callbackName = "test:block_quota_data_flush"
	blocked := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "quota_data" {
			return
		}
		once.Do(func() { close(blocked) })
		<-release
	}))
	t.Cleanup(func() {
		_ = DB.Callback().Query().Remove(callbackName)
	})

	flushDone := make(chan struct{})
	go func() {
		SaveQuotaDataCache()
		close(flushDone)
	}()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("quota data flush did not reach database query")
	}

	logDone := make(chan struct{})
	go func() {
		LogQuotaData(QuotaDataLogParams{
			UserID: 1, Username: "alice", ModelName: "gpt-a", CreatedAt: 3600,
			UseGroup: "vip", TokenID: 11, ChannelID: 1, NodeName: "node-a", Quota: 50, TokenUsed: 20,
		})
		close(logDone)
	}()
	select {
	case <-logDone:
	case <-time.After(time.Second):
		t.Fatal("LogQuotaData was blocked by database flush")
	}

	close(release)
	select {
	case <-flushDone:
	case <-time.After(time.Second):
		t.Fatal("quota data flush did not finish")
	}
	require.NoError(t, DB.Callback().Query().Remove(callbackName))

	CacheQuotaDataLock.Lock()
	remaining := make([]QuotaData, 0, len(CacheQuotaData))
	for _, quotaData := range CacheQuotaData {
		remaining = append(remaining, *quotaData)
	}
	CacheQuotaDataLock.Unlock()
	require.Len(t, remaining, 1)
	require.Equal(t, 1, remaining[0].Count)
	require.Equal(t, 50, remaining[0].Quota)
	require.Equal(t, 20, remaining[0].TokenUsed)
}

func TestSaveQuotaDataCacheRetainsFailedEntriesForRetry(t *testing.T) {
	truncateTables(t)
	CacheQuotaDataLock.Lock()
	CacheQuotaData = make(map[string]*QuotaData)
	CacheQuotaDataLock.Unlock()

	LogQuotaData(QuotaDataLogParams{
		UserID: 2, Username: "bob", ModelName: "gpt-b", CreatedAt: 7200,
		UseGroup: "default", TokenID: 22, ChannelID: 2, NodeName: "node-b", Quota: 75, TokenUsed: 30,
	})

	const callbackName = "test:fail_quota_data_flush"
	require.NoError(t, DB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "quota_data" {
			tx.AddError(errors.New("forced quota data query failure"))
		}
	}))
	t.Cleanup(func() {
		_ = DB.Callback().Query().Remove(callbackName)
	})

	SaveQuotaDataCache()

	CacheQuotaDataLock.Lock()
	failedCount := len(CacheQuotaData)
	CacheQuotaDataLock.Unlock()
	require.Equal(t, 1, failedCount)
	require.NoError(t, DB.Callback().Query().Remove(callbackName))

	SaveQuotaDataCache()

	CacheQuotaDataLock.Lock()
	remainingCount := len(CacheQuotaData)
	CacheQuotaDataLock.Unlock()
	require.Zero(t, remainingCount)
	var row QuotaData
	require.NoError(t, DB.Where("user_id = ? AND model_name = ?", 2, "gpt-b").First(&row).Error)
	require.Equal(t, 75, row.Quota)
	require.Equal(t, 30, row.TokenUsed)
}

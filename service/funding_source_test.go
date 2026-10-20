package service

import (
	"fmt"
	"os"
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

func seedTemporaryCheckin(t *testing.T, userId, remaining int, expiresAt int64) *model.Checkin {
	t.Helper()
	c := &model.Checkin{
		UserId:         userId,
		CheckinDate:    time.Now().Format("2006-01-02"),
		QuotaAwarded:   remaining,
		QuotaType:      model.CheckinQuotaTypeTemporary,
		QuotaRemaining: remaining,
		QuotaExpiresAt: expiresAt,
		CreatedAt:      common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(c).Error)
	return c
}

func TestWalletFunding_PreConsumeTemporaryFirst(t *testing.T) {
	truncate(t)
	seedUser(t, 1, 10000)
	now := common.NowInCheckinTimezone().Unix()
	seedTemporaryCheckin(t, 1, 3000, now+3600)

	funding := &WalletFunding{userId: 1}
	require.NoError(t, funding.PreConsume(5000))
	assert.Equal(t, 3000, funding.tempConsumed)
	assert.Equal(t, 2000, funding.permConsumed)
	assert.Equal(t, 5000, funding.consumed)
}

func TestWalletFundingRepeatedPreConsumeRefundPreservesAllBuckets(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_WALLET_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_WALLET_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_WALLET_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_WALLET_POSTGRES_DSN is not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousType := model.DB, common.MainDatabaseType()
			model.DB = db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			t.Cleanup(func() {
				model.DB = previousDB
				common.SetMainDatabaseType(previousType)
				require.NoError(t, sqlDB.Close())
			})
			for _, expireFirst := range []bool{false, true} {
				t.Run(fmt.Sprintf("expire_first=%t", expireFirst), func(t *testing.T) {
					require.NoError(t, db.Migrator().DropTable(&model.User{}, &model.Checkin{}))
					require.NoError(t, db.AutoMigrate(&model.User{}, &model.Checkin{}))
					t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&model.User{}, &model.Checkin{})) })
					seedUser(t, 1, 10000)
					now := common.NowInCheckinTimezone().Unix()
					first := seedTemporaryCheckinOn(t, 1, 1000, now+3600, "2026-10-18")
					second := seedTemporaryCheckinOn(t, 1, 2000, now+7200, "2026-10-19")
					funding := &WalletFunding{userId: 1}
					require.NoError(t, funding.PreConsume(700))
					require.NoError(t, funding.PreConsume(4300))
					assert.Equal(t, 5000, funding.consumed)
					assert.Equal(t, 3000, funding.tempConsumed)
					assert.Equal(t, 2000, funding.permConsumed)
					if expireFirst {
						require.NoError(t, model.DB.Model(first).Update("quota_expires_at", now-1).Error)
					}
					require.NoError(t, funding.Refund())
					require.NoError(t, funding.Refund())
					var user model.User
					require.NoError(t, model.DB.First(&user, 1).Error)
					assert.Equal(t, 10000, user.Quota)
					require.NoError(t, model.DB.First(second, second.Id).Error)
					assert.Equal(t, 2000, second.QuotaRemaining)
					active, err := model.GetActiveTemporaryQuota(1)
					require.NoError(t, err)
					want := 3000
					if expireFirst {
						want = 2000
					}
					assert.Equal(t, want, active)
				})
			}
		})
	}
}

func TestWalletFunding_SettleTopUpAndPartialRefund(t *testing.T) {
	truncate(t)
	seedUser(t, 1, 10000)
	now := common.NowInCheckinTimezone().Unix()
	seedTemporaryCheckin(t, 1, 3000, now+3600)

	funding := &WalletFunding{userId: 1}
	require.NoError(t, funding.PreConsume(5000)) // temp 3000 + perm 2000

	// 补扣 1000：限时已用完，全走永久
	require.NoError(t, funding.Settle(1000))
	assert.Equal(t, 3000, funding.tempConsumed)
	assert.Equal(t, 3000, funding.permConsumed)

	// 部分退款 2000：优先退永久
	require.NoError(t, funding.Settle(-2000))
	assert.Equal(t, 3000, funding.tempConsumed)
	assert.Equal(t, 1000, funding.permConsumed)
}

func TestWalletFunding_RefundRestoresPermanentFirst(t *testing.T) {
	truncate(t)
	seedUser(t, 1, 10000)
	now := common.NowInCheckinTimezone().Unix()
	seedTemporaryCheckin(t, 1, 3000, now+3600)

	funding := &WalletFunding{userId: 1}
	require.NoError(t, funding.PreConsume(5000)) // temp 3000 + perm 2000
	require.NoError(t, funding.Refund())

	// 永久恢复 2000，限时恢复 3000
	var u model.User
	require.NoError(t, model.DB.First(&u, 1).Error)
	assert.Equal(t, 10000, u.Quota)
	quota, err := model.GetActiveTemporaryQuota(1)
	require.NoError(t, err)
	assert.Equal(t, 3000, quota)
}

func TestWalletFunding_RefundExpiredTemporaryNotRestored(t *testing.T) {
	truncate(t)
	seedUser(t, 1, 10000)
	now := common.NowInCheckinTimezone().Unix()
	c := seedTemporaryCheckin(t, 1, 3000, now+3600)

	funding := &WalletFunding{userId: 1}
	require.NoError(t, funding.PreConsume(5000)) // temp 3000 + perm 2000

	// 模拟跨午夜：限时额度过期
	require.NoError(t, model.DB.Model(&model.Checkin{}).Where("id = ?", c.Id).Update("quota_expires_at", now-60).Error)

	require.NoError(t, funding.Refund())

	// 仅永久恢复 2000；过期限时 3000 不恢复
	var u model.User
	require.NoError(t, model.DB.First(&u, 1).Error)
	assert.Equal(t, 10000, u.Quota)
	quota, err := model.GetActiveTemporaryQuota(1)
	require.NoError(t, err)
	assert.Equal(t, 0, quota)
}

func TestWalletFunding_SourceAndIdempotentRefund(t *testing.T) {
	truncate(t)
	seedUser(t, 1, 10000)

	funding := &WalletFunding{userId: 1}
	assert.Equal(t, BillingSourceWallet, funding.Source())

	// 未预扣时退款是 no-op
	require.NoError(t, funding.Refund())
	var u model.User
	require.NoError(t, model.DB.First(&u, 1).Error)
	assert.Equal(t, 10000, u.Quota)
}

func TestWalletFunding_MultiBucketRefundSyncsAllocations(t *testing.T) {
	truncate(t)
	seedUser(t, 1, 10000)
	now := common.NowInCheckinTimezone().Unix()
	// 两个有效额度桶（跨午夜场景）
	c1 := seedTemporaryCheckinOn(t, 1, 2000, now+3600, "2026-08-07")
	c2 := seedTemporaryCheckinOn(t, 1, 3000, now+7200, "2026-08-08")

	funding := &WalletFunding{userId: 1}
	// 预扣 4000：先扣昨天桶 2000，再扣今天桶 2000
	require.NoError(t, funding.PreConsume(4000))
	assert.Equal(t, 4000, funding.tempConsumed)
	assert.Equal(t, 0, funding.permConsumed)
	require.Len(t, funding.allocations, 2)
	assert.Equal(t, c1.Id, funding.allocations[0].CheckinId)
	assert.Equal(t, c2.Id, funding.allocations[1].CheckinId)

	// 模拟跨午夜：昨天桶过期，今天桶仍有效
	require.NoError(t, model.DB.Model(&model.Checkin{}).Where("id = ?", c1.Id).Update("quota_expires_at", now-60).Error)

	// 完整退款：今天桶 2000 恢复（逆序先退），昨天桶 2000 过期丢弃
	require.NoError(t, funding.Refund())

	// 再次退款不能复活过期桶，也不能重复恢复仍有效的桶。
	require.NoError(t, funding.Refund())
	require.NoError(t, model.DB.First(c1, c1.Id).Error)
	require.NoError(t, model.DB.First(c2, c2.Id).Error)
	assert.Zero(t, c1.QuotaRemaining)
	assert.Equal(t, 3000, c2.QuotaRemaining)

	var u model.User
	require.NoError(t, model.DB.First(&u, 1).Error)
	assert.Equal(t, 10000, u.Quota)
	quota, err := model.GetActiveTemporaryQuota(1)
	require.NoError(t, err)
	assert.Equal(t, 3000, quota) // 今天桶 c2 恢复满
}

func TestWalletFunding_PartialRefundKeepsRemainingAllocation(t *testing.T) {
	truncate(t)
	seedUser(t, 1, 10000)
	now := common.NowInCheckinTimezone().Unix()
	c := seedTemporaryCheckinOn(t, 1, 3000, now+3600, "2026-08-08")

	funding := &WalletFunding{userId: 1}
	require.NoError(t, funding.PreConsume(5000)) // temp 3000 + perm 2000

	// 部分退款 2000：优先退永久
	require.NoError(t, funding.Settle(-2000))
	assert.Equal(t, 3000, funding.tempConsumed)
	assert.Equal(t, 0, funding.permConsumed)
	require.Len(t, funding.allocations, 1)
	assert.Equal(t, c.Id, funding.allocations[0].CheckinId)
	assert.Equal(t, 3000, funding.allocations[0].Amount)

	// 再次退款 3000：退限时
	require.NoError(t, funding.Settle(-3000))
	assert.Equal(t, 0, funding.tempConsumed)
	assert.Empty(t, funding.allocations)
}

// seedTemporaryCheckinOn 创建指定日期的限时额度记录（区分同一用户的多个桶）
func seedTemporaryCheckinOn(t *testing.T, userId, remaining int, expiresAt int64, checkinDate string) *model.Checkin {
	t.Helper()
	c := &model.Checkin{
		UserId:         userId,
		CheckinDate:    checkinDate,
		QuotaAwarded:   remaining,
		QuotaType:      model.CheckinQuotaTypeTemporary,
		QuotaRemaining: remaining,
		QuotaExpiresAt: expiresAt,
		CreatedAt:      common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(c).Error)
	return c
}

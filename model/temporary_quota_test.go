package model

import (
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 签到限时额度钱包扣费测试
// ---------------------------------------------------------------------------

func seedWalletUser(t *testing.T, quota int) *User {
	t.Helper()
	user := &User{Username: "temp_wallet_" + time.Now().Format("150405.000000"), Password: "password", Quota: quota}
	require.NoError(t, DB.Create(user).Error)
	return user
}

// seedActiveTemporary 创建一条限时额度记录（checkinDate 用于区分同一用户的多个桶）
func seedActiveTemporary(t *testing.T, userId, remaining, expiresAt int64, checkinDate string) *Checkin {
	t.Helper()
	c := &Checkin{
		UserId:         int(userId),
		CheckinDate:    checkinDate,
		QuotaAwarded:   int(remaining),
		QuotaType:      CheckinQuotaTypeTemporary,
		QuotaRemaining: int(remaining),
		QuotaExpiresAt: expiresAt,
		CreatedAt:      common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(c).Error)
	return c
}

func getPermanentQuota(t *testing.T, userId int) int {
	t.Helper()
	var u User
	require.NoError(t, DB.First(&u, userId).Error)
	return u.Quota
}

func TestGetActiveTemporaryQuota_ExpiredExcluded(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 1000)
	now := common.NowInStartupTimezone().Unix()

	// 有效记录（今天）
	seedActiveTemporary(t, int64(user.Id), 500, now+3600, "2026-08-08")
	// 过期记录（昨天，已失效）
	seedActiveTemporary(t, int64(user.Id), 300, now-60, "2026-08-07")

	quota, err := GetActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 500, quota)

	has, err := HasActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestPreConsumeWallet_TemporaryFirst(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 10000)
	now := common.NowInStartupTimezone().Unix()
	c := seedActiveTemporary(t, int64(user.Id), 3000, now+3600, "2026-08-08")

	// 扣 5000：限时 3000 + 永久 2000
	split, err := PreConsumeWallet(user.Id, 5000)
	require.NoError(t, err)
	assert.Equal(t, 3000, split.Temporary)
	assert.Equal(t, 2000, split.Permanent)
	require.Len(t, split.Allocations, 1)
	assert.Equal(t, c.Id, split.Allocations[0].CheckinId)
	assert.Equal(t, 3000, split.Allocations[0].Amount)

	// 限时额度归零
	quota, err := GetActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 0, quota)

	// 永久余额减少 2000
	assert.Equal(t, 8000, getPermanentQuota(t, user.Id))
}

func TestPreConsumeWallet_PermanentOnly(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 10000)

	// 无有效限时额度时全扣永久
	split, err := PreConsumeWallet(user.Id, 4000)
	require.NoError(t, err)
	assert.Equal(t, 0, split.Temporary)
	assert.Equal(t, 4000, split.Permanent)
	assert.Equal(t, 6000, getPermanentQuota(t, user.Id))
}

func TestPreConsumeWallet_TemporaryEnough(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 10000)
	now := common.NowInStartupTimezone().Unix()
	seedActiveTemporary(t, int64(user.Id), 3000, now+3600, "2026-08-08")

	// 扣 2000：全部从限时扣，永久不变
	split, err := PreConsumeWallet(user.Id, 2000)
	require.NoError(t, err)
	assert.Equal(t, 2000, split.Temporary)
	assert.Equal(t, 0, split.Permanent)

	quota, err := GetActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1000, quota)
	assert.Equal(t, 10000, getPermanentQuota(t, user.Id))
}

func TestPreConsumeWallet_InsufficientBalance(t *testing.T) {
	truncateTables(t)
	// 永久 0 + 限时 1000，预扣 1500 应失败且不产生负余额
	user := seedWalletUser(t, 0)
	now := common.NowInStartupTimezone().Unix()
	seedActiveTemporary(t, int64(user.Id), 1000, now+3600, "2026-08-08")

	split, err := PreConsumeWallet(user.Id, 1500)
	require.ErrorIs(t, err, ErrInsufficientWalletBalance)
	assert.Nil(t, split)

	// 不扣任何东西：永久仍 0，限时仍 1000
	assert.Equal(t, 0, getPermanentQuota(t, user.Id))
	quota, err := GetActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1000, quota)
}

func TestPreConsumeWallet_MultiBucketAllocation(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 10000)
	now := common.NowInStartupTimezone().Unix()
	// 两个有效额度桶（跨午夜场景：昨天的桶未过期 + 今天的桶）
	c1 := seedActiveTemporary(t, int64(user.Id), 2000, now+3600, "2026-08-07")
	c2 := seedActiveTemporary(t, int64(user.Id), 3000, now+7200, "2026-08-08")

	split, err := PreConsumeWallet(user.Id, 4000)
	require.NoError(t, err)
	assert.Equal(t, 4000, split.Temporary)
	assert.Equal(t, 0, split.Permanent)
	require.Len(t, split.Allocations, 2)
	assert.Equal(t, c1.Id, split.Allocations[0].CheckinId)
	assert.Equal(t, 2000, split.Allocations[0].Amount)
	assert.Equal(t, c2.Id, split.Allocations[1].CheckinId)
	assert.Equal(t, 2000, split.Allocations[1].Amount)
}

func TestRefundWallet_PermanentFirst(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 10000)
	now := common.NowInStartupTimezone().Unix()
	seedActiveTemporary(t, int64(user.Id), 3000, now+3600, "2026-08-08")

	split, err := PreConsumeWallet(user.Id, 5000)
	require.NoError(t, err)
	assert.Equal(t, 3000, split.Temporary)
	assert.Equal(t, 2000, split.Permanent)

	// 退还 5000：优先退永久 2000，再退限时 3000
	result, err := RefundWallet(user.Id, 5000, split)
	require.NoError(t, err)
	assert.Equal(t, 2000, result.Permanent)
	assert.Equal(t, 3000, result.Temporary)

	// 永久和限时都恢复
	assert.Equal(t, 10000, getPermanentQuota(t, user.Id))
	quota, err := GetActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 3000, quota)
}

func TestRefundWallet_ExpiredTemporaryNotRestored(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 10000)
	now := common.NowInStartupTimezone().Unix()
	c := seedActiveTemporary(t, int64(user.Id), 3000, now+3600, "2026-08-08")

	split, err := PreConsumeWallet(user.Id, 5000)
	require.NoError(t, err)
	assert.Equal(t, 3000, split.Temporary)
	assert.Equal(t, 2000, split.Permanent)

	// 模拟跨午夜：限时额度已过期
	require.NoError(t, DB.Model(&Checkin{}).Where("id = ?", c.Id).Update("quota_expires_at", now-60).Error)
	result, err := RefundWallet(user.Id, 5000, split)
	require.NoError(t, err)
	assert.Equal(t, 2000, result.Permanent)
	assert.Equal(t, 0, result.Temporary)

	assert.Equal(t, 10000, getPermanentQuota(t, user.Id))
	quota, err := GetActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 0, quota)
}

func TestRefundWallet_MultiBucketExpiredNotRestored(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 10000)
	now := common.NowInStartupTimezone().Unix()
	// 昨天的桶（预扣后过期）+ 今天的桶（预扣后仍有效）
	c1 := seedActiveTemporary(t, int64(user.Id), 2000, now+3600, "2026-08-07")
	c2 := seedActiveTemporary(t, int64(user.Id), 3000, now+7200, "2026-08-08")

	// 预扣 4000：先扣昨天桶 2000，再扣今天桶 2000
	split, err := PreConsumeWallet(user.Id, 4000)
	require.NoError(t, err)
	require.Len(t, split.Allocations, 2)

	// 模拟跨午夜：昨天桶过期，今天桶仍有效
	require.NoError(t, DB.Model(&Checkin{}).Where("id = ?", c1.Id).Update("quota_expires_at", now-60).Error)

	// 退款 4000：昨天桶 2000 过期丢弃，今天桶 2000 恢复（逆序先退今天桶）
	result, err := RefundWallet(user.Id, 4000, split)
	require.NoError(t, err)
	assert.Equal(t, 0, result.Permanent)
	assert.Equal(t, 2000, result.Temporary)
	require.Len(t, result.Allocations, 1)
	assert.Equal(t, c2.Id, result.Allocations[0].CheckinId)

	assert.Equal(t, 10000, getPermanentQuota(t, user.Id))
	// 今天桶恢复满 3000（预扣 2000 + 剩余 1000），昨天桶过期丢弃
	quota, err := GetActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 3000, quota)
}

func TestRefundWallet_NoSplitRefundsPermanent(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 10000)

	// 无拆分信息（旧数据）：全额按永久额度退还
	result, err := RefundWallet(user.Id, 3000, nil)
	require.NoError(t, err)
	assert.Equal(t, 3000, result.Permanent)
	assert.Equal(t, 0, result.Temporary)
	assert.Equal(t, 13000, getPermanentQuota(t, user.Id))
}

func TestPreConsumeWallet_ConcurrentNoDoubleSpend(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 100000)
	now := common.NowInStartupTimezone().Unix()
	seedActiveTemporary(t, int64(user.Id), 3000, now+3600, "2026-08-08")

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = PreConsumeWallet(user.Id, 1000)
		}(i)
	}
	wg.Wait()

	for i := range errs {
		require.NoError(t, errs[i])
	}

	// 限时额度 3000 被 3 次请求消费完，剩余 7 次从永久扣 7000
	quota, err := GetActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 0, quota)
	assert.Equal(t, 100000-7000, getPermanentQuota(t, user.Id))
}

func TestPreConsumeWallet_ConcurrentNoNegativeBalance(t *testing.T) {
	truncateTables(t)
	// 永久 0 + 限时 1000：并发预扣 1000，只允许一个成功消耗限时，不会把永久扣成负数
	user := seedWalletUser(t, 0)
	now := common.NowInStartupTimezone().Unix()
	seedActiveTemporary(t, int64(user.Id), 1000, now+3600, "2026-08-08")

	const goroutines = 5
	var wg sync.WaitGroup
	wg.Add(goroutines)
	errs := make([]error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = PreConsumeWallet(user.Id, 1000)
		}(i)
	}
	wg.Wait()

	successCount := 0
	for i := range errs {
		if errs[i] == nil {
			successCount++
		} else {
			require.ErrorIs(t, errs[i], ErrInsufficientWalletBalance)
		}
	}
	// 限时额度 1000 只够一个请求
	assert.Equal(t, 1, successCount)

	// 永久余额不为负
	assert.GreaterOrEqual(t, getPermanentQuota(t, user.Id), 0)
	quota, err := GetActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 0, quota)
}

func TestTopUpWallet_ContinuesTemporaryFirst(t *testing.T) {
	truncateTables(t)
	user := seedWalletUser(t, 10000)
	now := common.NowInStartupTimezone().Unix()
	seedActiveTemporary(t, int64(user.Id), 3000, now+3600, "2026-08-08")

	// 预扣 5000：限时 3000 + 永久 2000
	split, err := PreConsumeWallet(user.Id, 5000)
	require.NoError(t, err)
	assert.Equal(t, 3000, split.Temporary)

	// 补扣 1000：限时已用完，全部从永久扣（结算补扣允许欠费）
	topUp, err := TopUpWallet(user.Id, 1000)
	require.NoError(t, err)
	assert.Equal(t, 0, topUp.Temporary)
	assert.Equal(t, 1000, topUp.Permanent)
	assert.Equal(t, 10000-3000, getPermanentQuota(t, user.Id))
}

func TestTopUpWallet_AllowsArrears(t *testing.T) {
	truncateTables(t)
	// 永久 1000 + 限时 2000，预扣 2500（限时 2000 + 永久 500），补扣 2000 允许欠费
	user := seedWalletUser(t, 1000)
	now := common.NowInStartupTimezone().Unix()
	seedActiveTemporary(t, int64(user.Id), 2000, now+3600, "2026-08-08")

	split, err := PreConsumeWallet(user.Id, 2500)
	require.NoError(t, err)
	assert.Equal(t, 2000, split.Temporary)
	assert.Equal(t, 500, split.Permanent)

	// 结算补扣：允许负余额
	topUp, err := TopUpWallet(user.Id, 2000)
	require.NoError(t, err)
	assert.Equal(t, 0, topUp.Temporary)
	assert.Equal(t, 2000, topUp.Permanent)
	assert.Equal(t, -1500, getPermanentQuota(t, user.Id))
}

package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBillingSessionReserveRollbackSyncsSplit 验证 Reserve 预扣令牌失败回滚后，
// WalletFunding 的累计拆分与额度桶列表同步更新，避免后续退款按过期累计重复退永久额度。
func TestBillingSessionReserveRollbackSyncsSplit(t *testing.T) {
	truncate(t)
	seedUser(t, 1, 10000)
	// 令牌额度为 0，导致 Reserve 阶段的 reserveToken 失败
	seedToken(t, 1, 1, "sk-reserve-rollback", 0)
	now := common.NowInStartupTimezone().Unix()
	c := seedTemporaryCheckinOn(t, 1, 3000, now+3600, "2026-08-08")

	// 初始预扣 5000（限时 3000 + 永久 2000），模拟已完成的 preConsume
	funding := &WalletFunding{userId: 1}
	require.NoError(t, funding.PreConsume(5000))
	assert.Equal(t, 3000, funding.tempConsumed)
	assert.Equal(t, 2000, funding.permConsumed)

	relayInfo := &relaycommon.RelayInfo{
		UserId:     1,
		TokenId:    1,
		TokenKey:   "sk-reserve-rollback",
		IsPlayground: false,
	}
	session := &BillingSession{
		relayInfo:        relayInfo,
		funding:          funding,
		preConsumedQuota: 5000,
		tokenConsumed:    5000,
	}
	relayInfo.Billing = session

	// Reserve 补扣 3000：资金来源先扣（永久 3000），令牌预扣失败后回滚
	require.Error(t, session.Reserve(8000))

	// 回滚后拆分状态同步：只回滚了本次补扣的 3000 永久额度
	assert.Equal(t, 5000, funding.consumed)
	assert.Equal(t, 3000, funding.tempConsumed)
	assert.Equal(t, 2000, funding.permConsumed)
	require.Len(t, funding.allocations, 1)
	assert.Equal(t, c.Id, funding.allocations[0].CheckinId)
	assert.Equal(t, 3000, funding.allocations[0].Amount)

	// 用户永久额度 = 10000 - 2000（初始预扣的永久部分）
	var u model.User
	require.NoError(t, model.DB.First(&u, 1).Error)
	assert.Equal(t, 8000, u.Quota)

	// 后续完整退款按剩余拆分执行：优先退永久 2000，再退限时 3000（未过期）
	require.NoError(t, funding.Refund())
	assert.Equal(t, 0, funding.consumed)
	assert.Equal(t, 0, funding.tempConsumed)
	assert.Equal(t, 0, funding.permConsumed)
	assert.Empty(t, funding.allocations)

	require.NoError(t, model.DB.First(&u, 1).Error)
	assert.Equal(t, 10000, u.Quota)
	quota, err := model.GetActiveTemporaryQuota(1)
	require.NoError(t, err)
	assert.Equal(t, 3000, quota)
}

package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setCheckinSetting(t *testing.T, setting operation_setting.CheckinSetting) {
	t.Helper()
	prev := *operation_setting.GetCheckinSetting()
	*operation_setting.GetCheckinSetting() = setting
	t.Cleanup(func() {
		*operation_setting.GetCheckinSetting() = prev
	})
}

func TestUserCheckin_PermanentReward(t *testing.T) {
	truncateTables(t)
	setCheckinSetting(t, operation_setting.CheckinSetting{
		Enabled:               true,
		RewardType:            operation_setting.RewardTypePermanent,
		AvailableFromMinutes:  0,
		RandomMode:            false,
		FixedQuota:            5000,
	})

	user := &User{Username: "checkin_perm_user", Password: "password", Quota: 10000}
	require.NoError(t, DB.Create(user).Error)

	checkin, err := UserCheckin(user.Id)
	require.NoError(t, err)
	assert.Equal(t, CheckinQuotaTypePermanent, checkin.QuotaType)
	assert.Equal(t, 5000, checkin.QuotaAwarded)
	assert.Equal(t, 0, checkin.QuotaRemaining)

	// 永久余额增加 5000
	var u User
	require.NoError(t, DB.First(&u, user.Id).Error)
	assert.Equal(t, 15000, u.Quota)

	// 当天重复签到被拒
	_, err = UserCheckin(user.Id)
	require.Error(t, err)
}

func TestUserCheckin_TemporaryReward(t *testing.T) {
	truncateTables(t)
	prevTZ := common.StartupTimezoneName()
	t.Setenv("TZ", "Asia/Shanghai")
	common.InitStartupTimezone()
	t.Cleanup(func() {
		if prevTZ != "" {
			t.Setenv("TZ", prevTZ)
		}
		common.InitStartupTimezone()
	})

	setCheckinSetting(t, operation_setting.CheckinSetting{
		Enabled:               true,
		RewardType:            operation_setting.RewardTypeTemporary,
		AvailableFromMinutes:  0,
		RandomMode:            false,
		FixedQuota:            5000,
	})

	user := &User{Username: "checkin_temp_user", Password: "password", Quota: 10000}
	require.NoError(t, DB.Create(user).Error)

	checkin, err := UserCheckin(user.Id)
	require.NoError(t, err)
	assert.Equal(t, CheckinQuotaTypeTemporary, checkin.QuotaType)
	assert.Equal(t, 5000, checkin.QuotaAwarded)
	assert.Equal(t, 5000, checkin.QuotaRemaining)
	require.Greater(t, checkin.QuotaExpiresAt, int64(0))

	// 永久余额不变
	var u User
	require.NoError(t, DB.First(&u, user.Id).Error)
	assert.Equal(t, 10000, u.Quota)

	// 限时额度可查询
	quota, err := GetActiveTemporaryQuota(user.Id)
	require.NoError(t, err)
	assert.Equal(t, 5000, quota)

	// 失效时间为次日 00:00（Asia/Shanghai）
	now := common.NowInStartupTimezone()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	tomorrowStart := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	assert.Equal(t, tomorrowStart.Unix(), checkin.QuotaExpiresAt)
}

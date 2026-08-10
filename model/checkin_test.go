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
		Enabled:              true,
		RewardType:           operation_setting.RewardTypePermanent,
		AvailableFromMinutes: 0,
		RandomMode:           false,
		FixedQuota:           5000,
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
	setCheckinSetting(t, operation_setting.CheckinSetting{
		Enabled:              true,
		RewardType:           operation_setting.RewardTypeTemporary,
		AvailableFromMinutes: 0,
		RandomMode:           false,
		FixedQuota:           5000,
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

	// 失效时间为北京时间次日 00:00
	now := common.NowInCheckinTimezone()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	tomorrowStart := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
	assert.Equal(t, tomorrowStart.Unix(), checkin.QuotaExpiresAt)
}

func TestCheckinAvailableCheck_UsesBeijingTimezone(t *testing.T) {
	setting := &operation_setting.CheckinSetting{AvailableFromMinutes: 480}
	serverLoc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	// 纽约 2026-08-07 19:59 等于北京时间 2026-08-08 07:59。
	beforeOpen := time.Date(2026, 8, 7, 19, 59, 0, 0, serverLoc)
	require.Error(t, checkinAvailableCheck(setting, beforeOpen))

	// 纽约 2026-08-07 20:00 等于北京时间 2026-08-08 08:00。
	atOpen := time.Date(2026, 8, 7, 20, 0, 0, 0, serverLoc)
	require.NoError(t, checkinAvailableCheck(setting, atOpen))
}

func TestNextDayMidnightUnix_UsesBeijingTimezone(t *testing.T) {
	serverLoc, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)
	serverNow := time.Date(2026, 8, 8, 8, 30, 0, 0, serverLoc)

	shanghaiLoc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	expected := time.Date(2026, 8, 9, 0, 0, 0, 0, shanghaiLoc).Unix()

	assert.Equal(t, expected, nextDayMidnightUnix(serverNow))
}

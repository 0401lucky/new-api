package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setStartupTimezone 在测试内临时切换服务启动时区，结束后恢复。
func setStartupTimezone(t *testing.T, tz string) {
	t.Helper()
	t.Setenv("TZ", tz)
	common.InitStartupTimezone()
	t.Cleanup(func() {
		common.InitStartupTimezone()
	})
}

// withCheckinSetting 临时替换签到配置，结束后恢复。
func withCheckinSetting(t *testing.T, setting operation_setting.CheckinSetting) {
	t.Helper()
	prev := *operation_setting.GetCheckinSetting()
	*operation_setting.GetCheckinSetting() = setting
	t.Cleanup(func() {
		*operation_setting.GetCheckinSetting() = prev
	})
}

// shanghai 构造 Asia/Shanghai 时区的指定日期时间
func shanghai(t *testing.T, y int, month time.Month, d, h, min int) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	return time.Date(y, month, d, h, min, 0, 0, loc)
}

func TestComputeCheckinTimeInfo_NotOpenBeforeAvailableFrom(t *testing.T) {
	setStartupTimezone(t, "Asia/Shanghai")
	withCheckinSetting(t, operation_setting.CheckinSetting{
		Enabled:               true,
		RewardType:            operation_setting.RewardTypePermanent,
		AvailableFromMinutes:  480, // 08:00
		RandomMode:            true,
		MinQuota:              100,
		MaxQuota:              200,
	})

	// 07:59 开放前
	info, err := ComputeCheckinTimeInfoAt(1, shanghai(t, 2026, 8, 8, 7, 59), false)
	require.NoError(t, err)
	assert.Equal(t, CheckinStateNotOpen, info.State)
	assert.Equal(t, "2026-08-08", info.CurrentDate)
	// next_transition 为 08:00
	assert.Equal(t, shanghai(t, 2026, 8, 8, 8, 0).Unix(), info.NextTransitionAt)
	// 失效时间为次日 00:00
	assert.Equal(t, shanghai(t, 2026, 8, 9, 0, 0).Unix(), info.ExpiresAt)
}

func TestComputeCheckinTimeInfo_AvailableAtExactOpenTime(t *testing.T) {
	setStartupTimezone(t, "Asia/Shanghai")
	withCheckinSetting(t, operation_setting.CheckinSetting{
		Enabled:               true,
		RewardType:            operation_setting.RewardTypePermanent,
		AvailableFromMinutes:  480, // 08:00
	})

	// 08:00 整点开放
	info, err := ComputeCheckinTimeInfoAt(1, shanghai(t, 2026, 8, 8, 8, 0), false)
	require.NoError(t, err)
	assert.Equal(t, CheckinStateAvailable, info.State)
	assert.Equal(t, shanghai(t, 2026, 8, 8, 8, 0).Unix(), info.AvailableFrom)
	assert.Equal(t, shanghai(t, 2026, 8, 9, 0, 0).Unix(), info.NextTransitionAt)
}

func TestComputeCheckinTimeInfo_AvailableLateEvening(t *testing.T) {
	setStartupTimezone(t, "Asia/Shanghai")
	withCheckinSetting(t, operation_setting.CheckinSetting{
		Enabled:               true,
		RewardType:            operation_setting.RewardTypePermanent,
		AvailableFromMinutes:  480,
	})

	// 23:59 仍可签到，额度失效时间为次日 00:00
	info, err := ComputeCheckinTimeInfoAt(1, shanghai(t, 2026, 8, 8, 23, 59), false)
	require.NoError(t, err)
	assert.Equal(t, CheckinStateAvailable, info.State)
	assert.Equal(t, shanghai(t, 2026, 8, 9, 0, 0).Unix(), info.ExpiresAt)
}

func TestComputeCheckinTimeInfo_NextDayMidnightStartsNewCycle(t *testing.T) {
	setStartupTimezone(t, "Asia/Shanghai")
	withCheckinSetting(t, operation_setting.CheckinSetting{
		Enabled:               true,
		RewardType:            operation_setting.RewardTypePermanent,
		AvailableFromMinutes:  480, // 08:00 开放
	})

	// 次日 00:00 进入新周期：尚未到开放时间
	info, err := ComputeCheckinTimeInfoAt(1, shanghai(t, 2026, 8, 9, 0, 0), false)
	require.NoError(t, err)
	assert.Equal(t, CheckinStateNotOpen, info.State)
	assert.Equal(t, "2026-08-09", info.CurrentDate)
	assert.Equal(t, shanghai(t, 2026, 8, 9, 8, 0).Unix(), info.NextTransitionAt)
}

func TestComputeCheckinTimeInfo_CheckedState(t *testing.T) {
	setStartupTimezone(t, "Asia/Shanghai")
	withCheckinSetting(t, operation_setting.CheckinSetting{
		Enabled:               true,
		RewardType:            operation_setting.RewardTypePermanent,
		AvailableFromMinutes:  480,
	})

	info, err := ComputeCheckinTimeInfoAt(1, shanghai(t, 2026, 8, 8, 10, 0), true)
	require.NoError(t, err)
	assert.Equal(t, CheckinStateChecked, info.State)
}

func TestComputeCheckinTimeInfo_DefaultOpenMidnight(t *testing.T) {
	setStartupTimezone(t, "Asia/Shanghai")
	withCheckinSetting(t, operation_setting.CheckinSetting{
		Enabled:               true,
		RewardType:            operation_setting.RewardTypePermanent,
		AvailableFromMinutes:  0, // 00:00 开放（默认）
	})

	// 00:00 整点即可签到
	info, err := ComputeCheckinTimeInfoAt(1, shanghai(t, 2026, 8, 8, 0, 0), false)
	require.NoError(t, err)
	assert.Equal(t, CheckinStateAvailable, info.State)
	assert.Equal(t, shanghai(t, 2026, 8, 8, 0, 0).Unix(), info.AvailableFrom)
}

package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlackroomBanBlockMessageUsesBeijingTime(t *testing.T) {
	previousLocation := time.Local
	time.Local = time.FixedZone("server-local", -5*60*60)
	t.Cleanup(func() {
		time.Local = previousLocation
	})

	ban := &BlackroomBan{
		Reason:      "测试封禁",
		BannedUntil: time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC).Unix(),
	}

	require.Equal(
		t,
		"账号已进入小黑屋：测试封禁，解封时间：2026-08-08 08:00:00（北京时间）",
		ban.BlockMessage(),
	)
}

func TestBlackroomBanLifecycle(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "blackroom-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)

	ban, created, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             BlackroomBanSourceManual,
		Reason:             "测试封禁",
		BanDurationSeconds: 3600,
		BannedUntil:        common.GetTimestamp() + 3600,
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, BlackroomBanStatusActive, ban.Status)

	cached, err := GetActiveBlackroomBanCached(user.Id)
	require.NoError(t, err)
	require.Equal(t, ban.Id, cached.Id)

	released, err := ReleaseBlackroomBan(ban.Id, 1, "测试解封")
	require.NoError(t, err)
	require.Equal(t, BlackroomBanStatusReleased, released.Status)

	_, err = GetActiveBlackroomBanCached(user.Id)
	require.Error(t, err)
}

func TestExpireDueBlackroomBans(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "blackroom-expire-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)

	ban, _, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             BlackroomBanSourceAuto,
		Reason:             "测试过期",
		BanDurationSeconds: 1,
		BannedUntil:        common.GetTimestamp() - 1,
	})
	require.NoError(t, err)

	count, err := ExpireDueBlackroomBans()
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	expired, err := GetBlackroomBanByID(ban.Id)
	require.NoError(t, err)
	require.Equal(t, BlackroomBanStatusExpired, expired.Status)
}

func TestUpsertActiveBlackroomBanKeepsSingleActiveRecord(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "blackroom-single-active-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)

	first, created, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             BlackroomBanSourceAuto,
		Reason:             "首次封禁",
		BanDurationSeconds: 3600,
		BannedUntil:        common.GetTimestamp() + 3600,
	})
	require.NoError(t, err)
	require.True(t, created)

	second, created, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             BlackroomBanSourceAuto,
		Reason:             "更新封禁",
		IpCount:            12,
		BanDurationSeconds: 7200,
		BannedUntil:        common.GetTimestamp() + 7200,
	})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, first.Id, second.Id)
	require.Equal(t, "更新封禁", second.Reason)

	var activeCount int64
	require.NoError(t, DB.Model(&BlackroomBan{}).
		Where("user_id = ? AND status = ?", user.Id, BlackroomBanStatusActive).
		Count(&activeCount).Error)
	require.Equal(t, int64(1), activeCount)
	require.NotNil(t, second.ActiveKey)

	released, err := ReleaseBlackroomBan(second.Id, 1, "测试释放唯一键")
	require.NoError(t, err)
	require.Equal(t, BlackroomBanStatusReleased, released.Status)
	require.Nil(t, released.ActiveKey)

	third, created, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             BlackroomBanSourceManual,
		Reason:             "释放后再次封禁",
		BanDurationSeconds: 3600,
		BannedUntil:        common.GetTimestamp() + 3600,
	})
	require.NoError(t, err)
	require.True(t, created)
	require.NotEqual(t, second.Id, third.Id)
	require.NotNil(t, third.ActiveKey)
}

func TestGetLatestBlackroomBanWindowEnd(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "blackroom-window-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)

	windowEnd, err := GetLatestBlackroomBanWindowEnd(user.Id)
	require.NoError(t, err)
	require.Equal(t, int64(0), windowEnd)

	require.NoError(t, DB.Create(&BlackroomBan{
		UserId:      user.Id,
		Username:    user.Username,
		Status:      BlackroomBanStatusExpired,
		Source:      BlackroomBanSourceAuto,
		WindowStart: 500,
		WindowEnd:   1000,
	}).Error)
	require.NoError(t, DB.Create(&BlackroomBan{
		UserId:      user.Id,
		Username:    user.Username,
		Status:      BlackroomBanStatusReleased,
		Source:      BlackroomBanSourceManual,
		WindowStart: 1500,
		WindowEnd:   2000,
	}).Error)

	windowEnd, err = GetLatestBlackroomBanWindowEnd(user.Id)
	require.NoError(t, err)
	require.Equal(t, int64(2000), windowEnd)
}

func TestExpireDueBlackroomBansRestoresExternalUserStatus(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "blackroom-expire-external-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusDisabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)

	ban, _, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             BlackroomBanSourceExternal,
		Reason:             "外部封禁到期",
		BanDurationSeconds: 1,
		BannedUntil:        common.GetTimestamp() - 1,
	})
	require.NoError(t, err)

	count, err := ExpireDueBlackroomBans()
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	expired, err := GetBlackroomBanByID(ban.Id)
	require.NoError(t, err)
	require.Equal(t, BlackroomBanStatusExpired, expired.Status)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, reloaded.Status)
}

func TestExpireDueBlackroomBansKeepsNonExternalUserStatus(t *testing.T) {
	truncateTables(t)

	// status 预置为禁用，验证 auto 来源到期不会误恢复账号状态
	user := User{
		Username: "blackroom-expire-auto-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusDisabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)

	_, _, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             BlackroomBanSourceAuto,
		Reason:             "自动封禁到期",
		BanDurationSeconds: 1,
		BannedUntil:        common.GetTimestamp() - 1,
	})
	require.NoError(t, err)

	count, err := ExpireDueBlackroomBans()
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusDisabled, reloaded.Status)
}

func TestSetBlackroomUserStatus(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "blackroom-status-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)

	require.NoError(t, SetBlackroomUserStatus(user.Id, common.UserStatusDisabled))
	var reloaded User
	require.NoError(t, DB.First(&reloaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusDisabled, reloaded.Status)

	require.NoError(t, SetBlackroomUserStatus(user.Id, common.UserStatusEnabled))
	require.NoError(t, DB.First(&reloaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, reloaded.Status)
}

func TestReleaseExternalBlackroomBanRestoresUserStatus(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "blackroom-external-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusDisabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)

	ban, _, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:   user.Id,
		Username: user.Username,
		Source:   BlackroomBanSourceExternal,
		Reason:   "外部封禁",
	})
	require.NoError(t, err)

	_, err = ReleaseBlackroomBan(ban.Id, 1, "测试解封")
	require.NoError(t, err)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusEnabled, reloaded.Status)
}

func TestReleaseAutoBlackroomBanKeepsUserStatus(t *testing.T) {
	truncateTables(t)

	// status 预置为禁用，验证 auto 来源解封不会改动它
	user := User{
		Username: "blackroom-auto-status-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusDisabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)

	ban, _, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:   user.Id,
		Username: user.Username,
		Source:   BlackroomBanSourceAuto,
		Reason:   "自动封禁",
	})
	require.NoError(t, err)

	_, err = ReleaseBlackroomBan(ban.Id, 1, "测试解封")
	require.NoError(t, err)

	var reloaded User
	require.NoError(t, DB.First(&reloaded, "id = ?", user.Id).Error)
	require.Equal(t, common.UserStatusDisabled, reloaded.Status)
}

// 每次封禁变更都留下不可变事件：状态表被覆盖掉的中间状态仍可追溯。
func TestBlackroomBanEventsRecordLifecycle(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "blackroom-event-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)
	now := common.GetTimestamp()

	ban, created, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             BlackroomBanSourceAuto,
		Reason:             "首次命中",
		BanDurationSeconds: 3600,
		BannedUntil:        now + 3600,
	})
	require.NoError(t, err)
	require.True(t, created)

	events, err := ListBlackroomBanEvents(user.Id, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, BlackroomBanEventApply, events[0].EventType)
	assert.Equal(t, "首次命中", events[0].Reason)
	assert.Equal(t, now+3600, events[0].BannedUntil)

	// 同一用户再次命中会覆盖状态表上的 reason/evidence，事件表必须留住两条。
	_, created, err = UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             BlackroomBanSourceAuto,
		Reason:             "再次命中",
		BanDurationSeconds: 7200,
		BannedUntil:        now + 7200,
	})
	require.NoError(t, err)
	require.False(t, created)

	events, err = ListBlackroomBanEvents(user.Id, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, BlackroomBanEventReapply, events[0].EventType)
	assert.Equal(t, "再次命中", events[0].Reason)
	assert.Equal(t, BlackroomBanEventApply, events[1].EventType)
	assert.Equal(t, "首次命中", events[1].Reason)

	_, err = ReleaseBlackroomBan(ban.Id, 42, "误判")
	require.NoError(t, err)

	events, err = ListBlackroomBanEvents(user.Id, 10)
	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.Equal(t, BlackroomBanEventRelease, events[0].EventType)
	assert.Equal(t, 42, events[0].ActorUserId)
}

// 到期封禁同样留下事件。
func TestBlackroomBanEventsRecordExpiry(t *testing.T) {
	truncateTables(t)

	user := User{
		Username: "blackroom-expire-event-user",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, DB.Create(&user).Error)
	now := common.GetTimestamp()

	_, _, err := UpsertActiveBlackroomBan(BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             BlackroomBanSourceAuto,
		Reason:             "到期用例",
		BanDurationSeconds: 60,
		BannedUntil:        now - 1,
	})
	require.NoError(t, err)

	count, err := ExpireDueBlackroomBans()
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	events, err := ListBlackroomBanEvents(user.Id, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, BlackroomBanEventExpire, events[0].EventType)
	assert.Equal(t, "到期用例", events[0].Reason)
}

// 事件是审计凭据，不允许改写或删除。
func TestBlackroomBanEventsAreImmutable(t *testing.T) {
	truncateTables(t)

	event := BlackroomBanEvent{
		UserId:    77,
		EventType: BlackroomBanEventApply,
		Reason:    "不可变用例",
		CreatedAt: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&event).Error)

	require.ErrorIs(t, DB.Model(&BlackroomBanEvent{}).
		Where("id = ?", event.Id).
		Update("reason", "被改写").Error, ErrBlackroomBanEventImmutable)
	require.ErrorIs(t, DB.Delete(&BlackroomBanEvent{}, event.Id).Error, ErrBlackroomBanEventImmutable)

	var reloaded BlackroomBanEvent
	require.NoError(t, DB.First(&reloaded, event.Id).Error)
	assert.Equal(t, "不可变用例", reloaded.Reason)
}

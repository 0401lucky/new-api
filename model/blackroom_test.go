package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

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

package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedBlackroomConsumeLogs(t *testing.T, userID int, ipPrefix string, ipCount int, createdAt int64) {
	t.Helper()
	for i := 0; i < ipCount; i++ {
		require.NoError(t, model.LOG_DB.Create(&model.Log{
			UserId:    userID,
			Type:      model.LogTypeConsume,
			Ip:        fmt.Sprintf("%s.%d", ipPrefix, i+1),
			CreatedAt: createdAt,
			Group:     "default",
		}).Error)
	}
}

func blackroomCandidateFor(userID int, ipCount int) model.BlackroomIPCandidate {
	return model.BlackroomIPCandidate{
		UserId:       userID,
		Username:     "ext_user",
		UserGroup:    "default",
		IpCount:      ipCount,
		RequestCount: ipCount,
	}
}

// 上次封禁窗口内的旧日志不得再次触发封禁：到期后无新违规时必须跳过。
func TestHandleBlackroomCandidateSkipsAlreadyPunishedLogs(t *testing.T) {
	truncate(t)

	userID := 9101
	seedBlackroomUser(t, userID, common.RoleCommonUser)
	now := common.GetTimestamp()
	windowStart := now - 86400

	seedBlackroomConsumeLogs(t, userID, "10.0.0", 10, now-7200)
	require.NoError(t, model.DB.Create(&model.BlackroomBan{
		UserId:             userID,
		Username:           "ext_user",
		Status:             model.BlackroomBanStatusExpired,
		Source:             model.BlackroomBanSourceAuto,
		WindowStart:        windowStart - 3600,
		WindowEnd:          now - 3600,
		BanDurationSeconds: 3600,
		BannedUntil:        now - 60,
	}).Error)

	created, updated, skipped, err := handleBlackroomCandidate(testBlackroomSetting(), blackroomCandidateFor(userID, 10), windowStart, now)
	require.NoError(t, err)
	assert.False(t, created)
	assert.False(t, updated)
	assert.True(t, skipped)

	var banCount int64
	require.NoError(t, model.DB.Model(&model.BlackroomBan{}).
		Where("user_id = ?", userID).
		Count(&banCount).Error)
	assert.Equal(t, int64(1), banCount)
}

// 生效中的封禁不得被旧日志续期：banned_until 必须保持不变。
func TestHandleBlackroomCandidateDoesNotExtendActiveBanWithOldLogs(t *testing.T) {
	truncate(t)

	userID := 9102
	seedBlackroomUser(t, userID, common.RoleCommonUser)
	now := common.GetTimestamp()
	windowStart := now - 86400
	bannedUntil := now + 1800

	seedBlackroomConsumeLogs(t, userID, "10.0.1", 10, now-7200)
	ban, _, err := model.UpsertActiveBlackroomBan(model.BlackroomBanInput{
		UserId:             userID,
		Username:           "ext_user",
		Source:             model.BlackroomBanSourceAuto,
		Reason:             "首次封禁",
		IpCount:            10,
		WindowStart:        windowStart - 600,
		WindowEnd:          now - 600,
		BanDurationSeconds: 21600,
		BannedUntil:        bannedUntil,
	})
	require.NoError(t, err)

	created, updated, skipped, err := handleBlackroomCandidate(testBlackroomSetting(), blackroomCandidateFor(userID, 10), windowStart, now)
	require.NoError(t, err)
	assert.False(t, created)
	assert.False(t, updated)
	assert.True(t, skipped)

	reloaded, err := model.GetBlackroomBanByID(ban.Id)
	require.NoError(t, err)
	assert.Equal(t, bannedUntil, reloaded.BannedUntil)
}

// 上次封禁之后产生的新违规日志必须按新日志的 IP 数重新定罪，而不是全窗口累计值。
func TestHandleBlackroomCandidateBansAgainOnFreshViolations(t *testing.T) {
	truncate(t)

	userID := 9103
	seedBlackroomUser(t, userID, common.RoleCommonUser)
	now := common.GetTimestamp()
	windowStart := now - 86400
	lastWindowEnd := now - 3600

	seedBlackroomConsumeLogs(t, userID, "10.0.2", 10, now-7200)
	require.NoError(t, model.DB.Create(&model.BlackroomBan{
		UserId:             userID,
		Username:           "ext_user",
		Status:             model.BlackroomBanStatusExpired,
		Source:             model.BlackroomBanSourceAuto,
		WindowStart:        windowStart - 3600,
		WindowEnd:          lastWindowEnd,
		BanDurationSeconds: 3600,
		BannedUntil:        now - 60,
	}).Error)
	seedBlackroomConsumeLogs(t, userID, "10.0.3", 8, now-1800)

	// 全窗口去重 IP 为 18，若未按上次封禁裁剪窗口会误判为永久封禁
	created, updated, skipped, err := handleBlackroomCandidate(testBlackroomSetting(), blackroomCandidateFor(userID, 18), windowStart, now)
	require.NoError(t, err)
	assert.True(t, created)
	assert.False(t, updated)
	assert.False(t, skipped)

	ban, err := model.GetActiveBlackroomBan(userID)
	require.NoError(t, err)
	assert.Equal(t, 8, ban.IpCount)
	assert.Equal(t, int64(6*3600), ban.BanDurationSeconds)
	require.Greater(t, ban.BannedUntil, int64(0))
	assert.GreaterOrEqual(t, ban.BannedUntil, now+6*3600)
	assert.Greater(t, ban.WindowStart, lastWindowEnd)
	assert.Contains(t, ban.Reason, "上次封禁后")
}

// 无历史封禁记录的候选用户仍按完整回看窗口正常封禁。
func TestHandleBlackroomCandidateBansNewUser(t *testing.T) {
	truncate(t)

	userID := 9104
	seedBlackroomUser(t, userID, common.RoleCommonUser)
	now := common.GetTimestamp()
	windowStart := now - 86400

	seedBlackroomConsumeLogs(t, userID, "10.0.4", 10, now-7200)

	created, updated, skipped, err := handleBlackroomCandidate(testBlackroomSetting(), blackroomCandidateFor(userID, 10), windowStart, now)
	require.NoError(t, err)
	assert.True(t, created)
	assert.False(t, updated)
	assert.False(t, skipped)

	ban, err := model.GetActiveBlackroomBan(userID)
	require.NoError(t, err)
	assert.Equal(t, 10, ban.IpCount)
	assert.Equal(t, int64(6*3600), ban.BanDurationSeconds)
	assert.GreaterOrEqual(t, ban.BannedUntil, now+6*3600)
	assert.Contains(t, ban.Reason, "24 小时内使用了 10 个不同 IP")
}

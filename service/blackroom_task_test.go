package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// seedBlackroomGeoObservation 造出一条带完整地理解析结果的观测。
func seedBlackroomGeoObservation(t *testing.T, userID int, ip string, country string, asn int64, observedAt int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.BlackroomIPMinute{
		UserId:           userID,
		Ip:               ip,
		Minute:           observedAt - observedAt%60,
		FirstSeenAt:      observedAt,
		LastSeenAt:       observedAt,
		RequestCount:     1,
		CountryISO:       country,
		AsnNumber:        asn,
		IpKind:           "public",
		EvidenceEligible: true,
	}).Error)
}

// seedBlackroomObservations 在审计表里造出 ipCount 个不同 IP 的观测，
// 这些观测发生在同一分钟。
func seedBlackroomObservations(t *testing.T, userID int, ipPrefix string, ipCount int, observedAt int64) {
	t.Helper()
	rows := make([]model.BlackroomIPMinute, 0, ipCount)
	for i := 0; i < ipCount; i++ {
		rows = append(rows, model.BlackroomIPMinute{
			UserId:       userID,
			Ip:           fmt.Sprintf("%s.%d", ipPrefix, i+1),
			Minute:       observedAt - observedAt%60,
			FirstSeenAt:  observedAt,
			LastSeenAt:   observedAt,
			RequestCount: 1,
		})
	}
	require.NoError(t, model.DB.Create(&rows).Error)
}

// 上次封禁窗口内的旧日志不得再次触发封禁：到期后无新违规时必须跳过。
func TestHandleBlackroomCandidateSkipsAlreadyPunishedLogs(t *testing.T) {
	truncate(t)

	userID := 9101
	seedBlackroomUser(t, userID, common.RoleCommonUser)
	now := common.GetTimestamp()
	windowStart := now - 86400

	seedBlackroomObservations(t, userID, "10.0.0", 10, now-7200)
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

	created, updated, skipped, err := handleBlackroomCandidate(testBlackroomSetting(), userID, now)
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

	seedBlackroomObservations(t, userID, "10.0.1", 10, now-7200)
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

	created, updated, skipped, err := handleBlackroomCandidate(testBlackroomSetting(), userID, now)
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

	seedBlackroomObservations(t, userID, "10.0.2", 10, now-7200)
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
	seedBlackroomObservations(t, userID, "10.0.3", 8, now-1800)

	// 全窗口去重 IP 为 18，若未按上次封禁裁剪窗口会误判为永久封禁
	created, updated, skipped, err := handleBlackroomCandidate(testBlackroomSetting(), userID, now)
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

	seedBlackroomObservations(t, userID, "10.0.4", 10, now-7200)

	created, updated, skipped, err := handleBlackroomCandidate(testBlackroomSetting(), userID, now)
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

// 影子模式下命中规则只留事件、不封禁，用于上线前先观察规则会命中谁。
func TestHandleBlackroomCandidateShadowModeRecordsWithoutBanning(t *testing.T) {
	truncate(t)

	userID := 9105
	seedBlackroomUser(t, userID, common.RoleCommonUser)
	now := common.GetTimestamp()

	seedBlackroomObservations(t, userID, "10.0.5", 10, now-1800)

	setting := testBlackroomSetting()
	setting.ShadowMode = true

	created, updated, skipped, err := handleBlackroomCandidate(setting, userID, now)
	require.NoError(t, err)
	assert.False(t, created)
	assert.False(t, updated)
	assert.True(t, skipped)

	_, err = model.GetActiveBlackroomBan(userID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	events, err := model.ListBlackroomBanEvents(userID, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, model.BlackroomBanEventShadowMatch, events[0].EventType)
	assert.Contains(t, events[0].Reason, "10 个不同 IP")
}

// 地理规则命中：短时间跨越多个国家与 ASN。IP 数低于阶梯阈值，因此这里
// 命中的只可能是地理规则。
func TestHandleBlackroomCandidateGeoRuleBansCrossCountryJump(t *testing.T) {
	truncate(t)

	userID := 9106
	seedBlackroomUser(t, userID, common.RoleCommonUser)
	now := common.GetTimestamp()

	seedBlackroomGeoObservation(t, userID, "203.0.113.10", "SG", 4657, now-180)
	seedBlackroomGeoObservation(t, userID, "203.0.113.11", "JP", 2497, now-120)
	seedBlackroomGeoObservation(t, userID, "203.0.113.12", "US", 15169, now-60)

	setting := testBlackroomSetting()
	setting.GeoEnabled = true
	setting.GeoCountryCount = 3
	setting.GeoASNCount = 3
	setting.GeoMinGapSeconds = 180
	setting.GeoDurationHours = 72

	created, updated, skipped, err := handleBlackroomCandidate(setting, userID, now)
	require.NoError(t, err)
	assert.True(t, created)
	assert.False(t, updated)
	assert.False(t, skipped)

	ban, err := model.GetActiveBlackroomBan(userID)
	require.NoError(t, err)
	assert.Equal(t, int64(72*3600), ban.BanDurationSeconds)
	assert.Contains(t, ban.Reason, "3 个国家 / 3 个 ASN")
	assert.Contains(t, ban.Reason, "60 秒内跨越")
}

// 相同 IP 数但缺少快速切换（间隔超过阈值）时不命中地理规则。
func TestHandleBlackroomCandidateGeoRuleIgnoresSlowSwitch(t *testing.T) {
	truncate(t)

	userID := 9107
	seedBlackroomUser(t, userID, common.RoleCommonUser)
	now := common.GetTimestamp()

	// 三个国家与 ASN 都在，但间隔 3600 秒，远超 180 秒阈值。
	seedBlackroomGeoObservation(t, userID, "203.0.113.10", "SG", 4657, now-7200)
	seedBlackroomGeoObservation(t, userID, "203.0.113.11", "JP", 2497, now-3600)
	seedBlackroomGeoObservation(t, userID, "203.0.113.12", "US", 15169, now-60)

	setting := testBlackroomSetting()
	setting.GeoEnabled = true
	setting.GeoCountryCount = 3
	setting.GeoASNCount = 3
	setting.GeoMinGapSeconds = 180

	created, updated, skipped, err := handleBlackroomCandidate(setting, userID, now)
	require.NoError(t, err)
	assert.False(t, created)
	assert.False(t, updated)
	assert.True(t, skipped)
}

// 关闭地理判定时，同样的跨国观测不触发封禁。
func TestHandleBlackroomCandidateGeoRuleDisabled(t *testing.T) {
	truncate(t)

	userID := 9108
	seedBlackroomUser(t, userID, common.RoleCommonUser)
	now := common.GetTimestamp()

	seedBlackroomGeoObservation(t, userID, "203.0.113.10", "SG", 4657, now-180)
	seedBlackroomGeoObservation(t, userID, "203.0.113.11", "JP", 2497, now-120)
	seedBlackroomGeoObservation(t, userID, "203.0.113.12", "US", 15169, now-60)

	setting := testBlackroomSetting()
	setting.GeoEnabled = false

	created, updated, skipped, err := handleBlackroomCandidate(setting, userID, now)
	require.NoError(t, err)
	assert.False(t, created)
	assert.False(t, updated)
	assert.True(t, skipped)
}

// 豁免名单里的用户即使超过阈值也不参与自动判定。
func TestHandleBlackroomCandidateSkipsExemptUsers(t *testing.T) {
	truncate(t)

	now := common.GetTimestamp()
	userID := 9109
	seedBlackroomUser(t, userID, common.RoleCommonUser)
	seedBlackroomObservations(t, userID, "10.0.6", 10, now-1800)

	setting := testBlackroomSetting()
	setting.ExemptUserIDs = []int{userID}

	_, _, skipped, err := handleBlackroomCandidate(setting, userID, now)
	require.NoError(t, err)
	assert.True(t, skipped)

	var banCount int64
	require.NoError(t, model.DB.Model(&model.BlackroomBan{}).Count(&banCount).Error)
	assert.Equal(t, int64(0), banCount)
}

// 管理员账号不参与自动判定。
func TestHandleBlackroomCandidateSkipsAdminUsers(t *testing.T) {
	truncate(t)

	now := common.GetTimestamp()
	userID := 9110
	seedBlackroomUser(t, userID, common.RoleAdminUser)
	seedBlackroomObservations(t, userID, "10.0.7", 10, now-1800)

	_, _, skipped, err := handleBlackroomCandidate(testBlackroomSetting(), userID, now)
	require.NoError(t, err)
	assert.True(t, skipped)

	var banCount int64
	require.NoError(t, model.DB.Model(&model.BlackroomBan{}).Count(&banCount).Error)
	assert.Equal(t, int64(0), banCount)
}

package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func testBlackroomSetting() *operation_setting.BlackroomSetting {
	return &operation_setting.BlackroomSetting{
		Enabled:        true,
		AutoBanEnabled: true,
		LookbackHours:  24,
		Rules: []operation_setting.BlackroomRule{
			{IPCount: 8, DurationHours: 6},
			{IPCount: 13, DurationHours: 72},
			{IPCount: 17, Permanent: true},
		},
		EscalationWindowDays:        30,
		EscalationTemporaryBanCount: 3,
	}
}

func TestResolveBlackroomBanDecision_NoRuleMatch(t *testing.T) {
	truncate(t)
	decision, err := resolveBlackroomBanDecision(testBlackroomSetting(), 1, 5, common.GetTimestamp(), true)
	require.NoError(t, err)
	require.False(t, decision.Matched)
}

func TestResolveBlackroomBanDecision_TemporaryRule(t *testing.T) {
	truncate(t)
	now := common.GetTimestamp()
	decision, err := resolveBlackroomBanDecision(testBlackroomSetting(), 1, 13, now, true)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.False(t, decision.Permanent)
	require.Equal(t, int64(72*3600), decision.DurationSeconds)
	require.Equal(t, now+int64(72*3600), decision.BannedUntil)
}

func TestResolveBlackroomBanDecision_PermanentRule(t *testing.T) {
	truncate(t)
	decision, err := resolveBlackroomBanDecision(testBlackroomSetting(), 1, 17, common.GetTimestamp(), true)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.True(t, decision.Permanent)
	require.Equal(t, int64(0), decision.BannedUntil)
}

func TestResolveBlackroomBanDecision_Escalation(t *testing.T) {
	truncate(t)
	setting := testBlackroomSetting()
	setting.EscalationTemporaryBanCount = 2
	now := common.GetTimestamp()
	userID := 4242

	_, _, err := model.UpsertActiveBlackroomBan(model.BlackroomBanInput{
		UserId:             userID,
		Source:             model.BlackroomBanSourceAuto,
		Reason:             "历史临时封禁",
		BanDurationSeconds: 6 * 3600,
		BannedUntil:        now - 3600,
	})
	require.NoError(t, err)

	decision, err := resolveBlackroomBanDecision(setting, userID, 8, now, true)
	require.NoError(t, err)
	require.True(t, decision.Matched)
	require.True(t, decision.Permanent)
	require.True(t, decision.Escalated)
}

func seedBlackroomUser(t *testing.T, id int, role int) {
	t.Helper()
	user := &model.User{
		Id:       id,
		Username: "ext_user",
		Role:     role,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, model.DB.Create(user).Error)
}

func TestCreateExternalBlackroomBan_TieredRule(t *testing.T) {
	truncate(t)
	seedBlackroomUser(t, 9001, common.RoleCommonUser)

	ban, err := CreateExternalBlackroomBan(9001, 13, "外部风控", "", false, 0)
	require.NoError(t, err)
	require.Equal(t, model.BlackroomBanSourceExternal, ban.Source)
	require.Equal(t, "外部风控", ban.Reason)
	require.Equal(t, int64(72*3600), ban.BanDurationSeconds)
	require.False(t, ban.IsPermanent())

	var reloaded model.User
	require.NoError(t, model.DB.First(&reloaded, "id = ?", 9001).Error)
	require.Equal(t, common.UserStatusDisabled, reloaded.Status)
}

func TestCreateExternalBlackroomBan_PermanentFlag(t *testing.T) {
	truncate(t)
	seedBlackroomUser(t, 9002, common.RoleCommonUser)

	ban, err := CreateExternalBlackroomBan(9002, 0, "", "", true, 0)
	require.NoError(t, err)
	require.True(t, ban.IsPermanent())
	require.NotEmpty(t, ban.Reason)
}

func TestCreateExternalBlackroomBan_ExplicitDuration(t *testing.T) {
	truncate(t)
	seedBlackroomUser(t, 9003, common.RoleCommonUser)

	ban, err := CreateExternalBlackroomBan(9003, 0, "", "", false, 5)
	require.NoError(t, err)
	require.Equal(t, int64(5*3600), ban.BanDurationSeconds)
	require.False(t, ban.IsPermanent())
}

func TestCreateExternalBlackroomBan_NoRulePermanentFallback(t *testing.T) {
	truncate(t)
	seedBlackroomUser(t, 9004, common.RoleCommonUser)

	ban, err := CreateExternalBlackroomBan(9004, 3, "", "", false, 0)
	require.NoError(t, err)
	require.True(t, ban.IsPermanent())
}

func TestCreateExternalBlackroomBan_RejectsAdmin(t *testing.T) {
	truncate(t)
	seedBlackroomUser(t, 9005, common.RoleAdminUser)

	_, err := CreateExternalBlackroomBan(9005, 17, "", "", false, 0)
	require.Error(t, err)
}

func TestCreateExternalBlackroomBan_KeepsManualBan(t *testing.T) {
	truncate(t)
	seedBlackroomUser(t, 9006, common.RoleCommonUser)

	manualBan, _, err := model.UpsertActiveBlackroomBan(model.BlackroomBanInput{
		UserId:   9006,
		Username: "ext_user",
		Source:   model.BlackroomBanSourceManual,
		Reason:   "管理员手动封禁",
	})
	require.NoError(t, err)

	ban, err := CreateExternalBlackroomBan(9006, 17, "外部风控", "", false, 0)
	require.NoError(t, err)
	require.Equal(t, manualBan.Id, ban.Id)
	require.Equal(t, model.BlackroomBanSourceManual, ban.Source)
	require.Equal(t, "管理员手动封禁", ban.Reason)
}

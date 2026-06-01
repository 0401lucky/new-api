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

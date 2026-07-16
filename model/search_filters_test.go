package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestSearchUsersMatchesPartialUserID(t *testing.T) {
	truncateTables(t)

	users := []User{
		{Id: 3, Username: "user-three", Password: "password", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-3"},
		{Id: 13, Username: "user-thirteen", Password: "password", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-13"},
		{Id: 42, Username: "user-forty-two", Password: "password", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-42"},
		{Id: 163, Username: "user-one-six-three", Password: "password", Status: common.UserStatusEnabled, Group: "default", AffCode: "aff-163"},
	}
	for i := range users {
		require.NoError(t, DB.Create(&users[i]).Error)
	}

	matched, total, err := SearchUsers("3", "", nil, nil, 0, 20)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)

	matchedIDs := make([]int, 0, len(matched))
	for _, user := range matched {
		matchedIDs = append(matchedIDs, user.Id)
	}
	require.ElementsMatch(t, []int{3, 13, 163}, matchedIDs)
}

func TestGetAllLogsFiltersByExactUserID(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	logs := []Log{
		{UserId: 3, Username: "target", Type: LogTypeConsume, CreatedAt: now, Quota: 10, PromptTokens: 1, CompletionTokens: 2},
		{UserId: 13, Username: "3", Type: LogTypeConsume, CreatedAt: now, Quota: 20, PromptTokens: 3, CompletionTokens: 4},
		{UserId: 163, Username: "other", Type: LogTypeConsume, CreatedAt: now, Quota: 30, PromptTokens: 5, CompletionTokens: 6},
	}
	for i := range logs {
		require.NoError(t, DB.Create(&logs[i]).Error)
	}

	matched, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", 3, "", 0, 20, 0, "", "", "", false)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, matched, 1)
	require.Equal(t, 3, matched[0].UserId)

	stat, err := SumUsedQuota(LogTypeUnknown, 0, 0, "", "", 3, "", 0, "")
	require.NoError(t, err)
	require.Equal(t, 10, stat.Quota)
	require.Equal(t, 1, stat.Rpm)
	require.Equal(t, 3, stat.Tpm)
}

func TestSumUsedQuotaEmptyResultReturnsZero(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM logs").Error)

	now := common.GetTimestamp()
	stat, err := SumUsedQuota(LogTypeUnknown, now+3600, now+7200, "", "", 0, "", 0, "")
	require.NoError(t, err)
	require.Equal(t, 0, stat.Quota)
	require.Equal(t, 0, stat.Rpm)
	require.Equal(t, 0, stat.Tpm)
}

func TestGetAllLogsFiltersNumericUsername(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	logs := []Log{
		{UserId: 3, Username: "target", Type: LogTypeConsume, CreatedAt: now, Quota: 10, PromptTokens: 1, CompletionTokens: 2},
		{UserId: 13, Username: "3", Type: LogTypeConsume, CreatedAt: now, Quota: 20, PromptTokens: 3, CompletionTokens: 4},
	}
	for i := range logs {
		require.NoError(t, DB.Create(&logs[i]).Error)
	}

	matched, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "3", 0, "", 0, 20, 0, "", "", "", false)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, matched, 1)
	require.Equal(t, 13, matched[0].UserId)
}

func TestGetAllLogsFiltersPromptCheckEntries(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	logs := []Log{
		{
			UserId:    3,
			Username:  "target",
			Type:      LogTypeError,
			CreatedAt: now,
			Content:   "prompt check block",
			Other: common.MapToJsonStr(map[string]interface{}{
				"prompt_check": map[string]interface{}{
					"action": "block",
					"score":  100,
				},
			}),
		},
		{
			UserId:    3,
			Username:  "target",
			Type:      LogTypeError,
			CreatedAt: now,
			Content:   "other error",
			Other: common.MapToJsonStr(map[string]interface{}{
				"reject_reason": "upstream_error",
			}),
		},
	}
	for i := range logs {
		require.NoError(t, DB.Create(&logs[i]).Error)
	}

	matched, total, err := GetAllLogs(LogTypeError, 0, 0, "", "", 0, "", 0, 20, 0, "", "", "", true)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, matched, 1)
	require.Contains(t, matched[0].Other, "prompt_check")
}

func TestGetFlowQuotaDataAggregatesConsumeLogs(t *testing.T) {
	truncateTables(t)

	oldNodeName := common.NodeName
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.NodeName = "node-a"
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.NodeName = oldNodeName
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	require.NoError(t, DB.Create(&Channel{
		Id:    7,
		Name:  "primary-channel",
		Key:   "sk-test",
		Group: "vip",
	}).Error)
	require.NoError(t, DB.Create(&Token{Id: 11, UserId: 3, Name: "main-key", Key: "sk-test"}).Error)

	now := common.GetTimestamp()
	quotaRows := []QuotaData{
		{UserID: 3, Username: "target", ModelName: "gpt-test", CreatedAt: now, UseGroup: "vip", TokenID: 11, ChannelID: 7, NodeName: "node-a", TokenUsed: 3, Count: 1, Quota: 10},
		{UserID: 3, Username: "target", ModelName: "gpt-test", CreatedAt: now + 1, UseGroup: "vip", TokenID: 11, ChannelID: 7, NodeName: "node-a", TokenUsed: 7, Count: 1, Quota: 20},
	}
	require.NoError(t, DB.Create(&quotaRows).Error)

	rows, err := GetFlowQuotaData(now-10, now+10, "", 3, common.RoleRootUser)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	row := rows[0]
	require.Equal(t, 3, row.UserID)
	require.Equal(t, "target", row.Username)
	require.Equal(t, "node-a", row.NodeName)
	require.Equal(t, "vip", row.UseGroup)
	require.Equal(t, 11, row.TokenID)
	require.Equal(t, "main-key", row.TokenName)
	require.Equal(t, 7, row.ChannelID)
	require.Equal(t, "primary-channel", row.ChannelName)
	require.Equal(t, "gpt-test", row.ModelName)
	require.Equal(t, 10, row.TokenUsed)
	require.Equal(t, 2, row.Count)
	require.Equal(t, 30, row.Quota)
}

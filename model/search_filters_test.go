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

	matched, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", 3, "", 0, 20, 0, "", "", "")
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

	matched, total, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "3", 0, "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, matched, 1)
	require.Equal(t, 13, matched[0].UserId)
}

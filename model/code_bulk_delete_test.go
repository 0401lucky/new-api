package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestDeleteRedemptionsValidInvalidAreComplementary(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	codes := []Redemption{
		{Key: "redemption-valid-never", Status: common.RedemptionCodeStatusEnabled, ExpiredTime: 0},
		{Key: "redemption-valid-boundary", Status: common.RedemptionCodeStatusEnabled, ExpiredTime: now},
		{Key: "redemption-valid-future", Status: common.RedemptionCodeStatusEnabled, ExpiredTime: now + 3600},
		{Key: "redemption-invalid-expired", Status: common.RedemptionCodeStatusEnabled, ExpiredTime: now - 1},
		{Key: "redemption-invalid-used", Status: common.RedemptionCodeStatusUsed, ExpiredTime: 0},
		{Key: "redemption-invalid-disabled", Status: common.RedemptionCodeStatusDisabled, ExpiredTime: now + 3600},
		{Key: "redemption-unknown-status", Status: 99, ExpiredTime: 0},
	}
	for i := range codes {
		require.NoError(t, DB.Create(&codes[i]).Error)
	}

	rows, err := deleteValidRedemptionsAt(now)
	require.NoError(t, err)
	require.Equal(t, int64(3), rows)

	var remainingKeys []string
	require.NoError(t, DB.Model(&Redemption{}).Order("key").Pluck("key", &remainingKeys).Error)
	require.ElementsMatch(t, []string{
		"redemption-invalid-disabled",
		"redemption-invalid-expired",
		"redemption-invalid-used",
		"redemption-unknown-status",
	}, remainingKeys)

	rows, err = deleteInvalidRedemptionsAt(now)
	require.NoError(t, err)
	require.Equal(t, int64(3), rows)

	remainingKeys = nil
	require.NoError(t, DB.Model(&Redemption{}).Pluck("key", &remainingKeys).Error)
	require.Equal(t, []string{"redemption-unknown-status"}, remainingKeys)
}

func TestDeleteInvitationCodesValidInvalidAreComplementary(t *testing.T) {
	truncateTables(t)
	now := common.GetTimestamp()
	codes := []InvitationCode{
		{Key: "invitation-valid-never", Status: common.InvitationCodeStatusEnabled, ExpiredTime: 0},
		{Key: "invitation-valid-boundary", Status: common.InvitationCodeStatusEnabled, ExpiredTime: now},
		{Key: "invitation-valid-future", Status: common.InvitationCodeStatusEnabled, ExpiredTime: now + 3600},
		{Key: "invitation-invalid-expired", Status: common.InvitationCodeStatusEnabled, ExpiredTime: now - 1},
		{Key: "invitation-invalid-used", Status: common.InvitationCodeStatusUsed, ExpiredTime: 0},
		{Key: "invitation-invalid-disabled", Status: common.InvitationCodeStatusDisabled, ExpiredTime: now + 3600},
		{Key: "invitation-unknown-status", Status: 99, ExpiredTime: 0},
	}
	for i := range codes {
		require.NoError(t, DB.Create(&codes[i]).Error)
	}

	rows, err := deleteValidInvitationCodesAt(now)
	require.NoError(t, err)
	require.Equal(t, int64(3), rows)

	var remainingKeys []string
	require.NoError(t, DB.Model(&InvitationCode{}).Order("key").Pluck("key", &remainingKeys).Error)
	require.ElementsMatch(t, []string{
		"invitation-invalid-disabled",
		"invitation-invalid-expired",
		"invitation-invalid-used",
		"invitation-unknown-status",
	}, remainingKeys)

	rows, err = deleteInvalidInvitationCodesAt(now)
	require.NoError(t, err)
	require.Equal(t, int64(3), rows)

	remainingKeys = nil
	require.NoError(t, DB.Model(&InvitationCode{}).Pluck("key", &remainingKeys).Error)
	require.Equal(t, []string{"invitation-unknown-status"}, remainingKeys)
}

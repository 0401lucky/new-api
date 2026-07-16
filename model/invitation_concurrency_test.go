package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUseInvitationCodeCannotBeReused(t *testing.T) {
	truncateTables(t)

	code := &InvitationCode{
		UserId:      1,
		Key:         "single-use-invitation-code",
		Status:      common.InvitationCodeStatusEnabled,
		CreatedTime: common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(code).Error)

	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return UseInvitationCodeWithTx(tx, code.Key, 101)
	}))
	err := DB.Transaction(func(tx *gorm.DB) error {
		return UseInvitationCodeWithTx(tx, code.Key, 202)
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已被使用")

	var stored InvitationCode
	require.NoError(t, DB.First(&stored, code.Id).Error)
	assert.Equal(t, common.InvitationCodeStatusUsed, stored.Status)
	assert.Equal(t, 101, stored.UsedUserId)
	assert.NotZero(t, stored.UsedTime)
}

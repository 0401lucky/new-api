package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdatePendingTopUpOnlyOneStaleSnapshotCanComplete(t *testing.T) {
	truncateTables(t)

	topUp := &TopUp{
		UserId:          1,
		TradeNo:         "topup-cas-stale",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, DB.Create(topUp).Error)

	first := *topUp
	second := *topUp
	updated, err := updatePendingTopUp(DB, &first, map[string]interface{}{
		"status":        common.TopUpStatusSuccess,
		"complete_time": int64(100),
	})
	require.NoError(t, err)
	assert.True(t, updated)

	updated, err = updatePendingTopUp(DB, &second, map[string]interface{}{
		"status":        common.TopUpStatusSuccess,
		"complete_time": int64(200),
	})
	require.NoError(t, err)
	assert.False(t, updated)

	var stored TopUp
	require.NoError(t, DB.First(&stored, topUp.Id).Error)
	assert.Equal(t, common.TopUpStatusSuccess, stored.Status)
	assert.Equal(t, int64(100), stored.CompleteTime)
}

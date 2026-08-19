package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrantTemporaryQuota_CreateBucket(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "grant_temp_new", Password: "password", Quota: 100}
	require.NoError(t, DB.Create(user).Error)

	now := common.NowInCheckinTimezone()
	checkin, err := GrantTemporaryQuota(user.Id, 5000)
	require.NoError(t, err)
	assert.Equal(t, CheckinQuotaTypeTemporary, checkin.QuotaType)
	assert.Equal(t, 5000, checkin.QuotaAwarded)
	assert.Equal(t, 5000, checkin.QuotaRemaining)
	assert.Equal(t, now.Format("2006-01-02"), checkin.CheckinDate)
	assert.Equal(t, nextDayMidnightUnix(now), checkin.QuotaExpiresAt)

	// 永久额度不受影响
	var u User
	require.NoError(t, DB.First(&u, user.Id).Error)
	assert.Equal(t, 100, u.Quota)
}

func TestGrantTemporaryQuota_Accumulate(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "grant_temp_add", Password: "password"}
	require.NoError(t, DB.Create(user).Error)

	first, err := GrantTemporaryQuota(user.Id, 5000)
	require.NoError(t, err)
	second, err := GrantTemporaryQuota(user.Id, 3000)
	require.NoError(t, err)

	assert.Equal(t, first.Id, second.Id)
	assert.Equal(t, 8000, second.QuotaAwarded)
	assert.Equal(t, 8000, second.QuotaRemaining)
	assert.Equal(t, first.QuotaExpiresAt, second.QuotaExpiresAt)

	var count int64
	require.NoError(t, DB.Model(&Checkin{}).Where("user_id = ?", user.Id).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestGrantTemporaryQuota_PermanentConflict(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "grant_temp_conflict", Password: "password"}
	require.NoError(t, DB.Create(user).Error)

	today := common.NowInCheckinTimezone().Format("2006-01-02")
	permanent := &Checkin{
		UserId:       user.Id,
		CheckinDate:  today,
		QuotaAwarded: 1000,
		QuotaType:    CheckinQuotaTypePermanent,
		CreatedAt:    common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(permanent).Error)

	_, err := GrantTemporaryQuota(user.Id, 5000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "永久签到记录")

	// 既有永久记录未被篡改
	var stored Checkin
	require.NoError(t, DB.First(&stored, permanent.Id).Error)
	assert.Equal(t, CheckinQuotaTypePermanent, stored.QuotaType)
	assert.Equal(t, 1000, stored.QuotaAwarded)
	assert.Equal(t, 0, stored.QuotaRemaining)
	assert.Equal(t, int64(0), stored.QuotaExpiresAt)
}

func TestGrantTemporaryQuota_InvalidInput(t *testing.T) {
	truncateTables(t)
	user := &User{Username: "grant_temp_invalid", Password: "password"}
	require.NoError(t, DB.Create(user).Error)

	_, err := GrantTemporaryQuota(user.Id, 0)
	require.Error(t, err)

	_, err = GrantTemporaryQuota(user.Id+9999, 100)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "用户不存在")
}

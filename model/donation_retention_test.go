package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDonationSQLiteBackupRetainsIdentityAndRewards(t *testing.T) {
	f := newDonationFixture(t, "sqlite", true)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	const key = "synthetic-backup-donation-key"
	batch := prepareDonationTest(t, f.store, f.user.Id, f.campaign, key)
	require.NoError(t, f.store.ApplyReceipt(t.Context(), donationReceiptFor(batch)))
	require.NoError(t, f.store.SettleBatch(t.Context(), batch.ID))
	var rewardsBeforeBackup int64
	require.NoError(t, f.db.Model(&DonationReward{}).Count(&rewardsBeforeBackup).Error)

	// VACUUM INTO includes committed WAL data in a standalone SQLite backup.
	backupPath := filepath.Join(t.TempDir(), "backup.db")
	require.NoError(t, f.db.Exec("VACUUM INTO ?", backupPath).Error)
	backup, err := os.ReadFile(backupPath)
	require.NoError(t, err)
	restoredPath := filepath.Join(t.TempDir(), "restored.db")
	require.NoError(t, os.WriteFile(restoredPath, backup, 0600))
	restored, err := gorm.Open(sqlite.Open(restoredPath), &gorm.Config{
		NamingStrategy: f.db.Config.NamingStrategy,
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	restoredSQL, err := restored.DB()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, restoredSQL.Close()) })
	require.NoError(t, MigrateDonations(restored))
	store, err := OpenDonationStore(restored)
	require.NoError(t, err)
	assert.Equal(t, f.store.Fingerprint(key), store.Fingerprint(key))
	connection, token, err := store.Connection(t.Context())
	require.NoError(t, err)
	assert.True(t, token == strings.Repeat("t", 40), "backup must retain decryptable integration credentials")

	// Changing the connection credential and address must not reset key identity.
	connection.BaseURL = "https://restored-donation.example"
	require.NoError(t, store.SaveConnection(t.Context(), connection, strings.Repeat("r", 40), connection.Version))
	store, err = OpenDonationStore(restored)
	require.NoError(t, err)
	assert.Equal(t, f.store.Fingerprint(key), store.Fingerprint(key))
	duplicate := prepareDonationTest(t, store, f.otherUser.Id, f.campaign, key)
	require.Len(t, duplicate.Items, 1)
	assert.Equal(t, "duplicate", duplicate.Items[0].State)
	assert.False(t, duplicate.Items[0].Dispatch)
	credited, err := store.CreditDonationReward(t.Context(), batch.Items[0].ID)
	require.NoError(t, err)
	assert.False(t, credited)
	history, err := store.Batch(t.Context(), batch.ID, f.user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 25, history.Summary.RewardedQuota)
	assert.Equal(t, "rewarded", history.Items[0].RewardState)
	assert.NotNil(t, history.Items[0].CredentialID)
	var user User
	require.NoError(t, restored.First(&user, f.user.Id).Error)
	assert.Equal(t, 125, user.Quota)
	var rewards int64
	require.NoError(t, restored.Model(&DonationReward{}).Count(&rewards).Error)
	assert.Equal(t, rewardsBeforeBackup, rewards, "backup retains both released history and newly credited rewards")
}

func TestDonationPermanentRewardSurvivesCheckinRetention(t *testing.T) {
	f := newDonationFixture(t, "sqlite", true)
	oldRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedis })
	// This private SQLite file can expose the existing Checkin model's fixed
	// table name so the real temporary-quota query runs against the fixture.
	require.NoError(t, f.db.Migrator().RenameTable(f.checkinTable, "checkins"))
	now := common.NowInCheckinTimezone()
	yesterday := now.AddDate(0, 0, -1)
	require.NoError(t, f.db.Model(&Checkin{}).Where("user_id = ?", f.user.Id).Updates(map[string]any{
		"checkin_date": yesterday.Format("2006-01-02"), "quota_expires_at": yesterday.Unix(),
	}).Error)
	batch := prepareDonationTest(t, f.store, f.user.Id, f.campaign, "synthetic-permanent-retention-key")
	require.NoError(t, f.store.ApplyReceipt(t.Context(), donationReceiptFor(batch)))
	require.NoError(t, f.store.SettleBatch(t.Context(), batch.ID))
	activeQuota, err := GetActiveTemporaryQuota(f.user.Id)
	require.NoError(t, err)
	assert.Zero(t, activeQuota)
	var expired Checkin
	require.NoError(t, f.db.Where("user_id = ?", f.user.Id).First(&expired).Error)
	assert.Equal(t, 9, expired.QuotaRemaining, "donation must not add to the temporary bucket")

	result := f.db.Where("user_id = ? AND quota_type = ? AND quota_expires_at <= ?", f.user.Id, "temporary", now.Unix()).Delete(&Checkin{})
	require.NoError(t, result.Error)
	assert.EqualValues(t, 1, result.RowsAffected)
	activeQuota, err = GetActiveTemporaryQuota(f.user.Id)
	require.NoError(t, err)
	assert.Zero(t, activeQuota)
	var user User
	require.NoError(t, f.db.First(&user, f.user.Id).Error)
	assert.Equal(t, 125, user.Quota, "expired bucket retention must not deduct a permanent donation reward")
	history, err := f.store.Batch(t.Context(), batch.ID, f.user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 25, history.Summary.RewardedQuota)
	credited, err := f.store.CreditDonationReward(t.Context(), batch.Items[0].ID)
	require.NoError(t, err)
	assert.False(t, credited)
}

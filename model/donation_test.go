package model

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

type donationFixture struct {
	db           *gorm.DB
	other        *gorm.DB
	store        *DonationStore
	otherStore   *DonationStore
	user         User
	otherUser    User
	campaign     DonationCampaign
	checkinTable string
}

func newDonationFixture(t *testing.T, dialect string, upgrade bool) donationFixture {
	t.Helper()
	dsn := "local"
	switch dialect {
	case "mysql":
		dsn = os.Getenv("TEST_MYSQL_DSN")
	case "postgres":
		dsn = os.Getenv("TEST_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skip("real database DSN is not configured")
	}
	t.Setenv("DONATION_MATRIX_DSN", dsn)
	oldSQLite := common.SQLitePath
	common.SQLitePath = filepath.Join(t.TempDir(), "donations.db") + "?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate"
	t.Cleanup(func() { common.SQLitePath = oldSQLite })
	prefix := "dv_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "") + "_"
	db, dbType, err := chooseDB("DONATION_MATRIX_DSN", false)
	require.NoError(t, err)
	db.Config.NamingStrategy = schema.NamingStrategy{TablePrefix: prefix}
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	other, _, err := chooseDB("DONATION_MATRIX_DSN", false)
	require.NoError(t, err)
	other.Config.NamingStrategy = schema.NamingStrategy{TablePrefix: prefix}
	other = other.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	oldDB, oldLog, oldMainType, oldLogType := DB, LOG_DB, common.MainDatabaseType(), common.LogDatabaseType()
	DB, LOG_DB = db, nil
	common.SetDatabaseTypes(dbType, dbType)
	initCol()
	t.Cleanup(func() { DB, LOG_DB = oldDB, oldLog; common.SetDatabaseTypes(oldMainType, oldLogType); initCol() })
	var version string
	versionSQL := "SELECT VERSION()"
	if dialect == "sqlite" {
		versionSQL = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
	t.Logf("real database %s version=%s upgrade=%v prefix=%s", dialect, version, upgrade, prefix)
	checkinTable := prefix + "checkins"
	t.Cleanup(func() {
		// Only this fixture's explicitly prefixed tables are removed.
		require.True(t, strings.HasPrefix(prefix, "dv_"))
		for _, record := range []any{&DonationEvent{}, &DonationRetry{}, &DonationReward{}, &DonationItem{}, &DonationBatch{}, &DonationResource{}, &DonationCampaignRevision{}, &DonationCampaign{}, &DonationConnection{}, &DonationSecret{}, &User{}, &TopUp{}} {
			assert.NoError(t, db.Migrator().DropTable(record))
		}
		assert.NoError(t, db.Migrator().DropTable(checkinTable))
		for _, handle := range []*gorm.DB{other, db} {
			sqlDB, err := handle.DB()
			require.NoError(t, err)
			assert.NoError(t, sqlDB.Close())
		}
	})
	if !upgrade {
		require.NoError(t, MigrateDonations(db))
	}
	// These unchanged baseline User/TopUp/Checkin models represent the released
	// wallet and temporary bucket before the additive donation migration.
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}))
	require.NoError(t, db.Table(checkinTable).AutoMigrate(&Checkin{}))
	user := User{Username: "donor", Password: "synthetic", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 100, AuthVersion: 1, AffCode: "donor", LinuxDOId: "community-1"}
	otherUser := User{Username: "other", Password: "synthetic", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 100, AuthVersion: 1, AffCode: "other"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&otherUser).Error)
	topUp := TopUp{UserId: user.Id, Amount: 7, TradeNo: "baseline-order", Status: common.TopUpStatusSuccess}
	require.NoError(t, db.Create(&topUp).Error)
	checkin := Checkin{UserId: user.Id, CheckinDate: "2026-09-14", QuotaAwarded: 9, QuotaType: "temporary", QuotaRemaining: 9, QuotaExpiresAt: 1790000000}
	require.NoError(t, db.Table(checkinTable).Create(&checkin).Error)
	require.NoError(t, MigrateDonations(db))
	recorder := &migrationSQLRecorder{}
	for range 2 {
		recorder.reset()
		require.NoError(t, MigrateDonations(db.Session(&gorm.Session{Logger: recorder})))
		assert.Empty(t, recorder.schemaMutations(), "restart must not rewrite schema")
	}
	var saved User
	require.NoError(t, db.First(&saved, user.Id).Error)
	assert.Equal(t, user.Quota, saved.Quota)
	var savedTopUp TopUp
	require.NoError(t, db.First(&savedTopUp, topUp.Id).Error)
	assert.Equal(t, topUp, savedTopUp)
	var savedCheckin Checkin
	require.NoError(t, db.Table(checkinTable).First(&savedCheckin, checkin.Id).Error)
	assert.Equal(t, checkin, savedCheckin)
	assert.Error(t, db.Create(&User{Username: user.Username, Password: "duplicate", AffCode: "different"}).Error)
	assert.Error(t, db.Create(&TopUp{TradeNo: topUp.TradeNo}).Error)
	var stores [2]*DonationStore
	var openErrors [2]error
	start := make(chan struct{})
	var opening sync.WaitGroup
	for i, handle := range []*gorm.DB{db, other} {
		opening.Go(func() { <-start; stores[i], openErrors[i] = OpenDonationStore(handle) })
	}
	close(start)
	opening.Wait()
	require.NoError(t, openErrors[0])
	require.NoError(t, openErrors[1])
	store, otherStore := stores[0], stores[1]
	assert.Equal(t, store.Fingerprint("case-Sensitive-Key"), otherStore.Fingerprint("case-Sensitive-Key"))
	connection := DonationConnection{BaseURL: "https://donation.example", InstanceID: uuid.NewString(), SourceID: uuid.NewString()}
	require.NoError(t, store.SaveConnection(t.Context(), connection, strings.Repeat("t", 40), 0))
	campaign, err := store.SaveCampaign(t.Context(), DonationCampaign{Name: "Donation", Description: "baseline", InstanceID: connection.InstanceID, SourceID: connection.SourceID, GroupID: 1, GroupName: "empty group", TargetRevision: strings.Repeat("a", 64), RewardQuota: 25, Enabled: true}, 0)
	require.NoError(t, err)
	return donationFixture{db: db, other: other, store: store, otherStore: otherStore, user: user, otherUser: otherUser, campaign: campaign, checkinTable: checkinTable}
}

func prepareDonationTest(t *testing.T, store *DonationStore, userID int, campaign DonationCampaign, text string) DonationBatchDetail {
	t.Helper()
	lines, digest, err := store.ParseLines(campaign.ID, text)
	require.NoError(t, err)
	batch, err := store.PrepareBatch(t.Context(), userID, uuid.NewString(), digest, campaign, lines)
	require.NoError(t, err)
	detail, err := store.Batch(t.Context(), batch.ID, userID)
	require.NoError(t, err)
	return detail
}

func donationReceiptFor(batch DonationBatchDetail, states ...string) DonationReceipt {
	receipt := DonationReceipt{InstanceID: batch.InstanceID, SourceID: batch.SourceID, BatchID: batch.ID, GroupID: batch.GroupID, TargetRevision: batch.TargetRevision, CreatedAtMS: batch.CreatedAtMS, State: "processing"}
	index := 0
	for _, item := range batch.Items {
		if !item.Dispatch {
			continue
		}
		state := "accepted"
		if index < len(states) {
			state = states[index]
		}
		result := DonationReceiptItem{ItemID: item.ID, State: state}
		if state == "accepted" {
			id, at := uint64(index+100), int64(1789300000000)
			result.CredentialID, result.AcceptedAtMS = &id, &at
		} else if state == "retry_pending" {
			result.Retryable, result.ReasonCode = true, "rate_limited"
		} else if state == "invalid" {
			result.ReasonCode = "invalid_credential"
		}
		receipt.Items = append(receipt.Items, result)
		index++
	}
	return receipt
}

func TestDonationDatabase(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			for _, upgrade := range []bool{false, true} {
				t.Run(fmt.Sprintf("upgrade_%v", upgrade), func(t *testing.T) {
					fixture := newDonationFixture(t, dialect, upgrade)
					t.Run("mixed_lines_and_frozen_rewards", func(t *testing.T) { testDonationMixed(t, fixture) })
					t.Run("cross_connection_ownership_and_ledger", func(t *testing.T) { testDonationConcurrency(t, fixture) })
					t.Run("rollback_limits_and_account_pause", func(t *testing.T) { testDonationRollback(t, fixture) })
				})
			}
		})
	}
}

func testDonationMixed(t *testing.T, f donationFixture) {
	ctx := t.Context()
	text := "\r\n first-valid-key-0001 \r\nbad key\r\nfirst-valid-key-0001\r\nsecond-valid-key-0002\nexisting-key-0003\nwaiting-key-0004\n"
	batch := prepareDonationTest(t, f.store, f.user.Id, f.campaign, text)
	assert.Equal(t, []int{2, 3, 4, 5, 6, 7}, []int{batch.Items[0].Line, batch.Items[1].Line, batch.Items[2].Line, batch.Items[3].Line, batch.Items[4].Line, batch.Items[5].Line})
	assert.Equal(t, "invalid", batch.Items[1].State)
	assert.Equal(t, "duplicate", batch.Items[2].State)
	assert.NotContains(t, batch.Items[1].KeyMask, "bad key")
	lines, digest, err := f.store.ParseLines(f.campaign.ID, text)
	require.NoError(t, err)
	replayed, err := f.store.PrepareBatch(ctx, f.user.Id, batch.RequestKey, digest, f.campaign, lines)
	require.NoError(t, err)
	assert.Equal(t, batch.ID, replayed.ID)
	_, err = f.store.PrepareBatch(ctx, f.user.Id, batch.RequestKey, "changed", f.campaign, lines)
	assert.ErrorIs(t, err, ErrDonationConflict)
	receipt := donationReceiptFor(batch, "accepted", "invalid", "existing", "retry_pending")
	// Responses may be reordered, but association stays by stable item ID.
	receipt.Items[0], receipt.Items[3] = receipt.Items[3], receipt.Items[0]
	broken := receipt
	broken.InstanceID = uuid.NewString()
	assert.ErrorIs(t, f.store.ApplyReceipt(ctx, broken), ErrDonationReceipt)
	broken = receipt
	broken.GroupID++
	assert.ErrorIs(t, f.store.ApplyReceipt(ctx, broken), ErrDonationReceipt)
	broken = receipt
	broken.Items = append([]DonationReceiptItem(nil), receipt.Items...)
	broken.Items[1].ItemID = broken.Items[0].ItemID
	assert.ErrorIs(t, f.store.ApplyReceipt(ctx, broken), ErrDonationReceipt)
	require.NoError(t, f.store.ApplyReceipt(ctx, receipt))
	closed := f.campaign
	closed.Enabled = false
	closed.RewardQuota = 999
	closed, err = f.store.SaveCampaign(ctx, closed, f.campaign.Version)
	require.NoError(t, err)
	require.NoError(t, f.store.SettleBatch(ctx, batch.ID))
	result, err := f.store.Batch(ctx, batch.ID, f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Summary.Accepted)
	assert.EqualValues(t, 25, result.Summary.RewardedQuota)
	assert.Equal(t, "invalid", result.Items[3].State)
	assert.Equal(t, "existing", result.Items[4].State)
	assert.Equal(t, "retry_pending", result.Items[5].State)
	_, err = f.store.Batch(ctx, batch.ID, f.otherUser.Id)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	// Old responses cannot demote a credited item or create a new reward.
	stale := donationReceiptFor(batch, "queued", "invalid", "existing", "retry_pending")
	require.NoError(t, f.store.ApplyReceipt(ctx, stale))
	credited, err := f.store.CreditDonationReward(ctx, batch.Items[0].ID)
	require.NoError(t, err)
	assert.False(t, credited)
	var user User
	require.NoError(t, f.db.First(&user, f.user.Id).Error)
	assert.Equal(t, 125, user.Quota)
	var buckets int64
	require.NoError(t, f.db.Table(f.checkinTable).Count(&buckets).Error)
	assert.EqualValues(t, 1, buckets)
	var checkin Checkin
	require.NoError(t, f.db.Table(f.checkinTable).First(&checkin).Error)
	assert.Equal(t, 9, checkin.QuotaRemaining)
	// Restore the live campaign; the accepted batch retains its original rules.
	closed.Enabled, closed.RewardQuota = true, 25
	_, err = f.store.SaveCampaign(ctx, closed, closed.Version)
	require.NoError(t, err)
	var revisions int64
	require.NoError(t, f.db.Model(&DonationCampaignRevision{}).Count(&revisions).Error)
	assert.EqualValues(t, 3, revisions)
}

func testDonationConcurrency(t *testing.T, f donationFixture) {
	campaign, err := f.store.Campaign(t.Context(), f.campaign.ID)
	require.NoError(t, err)
	second := campaign
	second.ID = 0
	second.GroupID = 2
	second.Name = "Different campaign"
	second, err = f.store.SaveCampaign(t.Context(), second, 0)
	require.NoError(t, err)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var batches [2]DonationBatch
	var errs [2]error
	stores, users, campaigns := []*DonationStore{f.store, f.otherStore}, []int{f.user.Id, f.otherUser.Id}, []DonationCampaign{campaign, second}
	for i := range 2 {
		lines, digest, err := stores[i].ParseLines(campaigns[i].ID, "concurrent-resource-key")
		require.NoError(t, err)
		wg.Go(func() {
			<-start
			batches[i], errs[i] = stores[i].PrepareBatch(t.Context(), users[i], uuid.NewString(), digest, campaigns[i], lines)
		})
	}
	close(start)
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	var owner DonationBatchDetail
	dispatched := 0
	for _, batch := range batches {
		detail, err := f.store.Batch(t.Context(), batch.ID, 0)
		require.NoError(t, err)
		if detail.Items[0].Dispatch {
			owner = detail
			dispatched++
		} else {
			assert.Equal(t, "resource_processing", detail.Items[0].ReasonCode)
		}
	}
	require.Equal(t, 1, dispatched, "fresh contention must yield one owner, not zero or two")
	require.NoError(t, f.store.ApplyReceipt(t.Context(), donationReceiptFor(owner)))
	start = make(chan struct{})
	var credited [2]bool
	for i := range 2 {
		wg.Go(func() { <-start; credited[i], errs[i] = stores[i].CreditDonationReward(t.Context(), owner.Items[0].ID) })
	}
	close(start)
	wg.Wait()
	require.NoError(t, errs[0])
	require.NoError(t, errs[1])
	assert.NotEqual(t, credited[0], credited[1])
	var rewards []DonationReward
	require.NoError(t, f.db.Where("item_id = ?", owner.Items[0].ID).Find(&rewards).Error)
	require.Len(t, rewards, 1)
	assert.Equal(t, owner.UserID, rewards[0].UserID)
	assert.Error(t, f.db.Create(&DonationReward{ID: uuid.NewString(), Fingerprint: rewards[0].Fingerprint, ItemID: uuid.NewString(), UserID: owner.UserID, Quota: 25}).Error)
	assert.Error(t, f.db.Create(&DonationReward{ID: uuid.NewString(), Fingerprint: f.store.Fingerprint("other-key"), ItemID: rewards[0].ItemID, UserID: owner.UserID, Quota: 25}).Error)
	// Retained resource identity survives restarting the store and later deletion
	// of external credentials/logs; no mutable remote inventory is consulted.
	restarted, err := OpenDonationStore(f.other)
	require.NoError(t, err)
	duplicate := prepareDonationTest(t, restarted, f.otherUser.Id, second, " concurrent-resource-key ")
	assert.Equal(t, "already_donated", duplicate.Items[0].ReasonCode)
	for field, value := range map[string]any{"reward_quota": 26, "user_id": f.otherUser.Id + f.user.Id + 1} {
		require.NoError(t, f.db.Model(&DonationBatch{}).Where("id = ?", owner.ID).Update(field, value).Error)
		_, err := f.store.CreditDonationReward(t.Context(), owner.Items[0].ID)
		assert.ErrorIs(t, err, ErrDonationConflict)
		require.NoError(t, f.db.Model(&DonationBatch{}).Where("id = ?", owner.ID).Updates(map[string]any{"user_id": owner.UserID, "reward_quota": owner.RewardQuota}).Error)
	}
	_, total, err := f.store.Records(t.Context(), DonationRecordFilter{ItemID: owner.Items[0].ID, UserID: owner.UserID, CredentialID: 100}, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
}

func testDonationRollback(t *testing.T, f donationFixture) {
	campaign, err := f.store.Campaign(t.Context(), f.campaign.ID)
	require.NoError(t, err)
	batch := prepareDonationTest(t, f.store, f.user.Id, campaign, "rollback-donation-key")
	require.NoError(t, f.store.ApplyReceipt(t.Context(), donationReceiptFor(batch)))
	var before User
	require.NoError(t, f.db.First(&before, f.user.Id).Error)
	forced := errors.New("transaction interrupted after wallet update")
	require.NoError(t, f.db.Callback().Create().Before("gorm:create").Register("donation_test_rollback", func(tx *gorm.DB) {
		if event, ok := tx.Statement.Dest.(*DonationEvent); ok && event.State == "rewarded" {
			tx.AddError(forced)
		}
	}))
	_, err = f.store.CreditDonationReward(t.Context(), batch.Items[0].ID)
	assert.ErrorIs(t, err, forced)
	require.NoError(t, f.db.Callback().Create().Remove("donation_test_rollback"))
	var after User
	require.NoError(t, f.db.First(&after, f.user.Id).Error)
	assert.Equal(t, before.Quota, after.Quota)
	var count int64
	require.NoError(t, f.db.Model(&DonationReward{}).Where("item_id = ?", batch.Items[0].ID).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, f.db.Model(&User{}).Where("id = ?", f.user.Id).Update("status", common.UserStatusDisabled).Error)
	credited, err := f.store.CreditDonationReward(t.Context(), batch.Items[0].ID)
	require.NoError(t, err)
	assert.False(t, credited)
	paused, err := f.store.Batch(t.Context(), batch.ID, f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, "paused", paused.Items[0].RewardState)
	require.NoError(t, f.db.Model(&User{}).Where("id = ?", f.user.Id).Updates(map[string]any{"status": common.UserStatusEnabled, "quota": common.MaxWalletQuota - 24}).Error)
	_, err = f.store.CreditDonationReward(t.Context(), batch.Items[0].ID)
	assert.ErrorIs(t, err, ErrTopUpQuotaLimitExceeded)
	require.NoError(t, f.db.Model(&User{}).Where("id = ?", f.user.Id).Update("quota", before.Quota).Error)
	credited, err = f.store.CreditDonationReward(t.Context(), batch.Items[0].ID)
	require.NoError(t, err)
	assert.True(t, credited)
	require.NoError(t, f.db.First(&after, f.user.Id).Error)
	assert.Equal(t, before.Quota+25, after.Quota)
	// A confirmed invalid attempt releases only undecided ownership.
	failed := prepareDonationTest(t, f.store, f.user.Id, campaign, "previous-invalid-key")
	require.NoError(t, f.store.ApplyReceipt(t.Context(), donationReceiptFor(failed, "invalid")))
	newAttempt := prepareDonationTest(t, f.otherStore, f.otherUser.Id, campaign, "previous-invalid-key")
	assert.True(t, newAttempt.Items[0].Dispatch)
	// An unconfirmed request remains owned after lease expiry and repeated 404s.
	unknown := prepareDonationTest(t, f.store, f.user.Id, campaign, "unknown-result-key")
	require.NoError(t, f.store.MarkBatchError(t.Context(), unknown.ID, "receipt_not_found"))
	require.NoError(t, f.db.Model(&DonationBatch{}).Where("id = ?", unknown.ID).Update("lease_until_ms", time.Now().Add(-time.Hour).UnixMilli()).Error)
	duplicate := prepareDonationTest(t, f.otherStore, f.otherUser.Id, campaign, "unknown-result-key")
	assert.False(t, duplicate.Items[0].Dispatch)
	assert.Equal(t, "resource_processing", duplicate.Items[0].ReasonCode)
}

func TestDonationCacheAndIdentity(t *testing.T) {
	f := newDonationFixture(t, "sqlite", true)
	useUserCacheMiniRedis(t)
	require.NoError(t, populateUserCache(f.user))
	require.NoError(t, cacheDecrUserQuota(f.user.Id, 17))
	batch := prepareDonationTest(t, f.store, f.user.Id, f.campaign, "cache-reservation-key")
	require.NoError(t, f.store.ApplyReceipt(t.Context(), donationReceiptFor(batch)))
	credited, err := f.store.CreditDonationReward(t.Context(), batch.Items[0].ID)
	require.NoError(t, err)
	assert.True(t, credited)
	quota, err := getUserQuotaCache(f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, 108, quota)
	credited, err = f.otherStore.CreditDonationReward(t.Context(), batch.Items[0].ID)
	require.NoError(t, err)
	assert.False(t, credited)
	quota, err = getUserQuotaCache(f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, 108, quota)
	var stored DonationSecret
	require.NoError(t, f.db.First(&stored).Error)
	var logOutput strings.Builder
	debugDB := f.db.Session(&gorm.Session{Logger: newGormLogger(&logOutput).LogMode(logger.Info)})
	_, err = OpenDonationStore(debugDB)
	require.NoError(t, err)
	assert.NotContains(t, logOutput.String(), stored.Material)
	require.NoError(t, f.db.Model(&DonationSecret{}).Where("slot = ?", "v1").Update("material", "corrupted").Error)
	_, err = OpenDonationStore(f.db)
	assert.ErrorIs(t, err, ErrDonationSecret)
	require.NoError(t, f.db.Where("slot = ?", "v1").Delete(&DonationSecret{}).Error)
	_, err = OpenDonationStore(f.db)
	assert.ErrorIs(t, err, ErrDonationSecret, "missing history key must never create new reward eligibility")
	_, err = f.store.PrepareBatch(context.Background(), f.user.Id, uuid.NewString(), "unused", f.campaign, []DonationLine{{Line: 1, ID: uuid.NewString(), Fingerprint: "unused"}})
	assert.ErrorIs(t, err, ErrDonationSecret, "an already-open store also fails closed")
}

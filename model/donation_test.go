package model

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
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
	"gorm.io/gorm/clause"
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
	if dialect == "postgres" {
		// PostgreSQL index names are schema-scoped, including explicitly named
		// composite indexes. Table prefixes alone cannot isolate two fixtures.
		adminDB := db
		require.NoError(t, adminDB.Exec("CREATE SCHEMA ?", clause.Table{Name: prefix}).Error)
		t.Cleanup(func() {
			assert.NoError(t, adminDB.Exec("DROP SCHEMA ? CASCADE", clause.Table{Name: prefix}).Error)
			sqlDB, err := adminDB.DB()
			require.NoError(t, err)
			assert.NoError(t, sqlDB.Close())
		})
		parsed, err := url.Parse(dsn)
		require.NoError(t, err)
		query := parsed.Query()
		query.Set("search_path", prefix)
		parsed.RawQuery = query.Encode()
		t.Setenv("DONATION_MATRIX_DSN", parsed.String())
		db, dbType, err = chooseDB("DONATION_MATRIX_DSN", false)
		require.NoError(t, err)
	}
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
		for _, record := range []any{&DonationTestAttempt{}, &DonationReviewAction{}, &DonationEvent{}, &DonationRetry{}, &DonationReward{}, &DonationItem{}, &DonationBatch{}, &DonationResource{}, &DonationCampaignRevision{}, &DonationCampaign{}, &DonationConnection{}, &DonationSecret{}, &User{}, &TopUp{}} {
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
	var verifyLegacy func()
	if upgrade {
		verifyLegacy = seedDonationLegacyHistory(t, db, user)
	}
	require.NoError(t, MigrateDonations(db))
	recorder := &migrationSQLRecorder{}
	for range 2 {
		recorder.reset()
		require.NoError(t, MigrateDonations(db.Session(&gorm.Session{Logger: recorder})))
		assert.Empty(t, recorder.schemaMutations(), "restart must not rewrite schema")
	}
	if verifyLegacy != nil {
		verifyLegacy()
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
	connection, _, err := store.Connection(t.Context())
	require.NoError(t, err)
	if connection.ID == 0 {
		connection = DonationConnection{BaseURL: "https://donation.example", InstanceID: uuid.NewString(), SourceID: uuid.NewString()}
		require.NoError(t, store.SaveConnection(t.Context(), connection, strings.Repeat("t", 40), 0))
	}
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
	if batch.ValidationMode == DonationModeManualReview {
		receipt.ValidationMode = DonationModeManualReview
	}
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
		if batch.ValidationMode == DonationModeManualReview {
			result.EffectiveMode, result.ReviewTargetRevision = DonationModeManualReview, batch.TargetRevision
			result.ItemRevision, result.StagingExpiresAtMS = int64Ptr(1), item.StagingExpiresAtMS
		}
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
					t.Run("manual_mode_migration", func(t *testing.T) { testDonationManualMigration(t, fixture) })
					t.Run("manual_review_and_reward", func(t *testing.T) { testDonationManualReview(t, fixture) })
					t.Run("manual_acceptance_requires_approval", func(t *testing.T) { testDonationManualAcceptanceRequiresApproval(t, fixture) })
					t.Run("manual_approval_finds_existing", func(t *testing.T) { testDonationManualApprovalFindsExisting(t, fixture) })
					t.Run("manual_cross_connection_intents", func(t *testing.T) { testDonationManualIntentConcurrency(t, fixture) })
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
	require.NoError(t, f.db.Model(&DonationCampaignRevision{}).Where("campaign_id = ?", f.campaign.ID).Count(&revisions).Error)
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

// testDonationManualMigration proves that a released NULL/empty mode is
// canonicalized to auto, that the new review/test structures exist afterwards,
// and that repeating the migration rewrites nothing.
func testDonationManualMigration(t *testing.T, f donationFixture) {
	require.NoError(t, f.db.Model(&DonationCampaign{}).Where("id = ?", f.campaign.ID).Update("validation_mode", gorm.Expr("NULL")).Error)
	require.NoError(t, f.db.Model(&DonationBatch{}).Where("1 = 1").Update("validation_mode", "").Error)
	require.NoError(t, f.db.Model(&DonationItem{}).Where("1 = 1").Update("effective_mode", "").Error)
	recorder := &migrationSQLRecorder{}
	require.NoError(t, MigrateDonations(f.db.Session(&gorm.Session{Logger: recorder})))
	assert.Empty(t, recorder.schemaMutations(), "a repeated migration must not rewrite schema")

	var campaign DonationCampaign
	require.NoError(t, f.db.First(&campaign, f.campaign.ID).Error)
	assert.Equal(t, DonationModeAuto, campaign.ValidationMode)

	// The frozen campaign revision JSON is never rewritten by the migration.
	var revision DonationCampaignRevision
	require.NoError(t, f.db.Where("campaign_id = ? AND version = ?", f.campaign.ID, campaign.Version).First(&revision).Error)
	assert.NotContains(t, revision.Snapshot, DonationModeManualReview)

	for _, table := range []any{&DonationReviewAction{}, &DonationTestAttempt{}} {
		assert.True(t, f.db.Migrator().HasTable(table))
	}
	// Assert the uniqueness guarantees with real writes. An index with the same
	// name on another PostgreSQL table must not make this look migrated.
	action := DonationReviewAction{ActorID: 19, ActionID: uuid.NewString(), Status: DonationActionRejected}
	require.NoError(t, f.db.Create(&action).Error)
	action.ID = 0
	assert.True(t, donationUniqueError(f.db.Create(&action).Error), "review actor/action must be unique")
	attempt := DonationTestAttempt{ActorID: 19, TestID: uuid.NewString(), State: DonationTestInterrupted}
	require.NoError(t, f.db.Create(&attempt).Error)
	attempt.ID = 0
	assert.True(t, donationUniqueError(f.db.Create(&attempt).Error), "test actor/test must be unique")
	assert.True(t, f.db.Migrator().HasColumn(&DonationItem{}, "review_state"))
	assert.True(t, f.db.Migrator().HasColumn(&DonationItem{}, "staging_expires_at_ms"))
	assert.True(t, f.db.Migrator().HasColumn(&DonationBatch{}, "cold_poll_at_ms"))

	_, err := f.store.SaveCampaign(t.Context(), DonationCampaign{Name: "Bad mode", InstanceID: f.campaign.InstanceID, SourceID: f.campaign.SourceID,
		GroupID: 1, TargetRevision: strings.Repeat("c", 64), ValidationMode: "sometimes", RewardQuota: 25, Enabled: true}, 0)
	assert.ErrorIs(t, err, ErrDonationInput)
}

// testDonationManualReview covers the whole manual path: no credential and no
// reward while pending, a reward only after an applied approval plus the
// matching acceptance, and idempotent action replays.
func testDonationManualReview(t *testing.T, f donationFixture) {
	ctx := t.Context()
	campaign, err := f.store.SaveCampaign(ctx, DonationCampaign{Name: "Manual", InstanceID: f.campaign.InstanceID, SourceID: f.campaign.SourceID,
		GroupID: 1, GroupName: "manual group", TargetRevision: strings.Repeat("b", 64), ValidationMode: DonationModeManualReview, RewardQuota: 25, Enabled: true}, 0)
	require.NoError(t, err)
	assert.Equal(t, DonationModeManualReview, campaign.ValidationMode)

	batch := prepareDonationTest(t, f.store, f.user.Id, campaign, "manual-review-key-0001")
	assert.Equal(t, DonationModeManualReview, batch.ValidationMode)
	assert.Equal(t, DonationModeManualReview, batch.Items[0].EffectiveMode)
	assert.NotNil(t, batch.Items[0].StagingExpiresAtMS)

	receipt := donationReceiptFor(batch, "pending_review")
	receipt.TargetRevision = campaign.TargetRevision
	require.NoError(t, f.store.ApplyReceipt(ctx, receipt))
	detail, err := f.store.Batch(ctx, batch.ID, f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, detail.Summary.PendingReview)
	assert.Zero(t, detail.Summary.Processing)
	assert.Zero(t, detail.Summary.Rewarded)
	assert.Zero(t, detail.Summary.RewardedQuota)
	assert.Nil(t, detail.Items[0].CredentialID)
	assert.Equal(t, "none", detail.Items[0].RewardState)

	item := detail.Items[0]
	approved, err := f.store.CreditDonationReward(ctx, item.ID)
	require.Error(t, err)
	assert.False(t, approved)

	// The receiver reports the authoritative revision and staging deadline.
	receipt.Items[0].ItemRevision = int64Ptr(3)
	require.NoError(t, f.store.ApplyReceipt(ctx, receipt))
	actionID := uuid.NewString()
	intent, err := f.store.PrepareReviewAction(ctx, 7, item.ID, actionID, DonationActionApprove, 3, campaign.TargetRevision, "checked manually")
	require.NoError(t, err)
	require.True(t, intent.Created)
	// The prepared intent immediately wakes hot recovery for this batch.
	var woke DonationBatch
	require.NoError(t, f.db.Where("id = ?", batch.ID).First(&woke).Error)
	assert.True(t, woke.NeedsRecovery)

	// A replay with the same UUID is idempotent; a different meaning conflicts.
	replay, err := f.store.PrepareReviewAction(ctx, 7, item.ID, actionID, DonationActionApprove, 3, campaign.TargetRevision, "checked manually")
	require.NoError(t, err)
	assert.False(t, replay.Created)
	_, err = f.store.PrepareReviewAction(ctx, 7, item.ID, actionID, DonationActionReject, 3, campaign.TargetRevision, "changed my mind")
	assert.ErrorIs(t, err, ErrDonationConflict)

	saved, err := f.store.ApplyReviewOutcome(ctx, intent.Action, DonationActionApplied, "", 4, campaign.TargetRevision, time.Now().UnixMilli())
	require.NoError(t, err)
	assert.Equal(t, DonationActionApplied, saved.Status)

	accepted := donationReceiptFor(batch, "accepted")
	accepted.TargetRevision = campaign.TargetRevision
	accepted.Items[0].ItemRevision = int64Ptr(5)
	accepted.Items[0].EffectiveMode = DonationModeManualReview
	accepted.Items[0].ReviewTargetRevision = campaign.TargetRevision
	accepted.Items[0].ReviewActionID = actionID
	// Keep receiver decision literals independent from local command constants.
	accepted.Items[0].ReviewDecision = "approved"
	accepted.Items[0].ReviewedAtMS = saved.AppliedAtMS
	for _, test := range []struct {
		name   string
		change func(*DonationReceiptItem)
	}{
		{"command_as_decision", func(item *DonationReceiptItem) { item.ReviewDecision = "approve" }},
		{"opposite_decision", func(item *DonationReceiptItem) { item.ReviewDecision = "rejected" }},
		{"wrong_action", func(item *DonationReceiptItem) { item.ReviewActionID = uuid.NewString() }},
		{"wrong_target", func(item *DonationReceiptItem) { item.ReviewTargetRevision = strings.Repeat("f", 64) }},
		{"wrong_action_time", func(item *DonationReceiptItem) { item.ReviewedAtMS = int64Ptr(*saved.AppliedAtMS + 1) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			broken := accepted
			broken.Items = append([]DonationReceiptItem(nil), accepted.Items...)
			test.change(&broken.Items[0])
			assert.ErrorIs(t, f.store.ApplyReceipt(ctx, broken), ErrDonationReceipt)
		})
	}
	require.NoError(t, f.store.ApplyReceipt(ctx, accepted))
	require.NoError(t, f.store.ApplyReceipt(ctx, accepted), "equal revision and facts may replay")
	contradiction := accepted
	contradiction.Items = append([]DonationReceiptItem(nil), accepted.Items...)
	contradiction.Items[0].ReasonCode = "runtime_pending"
	assert.ErrorIs(t, f.store.ApplyReceipt(ctx, contradiction), ErrDonationReceipt, "equal revision cannot change facts")

	credited, err := f.store.CreditDonationReward(ctx, item.ID)
	require.NoError(t, err)
	assert.True(t, credited)
	credited, err = f.store.CreditDonationReward(ctx, item.ID)
	require.NoError(t, err)
	assert.False(t, credited, "the permanent reward is credited exactly once")
	var rewardCount int64
	require.NoError(t, f.db.Model(&DonationReward{}).Where("item_id = ?", item.ID).Count(&rewardCount).Error)
	assert.Equal(t, int64(1), rewardCount)
}

// testDonationManualAcceptanceRequiresApproval proves an accepted receipt alone
// never confirms a payout for a manual item, and that a rejection releases only
// undecided ownership while projecting the reason to the donor.
func testDonationManualAcceptanceRequiresApproval(t *testing.T, f donationFixture) {
	ctx := t.Context()
	campaign, err := f.store.SaveCampaign(ctx, DonationCampaign{Name: "Manual rejection", InstanceID: f.campaign.InstanceID, SourceID: f.campaign.SourceID,
		GroupID: 1, GroupName: "manual group", TargetRevision: strings.Repeat("d", 64), ValidationMode: DonationModeManualReview, RewardQuota: 25, Enabled: true}, 0)
	require.NoError(t, err)
	batch := prepareDonationTest(t, f.store, f.user.Id, campaign, "manual-no-approval-0001\nmanual-reject-key-0002")
	receipt := donationReceiptFor(batch, "pending_review", "pending_review")
	receipt.TargetRevision = campaign.TargetRevision
	require.NoError(t, f.store.ApplyReceipt(ctx, receipt))
	detail, err := f.store.Batch(ctx, batch.ID, f.user.Id)
	require.NoError(t, err)
	require.Len(t, detail.Items, 2)
	assert.Equal(t, 2, detail.Summary.PendingReview)

	// An acceptance that arrives without any applied approval fact cannot be paid.
	accepted := donationReceiptFor(batch, "accepted", "pending_review")
	accepted.TargetRevision = campaign.TargetRevision
	accepted.Items[0].ItemRevision = int64Ptr(4)
	accepted.Items[0].EffectiveMode = DonationModeManualReview
	accepted.Items[0].ReviewTargetRevision = campaign.TargetRevision
	require.ErrorIs(t, f.store.ApplyReceipt(ctx, accepted), ErrDonationReceipt)
	credited, err := f.store.CreditDonationReward(ctx, detail.Items[0].ID)
	require.ErrorIs(t, err, ErrDonationReceipt)
	assert.False(t, credited)
	var unapproved DonationItem
	require.NoError(t, f.db.First(&unapproved, "id = ?", detail.Items[0].ID).Error)
	assert.Equal(t, "pending_review", unapproved.State)
	assert.Equal(t, "none", unapproved.RewardState)

	// Rejection releases the undecided owner and exposes only the donor-visible note.
	rejectID := uuid.NewString()
	intent, err := f.store.PrepareReviewAction(ctx, 7, detail.Items[1].ID, rejectID, DonationActionReject, 1, "", "the key is already rate limited upstream")
	require.NoError(t, err)
	saved, err := f.store.ApplyReviewOutcome(ctx, intent.Action, DonationActionApplied, "", 2, "", time.Now().UnixMilli())
	require.NoError(t, err)
	receipt.Items[1].State, receipt.Items[1].ReasonCode = "rejected", "review_rejected"
	receipt.Items[1].ItemRevision = int64Ptr(2)
	receipt.Items[1].ReviewActionID, receipt.Items[1].ReviewDecision, receipt.Items[1].ReviewedAtMS = rejectID, "rejected", saved.AppliedAtMS
	require.NoError(t, f.store.ApplyReceipt(ctx, receipt))
	rejected, err := f.store.Batch(ctx, batch.ID, f.user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, rejected.Summary.Rejected)
	assert.Equal(t, "rejected", rejected.Items[1].State)
	assert.Equal(t, "review_rejected", rejected.Items[1].ReasonCode)
	assert.Equal(t, "the key is already rate limited upstream", rejected.Items[1].ReviewNote)
	assert.Empty(t, rejected.Items[0].ReviewNote, "an approval note is never projected to the donor")
	var resource DonationResource
	require.NoError(t, f.db.Where("fingerprint = ?", rejected.Items[1].Fingerprint).First(&resource).Error)
	assert.Empty(t, resource.OwnerItemID)
	assert.False(t, resource.Acquired)
}

func int64Ptr(value int64) *int64 { return &value }

func testDonationManualApprovalFindsExisting(t *testing.T, f donationFixture) {
	t.Helper()
	ctx := t.Context()
	campaign := f.campaign
	campaign.ID, campaign.Version, campaign.ValidationMode = 0, 0, DonationModeManualReview
	campaign, err := f.store.SaveCampaign(ctx, campaign, 0)
	require.NoError(t, err)
	for _, status := range []string{DonationActionApplied, DonationActionPending} {
		t.Run(status, func(t *testing.T) {
			batch := prepareDonationTest(t, f.store, f.user.Id, campaign, "manual-existing-"+status)
			receipt := donationReceiptFor(batch, "pending_review")
			require.NoError(t, f.store.ApplyReceipt(ctx, receipt))
			itemID, actionID := batch.Items[0].ID, uuid.NewString()
			intent, err := f.store.PrepareReviewAction(ctx, 7, itemID, actionID, DonationActionApprove, 1, campaign.TargetRevision, "")
			require.NoError(t, err)
			appliedAtMS := time.Now().UnixMilli()
			// Inventory won first: the receiver finishes existing, then stamps the
			// approval in the same transaction. That final revision is also the
			// action's effect revision, even if its response is lost.
			receipt.Items[0].State, receipt.Items[0].ReasonCode = "existing", "already_exists"
			receipt.Items[0].ItemRevision = int64Ptr(3)
			receipt.Items[0].ReviewActionID, receipt.Items[0].ReviewDecision = actionID, "approved"
			receipt.Items[0].ReviewedAtMS = &appliedAtMS
			if status == DonationActionApplied {
				_, err = f.store.ApplyReviewOutcome(ctx, intent.Action, DonationActionApplied, "", 3, campaign.TargetRevision, appliedAtMS)
				require.NoError(t, err)
			}
			require.NoError(t, f.store.ApplyReceipt(ctx, receipt))
			require.NoError(t, f.store.ApplyReceipt(ctx, receipt))
			_, err = f.store.ApplyReviewOutcome(ctx, intent.Action, DonationActionApplied, "", 3, campaign.TargetRevision, appliedAtMS)
			require.NoError(t, err, "the confirmed action query must agree with the full receipt")
			detail, err := f.store.Batch(ctx, batch.ID, f.user.Id)
			require.NoError(t, err)
			assert.Equal(t, "existing", detail.Items[0].State)
			assert.Equal(t, "approved", detail.Items[0].ReviewDecision)
			assert.Equal(t, 1, detail.Summary.Duplicate)
			assert.Zero(t, detail.Summary.RewardedQuota)
			credited, err := f.store.CreditDonationReward(ctx, itemID)
			assert.ErrorIs(t, err, ErrDonationReceipt)
			assert.False(t, credited)
			var resource DonationResource
			require.NoError(t, f.db.Where("fingerprint = ?", batch.Items[0].Fingerprint).First(&resource).Error)
			assert.True(t, resource.Acquired)
			assert.Empty(t, resource.AcceptedItemID, "inventory must not claim a donation acquisition")
		})
	}
}

func TestDonationReviewIntentBinding(t *testing.T) {
	f := newDonationFixture(t, "sqlite", false)
	campaign := f.campaign
	campaign.ID, campaign.Version, campaign.ValidationMode = 0, 0, DonationModeManualReview
	campaign, err := f.store.SaveCampaign(t.Context(), campaign, 0)
	require.NoError(t, err)
	batch := prepareDonationTest(t, f.store, f.user.Id, campaign, "review-intent-a\nreview-intent-b")
	receipt := donationReceiptFor(batch, "pending_review", "pending_review")
	for i := range receipt.Items {
		receipt.Items[i].EffectiveMode = DonationModeManualReview
		receipt.Items[i].ItemRevision = int64Ptr(1)
		receipt.Items[i].ReviewTargetRevision = campaign.TargetRevision
	}
	require.NoError(t, f.store.ApplyReceipt(t.Context(), receipt))
	id := uuid.NewString()
	intent, err := f.store.PrepareReviewAction(t.Context(), 7, batch.Items[0].ID, id, DonationActionApprove, 1, campaign.TargetRevision, "")
	require.NoError(t, err)
	_, err = f.store.PrepareReviewAction(t.Context(), 8, batch.Items[1].ID, id, DonationActionApprove, 1, campaign.TargetRevision, "")
	assert.ErrorIs(t, err, ErrDonationConflict, "a source-wide action ID cannot belong to two actors")
	_, err = f.store.PrepareReviewAction(t.Context(), 7, batch.Items[0].ID, uuid.NewString(), DonationActionReject, 1, "", "opposite decision")
	assert.ErrorIs(t, err, ErrDonationConflict, "an uncertain intent cannot be replaced by an opposite decision")
	_, err = f.store.ApplyReviewOutcome(t.Context(), intent.Action, DonationActionApplied, "", 2, strings.Repeat("f", 64), time.Now().UnixMilli())
	assert.ErrorIs(t, err, ErrDonationReceipt, "the confirmed target must match the saved intent")
}

func testDonationManualIntentConcurrency(t *testing.T, f donationFixture) {
	t.Helper()
	campaign := f.campaign
	campaign.ID, campaign.Version, campaign.ValidationMode = 0, 0, DonationModeManualReview
	campaign, err := f.store.SaveCampaign(t.Context(), campaign, 0)
	require.NoError(t, err)
	batch := prepareDonationTest(t, f.store, f.user.Id, campaign, "intent-race-a\nintent-race-b\nintent-race-c\ntest-race-a\ntest-race-b\ntest-race-c")
	receipt := donationReceiptFor(batch, "pending_review", "pending_review", "pending_review", "pending_review", "pending_review", "pending_review")
	require.NoError(t, f.store.ApplyReceipt(t.Context(), receipt))
	stores := []*DonationStore{f.store, f.otherStore}
	for _, test := range []struct {
		name     string
		sameItem bool
		tests    bool
	}{
		{name: "opposite_decisions", sameItem: true},
		{name: "two_actors_one_action_id"},
		{name: "one_running_test", sameItem: true, tests: true},
		{name: "two_actors_one_test_id", tests: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			sharedID := uuid.NewString()
			var failures [2]error
			var attempts [2]DonationTestIntent
			start := make(chan struct{})
			var workers sync.WaitGroup
			for i := range 2 {
				workers.Go(func() {
					<-start
					index, requestID := i+1, sharedID
					if test.sameItem {
						index, requestID = 0, uuid.NewString()
					}
					if test.tests {
						attempts[i], failures[i] = stores[i].PrepareTestAttempt(t.Context(), 7+i, batch.Items[index+3].ID,
							requestID, 1, campaign.TargetRevision, "test-model", false, 1, strings.Repeat("c", 64))
						return
					}
					kind, note := DonationActionApprove, ""
					if test.sameItem && i == 1 {
						kind, note = DonationActionReject, "independent rejection"
					}
					_, failures[i] = stores[i].PrepareReviewAction(t.Context(), 7+i, batch.Items[index].ID,
						requestID, kind, 1, campaign.TargetRevision, note)
				})
			}
			close(start)
			workers.Wait()
			require.NotEqual(t, failures[0] == nil, failures[1] == nil, "one durable intent must win")
			for i, err := range failures {
				if err != nil {
					if test.tests && test.sameItem {
						assert.ErrorIs(t, err, ErrDonationTestBusy)
					} else {
						assert.ErrorIs(t, err, ErrDonationConflict)
					}
				} else if test.tests {
					require.True(t, attempts[i].Created)
					require.NoError(t, stores[i].FinishTestAttempt(t.Context(), attempts[i].Attempt.ID, DonationTestInterrupted, "interrupted", 0, 0, 0, 0, time.Now().UnixMilli()))
				}
			}
		})
	}
}

// These three models freeze the released schema before manual review. The
// remaining donation models are unchanged by this migration.
type donationLegacyCampaign struct {
	ID             int    `json:"id" gorm:"primaryKey"`
	Version        int    `json:"version"`
	Name           string `json:"name" gorm:"size:120"`
	Description    string `json:"description" gorm:"type:text"`
	InstanceID     string `json:"instance_id" gorm:"size:36"`
	SourceID       string `json:"-" gorm:"size:36"`
	GroupID        uint64 `json:"group_id"`
	GroupName      string `json:"group_name" gorm:"size:255"`
	TargetRevision string `json:"target_revision" gorm:"size:64"`
	RewardQuota    int    `json:"reward_quota" gorm:"type:bigint"`
	Enabled        bool   `json:"enabled"`
	CreatedAtMS    int64  `json:"created_at_ms"`
	UpdatedAtMS    int64  `json:"updated_at_ms"`
}

type donationLegacyBatch struct {
	ID              string `gorm:"primaryKey;size:36"`
	UserID          int    `gorm:"index;uniqueIndex:idx_donation_user_request,priority:1"`
	Username        string `gorm:"size:64"`
	LinuxDOID       string `gorm:"size:255"`
	RequestKey      string `gorm:"size:36;uniqueIndex:idx_donation_user_request,priority:2"`
	RequestDigest   string `gorm:"size:64"`
	SecretID        string `gorm:"size:64"`
	CampaignID      int    `gorm:"index"`
	CampaignVersion int
	CampaignName    string `gorm:"size:120"`
	InstanceID      string `gorm:"size:36"`
	SourceID        string `gorm:"size:36"`
	GroupID         uint64 `gorm:"index"`
	GroupName       string `gorm:"size:255"`
	TargetRevision  string `gorm:"size:64"`
	RewardQuota     int    `gorm:"type:bigint"`
	ReceptionState  string `gorm:"size:24"`
	LastError       string `gorm:"size:64"`
	CreatedAtMS     int64  `gorm:"index"`
	UpdatedAtMS     int64
	NeedsRecovery   bool  `gorm:"index:idx_donation_recovery,priority:1"`
	NextPollAtMS    int64 `gorm:"index:idx_donation_recovery,priority:2"`
	LeaseUntilMS    int64
	LeaseToken      string `gorm:"size:36"`
	PollAttempts    int
	SendAttempts    int
}

type donationLegacyItem struct {
	ID            string `gorm:"primaryKey;size:36"`
	BatchID       string `gorm:"size:36;index;uniqueIndex:idx_donation_batch_line,priority:1"`
	Line          int    `gorm:"uniqueIndex:idx_donation_batch_line,priority:2"`
	Fingerprint   string `gorm:"size:67;index"`
	Generation    int64
	KeyMask       string `gorm:"size:32"`
	Dispatch      bool
	DuplicateOf   string `gorm:"size:36"`
	State         string `gorm:"size:24;index"`
	ReasonCode    string `gorm:"size:64"`
	Retryable     bool
	CredentialID  *uint64 `gorm:"index"`
	AcceptedAtMS  *int64
	RewardState   string `gorm:"size:24;index"`
	RewardReason  string `gorm:"size:64"`
	RewardedQuota int    `gorm:"type:bigint"`
	RewardedAtMS  *int64
	CreatedAtMS   int64
	UpdatedAtMS   int64
}

type donationLegacyNamer struct{ schema.Namer }

func (n donationLegacyNamer) TableName(name string) string {
	switch name {
	case "donationLegacyCampaign":
		name = "DonationCampaign"
	case "donationLegacyBatch":
		name = "DonationBatch"
	case "donationLegacyItem":
		name = "DonationItem"
	}
	return n.Namer.TableName(name)
}

func seedDonationLegacyHistory(t *testing.T, db *gorm.DB, user User) func() {
	t.Helper()
	legacy := db.Session(&gorm.Session{NewDB: true})
	legacy.Config.NamingStrategy = donationLegacyNamer{db.NamingStrategy}
	require.NoError(t, legacy.AutoMigrate(&DonationSecret{}, &DonationConnection{}, &donationLegacyCampaign{}, &DonationCampaignRevision{},
		&donationLegacyBatch{}, &donationLegacyItem{}, &DonationResource{}, &DonationReward{}, &DonationRetry{}, &DonationEvent{}))
	assert.False(t, legacy.Migrator().HasColumn(&donationLegacyBatch{}, "validation_mode"))
	assert.False(t, legacy.Migrator().HasColumn(&donationLegacyItem{}, "item_revision"))

	material := []byte(strings.Repeat("d", 64))
	require.Len(t, material, 64)
	sum := sha256.Sum256(material)
	secret := DonationSecret{Slot: "v1", Material: base64.StdEncoding.EncodeToString(material), Checksum: hex.EncodeToString(sum[:])}
	require.NoError(t, legacy.Create(&secret).Error)
	store := &DonationStore{DB: legacy, secretID: secret.Checksum, fingerprintKey: material[:32], tokenKey: material[32:]}
	connection := DonationConnection{BaseURL: "https://donation.example", InstanceID: uuid.NewString(), SourceID: uuid.NewString()}
	require.NoError(t, store.SaveConnection(t.Context(), connection, strings.Repeat("t", 40), 0))
	now := time.Now().Add(-time.Hour).UnixMilli()
	campaign := donationLegacyCampaign{Version: 1, Name: "Released campaign", Description: "old history", InstanceID: connection.InstanceID,
		SourceID: connection.SourceID, GroupID: 1, GroupName: "legacy group", TargetRevision: strings.Repeat("a", 64), RewardQuota: 25, Enabled: true,
		CreatedAtMS: now, UpdatedAtMS: now}
	require.NoError(t, legacy.Create(&campaign).Error)
	snapshot, err := common.Marshal(campaign)
	require.NoError(t, err)
	revision := DonationCampaignRevision{CampaignID: campaign.ID, Version: 1, Snapshot: string(snapshot), CreatedAtMS: now}
	require.NoError(t, legacy.Create(&revision).Error)
	lines, digest, err := store.ParseLines(campaign.ID, "released-donation-key")
	require.NoError(t, err)
	batch := donationLegacyBatch{ID: uuid.NewString(), UserID: user.Id, Username: user.Username, LinuxDOID: user.LinuxDOId,
		RequestKey: uuid.NewString(), RequestDigest: digest, SecretID: secret.Checksum, CampaignID: campaign.ID, CampaignVersion: campaign.Version,
		CampaignName: campaign.Name, InstanceID: connection.InstanceID, SourceID: connection.SourceID, GroupID: campaign.GroupID, GroupName: campaign.GroupName,
		TargetRevision: campaign.TargetRevision, RewardQuota: 25, ReceptionState: "confirmed", CreatedAtMS: now, UpdatedAtMS: now, SendAttempts: 1}
	require.NoError(t, legacy.Create(&batch).Error)
	credentialID := uint64(71)
	item := donationLegacyItem{ID: lines[0].ID, BatchID: batch.ID, Line: 1, Fingerprint: lines[0].Fingerprint, Generation: 1, KeyMask: "masked", Dispatch: true,
		State: "accepted", CredentialID: &credentialID, AcceptedAtMS: &now, RewardState: "rewarded", RewardedQuota: 25, RewardedAtMS: &now,
		CreatedAtMS: now, UpdatedAtMS: now}
	require.NoError(t, legacy.Create(&item).Error)
	resource := DonationResource{Fingerprint: item.Fingerprint, SecretID: secret.Checksum, OwnerItemID: item.ID, Generation: 1, Acquired: true, AcceptedItemID: item.ID, UpdatedAtMS: now}
	reward := DonationReward{ID: uuid.NewString(), Fingerprint: item.Fingerprint, ItemID: item.ID, UserID: user.Id, Quota: 25, CreditedAtMS: now}
	retry := DonationRetry{ID: uuid.NewString(), UserID: user.Id, RequestKey: uuid.NewString(), BatchID: batch.ID, Digest: strings.Repeat("b", 64), ItemIDsJSON: "[]", Done: true, CreatedAtMS: now}
	require.NoError(t, legacy.Create(&resource).Error)
	require.NoError(t, legacy.Create(&reward).Error)
	require.NoError(t, legacy.Create(&retry).Error)
	require.NoError(t, legacy.Create(&DonationEvent{ItemID: item.ID, State: "rewarded", CreatedAtMS: now}).Error)

	return func() {
		var savedCampaign donationLegacyCampaign
		var savedBatch donationLegacyBatch
		var savedItem donationLegacyItem
		var savedReward DonationReward
		var savedResource DonationResource
		var savedRevision DonationCampaignRevision
		var savedRetry DonationRetry
		require.NoError(t, legacy.First(&savedCampaign, campaign.ID).Error)
		require.NoError(t, legacy.Where("id = ?", batch.ID).First(&savedBatch).Error)
		require.NoError(t, legacy.Where("id = ?", item.ID).First(&savedItem).Error)
		require.NoError(t, legacy.Where("id = ?", reward.ID).First(&savedReward).Error)
		require.NoError(t, legacy.Where("fingerprint = ?", resource.Fingerprint).First(&savedResource).Error)
		require.NoError(t, legacy.First(&savedRevision, revision.ID).Error)
		require.NoError(t, legacy.Where("id = ?", retry.ID).First(&savedRetry).Error)
		assert.Equal(t, campaign, savedCampaign)
		assert.Equal(t, batch, savedBatch)
		assert.Equal(t, item, savedItem)
		assert.Equal(t, reward, savedReward)
		assert.Equal(t, resource, savedResource)
		assert.Equal(t, revision, savedRevision, "the old JSON snapshot must remain byte-identical")
		assert.Equal(t, retry, savedRetry)
		upgraded, err := OpenDonationStore(db)
		require.NoError(t, err)
		assert.Equal(t, store.secretID, upgraded.secretID)
		currentCampaign, err := upgraded.Campaign(t.Context(), campaign.ID)
		require.NoError(t, err)
		assert.Equal(t, DonationModeAuto, currentCampaign.ValidationMode)
		replayed, err := upgraded.PrepareBatch(t.Context(), user.Id, batch.RequestKey, digest, currentCampaign, lines)
		require.NoError(t, err)
		assert.Equal(t, batch.ID, replayed.ID)
		assert.Equal(t, DonationModeAuto, replayed.ValidationMode)
		currentItem, _, err := upgraded.Item(t.Context(), item.ID)
		require.NoError(t, err)
		assert.Equal(t, DonationModeAuto, currentItem.EffectiveMode)
		assert.Zero(t, currentItem.ItemRevision)
		credited, err := upgraded.CreditDonationReward(t.Context(), item.ID)
		require.NoError(t, err)
		assert.False(t, credited)
		assert.True(t, donationUniqueError(db.Create(&DonationReward{ID: uuid.NewString(), Fingerprint: reward.Fingerprint, ItemID: uuid.NewString(), UserID: user.Id, Quota: 25}).Error))
		assert.True(t, donationUniqueError(db.Create(&DonationReward{ID: uuid.NewString(), Fingerprint: upgraded.Fingerprint("other-legacy-key"), ItemID: reward.ItemID, UserID: user.Id, Quota: 25}).Error))
	}
}

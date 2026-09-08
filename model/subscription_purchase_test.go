package model

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupSubscriptionPurchaseTestDB(t *testing.T, dialect common.DatabaseType) {
	t.Helper()
	var dialector gorm.Dialector
	switch dialect {
	case common.DatabaseTypeSQLite:
		dialector = sqlite.Open(filepath.Join(t.TempDir(), "subscriptions.db"))
	case common.DatabaseTypeMySQL:
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
		dialector = mysql.Open(dsn)
	case common.DatabaseTypePostgreSQL:
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN is not configured")
		}
		dialector = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
	}
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	tables := []interface{}{&User{}, &SubscriptionPlan{}, &UserSubscription{}, &SubscriptionOrder{}, &TopUp{}, &Log{}}
	for _, table := range tables {
		require.False(t, db.Migrator().HasTable(table), "subscription tests require an empty test database")
	}
	t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(tables...)) })
	require.NoError(t, db.AutoMigrate(tables...))

	previousDB, previousLogDB := DB, LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousBatch := common.RedisEnabled, common.BatchUpdateEnabled
	previousQuotaPerUnit := common.QuotaPerUnit
	DB, LOG_DB = db, db
	common.SetDatabaseTypes(dialect, dialect)
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	common.QuotaPerUnit = 500000
	initCol()
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled, common.BatchUpdateEnabled = previousRedis, previousBatch
		common.QuotaPerUnit = previousQuotaPerUnit
		initCol()
	})

	var version string
	versionQuery := "SELECT version()"
	if dialect == common.DatabaseTypeSQLite {
		versionQuery = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database: %s %s", dialect, version)
}

func seedSubscriptionPurchase(t *testing.T, limit int) (*User, *SubscriptionPlan) {
	t.Helper()
	plan := &SubscriptionPlan{
		Title:              "Renewable subscription",
		PriceAmount:        2,
		DurationUnit:       SubscriptionDurationDay,
		DurationValue:      7,
		Enabled:            true,
		MaxPurchasePerUser: limit,
		TotalAmount:        1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	InvalidateSubscriptionPlanCache(plan.Id)
	t.Cleanup(func() { InvalidateSubscriptionPlanCache(plan.Id) })
	user := &User{
		Username: fmt.Sprintf("subscription-limit-%d", plan.Id),
		AffCode:  fmt.Sprintf("sl%d", plan.Id),
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    5000000,
	}
	require.NoError(t, DB.Create(user).Error)
	return user, plan
}

func TestSubscriptionPurchaseLimit(t *testing.T) {
	for _, dialect := range []common.DatabaseType{common.DatabaseTypeSQLite, common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(dialect), func(t *testing.T) {
			setupSubscriptionPurchaseTestDB(t, dialect)
			t.Run("active_count_and_creation", func(t *testing.T) {
				cases := []struct {
					name        string
					status      string
					endOffset   int64
					amountUsed  int64
					copies      int
					limit       int
					activeCount int64
					blocked     bool
				}{
					{name: "active_blocks", status: "active", endOffset: 3600, copies: 1, limit: 1, activeCount: 1, blocked: true},
					{name: "expired_before_cleanup_allows", status: "active", endOffset: -3600, copies: 1, limit: 1},
					{name: "expiry_boundary_allows", status: "active", endOffset: 0, copies: 1, limit: 1},
					{name: "expired_status_allows", status: "expired", endOffset: 3600, copies: 1, limit: 1},
					{name: "cancelled_allows", status: "cancelled", endOffset: 3600, copies: 1, limit: 1},
					{name: "exhausted_quota_still_blocks", status: "active", endOffset: 3600, amountUsed: 1000, copies: 1, limit: 1, activeCount: 1, blocked: true},
					{name: "remaining_slot_allows", status: "active", endOffset: 3600, copies: 1, limit: 2, activeCount: 1},
					{name: "multiple_active_block", status: "active", endOffset: 3600, copies: 2, limit: 2, activeCount: 2, blocked: true},
					{name: "zero_limit_allows", status: "active", endOffset: 3600, copies: 1, limit: 0, activeCount: 1},
				}
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						user, plan := seedSubscriptionPurchase(t, tc.limit)
						now := GetDBTimestamp()
						for range tc.copies {
							require.NoError(t, DB.Create(&UserSubscription{
								UserId: user.Id, PlanId: plan.Id, Status: tc.status,
								StartTime: now - 7200, EndTime: now + tc.endOffset,
								AmountTotal: plan.TotalAmount, AmountUsed: tc.amountUsed,
							}).Error)
						}
						count, err := CountUserSubscriptionsByPlan(user.Id, plan.Id)
						require.NoError(t, err)
						assert.Equal(t, tc.activeCount, count)
						err = DB.Transaction(func(tx *gorm.DB) error {
							_, err := CreateUserSubscriptionFromPlanTx(tx, user.Id, plan, "order")
							return err
						})
						if tc.blocked {
							require.EqualError(t, err, "已达到该套餐购买上限")
							assert.EqualValues(t, tc.copies, countUserPlanSubs(t, user.Id, plan.Id))
							return
						}
						require.NoError(t, err)
						assert.EqualValues(t, tc.copies+1, countUserPlanSubs(t, user.Id, plan.Id))
					})
				}
			})

			t.Run("only_same_user_and_plan_occupy_slots", func(t *testing.T) {
				user, plan := seedSubscriptionPurchase(t, 2)
				otherUser, otherPlan := seedSubscriptionPurchase(t, 1)
				now := GetDBTimestamp()
				require.NoError(t, DB.Create(&[]UserSubscription{
					{UserId: user.Id, PlanId: plan.Id, Status: "active", EndTime: now + 3600},
					{UserId: user.Id, PlanId: plan.Id, Status: "active", EndTime: now - 3600},
					{UserId: user.Id, PlanId: otherPlan.Id, Status: "active", EndTime: now + 3600},
					{UserId: otherUser.Id, PlanId: plan.Id, Status: "active", EndTime: now + 3600},
				}).Error)
				count, err := CountUserSubscriptionsByPlan(user.Id, plan.Id)
				require.NoError(t, err)
				assert.EqualValues(t, 1, count)
				_, err = AdminBindSubscription(user.Id, plan.Id, "test")
				require.NoError(t, err)
				_, err = AdminBindSubscription(user.Id, plan.Id, "test")
				require.EqualError(t, err, "已达到该套餐购买上限")
			})

			t.Run("balance_renewal_and_limit_rollback", func(t *testing.T) {
				user, plan := seedSubscriptionPurchase(t, 1)
				require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id))
				require.EqualError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id), "已达到该套餐购买上限")
				var updated User
				require.NoError(t, DB.First(&updated, user.Id).Error)
				assert.Equal(t, 4000000, updated.Quota)

				// Expiry releases the slot even before the scheduled status update runs.
				require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", user.Id).
					Update("end_time", GetDBTimestamp()-1).Error)
				require.NoError(t, PurchaseSubscriptionWithBalance(user.Id, plan.Id))
				require.NoError(t, DB.First(&updated, user.Id).Error)
				assert.Equal(t, 3000000, updated.Quota)
				assert.EqualValues(t, 2, countUserPlanSubs(t, user.Id, plan.Id))
				var orders int64
				require.NoError(t, DB.Model(&SubscriptionOrder{}).Where("user_id = ? AND status = ?", user.Id, common.TopUpStatusSuccess).Count(&orders).Error)
				assert.EqualValues(t, 2, orders)
			})

			t.Run("payment_completion_renewal_is_idempotent", func(t *testing.T) {
				user, plan := seedSubscriptionPurchase(t, 1)
				firstTrade := fmt.Sprintf("subscription-limit-first-%d", plan.Id)
				secondTrade := fmt.Sprintf("subscription-limit-second-%d", plan.Id)
				for _, trade := range []string{firstTrade, secondTrade} {
					require.NoError(t, (&SubscriptionOrder{
						UserId: user.Id, PlanId: plan.Id, Money: plan.PriceAmount,
						TradeNo: trade, PaymentProvider: PaymentProviderStripe,
						PaymentMethod: PaymentMethodStripe, Status: common.TopUpStatusPending,
					}).Insert())
				}
				require.NoError(t, CompleteSubscriptionOrder(firstTrade, "", PaymentProviderStripe, ""))
				require.NoError(t, CompleteSubscriptionOrder(firstTrade, "", PaymentProviderStripe, ""))
				assert.EqualValues(t, 1, countUserPlanSubs(t, user.Id, plan.Id))
				require.EqualError(t, CompleteSubscriptionOrder(secondTrade, "", PaymentProviderStripe, ""), "已达到该套餐购买上限")
				pending := GetSubscriptionOrderByTradeNo(secondTrade)
				require.NotNil(t, pending)
				assert.Equal(t, common.TopUpStatusPending, pending.Status)

				require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", user.Id).
					Update("end_time", GetDBTimestamp()-1).Error)
				require.NoError(t, CompleteSubscriptionOrder(secondTrade, "", PaymentProviderStripe, ""))
				require.NoError(t, CompleteSubscriptionOrder(secondTrade, "", PaymentProviderStripe, ""))
				assert.EqualValues(t, 2, countUserPlanSubs(t, user.Id, plan.Id))
				count, err := CountUserSubscriptionsByPlan(user.Id, plan.Id)
				require.NoError(t, err)
				assert.EqualValues(t, 1, count)
				var topups int64
				require.NoError(t, DB.Model(&TopUp{}).Where("user_id = ?", user.Id).Count(&topups).Error)
				assert.EqualValues(t, 2, topups)
			})
		})
	}
}

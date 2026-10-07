package helper_test

import (
	"context"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/groupload"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestGroupMultiplierDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(":memory:")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN not configured")
				}
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{NamingStrategy: schema.NamingStrategy{TablePrefix: "multiplier_test_"}})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB, previousType, previousEnabled, previousRedis := model.DB, common.MainDatabaseType(), common.DynamicRatioEnabled, common.RedisEnabled
			previousGroups := ratio_setting.GroupRatio2JSONString()
			common.OptionMapRWMutex.Lock()
			previousOptions := maps.Clone(common.OptionMap)
			common.OptionMap = make(map[string]string)
			common.OptionMapRWMutex.Unlock()
			previousPolicies := previousOptions[model.GroupMultiplierPoliciesOption]
			if previousPolicies == "" {
				previousPolicies = "{}"
			}
			model.DB = db
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			common.RedisEnabled = false
			common.DynamicRatioEnabled = true
			t.Cleanup(func() {
				require.NoError(t, model.UpdateOptionsBulk(map[string]string{model.GroupMultiplierPoliciesOption: previousPolicies}))
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
				model.SetDynamicRatioRulesForTest(nil)
				require.NoError(t, db.Migrator().DropTable(&model.Option{}, &model.Task{}, &model.DynamicRatioRule{}))
				model.DB = previousDB
				common.SetMainDatabaseType(previousType)
				common.DynamicRatioEnabled = previousEnabled
				common.RedisEnabled = previousRedis
				common.OptionMapRWMutex.Lock()
				common.OptionMap = previousOptions
				common.OptionMapRWMutex.Unlock()
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&model.Option{}, &model.Task{}, &model.DynamicRatioRule{}))
			var dbVersion string
			versionSQL := "SELECT version()"
			if dialect == "sqlite" {
				versionSQL = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(versionSQL).Scan(&dbVersion).Error)
			t.Logf("%s: %s", dialect, dbVersion)
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"multiplier-a":2,"multiplier-b":3}`))
			require.NoError(t, model.UpdateOptionsBulk(map[string]string{model.GroupMultiplierPoliciesOption: "{}"}))
			minimum := int64(100)
			oldRule := model.DynamicRatioRule{Enable: true, Group: "multiplier-a", BalanceMinQuota: &minimum, Ratio: 0.5}
			require.NoError(t, db.Create(&oldRule).Error)
			model.InitDynamicRatioCache()
			policy, version := model.GetGroupMultiplierPolicy("multiplier-a")
			assert.Equal(t, model.MultiplierBalance, policy.Mode)
			assert.Equal(t, 0.5, model.GetMatchedDynamicRatioMatch("multiplier-a", "model", 100).Ratio)
			policy = model.GroupMultiplierPolicy{Mode: model.MultiplierConcurrency, Tiers: []model.ConcurrencyTier{{Minimum: 0, Multiplier: 1}, {Minimum: 8, Multiplier: 2}, {Minimum: 15, Multiplier: 3}}}
			require.NoError(t, model.SaveGroupMultiplierPolicy("multiplier-a", policy, version))
			_, otherVersion := model.GetGroupMultiplierPolicy("multiplier-b")
			require.NoError(t, model.SaveGroupMultiplierPolicy("multiplier-b", model.GroupMultiplierPolicy{Mode: model.MultiplierFixed}, otherVersion))
			secondPolicy, secondVersion := model.GetGroupMultiplierPolicy("multiplier-b")
			require.NoError(t, model.SaveGroupMultiplierPolicy("multiplier-b", secondPolicy, secondVersion), "an omitted tier list remains editable with the returned version")
			unchanged, _ := model.GetGroupMultiplierPolicy("multiplier-a")
			assert.Equal(t, policy, unchanged, "saving another group preserves this group's policy")
			assert.ErrorIs(t, model.SaveGroupMultiplierPolicy("multiplier-a", policy, version), model.ErrGroupMultiplierConflict)
			assert.Zero(t, model.GetMatchedDynamicRatioMatch("multiplier-a", "model", 100).Ratio, "balance rules are inactive")
			for i := range 7 {
				_, err := groupload.Register("multiplier-a", dialect+string(rune('a'+i)), time.Minute)
				require.NoError(t, err)
			}
			t.Cleanup(func() {
				for i := range 7 {
					require.NoError(t, groupload.Release("multiplier-a", dialect+string(rune('a'+i))))
				}
			})
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{UsingGroup: "multiplier-a", OriginModelName: "model", UserQuota: 100}
			t.Cleanup(func() { info.GroupLoadLease.Close() })
			price, err := helper.HandleGroupRatio(ctx, info)
			require.NoError(t, err)
			assert.Equal(t, 4.0, price.GroupRatio)
			require.NotNil(t, price.MultiplierSnapshot)
			assert.EqualValues(t, 8, price.MultiplierSnapshot.Concurrency)
			assert.EqualValues(t, 8, price.MultiplierSnapshot.Minimum)
			statusRecorder := httptest.NewRecorder()
			statusContext, _ := gin.CreateTestContext(statusRecorder)
			controller.GetGroupMultiplierPolicies(statusContext)
			var statusResponse struct {
				Success bool `json:"success"`
				Data    []struct {
					Group            string  `json:"group"`
					Concurrency      int64   `json:"concurrency"`
					EffectiveRatio   float64 `json:"effective_ratio"`
					NextRequestRatio float64 `json:"next_request_ratio"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(statusRecorder.Body.Bytes(), &statusResponse))
			require.True(t, statusResponse.Success)
			require.Len(t, statusResponse.Data, 2)
			assert.Equal(t, "multiplier-a", statusResponse.Data[0].Group)
			assert.EqualValues(t, 8, statusResponse.Data[0].Concurrency)
			assert.Equal(t, 4.0, statusResponse.Data[0].EffectiveRatio)
			assert.Equal(t, 4.0, statusResponse.Data[0].NextRequestRatio)
			conflictRecorder := httptest.NewRecorder()
			conflictContext, _ := gin.CreateTestContext(conflictRecorder)
			conflictContext.Request = httptest.NewRequest(http.MethodPut, "/api/dynamic_ratio/policies", strings.NewReader(`{"group":"multiplier-a","policy":{"mode":"fixed","tiers":[]},"expected_version":"stale"}`))
			controller.UpdateGroupMultiplierPolicy(conflictContext)
			assert.Equal(t, http.StatusConflict, conflictRecorder.Code)
			configBefore := map[string]string{}
			require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { configBefore[key] = value; return nil }))
			t.Cleanup(func() { require.NoError(t, config.GlobalConfig.LoadFromDB(configBefore)) })
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_mode": `{"multiplier-expression":"tiered_expr"}`, "billing_setting.billing_expr": `{"multiplier-expression":"tier(\"request\", fixed(0.01))"}`}))
			billingInfo := &relaycommon.RelayInfo{UsingGroup: "multiplier-a", OriginModelName: "multiplier-expression", UserQuota: 100, BillingRequestInput: &billingexpr.RequestInput{}}
			t.Cleanup(func() { billingInfo.GroupLoadLease.Close() })
			reservation, err := helper.ModelPriceHelper(ctx, billingInfo, 1000, &kittypes.TokenCountMeta{})
			require.NoError(t, err)
			assert.Equal(t, 20000, reservation.QuotaToPreConsume, "one-cent request at a total group multiplier of four")
			uninitializedRedis := common.RDB
			common.RedisEnabled, common.RDB = true, nil
			_, unavailableErr := helper.HandleGroupRatio(ctx, &relaycommon.RelayInfo{UsingGroup: "multiplier-a", UserQuota: 100})
			common.RedisEnabled, common.RDB = false, uninitializedRedis
			require.ErrorIs(t, unavailableErr, groupload.ErrUnavailable, "new concurrency-priced work must not use a fabricated load")
			_, version = model.GetGroupMultiplierPolicy("multiplier-a")
			policy.Mode = model.MultiplierFixed
			require.NoError(t, model.SaveGroupMultiplierPolicy("multiplier-a", policy, version))
			settled, charged, _ := service.TryTieredSettle(billingInfo, billingexpr.TokenParams{})
			require.True(t, settled)
			assert.Equal(t, 20000, charged, "settlement keeps admission pricing after a mode switch")
			billingInfo.GroupLoadLease.Close()
			retried, err := helper.HandleGroupRatio(ctx, info)
			require.NoError(t, err)
			assert.Equal(t, price, retried, "same-group retries keep their accepted policy and charge")
			fresh := &relaycommon.RelayInfo{UsingGroup: "multiplier-a", UserQuota: 100}
			fixed, err := helper.HandleGroupRatio(ctx, fresh)
			require.NoError(t, err)
			fresh.GroupLoadLease.Close()
			assert.Equal(t, 2.0, fixed.GroupRatio)
			preserved, version := model.GetGroupMultiplierPolicy("multiplier-a")
			assert.Equal(t, policy.Tiers, preserved.Tiers)
			preserved.Mode = model.MultiplierBalance
			require.NoError(t, model.SaveGroupMultiplierPolicy("multiplier-a", preserved, version))
			assert.Equal(t, 0.5, model.GetMatchedDynamicRatioMatch("multiplier-a", "model", 100).Ratio)
			var persistedRule model.DynamicRatioRule
			require.NoError(t, db.First(&persistedRule, oldRule.Id).Error)
			assert.Equal(t, oldRule.Ratio, persistedRule.Ratio)
			// Repeated startup reads preserve old rows and the new per-group policy.
			var option model.Option
			require.NoError(t, db.Scopes(model.WithOptionKey(model.GroupMultiplierPoliciesOption)).First(&option).Error)
			for range 2 {
				require.NoError(t, model.UpdateOptionsBulk(map[string]string{model.GroupMultiplierPoliciesOption: option.Value}))
				model.InitDynamicRatioCache()
			}
			restored, _ := model.GetGroupMultiplierPolicy("multiplier-a")
			assert.Equal(t, preserved, restored)
			// Switching groups moves the slot; returning uses the earlier price.
			ctx.Set("auto_group", "multiplier-b")
			other, err := helper.HandleGroupRatio(ctx, info)
			require.NoError(t, err)
			assert.Equal(t, 3.0, other.GroupRatio)
			count, err := groupload.Count("multiplier-a")
			require.NoError(t, err)
			assert.EqualValues(t, 7, count)
			ctx.Set("auto_group", "multiplier-a")
			retried, err = helper.HandleGroupRatio(ctx, info)
			require.NoError(t, err)
			assert.Equal(t, price, retried)
			info.GroupLoadLease.Close()
			// Task metadata survives the database codec and releases only at a terminal transition.
			lease, _, err := groupload.Acquire("multiplier-b")
			require.NoError(t, err)
			task := model.Task{TaskID: "multiplier-task", Group: "multiplier-b", Status: model.TaskStatusSubmitted, PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{GroupLoadSlot: lease.Slot, MultiplierSnapshot: price.MultiplierSnapshot}}}
			require.NoError(t, task.Insert())
			require.NoError(t, lease.Transfer(time.Minute))
			lease.Close()
			var fetched model.Task
			require.NoError(t, db.First(&fetched, task.ID).Error)
			assert.Equal(t, price.MultiplierSnapshot, fetched.PrivateData.BillingContext.MultiplierSnapshot)
			require.NoError(t, model.RestoreTaskGroupLoads())
			count, err = groupload.Count("multiplier-b")
			require.NoError(t, err)
			assert.EqualValues(t, 1, count)
			fetched.Status = model.TaskStatusSuccess
			won, err := fetched.UpdateWithStatus(model.TaskStatusSubmitted)
			require.NoError(t, err)
			require.True(t, won)
			_, err = groupload.Register(fetched.Group, fetched.GroupLoadSlot(), time.Minute)
			require.NoError(t, err)
			count, err = groupload.Count("multiplier-b")
			require.NoError(t, err)
			assert.Zero(t, count, "stale task recovery must not resurrect completed work")
		})
	}
}

func TestGroupMultiplierTierValidation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tiers []model.ConcurrencyTier
	}{
		{"empty", nil}, {"missing base", []model.ConcurrencyTier{{Minimum: 1, Multiplier: 1}}},
		{"duplicate threshold", []model.ConcurrencyTier{{Multiplier: 1}, {Multiplier: 2}}},
		{"decreasing price", []model.ConcurrencyTier{{Multiplier: 2}, {Minimum: 8, Multiplier: 1}}},
		{"negative", []model.ConcurrencyTier{{Multiplier: -1}}}, {"infinite", []model.ConcurrencyTier{{Multiplier: math.Inf(1)}}},
		{"nan", []model.ConcurrencyTier{{Multiplier: math.NaN()}}}, {"oversized", []model.ConcurrencyTier{{Multiplier: 1001}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Error(t, (model.GroupMultiplierPolicy{Mode: model.MultiplierConcurrency, Tiers: tc.tiers}).Validate())
		})
	}
	policy := model.GroupMultiplierPolicy{Mode: model.MultiplierConcurrency, Tiers: []model.ConcurrencyTier{{Multiplier: 1}, {Minimum: 8, Multiplier: 2}, {Minimum: 15, Multiplier: 3}}}
	require.NoError(t, policy.Validate())
	for _, tc := range []struct {
		count  int64
		factor float64
	}{{0, 1}, {7, 1}, {8, 2}, {14, 2}, {15, 3}, {100, 3}} {
		assert.Equal(t, tc.factor, policy.TierAt(tc.count).Multiplier)
	}
}

func TestGroupMultiplierRedisLeaseLifecycle(t *testing.T) {
	address := os.Getenv("TEST_REDIS_ADDR")
	if address == "" {
		t.Skip("TEST_REDIS_ADDR not configured")
	}
	client := redis.NewClient(&redis.Options{Addr: address, DB: 14})
	require.NoError(t, client.Ping(context.Background()).Err())
	previous, enabled := common.RDB, common.RedisEnabled
	common.RDB, common.RedisEnabled = client, true
	t.Cleanup(func() { common.RDB, common.RedisEnabled = previous, enabled; require.NoError(t, client.Close()) })
	group := "multiplier-test-" + common.GetUUID()
	first, count, err := groupload.Acquire(group)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
	second, count, err := groupload.Acquire(group)
	require.NoError(t, err)
	assert.EqualValues(t, 2, count)
	first.Close()
	first.Close()
	count, err = groupload.Count(group)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
	require.NoError(t, second.Transfer(time.Minute))
	second.Close()
	count, err = groupload.Count(group)
	require.NoError(t, err)
	assert.EqualValues(t, 1, count)
	require.NoError(t, groupload.Release(group, second.Slot))
	_, err = groupload.Register(group, second.Slot, time.Minute)
	require.NoError(t, err)
	count, err = groupload.Count(group)
	require.NoError(t, err)
	assert.Zero(t, count)
}

package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enablePaymentComplianceForTest(t *testing.T) {
	t.Helper()
	setting := operation_setting.GetPaymentSetting()
	prevConfirmed := setting.ComplianceConfirmed
	prevVersion := setting.ComplianceTermsVersion
	setting.ComplianceConfirmed = true
	setting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		setting.ComplianceConfirmed = prevConfirmed
		setting.ComplianceTermsVersion = prevVersion
	})
}

func seedGrantPlan(t *testing.T, plan *SubscriptionPlan) *SubscriptionPlan {
	t.Helper()
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func seedGrantUser(t *testing.T, id int, status int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "grant_user_" + strconv.Itoa(id),
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   status,
		Quota:    0,
		AffCode:  "g" + strconv.Itoa(id),
	}
	require.NoError(t, DB.Create(user).Error)
}

func countUserPlanSubs(t *testing.T, userId, planId int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).
		Where("user_id = ? AND plan_id = ?", userId, planId).
		Count(&count).Error)
	return count
}

func TestAdminGrantPlanToAllUsersGrantsAndSkips(t *testing.T) {
	truncateTables(t)
	InvalidateSubscriptionPlanCache(9701)

	plan := seedGrantPlan(t, &SubscriptionPlan{
		Id:            9701,
		Title:         "Trial",
		PriceAmount:   0,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationDay,
		DurationValue: 7,
		Enabled:       true,
		TotalAmount:   1000,
	})

	seedGrantUser(t, 501, common.UserStatusEnabled)
	seedGrantUser(t, 502, common.UserStatusEnabled)
	seedGrantUser(t, 503, common.UserStatusDisabled)
	// user 502 already has this plan → should skip
	require.NoError(t, DB.Create(&UserSubscription{
		UserId:      502,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		StartTime:   GetDBTimestamp(),
		EndTime:     GetDBTimestamp() + 3600,
		Status:      "active",
		Source:      "order",
	}).Error)

	result, err := AdminGrantPlanToAllUsers(plan.Id)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, plan.Id, result.PlanId)
	assert.Equal(t, 2, result.TotalUsers) // only enabled users
	assert.Equal(t, 1, result.GrantedCount)
	assert.Equal(t, 1, result.SkippedCount)
	assert.Zero(t, result.FailedCount)

	assert.EqualValues(t, 1, countUserPlanSubs(t, 501, plan.Id))
	assert.EqualValues(t, 1, countUserPlanSubs(t, 502, plan.Id))
	assert.EqualValues(t, 0, countUserPlanSubs(t, 503, plan.Id))

	var granted UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 501, plan.Id).First(&granted).Error)
	assert.Equal(t, "admin", granted.Source)
	assert.Equal(t, "active", granted.Status)
	assert.EqualValues(t, 1000, granted.AmountTotal)

	// second run is idempotent
	result2, err := AdminGrantPlanToAllUsers(plan.Id)
	require.NoError(t, err)
	assert.Equal(t, 2, result2.TotalUsers)
	assert.Zero(t, result2.GrantedCount)
	assert.Equal(t, 2, result2.SkippedCount)
}

func TestGrantAutoSubscriptionsToNewUser(t *testing.T) {
	truncateTables(t)
	enablePaymentComplianceForTest(t)
	InvalidateSubscriptionPlanCache(9801)
	InvalidateSubscriptionPlanCache(9802)

	autoPlan := seedGrantPlan(t, &SubscriptionPlan{
		Id:            9801,
		Title:         "New User Trial",
		PriceAmount:   0,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationDay,
		DurationValue: 3,
		Enabled:       true,
		AutoGrant:     true,
		TotalAmount:   500,
	})
	// disabled auto_grant plan should be ignored
	// GORM omits zero-value bools with default tags on Create; force enabled=false after insert.
	disabledAuto := seedGrantPlan(t, &SubscriptionPlan{
		Id:            9802,
		Title:         "Disabled Auto",
		PriceAmount:   0,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationDay,
		DurationValue: 3,
		Enabled:       true,
		AutoGrant:     true,
		TotalAmount:   500,
	})
	require.NoError(t, DB.Model(disabledAuto).Update("enabled", false).Error)
	// enabled but not auto_grant should be ignored
	seedGrantPlan(t, &SubscriptionPlan{
		Id:            9803,
		Title:         "Manual Only",
		PriceAmount:   0,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationDay,
		DurationValue: 3,
		Enabled:       true,
		AutoGrant:     false,
		TotalAmount:   500,
	})

	seedGrantUser(t, 601, common.UserStatusEnabled)

	require.NoError(t, GrantAutoSubscriptionsToNewUser(601))
	assert.EqualValues(t, 1, countUserPlanSubs(t, 601, autoPlan.Id))
	assert.EqualValues(t, 0, countUserPlanSubs(t, 601, 9802))
	assert.EqualValues(t, 0, countUserPlanSubs(t, 601, 9803))

	var sub UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 601, autoPlan.Id).First(&sub).Error)
	assert.Equal(t, "auto_grant", sub.Source)

	// second call skips existing
	require.NoError(t, GrantAutoSubscriptionsToNewUser(601))
	assert.EqualValues(t, 1, countUserPlanSubs(t, 601, autoPlan.Id))
}

func TestGrantAutoSubscriptionsToNewUserSkipsWithoutCompliance(t *testing.T) {
	truncateTables(t)
	setting := operation_setting.GetPaymentSetting()
	prevConfirmed := setting.ComplianceConfirmed
	prevVersion := setting.ComplianceTermsVersion
	setting.ComplianceConfirmed = false
	setting.ComplianceTermsVersion = ""
	t.Cleanup(func() {
		setting.ComplianceConfirmed = prevConfirmed
		setting.ComplianceTermsVersion = prevVersion
	})
	InvalidateSubscriptionPlanCache(9901)

	seedGrantPlan(t, &SubscriptionPlan{
		Id:            9901,
		Title:         "Need Compliance",
		PriceAmount:   0,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationDay,
		DurationValue: 1,
		Enabled:       true,
		AutoGrant:     true,
		TotalAmount:   100,
	})
	seedGrantUser(t, 701, common.UserStatusEnabled)

	require.NoError(t, GrantAutoSubscriptionsToNewUser(701))
	assert.EqualValues(t, 0, countUserPlanSubs(t, 701, 9901))
}

func TestBindSubscriptionIfAbsent(t *testing.T) {
	truncateTables(t)
	InvalidateSubscriptionPlanCache(9911)

	plan := seedGrantPlan(t, &SubscriptionPlan{
		Id:            9911,
		Title:         "Once",
		PriceAmount:   0,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationHour,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   10,
	})
	seedGrantUser(t, 801, common.UserStatusEnabled)

	granted, err := BindSubscriptionIfAbsent(801, plan, "admin")
	require.NoError(t, err)
	assert.True(t, granted)

	granted, err = BindSubscriptionIfAbsent(801, plan, "admin")
	require.NoError(t, err)
	assert.False(t, granted)
	assert.EqualValues(t, 1, countUserPlanSubs(t, 801, plan.Id))
}

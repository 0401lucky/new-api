package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminListUserSubscriptionsPageFiltersAndEnriches(t *testing.T) {
	truncateTables(t)
	InvalidateSubscriptionPlanCache(96001)

	planA := seedGrantPlan(t, &SubscriptionPlan{
		Id:            96001,
		Title:         "Trial A",
		PriceAmount:   0,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationDay,
		DurationValue: 7,
		Enabled:       true,
		TotalAmount:   1000,
	})
	planB := seedGrantPlan(t, &SubscriptionPlan{
		Id:            96002,
		Title:         "Pro B",
		PriceAmount:   10,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   5000,
	})
	seedGrantUser(t, 9101, common.UserStatusEnabled)
	seedGrantUser(t, 9102, common.UserStatusEnabled)

	now := GetDBTimestamp()
	require.NoError(t, DB.Create(&UserSubscription{
		UserId: 9101, PlanId: planA.Id, AmountTotal: 1000, AmountUsed: 200,
		StartTime: now - 100, EndTime: now + 86400, Status: "active", Source: "admin",
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		UserId: 9102, PlanId: planB.Id, AmountTotal: 5000, AmountUsed: 1000,
		StartTime: now - 100, EndTime: now + 86400, Status: "active", Source: "order",
	}).Error)
	require.NoError(t, DB.Create(&UserSubscription{
		UserId: 9101, PlanId: planB.Id, AmountTotal: 5000, AmountUsed: 5000,
		StartTime: now - 10000, EndTime: now - 10, Status: "active", Source: "admin",
	}).Error)

	// all
	items, total, err := AdminListUserSubscriptionsPage(AdminUserSubscriptionQuery{}, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Len(t, items, 3)

	// plan filter
	items, total, err = AdminListUserSubscriptionsPage(AdminUserSubscriptionQuery{PlanId: planA.Id}, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, "Trial A", items[0].PlanTitle)
	assert.Equal(t, "grant_user_9101", items[0].Username)
	assert.EqualValues(t, 200, items[0].Subscription.AmountUsed)

	// active only (excludes end_time past)
	items, total, err = AdminListUserSubscriptionsPage(AdminUserSubscriptionQuery{Status: "active"}, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Len(t, items, 2)

	// keyword username
	items, total, err = AdminListUserSubscriptionsPage(AdminUserSubscriptionQuery{Keyword: "grant_user_9102"}, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	assert.Equal(t, planB.Id, items[0].Subscription.PlanId)

	// keyword plan title
	items, total, err = AdminListUserSubscriptionsPage(AdminUserSubscriptionQuery{Keyword: "Trial"}, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	assert.Equal(t, planA.Id, items[0].Subscription.PlanId)

	// sort by amount_used desc: 5000, 1000, 200
	items, total, err = AdminListUserSubscriptionsPage(AdminUserSubscriptionQuery{
		OrderBy: "amount_used",
		Order:   "desc",
	}, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	require.Len(t, items, 3)
	assert.EqualValues(t, 5000, items[0].Subscription.AmountUsed)
	assert.EqualValues(t, 1000, items[1].Subscription.AmountUsed)
	assert.EqualValues(t, 200, items[2].Subscription.AmountUsed)

	// sort by amount_used asc
	items, _, err = AdminListUserSubscriptionsPage(AdminUserSubscriptionQuery{
		OrderBy: "amount_used",
		Order:   "asc",
	}, 0, 20)
	require.NoError(t, err)
	require.Len(t, items, 3)
	assert.EqualValues(t, 200, items[0].Subscription.AmountUsed)
	assert.EqualValues(t, 5000, items[2].Subscription.AmountUsed)
}

package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUserForPaymentGuardTest(t *testing.T, id int, quota int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "payment_guard_user",
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)
}

func insertSubscriptionPlanForPaymentGuardTest(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         "Guard Plan",
		PriceAmount:   9.99,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertSubscriptionOrderForPaymentGuardTest(t *testing.T, tradeNo string, userID int, planID int, paymentProvider string) {
	t.Helper()
	order := &SubscriptionOrder{
		UserId:          userID,
		PlanId:          planID,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, order.Insert())
}

func insertTopUpForPaymentGuardTest(t *testing.T, tradeNo string, userID int, paymentProvider string) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func insertEpayTopUpForPaymentGuardTest(t *testing.T, tradeNo string, userID int, amount int64, money float64) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          amount,
		Money:           money,
		TradeNo:         tradeNo,
		PaymentMethod:   "alipay",
		PaymentProvider: PaymentProviderEpay,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func getTopUpStatusForPaymentGuardTest(t *testing.T, tradeNo string) string {
	t.Helper()
	topUp := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	return topUp.Status
}

func countUserSubscriptionsForPaymentGuardTest(t *testing.T, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

func getUserQuotaForPaymentGuardTest(t *testing.T, userID int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func TestRechargeWaffoPancake_RejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 101, 0)
	insertTopUpForPaymentGuardTest(t, "waffo-pancake-guard", 101, PaymentProviderStripe)

	err := RechargeWaffoPancake("waffo-pancake-guard")
	require.Error(t, err)

	topUp := GetTopUpByTradeNo("waffo-pancake-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 101))
}

func TestUpdatePendingTopUpStatus_RejectsMismatchedPaymentProvider(t *testing.T) {
	testCases := []struct {
		name                    string
		tradeNo                 string
		storedPaymentProvider   string
		expectedPaymentProvider string
		targetStatus            string
	}{
		{
			name:                    "stripe expire",
			tradeNo:                 "stripe-expire-guard",
			storedPaymentProvider:   PaymentProviderCreem,
			expectedPaymentProvider: PaymentProviderStripe,
			targetStatus:            common.TopUpStatusExpired,
		},
		{
			name:                    "waffo failed",
			tradeNo:                 "waffo-failed-guard",
			storedPaymentProvider:   PaymentProviderStripe,
			expectedPaymentProvider: PaymentProviderWaffo,
			targetStatus:            common.TopUpStatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 150, 0)
			insertTopUpForPaymentGuardTest(t, tc.tradeNo, 150, tc.storedPaymentProvider)

			err := UpdatePendingTopUpStatus(tc.tradeNo, tc.expectedPaymentProvider, tc.targetStatus)
			require.ErrorIs(t, err, ErrPaymentMethodMismatch)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tc.tradeNo))
		})
	}
}

func TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 202, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 301)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-guard-order", 202, plan.Id, PaymentProviderStripe)

	err := CompleteSubscriptionOrder("sub-guard-order", `{"provider":"epay"}`, PaymentProviderEpay, "alipay")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-guard-order")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, 202))

	topUp := GetTopUpByTradeNo("sub-guard-order")
	assert.Nil(t, topUp)
}

func TestExpireSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 303, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 401)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-expire-guard", 303, plan.Id, PaymentProviderStripe)

	err := ExpireSubscriptionOrder("sub-expire-guard", PaymentProviderCreem)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-expire-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}

func TestCompleteEpayTopUp_CompletesAndAddsQuota(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 501, 100)
	insertEpayTopUpForPaymentGuardTest(t, "epay-complete-guard", 501, 2, 2.00)

	err := CompleteEpayTopUp("epay-complete-guard", CompleteEpayTopUpOptions{
		CallerIP:              "127.0.0.1",
		ActualPaymentMethod:   "alipay",
		CallbackPaymentMethod: PaymentProviderEpay,
		ProviderTradeNo:       "epay-provider-001",
		ProviderMoney:         "2.00",
		LogPrefix:             "使用在线充值成功",
	})
	require.NoError(t, err)

	topUp := GetTopUpByTradeNo("epay-complete-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	assert.Equal(t, "alipay", topUp.PaymentMethod)
	assert.NotZero(t, topUp.CompleteTime)
	assert.Equal(t, 100+int(2*common.QuotaPerUnit), getUserQuotaForPaymentGuardTest(t, 501))
}

func TestCompleteEpayTopUp_IsIdempotentAfterSuccess(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 502, 0)
	insertEpayTopUpForPaymentGuardTest(t, "epay-idempotent-guard", 502, 2, 2.00)

	err := CompleteEpayTopUp("epay-idempotent-guard", CompleteEpayTopUpOptions{
		CallerIP:              "127.0.0.1",
		ActualPaymentMethod:   "wechat",
		CallbackPaymentMethod: PaymentProviderEpay,
		ProviderTradeNo:       "epay-provider-002",
		ProviderMoney:         "2.00",
		LogPrefix:             "使用在线充值成功",
	})
	require.NoError(t, err)

	err = CompleteEpayTopUp("epay-idempotent-guard", CompleteEpayTopUpOptions{
		CallerIP:              "127.0.0.1",
		ActualPaymentMethod:   "wechat",
		CallbackPaymentMethod: PaymentProviderEpay,
		ProviderTradeNo:       "epay-provider-002",
		ProviderMoney:         "2.00",
		LogPrefix:             "使用在线充值成功",
	})
	require.NoError(t, err)

	assert.Equal(t, 2*int(common.QuotaPerUnit), getUserQuotaForPaymentGuardTest(t, 502))
}

func TestCompleteEpayTopUp_RejectsMoneyMismatch(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 503, 0)
	insertEpayTopUpForPaymentGuardTest(t, "epay-money-guard", 503, 2, 2.00)

	err := CompleteEpayTopUp("epay-money-guard", CompleteEpayTopUpOptions{
		CallerIP:              "127.0.0.1",
		ActualPaymentMethod:   "alipay",
		CallbackPaymentMethod: PaymentProviderEpay,
		ProviderTradeNo:       "epay-provider-003",
		ProviderMoney:         "9.99",
		LogPrefix:             "使用在线充值成功",
	})
	require.Error(t, err)

	topUp := GetTopUpByTradeNo("epay-money-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 503))
}

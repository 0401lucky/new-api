package service

import (
	"time"

	"github.com/QuantumNous/new-api/model"
)

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包 or 订阅）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet" 或 "subscription"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还）
	Settle(delta int) error
	// Refund 退还所有预扣费
	Refund() error
}

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现
// ---------------------------------------------------------------------------

type WalletFunding struct {
	userId   int
	consumed int // 实际预扣的用户额度

	// 限时额度资金拆分（累计）：记录本次消费中限时/永久额度的实际扣费拆分，
	// 用于结算补扣、退款和消费日志审计。Allocations 按额度桶记录，
	// 退款时逐桶恢复，避免跨午夜复活已过期额度。
	tempConsumed int                       // 累计限时额度扣除量
	permConsumed int                       // 累计永久额度扣除量
	allocations  []model.TemporaryAllocation // 累计限时额度桶分配
}

func (w *WalletFunding) Source() string { return BillingSourceWallet }

// split 返回当前累计资金拆分（供退款与日志使用）
func (w *WalletFunding) split() *model.WalletSplit {
	return &model.WalletSplit{
		Temporary:   w.tempConsumed,
		Permanent:   w.permConsumed,
		Allocations: w.allocations,
	}
}

// checkinId 返回最后一个限时额度桶的签到记录 ID（日志展示用），无则 0
func (w *WalletFunding) checkinId() int {
	if n := len(w.allocations); n > 0 {
		return w.allocations[n-1].CheckinId
	}
	return 0
}

// tempExpiresAt 返回最后一个限时额度桶的失效时间（日志展示用），无则 0
func (w *WalletFunding) tempExpiresAt() int64 {
	if n := len(w.allocations); n > 0 {
		return w.allocations[n-1].ExpiresAt
	}
	return 0
}

func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	split, err := model.PreConsumeWallet(w.userId, amount)
	if err != nil {
		return err
	}
	w.consumed = amount
	w.tempConsumed = split.Temporary
	w.permConsumed = split.Permanent
	w.allocations = split.Allocations
	return nil
}

func (w *WalletFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		// 补扣：继续限时优先、永久补足（允许欠费）
		split, err := model.TopUpWallet(w.userId, delta)
		if err != nil {
			return err
		}
		w.consumed += delta
		w.tempConsumed += split.Temporary
		w.permConsumed += split.Permanent
		w.allocations = append(w.allocations, split.Allocations...)
		return nil
	}
	// 退还：优先退永久额度，再退限时额度（过期不恢复）
	return w.refund(-delta)
}

func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 {
		return nil
	}
	return w.refund(w.consumed)
}

// refund 退还 amount：优先退永久额度，其次退限时额度（仅未过期可恢复）。
// 退款本身在数据库事务内完成，可安全重试；过期限时部分直接丢弃。
// 退款后同步累计拆分与额度桶列表，保证后续退款不会重复退已退部分。
func (w *WalletFunding) refund(amount int) error {
	result, err := model.RefundWallet(w.userId, amount, w.split())
	if err != nil {
		return err
	}
	w.tempConsumed -= result.Temporary
	w.permConsumed -= result.Permanent
	w.consumed -= result.Temporary + result.Permanent
	w.allocations = removeRefundedAllocations(w.allocations, result.Allocations)
	return nil
}

// removeRefundedAllocations 从累计额度桶列表中移除本次退款已恢复的桶。
// RefundWallet 逆序退还（后扣的先退），此处也从尾部匹配移除。
func removeRefundedAllocations(all, removed []model.TemporaryAllocation) []model.TemporaryAllocation {
	if len(removed) == 0 || len(all) == 0 {
		return all
	}
	result := all
	for _, rm := range removed {
		for i := len(result) - 1; i >= 0; i-- {
			if result[i].CheckinId == rm.CheckinId && result[i].Amount == rm.Amount {
				result = append(result[:i], result[i+1:]...)
				break
			}
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// SubscriptionFunding — 订阅资金来源实现
// ---------------------------------------------------------------------------

type SubscriptionFunding struct {
	requestId      string
	userId         int
	modelName      string
	amount         int64 // 预扣的订阅额度（subConsume）
	subscriptionId int
	preConsumed    int64
	// 以下字段在 PreConsume 成功后填充，供 RelayInfo 同步使用
	AmountTotal     int64
	AmountUsedAfter int64
	PlanId          int
	PlanTitle       string
}

func (s *SubscriptionFunding) Source() string { return BillingSourceSubscription }

func (s *SubscriptionFunding) PreConsume(_ int) error {
	// amount 参数被忽略，使用内部 s.amount（已在构造时根据 preConsumedQuota 计算）
	res, err := model.PreConsumeUserSubscription(s.requestId, s.userId, s.modelName, 0, s.amount)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	// 获取订阅计划信息
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		s.PlanId = planInfo.PlanId
		s.PlanTitle = planInfo.PlanTitle
	}
	return nil
}

func (s *SubscriptionFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return model.PostConsumeUserSubscriptionDelta(s.subscriptionId, int64(delta))
}

func (s *SubscriptionFunding) Refund() error {
	if s.preConsumed <= 0 {
		return nil
	}
	return refundWithRetry(func() error {
		return model.RefundSubscriptionPreConsume(s.requestId)
	})
}

// refundWithRetry 尝试多次执行退款操作以提高成功率，只能用于基于事务的退款函数！！！！！！
// try to refund with retries, only for refund functions based on transactions!!!
func refundWithRetry(fn func() error) error {
	if fn == nil {
		return nil
	}
	const maxAttempts = 3
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
	}
	return lastErr
}

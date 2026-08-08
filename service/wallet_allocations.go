package service

import (
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// 限时额度桶分配在 model 与 relay/common DTO 之间的转换。
// model.TemporaryAllocation 用于持久化与扣费原语；relay 侧 DTO 避免 relay/common 依赖 model。

func modelAllocationsToRelay(allocs []model.TemporaryAllocation) []relaycommon.TemporaryQuotaAllocation {
	if len(allocs) == 0 {
		return nil
	}
	out := make([]relaycommon.TemporaryQuotaAllocation, 0, len(allocs))
	for _, a := range allocs {
		out = append(out, relaycommon.TemporaryQuotaAllocation{
			CheckinId: a.CheckinId,
			ExpiresAt: a.ExpiresAt,
			Amount:    a.Amount,
		})
	}
	return out
}

func relayAllocationsToModel(allocs []relaycommon.TemporaryQuotaAllocation) []model.TemporaryAllocation {
	if len(allocs) == 0 {
		return nil
	}
	out := make([]model.TemporaryAllocation, 0, len(allocs))
	for _, a := range allocs {
		out = append(out, model.TemporaryAllocation{
			CheckinId: a.CheckinId,
			ExpiresAt: a.ExpiresAt,
			Amount:    a.Amount,
		})
	}
	return out
}

// removeRelayAllocations 从 relay 侧累计额度桶列表中移除本次退款已恢复的桶（逆序匹配）。
func removeRelayAllocations(all []relaycommon.TemporaryQuotaAllocation, removed []model.TemporaryAllocation) []relaycommon.TemporaryQuotaAllocation {
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

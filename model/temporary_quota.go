package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

// 签到限时额度类型常量（与 setting/operation_setting 保持一致，避免循环依赖）
const (
	CheckinQuotaTypePermanent = "permanent"
	CheckinQuotaTypeTemporary = "temporary"
)

// ErrInsufficientWalletBalance 钱包总可用额度（限时+永久）不足以完成预扣。
// 由 service 层转换为 403 InsufficientUserQuota 响应。
var ErrInsufficientWalletBalance = errors.New("钱包额度不足")

// TemporaryAllocation 一次钱包扣费中单个限时额度桶的分配。
// 按额度桶拆分保存，退款时逐桶恢复，避免把已过期额度错误恢复到其他桶。
type TemporaryAllocation struct {
	CheckinId int   `json:"checkin_id"`
	ExpiresAt int64 `json:"expires_at"`
	Amount    int   `json:"amount"`
}

// WalletSplit 一次钱包扣费/退款操作的资金拆分。
// 扣费时 Temporary/Permanent 为正数；退款时通过 RefundWallet 返回的
// Temporary/Permanent 表示实际退还的数量（均为正数），
// Allocations 表示实际恢复的限时额度桶。
type WalletSplit struct {
	Temporary   int                   // 限时额度变动量（扣为正，退为正）
	Permanent   int                   // 永久额度变动量（扣为正，退为正）
	Allocations []TemporaryAllocation // 按额度桶拆分（限时部分）
}

// Total 返回拆分总额度
func (s *WalletSplit) Total() int {
	return s.Temporary + s.Permanent
}

// LastCheckinId 返回最后一个限时额度桶的签到记录 ID（用于日志展示），无则返回 0。
func (s *WalletSplit) LastCheckinId() int {
	if n := len(s.Allocations); n > 0 {
		return s.Allocations[n-1].CheckinId
	}
	return 0
}

// LastExpiresAt 返回最后一个限时额度桶的失效时间（用于日志展示），无则返回 0。
func (s *WalletSplit) LastExpiresAt() int64 {
	if n := len(s.Allocations); n > 0 {
		return s.Allocations[n-1].ExpiresAt
	}
	return 0
}

// GetActiveTemporaryQuota 返回当前仍有效的限时额度总和。
// 已过期记录按零处理（通过 quota_expires_at 判断，不依赖定时清理任务）。
func GetActiveTemporaryQuota(userId int) (int, error) {
	now := common.NowInStartupTimezone().Unix()
	var total int64
	err := DB.Model(&Checkin{}).
		Where("user_id = ? AND quota_type = ? AND quota_remaining > 0 AND quota_expires_at > ?",
			userId, CheckinQuotaTypeTemporary, now).
		Select("COALESCE(SUM(quota_remaining), 0)").
		Scan(&total).Error
	return int(total), err
}

// HasActiveTemporaryQuota 判断用户是否存在有效限时额度。
func HasActiveTemporaryQuota(userId int) (bool, error) {
	now := common.NowInStartupTimezone().Unix()
	var count int64
	err := DB.Model(&Checkin{}).
		Where("user_id = ? AND quota_type = ? AND quota_remaining > 0 AND quota_expires_at > ?",
			userId, CheckinQuotaTypeTemporary, now).
		Count(&count).Error
	return count > 0, err
}

// GetActiveTemporaryQuotaExpiresAt 返回当前有效限时额度的失效时间（Unix 秒），无有效额度时返回 0。
func GetActiveTemporaryQuotaExpiresAt(userId int) int64 {
	now := common.NowInStartupTimezone().Unix()
	var expiresAt int64
	_ = DB.Model(&Checkin{}).
		Where("user_id = ? AND quota_type = ? AND quota_remaining > 0 AND quota_expires_at > ?",
			userId, CheckinQuotaTypeTemporary, now).
		Select("COALESCE(MAX(quota_expires_at), 0)").
		Scan(&expiresAt)
	return expiresAt
}

// GetWalletAvailableQuota 返回钱包总可用额度（永久额度 + 有效限时额度）。
// 永久额度走既有缓存路径，限时额度实时查询数据库。
func GetWalletAvailableQuota(userId int) (int, error) {
	permanent, err := GetUserQuota(userId, false)
	if err != nil {
		return 0, err
	}
	temporary, err := GetActiveTemporaryQuota(userId)
	if err != nil {
		return 0, err
	}
	return permanent + temporary, nil
}

// activeTemporaryCheckinsTx 在事务内查询并锁定用户所有有效限时签到记录。
// 按失效时间与 ID 排序，保证并发扣费确定性。
func activeTemporaryCheckinsTx(tx *gorm.DB, userId int) ([]Checkin, error) {
	now := common.NowInStartupTimezone().Unix()
	var records []Checkin
	err := lockForUpdate(tx).
		Where("user_id = ? AND quota_type = ? AND quota_remaining > 0 AND quota_expires_at > ?",
			userId, CheckinQuotaTypeTemporary, now).
		Order("quota_expires_at ASC, id ASC").
		Find(&records).Error
	return records, err
}

func decreaseUserQuotaTx(tx *gorm.DB, id int, quota int) error {
	if quota <= 0 {
		return nil
	}
	result := tx.Model(&User{}).Where("id = ?", id).
		Update("quota", gorm.Expr("quota - ?", quota))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func increaseUserQuotaTx(tx *gorm.DB, id int, quota int) error {
	if quota <= 0 {
		return nil
	}
	result := tx.Model(&User{}).Where("id = ?", id).
		Update("quota", gorm.Expr("quota + ?", quota))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// lockUserQuotaTx 在事务内锁定用户行并返回当前永久额度。
// 配合限时额度记录锁定，保证“余额判断 + 限时扣减 + 永久扣减”在同一事务内，
// 并发预扣不会把永久额度扣成负数。
func lockUserQuotaTx(tx *gorm.DB, userId int) (int, error) {
	var quota int
	result := lockForUpdate(tx).Model(&User{}).
		Where("id = ?", userId).
		Select("quota").
		Find(&quota)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	return quota, nil
}

// PreConsumeWallet 从钱包预扣 amount，优先使用有效限时额度，不足部分补扣永久额度。
// 预扣保证不会产生负余额：无有效限时额度时用原子条件扣减，
// 有限时额度时在同一事务内锁定额度桶与用户行并校验余额。
// 返回资金拆分；余额不足返回 ErrInsufficientWalletBalance。
func PreConsumeWallet(userId, amount int) (*WalletSplit, error) {
	if amount <= 0 {
		return &WalletSplit{}, nil
	}

	hasTemp, err := HasActiveTemporaryQuota(userId)
	if err != nil {
		return nil, err
	}
	if !hasTemp {
		// 快速路径：单条原子条件扣减，余额不足不扣、不产生负余额
		return consumePermanentAtomic(userId, amount)
	}

	return consumeWalletTx(userId, amount, true)
}

// TopUpWallet 结算补扣 amount（正数），继续限时优先、永久补足。
// 结算补扣发生在消费之后，余额不足的部分记为欠费（余额可为负），不中断请求。
func TopUpWallet(userId, amount int) (*WalletSplit, error) {
	if amount <= 0 {
		return &WalletSplit{}, nil
	}
	hasTemp, err := HasActiveTemporaryQuota(userId)
	if err != nil {
		return nil, err
	}
	if !hasTemp {
		// 快速路径：保持原有永久额度消费路径（允许欠费）
		if err := DecreaseUserQuota(userId, amount, false); err != nil {
			return nil, err
		}
		return &WalletSplit{Permanent: amount}, nil
	}
	return consumeWalletTx(userId, amount, false)
}

// consumePermanentAtomic 无有效限时额度时的预扣：原子条件扣减永久额度。
// 保证预扣不足时返回 ErrInsufficientWalletBalance，不会产生负余额。
func consumePermanentAtomic(userId, amount int) (*WalletSplit, error) {
	result := DB.Model(&User{}).
		Where("id = ? AND quota >= ?", userId, amount).
		Update("quota", gorm.Expr("quota - ?", amount))
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, ErrInsufficientWalletBalance
	}
	gopool.Go(func() {
		if err := cacheDecrUserQuota(userId, int64(amount)); err != nil {
			common.SysLog("failed to sync user quota cache after atomic pre-consume: " + err.Error())
		}
	})
	return &WalletSplit{Permanent: amount}, nil
}

// consumeWalletTx 在事务内完成限时优先、永久补足的扣费。
// checkBalance 为 true 时（预扣）校验钱包总可用额度，不足返回 ErrInsufficientWalletBalance；
// false 时（结算补扣）允许永久余额欠费。
func consumeWalletTx(userId, amount int, checkBalance bool) (*WalletSplit, error) {
	split := &WalletSplit{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 1) 锁定有效限时额度桶
		records, err := activeTemporaryCheckinsTx(tx, userId)
		if err != nil {
			return err
		}
		// 2) 锁定用户行获取永久额度
		permQuota, err := lockUserQuotaTx(tx, userId)
		if err != nil {
			return err
		}
		// 3) 预扣时校验钱包总可用额度
		if checkBalance {
			tempAvailable := 0
			for i := range records {
				tempAvailable += records[i].QuotaRemaining
			}
			if tempAvailable+permQuota < amount {
				return ErrInsufficientWalletBalance
			}
		}
		// 4) 扣减限时额度桶（记录每桶分配）
		remaining := amount
		for i := range records {
			if remaining <= 0 {
				break
			}
			r := &records[i]
			deduct := r.QuotaRemaining
			if deduct > remaining {
				deduct = remaining
			}
			if err := tx.Model(&Checkin{}).Where("id = ?", r.Id).
				Update("quota_remaining", gorm.Expr("quota_remaining - ?", deduct)).Error; err != nil {
				return err
			}
			split.Temporary += deduct
			split.Allocations = append(split.Allocations, TemporaryAllocation{
				CheckinId: r.Id,
				ExpiresAt: r.QuotaExpiresAt,
				Amount:    deduct,
			})
			remaining -= deduct
		}
		// 5) 扣减永久额度
		if remaining > 0 {
			split.Permanent = remaining
			if err := decreaseUserQuotaTx(tx, userId, remaining); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// 永久额度部分直写数据库（绕过批量更新），提交后手动同步缓存
	if split.Permanent > 0 {
		gopool.Go(func() {
			if err := cacheDecrUserQuota(userId, int64(split.Permanent)); err != nil {
				common.SysLog("failed to sync user quota cache after wallet temp pre-consume: " + err.Error())
			}
		})
	}
	return split, nil
}

// RefundWallet 退还 amount，优先退永久额度，其次退限时额度。
// 限时部分按额度桶逆序（后扣的先退）逐桶恢复，仅在未过期时可恢复；
// 已过期的桶直接丢弃（不转为永久余额，也不复活）。
// split 为本请求累计扣费拆分（Temporary/Permanent 均为正数，Allocations 为逐桶分配）。
// 返回实际退还拆分（正数表示退还量；Allocations 为实际恢复的桶）。
func RefundWallet(userId, amount int, split *WalletSplit) (*WalletSplit, error) {
	if amount <= 0 {
		return &WalletSplit{}, nil
	}
	// 优先退永久额度，其次退限时额度。
	// 无拆分信息（旧数据/未使用限时额度）时全额按永久额度退还。
	refundPerm := amount
	refundTemp := 0
	if split != nil && split.Temporary+split.Permanent > 0 {
		if refundPerm > split.Permanent {
			refundPerm = split.Permanent
		}
		refundTemp = amount - refundPerm
		if refundTemp > split.Temporary {
			refundTemp = split.Temporary
		}
	}

	result := &WalletSplit{Permanent: refundPerm}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if refundPerm > 0 {
			if err := increaseUserQuotaTx(tx, userId, refundPerm); err != nil {
				return err
			}
		}
		if refundTemp > 0 && split != nil && len(split.Allocations) > 0 {
			now := common.NowInStartupTimezone().Unix()
			remaining := refundTemp
			// 逆序遍历（后扣的先退），未过期桶可恢复，过期桶直接丢弃
			for i := len(split.Allocations) - 1; i >= 0 && remaining > 0; i-- {
				alloc := split.Allocations[i]
				restore := alloc.Amount
				if restore > remaining {
					restore = remaining
				}
				if alloc.ExpiresAt > now {
					res := lockForUpdate(tx).Model(&Checkin{}).
						Where("id = ? AND quota_type = ? AND quota_expires_at > ?",
							alloc.CheckinId, CheckinQuotaTypeTemporary, now).
						Update("quota_remaining", gorm.Expr("quota_remaining + ?", restore))
					if res.Error != nil {
						return res.Error
					}
					if res.RowsAffected > 0 {
						result.Temporary += restore
						result.Allocations = append(result.Allocations, TemporaryAllocation{
							CheckinId: alloc.CheckinId,
							ExpiresAt: alloc.ExpiresAt,
							Amount:    restore,
						})
					}
					// 桶记录已失效（如被清理）时静默不恢复，与过期同等处理
				}
				remaining -= restore
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result.Permanent > 0 {
		gopool.Go(func() {
			if err := cacheIncrUserQuota(userId, int64(result.Permanent)); err != nil {
				common.SysLog("failed to sync user quota cache after wallet temp refund: " + err.Error())
			}
		})
	}
	return result, nil
}

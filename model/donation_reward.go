package model

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DonationReceiptItem struct {
	ItemID       string  `json:"item_id"`
	State        string  `json:"state"`
	ReasonCode   string  `json:"reason_code"`
	CredentialID *uint64 `json:"credential_id"`
	AcceptedAtMS *int64  `json:"accepted_at_ms"`
	Retryable    bool    `json:"retryable"`
}

type DonationReceipt struct {
	// Filled by the service only after authenticating the configured endpoint's
	// capabilities. The remote payload cannot override the pinned identity.
	InstanceID     string                `json:"-"`
	SourceID       string                `json:"-"`
	BatchID        string                `json:"batch_id"`
	GroupID        uint64                `json:"group_id"`
	TargetRevision string                `json:"target_revision"`
	CreatedAtMS    int64                 `json:"created_at_ms"`
	State          string                `json:"state"`
	Items          []DonationReceiptItem `json:"items"`
}

// DonationReason projects a closed vocabulary: remote error text is never
// copied into business rows, API responses, or logs where it could contain keys.
func DonationReason(reason string) string {
	switch reason {
	case "", "invalid_format", "duplicate_item", "already_exists", "staging_expired", "retry_exhausted", "resource_busy", "runtime_pending",
		"target_changed", "group_disabled", "unsupported_input", "target_unavailable", "probe_unavailable", "credential_override", "group_deleted",
		"invalid_credential", "model_unavailable", "rate_limited", "timeout", "upstream_error", "probe_incompatible", "unknown":
		return reason
	default:
		return "unknown"
	}
}

func (s *DonationStore) ApplyReceipt(ctx context.Context, receipt DonationReceipt) error {
	if !ValidDonationID(receipt.BatchID) || len(receipt.Items) == 0 || len(receipt.Items) > DonationMaxItems || receipt.CreatedAtMS <= 0 {
		return ErrDonationReceipt
	}
	if receipt.State != "processing" && receipt.State != "completed" {
		return ErrDonationReceipt
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := s.verifySecret(tx); err != nil {
			return err
		}
		var batch DonationBatch
		if err := tx.Where("id = ?", receipt.BatchID).First(&batch).Error; err != nil {
			return err
		}
		if batch.InstanceID != receipt.InstanceID || batch.SourceID != receipt.SourceID || batch.GroupID != receipt.GroupID || batch.TargetRevision != receipt.TargetRevision || batch.SecretID != s.secretID {
			return ErrDonationReceipt
		}
		var items []DonationItem
		if err := tx.Where("batch_id = ? AND dispatch = ?", batch.ID, true).Find(&items).Error; err != nil {
			return err
		}
		if len(items) != len(receipt.Items) {
			return ErrDonationReceipt
		}
		incoming := make(map[string]DonationReceiptItem, len(receipt.Items))
		for _, item := range receipt.Items {
			if _, exists := incoming[item.ItemID]; exists || !ValidDonationID(item.ItemID) {
				return ErrDonationReceipt
			}
			switch item.State {
			case "accepted":
				if item.CredentialID == nil || *item.CredentialID == 0 || *item.CredentialID > uint64(common.MaxWalletQuota) || item.AcceptedAtMS == nil || *item.AcceptedAtMS <= 0 || item.Retryable {
					return ErrDonationReceipt
				}
			case "queued", "validating", "committing", "retry_pending", "invalid", "existing":
				if item.CredentialID != nil || item.AcceptedAtMS != nil {
					return ErrDonationReceipt
				}
			default:
				return ErrDonationReceipt
			}
			incoming[item.ItemID] = item
		}
		slices.SortFunc(items, func(a, b DonationItem) int { return strings.Compare(a.Fingerprint, b.Fingerprint) })
		for _, before := range items {
			remote, exists := incoming[before.ID]
			if !exists {
				return ErrDonationReceipt
			}
			var resource DonationResource
			if err := lockForUpdate(tx).Where("fingerprint = ?", before.Fingerprint).First(&resource).Error; err != nil {
				return err
			}
			var item DonationItem
			if err := lockForUpdate(tx).Where("id = ?", before.ID).First(&item).Error; err != nil {
				return err
			}
			if item.State == "accepted" {
				if remote.State == "accepted" && (item.CredentialID == nil || item.AcceptedAtMS == nil || *remote.CredentialID != *item.CredentialID || *remote.AcceptedAtMS != *item.AcceptedAtMS) {
					return ErrDonationReceipt
				}
				continue // Stale queued/failed observations cannot revoke an acceptance.
			}
			if item.State == "invalid" || item.State == "existing" {
				if remote.State != item.State {
					return ErrDonationReceipt
				}
				continue
			}
			if resource.OwnerItemID != item.ID || resource.Generation != item.Generation || resource.SecretID != s.secretID {
				return ErrDonationReceipt
			}
			now := time.Now().UnixMilli()
			reason := DonationReason(remote.ReasonCode)
			if item.State != remote.State || item.ReasonCode != reason {
				if err := tx.Create(&DonationEvent{ItemID: item.ID, State: remote.State, ReasonCode: reason, CreatedAtMS: now}).Error; err != nil {
					return err
				}
			}
			item.State, item.ReasonCode, item.Retryable, item.UpdatedAtMS = remote.State, reason, remote.Retryable, now
			switch remote.State {
			case "accepted":
				item.CredentialID, item.AcceptedAtMS, item.RewardState = remote.CredentialID, remote.AcceptedAtMS, "pending"
				resource.Acquired, resource.AcceptedItemID = true, item.ID
			case "existing":
				resource.Acquired = true // Inventory is also permanently ineligible.
			case "invalid":
				// Only this terminal, authenticated receipt proves non-acceptance.
				// A timeout, lease expiry, or a 404 never reaches this release path.
				resource.OwnerItemID = ""
			}
			resource.UpdatedAtMS = now
			if err := tx.Save(&resource).Error; err != nil {
				return err
			}
			if err := tx.Save(&item).Error; err != nil {
				return err
			}
		}
		return tx.Model(&DonationBatch{}).Where("id = ?", batch.ID).Updates(map[string]any{
			"reception_state": "confirmed", "last_error": "", "updated_at_ms": time.Now().UnixMilli()}).Error
	})
	return err
}

// CreditDonationReward is the sole award boundary. No request can supply a
// recipient or amount: both come from the immutable batch snapshot.
func (s *DonationStore) CreditDonationReward(ctx context.Context, itemID string) (bool, error) {
	var credited bool
	var reward DonationReward
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		credited = false
		if err := s.verifySecret(tx); err != nil {
			return err
		}
		var reference DonationItem
		if err := tx.Where("id = ?", itemID).First(&reference).Error; err != nil {
			return err
		}
		var resource DonationResource
		if err := lockForUpdate(tx).Where("fingerprint = ?", reference.Fingerprint).First(&resource).Error; err != nil {
			return err
		}
		var item DonationItem
		if err := lockForUpdate(tx).Where("id = ?", itemID).First(&item).Error; err != nil {
			return err
		}
		var batch DonationBatch
		if err := tx.Where("id = ?", item.BatchID).First(&batch).Error; err != nil {
			return err
		}
		if item.State != "accepted" || !item.Dispatch || item.CredentialID == nil || *item.CredentialID == 0 || item.AcceptedAtMS == nil || *item.AcceptedAtMS <= 0 || !resource.Acquired || resource.AcceptedItemID != item.ID || resource.OwnerItemID != item.ID || resource.Generation != item.Generation || batch.SecretID != s.secretID {
			return ErrDonationReceipt
		}
		var existing DonationReward
		// This must be a current read on MySQL REPEATABLE READ: a waiter may
		// have established its snapshot before the winner committed the ledger.
		err := lockForUpdate(tx).Where("fingerprint = ? OR item_id = ?", item.Fingerprint, item.ID).First(&existing).Error
		if err == nil {
			if existing.ItemID != item.ID || existing.Fingerprint != item.Fingerprint || existing.UserID != batch.UserID || existing.Quota != batch.RewardQuota || item.RewardState != "rewarded" || item.RewardedQuota != existing.Quota {
				return ErrDonationConflict
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if !validDonationQuota(batch.RewardQuota) || item.RewardState == "rewarded" {
			return ErrDonationConflict
		}
		var user User
		err = lockForUpdate(tx).Select("id", "status").First(&user, batch.UserID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) || user.Status != common.UserStatusEnabled {
			if item.RewardState != "paused" {
				if err := tx.Create(&DonationEvent{ItemID: item.ID, State: "reward_paused", ReasonCode: "account_disabled", CreatedAtMS: time.Now().UnixMilli()}).Error; err != nil {
					return err
				}
			}
			return tx.Model(&DonationItem{}).Where("id = ?", item.ID).Updates(map[string]any{"reward_state": "paused", "reward_reason": "account_disabled"}).Error
		}
		now := time.Now().UnixMilli()
		reward = DonationReward{ID: uuid.NewString(), Fingerprint: item.Fingerprint, ItemID: item.ID, UserID: batch.UserID, Quota: batch.RewardQuota, CreditedAtMS: now}
		if err := tx.Create(&reward).Error; donationUniqueError(err) {
			return errDonationRace
		} else if err != nil {
			return err
		}
		if err := creditTopUpQuota(tx, batch.UserID, batch.RewardQuota, nil); err != nil {
			return err
		}
		if err := tx.Model(&DonationItem{}).Where("id = ?", item.ID).Updates(map[string]any{
			"reward_state": "rewarded", "reward_reason": "", "rewarded_quota": batch.RewardQuota, "rewarded_at_ms": now, "updated_at_ms": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&DonationEvent{ItemID: item.ID, State: "rewarded", CreatedAtMS: now}).Error; err != nil {
			return err
		}
		credited = true
		return nil
	})
	if err != nil {
		reason := "reward_pending"
		if errors.Is(err, ErrTopUpQuotaLimitExceeded) {
			reason = "wallet_limit"
		}
		_ = s.DB.WithContext(ctx).Model(&DonationItem{}).Where("id = ? AND state = ? AND reward_state <> ?", itemID, "accepted", "rewarded").Update("reward_reason", reason).Error
		return false, err
	}
	if credited {
		// Only the transaction which newly committed the ledger applies this delta.
		// Cache misses hydrate later; replay never overwrites outstanding reserves.
		syncCreditUserQuotaCache(reward.UserID, reward.Quota, "donation reward")
		if LOG_DB != nil {
			RecordLog(reward.UserID, LogTypeSystem, fmt.Sprintf("Donation reward %s: permanent quota +%d (item %s)", reward.ID, reward.Quota, reward.ItemID))
		}
	}
	return credited, nil
}

func (s *DonationStore) SettleBatch(ctx context.Context, batchID string) error {
	var ids []string
	if err := s.DB.WithContext(ctx).Model(&DonationItem{}).Where("batch_id = ? AND state = ? AND reward_state <> ?", batchID, "accepted", "rewarded").Pluck("id", &ids).Error; err != nil {
		return err
	}
	var firstErr error
	for _, id := range ids {
		if _, err := s.CreditDonationReward(ctx, id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// Derive completion in one transaction so a concurrent explicit retry cannot
	// lose its recovery flag after inserting a pending action.
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var batch DonationBatch
		if err := lockForUpdate(tx).Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		var waiting, retries int64
		if err := tx.Model(&DonationItem{}).Where("batch_id = ? AND ((dispatch = ? AND state NOT IN ?) OR (state = ? AND reward_state <> ?))", batchID, true, []string{"accepted", "invalid", "existing"}, "accepted", "rewarded").Count(&waiting).Error; err != nil {
			return err
		}
		if err := tx.Model(&DonationRetry{}).Where("batch_id = ? AND done = ?", batchID, false).Count(&retries).Error; err != nil {
			return err
		}
		return tx.Model(&DonationBatch{}).Where("id = ?", batchID).Update("needs_recovery", waiting+retries > 0).Error
	})
	if firstErr != nil {
		return firstErr
	}
	return err
}

type DonationRecordFilter struct {
	UserID       int
	CampaignID   int
	GroupID      uint64
	State        string
	RewardState  string
	FromMS       int64
	ToMS         int64
	ItemID       string
	CredentialID uint64
}

type DonationRecord struct {
	Item   DonationItem    `json:"item"`
	Batch  DonationBatch   `json:"batch"`
	Reward *DonationReward `json:"reward"`
	Events []DonationEvent `json:"events,omitempty"`
}

func (s *DonationStore) Record(ctx context.Context, id string) (DonationRecord, error) {
	var record DonationRecord
	if err := s.DB.WithContext(ctx).Where("id = ?", id).First(&record.Item).Error; err != nil {
		return record, err
	}
	if err := s.DB.WithContext(ctx).Where("id = ?", record.Item.BatchID).First(&record.Batch).Error; err != nil {
		return record, err
	}
	var reward DonationReward
	err := s.DB.WithContext(ctx).Where("item_id = ?", id).First(&reward).Error
	if err == nil {
		record.Reward = &reward
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return record, err
	}
	record.Events = make([]DonationEvent, 0)
	err = s.DB.WithContext(ctx).Where("item_id = ?", id).Order("id").Find(&record.Events).Error
	return record, err
}

func (s *DonationStore) Records(ctx context.Context, filter DonationRecordFilter, offset, limit int) ([]DonationRecord, int64, error) {
	batchQuery := s.DB.WithContext(ctx).Model(&DonationBatch{}).Select("id")
	if filter.UserID > 0 {
		batchQuery = batchQuery.Where("user_id = ?", filter.UserID)
	}
	if filter.CampaignID > 0 {
		batchQuery = batchQuery.Where("campaign_id = ?", filter.CampaignID)
	}
	if filter.GroupID > 0 {
		batchQuery = batchQuery.Where("group_id = ?", filter.GroupID)
	}
	if filter.FromMS > 0 {
		batchQuery = batchQuery.Where("created_at_ms >= ?", filter.FromMS)
	}
	if filter.ToMS > 0 {
		batchQuery = batchQuery.Where("created_at_ms <= ?", filter.ToMS)
	}
	query := s.DB.WithContext(ctx).Model(&DonationItem{}).Where("batch_id IN (?)", batchQuery)
	if filter.State != "" {
		query = query.Where("state = ?", filter.State)
	}
	if filter.RewardState != "" {
		query = query.Where("reward_state = ?", filter.RewardState)
	}
	if filter.ItemID != "" {
		query = query.Where("id = ?", filter.ItemID)
	}
	if filter.CredentialID > 0 {
		query = query.Where("credential_id = ?", filter.CredentialID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ids []string
	if err := query.Order("created_at_ms desc, id desc").Offset(offset).Limit(limit).Pluck("id", &ids).Error; err != nil {
		return nil, 0, err
	}
	result := make([]DonationRecord, 0, len(ids))
	for _, id := range ids {
		record, err := s.Record(ctx, id)
		if err != nil {
			return nil, 0, err
		}
		record.Events = nil
		result = append(result, record)
	}
	return result, total, nil
}

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

	// Review facts are optional: the automatic branch omits them entirely and
	// the released receiver never sends them.
	ItemRevision         *int64 `json:"item_revision"`
	EffectiveMode        string `json:"effective_mode"`
	ReviewTargetRevision string `json:"review_target_revision"`
	EntryActionID        string `json:"entry_action_id"`
	ReviewActionID       string `json:"review_action_id"`
	ReviewDecision       string `json:"review_decision"`
	ReviewedAtMS         *int64 `json:"reviewed_at_ms"`
	StagingExpiresAtMS   *int64 `json:"staging_expires_at_ms"`
}

type DonationReceipt struct {
	// Filled by the service only after authenticating the configured endpoint's
	// capabilities. The remote payload cannot override the pinned identity.
	InstanceID     string                `json:"-"`
	SourceID       string                `json:"-"`
	BatchID        string                `json:"batch_id"`
	GroupID        uint64                `json:"group_id"`
	TargetRevision string                `json:"target_revision"`
	ValidationMode string                `json:"validation_mode,omitempty"`
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
		"invalid_credential", "model_unavailable", "rate_limited", "timeout", "upstream_error", "probe_incompatible", "review_rejected", "unknown":
		return reason
	default:
		return "unknown"
	}
}

// validateDonationReceiptItem bounds every optional field the versioned
// receiver may add. An automatic-only payload keeps working unchanged.
func validateDonationReceiptItem(item DonationReceiptItem) error {
	if item.ItemRevision != nil && *item.ItemRevision < 0 {
		return ErrDonationReceipt
	}
	if item.EffectiveMode != "" {
		if _, ok := NormalizeDonationValidationMode(item.EffectiveMode); !ok {
			return ErrDonationReceipt
		}
	}
	if item.ReviewTargetRevision != "" && !validDonationRevision(item.ReviewTargetRevision) {
		return ErrDonationReceipt
	}
	if item.ReviewActionID != "" && !ValidDonationID(item.ReviewActionID) {
		return ErrDonationReceipt
	}
	if item.EntryActionID != "" && !ValidDonationID(item.EntryActionID) {
		return ErrDonationReceipt
	}
	switch item.ReviewDecision {
	case "", DonationDecisionApproved, DonationDecisionRejected:
	default:
		return ErrDonationReceipt
	}
	if item.ReviewedAtMS != nil && *item.ReviewedAtMS <= 0 {
		return ErrDonationReceipt
	}
	if item.StagingExpiresAtMS != nil && *item.StagingExpiresAtMS <= 0 {
		return ErrDonationReceipt
	}
	return nil
}

// donationReviewContradiction reports whether the same item revision carries a
// different manual fact. Equal revisions must mean equal facts.
func donationReviewContradiction(item DonationItem, remote DonationReceiptItem) bool {
	mode, _ := NormalizeDonationValidationMode(remote.EffectiveMode)
	return item.State != remote.State || item.ReasonCode != DonationReason(remote.ReasonCode) || item.Retryable != remote.Retryable ||
		item.EffectiveMode != mode || item.ReviewTargetRevision != remote.ReviewTargetRevision || item.EntryActionID != remote.EntryActionID ||
		item.ReviewActionID != remote.ReviewActionID || item.ReviewDecision != remote.ReviewDecision ||
		!donationOptionalEqual(item.CredentialID, remote.CredentialID) || !donationOptionalEqual(item.AcceptedAtMS, remote.AcceptedAtMS) ||
		!donationOptionalEqual(item.ReviewedAtMS, remote.ReviewedAtMS) || !donationOptionalEqual(item.StagingExpiresAtMS, remote.StagingExpiresAtMS)
}

func donationOptionalEqual[T comparable](left, right *T) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// confirmReviewFactFromReceipt can confirm the original pending intent from a
// full item receipt. The decision, target, revision and source timestamp must
// all agree; a bare accepted state cannot manufacture an approval.
func (s *DonationStore) confirmReviewFactFromReceipt(tx *gorm.DB, item DonationItem, remote DonationReceiptItem) error {
	if remote.ReviewActionID == "" || remote.ReviewedAtMS == nil || remote.ItemRevision == nil || remote.ReviewDecision == "" {
		return ErrDonationReceipt
	}
	var kind string
	switch remote.ReviewDecision {
	case DonationDecisionApproved:
		kind = DonationActionApprove
	case DonationDecisionRejected:
		kind = DonationActionReject
	default:
		return ErrDonationReceipt
	}
	var action DonationReviewAction
	err := lockForUpdate(tx).Where("item_id = ? AND batch_id = ? AND action_id = ? AND kind = ?", item.ID, item.BatchID, remote.ReviewActionID, kind).First(&action).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrDonationReceipt
	}
	if err != nil {
		return err
	}
	if (action.ReviewTargetRevision != "" && action.ReviewTargetRevision != remote.ReviewTargetRevision) ||
		action.ExpectedItemRevision >= *remote.ItemRevision || action.Status == DonationActionRejected {
		return ErrDonationReceipt
	}
	if action.Status == DonationActionPending {
		effectRevision := action.ExpectedItemRevision + 1
		if remote.State == "existing" {
			// The approving transaction can first discover inventory, then stamp
			// its decision. This terminal receipt carries the action's final
			// revision; assuming a single increment would contradict its query.
			effectRevision = *remote.ItemRevision
		}
		_, err := s.applyReviewOutcomeTx(tx, action, DonationActionApplied, "", effectRevision, *remote.ReviewedAtMS)
		return err
	}
	if action.Status != DonationActionApplied || action.EffectRevision <= action.ExpectedItemRevision || action.EffectRevision > *remote.ItemRevision ||
		action.AppliedAtMS == nil || *action.AppliedAtMS != *remote.ReviewedAtMS {
		return ErrDonationReceipt
	}
	return nil
}

func donationRequiresManualReview(tx *gorm.DB, batch DonationBatch, item DonationItem) (bool, error) {
	batchMode, batchOK := NormalizeDonationValidationMode(batch.ValidationMode)
	mode, modeOK := NormalizeDonationValidationMode(item.EffectiveMode)
	if !batchOK || !modeOK {
		return false, ErrDonationReceipt
	}
	if batchMode == DonationModeManualReview || mode == DonationModeManualReview || item.EntryActionID != "" || item.ReviewActionID != "" || item.ReviewDecision != "" {
		return true, nil
	}
	var applied int64
	if err := tx.Model(&DonationReviewAction{}).Where("batch_id = ? AND item_id = ? AND status = ?", batch.ID, item.ID, DonationActionApplied).Count(&applied).Error; err != nil {
		return false, err
	}
	return applied > 0, nil
}

// manualApproveFact is shared by receipt application and the independent award
// boundary. It verifies the exact approval referenced by the stored acceptance.
func manualApproveFact(tx *gorm.DB, item DonationItem) (bool, error) {
	if item.EffectiveMode != DonationModeManualReview || item.ReviewActionID == "" || item.ReviewDecision != DonationDecisionApproved ||
		item.ReviewState != "approved" || item.ReviewTargetRevision == "" || item.ReviewedAtMS == nil || item.ItemRevision <= 0 {
		return false, ErrDonationReceipt
	}
	var applied DonationReviewAction
	err := lockForUpdate(tx).Where("item_id = ? AND batch_id = ? AND action_id = ? AND kind = ? AND status = ?", item.ID, item.BatchID, item.ReviewActionID, DonationActionApprove, DonationActionApplied).First(&applied).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if applied.ReviewTargetRevision != item.ReviewTargetRevision || applied.EffectRevision <= applied.ExpectedItemRevision || applied.EffectRevision > item.ItemRevision ||
		applied.AppliedAtMS == nil || *applied.AppliedAtMS != *item.ReviewedAtMS {
		return false, ErrDonationReceipt
	}
	return true, nil
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
		batchMode, batchOK := NormalizeDonationValidationMode(batch.ValidationMode)
		receiptMode, receiptOK := NormalizeDonationValidationMode(receipt.ValidationMode)
		if !batchOK || !receiptOK || receiptMode != batchMode {
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
			if err := validateDonationReceiptItem(item); err != nil {
				return err
			}
			switch item.State {
			case "accepted":
				if item.CredentialID == nil || *item.CredentialID == 0 || *item.CredentialID > uint64(common.MaxWalletQuota) || item.AcceptedAtMS == nil || *item.AcceptedAtMS <= 0 || item.Retryable {
					return ErrDonationReceipt
				}
			case "queued", "validating", "committing", "retry_pending", "invalid", "existing", "pending_review", "rejected":
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
			// An item that already observed a positive revision or carries a
			// manual fact never accepts a late or legacy observation; an equal
			// revision must carry equal facts. An item still at revision zero
			// keeps the released monotonic rules, because a receiver without the
			// versioned contract omits the revision on every single response.
			if item.ItemRevision > 0 {
				if remote.ItemRevision == nil || *remote.ItemRevision < item.ItemRevision {
					continue
				}
				if *remote.ItemRevision == item.ItemRevision {
					if donationReviewContradiction(item, remote) {
						return ErrDonationReceipt
					}
					continue
				}
			}
			manual, err := donationRequiresManualReview(tx, batch, item)
			if err != nil {
				return err
			}
			remoteMode, _ := NormalizeDonationValidationMode(remote.EffectiveMode)
			if manual && remoteMode != DonationModeManualReview {
				return ErrDonationReceipt
			}
			manual = manual || remoteMode == DonationModeManualReview
			if manual {
				if remote.ItemRevision == nil || *remote.ItemRevision <= 0 || !validDonationRevision(remote.ReviewTargetRevision) ||
					(item.ReviewTargetRevision != "" && item.ReviewTargetRevision != remote.ReviewTargetRevision) {
					return ErrDonationReceipt
				}
				if batchMode == DonationModeAuto {
					var entry DonationReviewAction
					if remote.EntryActionID == "" || (item.EntryActionID != "" && item.EntryActionID != remote.EntryActionID) {
						return ErrDonationReceipt
					}
					if err := lockForUpdate(tx).Where("batch_id = ? AND item_id = ? AND action_id = ? AND kind = ?", batch.ID, item.ID, remote.EntryActionID, DonationActionEnterReview).First(&entry).Error; err != nil {
						return ErrDonationReceipt
					}
					if entry.Status == DonationActionRejected || entry.ReviewTargetRevision != remote.ReviewTargetRevision || entry.ExpectedItemRevision >= *remote.ItemRevision {
						return ErrDonationReceipt
					}
				}
				switch remote.State {
				case "accepted", "committing":
					if remote.ReviewDecision != DonationDecisionApproved {
						return ErrDonationReceipt
					}
					if err := s.confirmReviewFactFromReceipt(tx, item, remote); err != nil {
						return err
					}
				case "rejected":
					if remote.ReviewDecision != DonationDecisionRejected {
						return ErrDonationReceipt
					}
					if err := s.confirmReviewFactFromReceipt(tx, item, remote); err != nil {
						return err
					}
				case "existing":
					if remote.ReviewDecision == DonationDecisionApproved {
						if err := s.confirmReviewFactFromReceipt(tx, item, remote); err != nil {
							return err
						}
						break
					}
					fallthrough
				case "pending_review", "invalid":
					if remote.ReviewActionID != "" || remote.ReviewDecision != "" || remote.ReviewedAtMS != nil {
						return ErrDonationReceipt
					}
				default:
					return ErrDonationReceipt
				}
			} else if remote.ReviewActionID != "" || remote.EntryActionID != "" || remote.ReviewDecision != "" || remote.ReviewTargetRevision != "" || remote.State == "pending_review" || remote.State == "rejected" {
				return ErrDonationReceipt
			}
			if item.State == "accepted" {
				if remote.State == "accepted" && (!donationOptionalEqual(item.CredentialID, remote.CredentialID) || !donationOptionalEqual(item.AcceptedAtMS, remote.AcceptedAtMS)) {
					return ErrDonationReceipt
				}
				continue // An old automatic observation cannot revoke acceptance.
			}
			if item.State == "invalid" || item.State == "existing" || item.State == "rejected" {
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
			if remote.ItemRevision != nil && *remote.ItemRevision > item.ItemRevision {
				item.ItemRevision = *remote.ItemRevision
			}
			item.EffectiveMode = remoteMode
			if remote.ReviewTargetRevision != "" {
				item.ReviewTargetRevision = remote.ReviewTargetRevision
			}
			if remote.StagingExpiresAtMS != nil {
				item.StagingExpiresAtMS = remote.StagingExpiresAtMS
			}
			item.EntryActionID, item.ReviewActionID, item.ReviewDecision, item.ReviewedAtMS = remote.EntryActionID, remote.ReviewActionID, remote.ReviewDecision, remote.ReviewedAtMS
			if manual {
				switch remote.ReviewDecision {
				case DonationDecisionApproved:
					item.ReviewState = "approved"
				case DonationDecisionRejected:
					item.ReviewState = "rejected"
				default:
					item.ReviewState = remote.State
				}
			}
			switch remote.State {
			case "accepted":
				item.CredentialID, item.AcceptedAtMS = remote.CredentialID, remote.AcceptedAtMS
				if manual {
					approved, err := manualApproveFact(tx, item)
					if err != nil {
						return err
					}
					if !approved {
						return ErrDonationReceipt
					}
				}
				item.RewardState = "pending"
				resource.Acquired, resource.AcceptedItemID = true, item.ID
			case "existing":
				resource.Acquired = true // Inventory is also permanently ineligible.
			case "invalid":
				// Only this terminal, authenticated receipt proves non-acceptance.
				// A timeout, lease expiry, or a 404 never reaches this release path.
				if remote.ReasonCode == "staging_expired" {
					item.ReviewState = "expired"
				}
				resource.OwnerItemID = ""
			case "rejected":
				item.ReviewState = "rejected"
				if !resource.Acquired {
					resource.OwnerItemID = ""
				}
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
		// A manual item needs both an applied approval for this item and the same
		// frozen manual target, and the matching accepted receipt. An accepted
		// receipt, a successful test, or a mere pending intent is not enough.
		manual, err := donationRequiresManualReview(tx, batch, item)
		if err != nil {
			return err
		}
		var pendingActions int64
		if err := tx.Model(&DonationReviewAction{}).Where("item_id = ? AND batch_id = ? AND status = ?", item.ID, batch.ID, DonationActionPending).Count(&pendingActions).Error; err != nil {
			return err
		}
		if pendingActions > 0 {
			return tx.Model(&DonationItem{}).Where("id = ?", item.ID).Update("reward_reason", "review_pending").Error
		}
		if manual {
			approved, err := manualApproveFact(tx, item)
			if err != nil {
				return err
			}
			if !approved {
				return tx.Model(&DonationItem{}).Where("id = ? AND reward_state <> ?", item.ID, "rewarded").
					Updates(map[string]any{"reward_reason": "review_pending", "updated_at_ms": time.Now().UnixMilli()}).Error
			}
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
	// lose its recovery flag after inserting a pending action. A batch that only
	// waits for a human decision leaves the hot queue and is scheduled for the
	// low-rate GET-only reconciliation instead.
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var batch DonationBatch
		if err := lockForUpdate(tx).Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		var waiting, receiving, rewards, actions, retries, pendingReview int64
		if err := tx.Model(&DonationItem{}).Where("batch_id = ? AND dispatch = ? AND state NOT IN ?", batchID, true,
			[]string{"accepted", "invalid", "existing", "pending_review", "rejected"}).Count(&waiting).Error; err != nil {
			return err
		}
		// An applied approval is mid-receive until the acceptance is observed, so
		// that item keeps the batch in the hot queue.
		confirmedDecisions := tx.Model(&DonationReviewAction{}).Select("item_id").Where("batch_id = ? AND status = ? AND kind IN ?", batchID, DonationActionApplied, []string{DonationActionApprove, DonationActionReject})
		if err := tx.Model(&DonationItem{}).Where("batch_id = ? AND dispatch = ? AND state = ? AND id IN (?)",
			batchID, true, "pending_review", confirmedDecisions).Count(&receiving).Error; err != nil {
			return err
		}
		if err := tx.Model(&DonationItem{}).Where("batch_id = ? AND state = ? AND reward_state <> ?", batchID, "accepted", "rewarded").Count(&rewards).Error; err != nil {
			return err
		}
		if err := tx.Model(&DonationReviewAction{}).Where("batch_id = ? AND status = ?", batchID, DonationActionPending).Count(&actions).Error; err != nil {
			return err
		}
		if err := tx.Model(&DonationRetry{}).Where("batch_id = ? AND done = ?", batchID, false).Count(&retries).Error; err != nil {
			return err
		}
		if err := tx.Model(&DonationItem{}).Where("batch_id = ? AND state = ? AND (review_state IS NULL OR review_state <> ?)",
			batchID, "pending_review", "approved").Count(&pendingReview).Error; err != nil {
			return err
		}
		needsRecovery := waiting+receiving+rewards+actions+retries > 0
		updates := map[string]any{"needs_recovery": needsRecovery}
		if !needsRecovery {
			switch {
			case pendingReview > 0 && batch.ColdPollAtMS == 0:
				updates["cold_poll_at_ms"] = now + donationColdIntervalMS
			case pendingReview == 0:
				updates["cold_poll_at_ms"] = 0
			}
		}
		return tx.Model(&DonationBatch{}).Where("id = ?", batchID).Updates(updates).Error
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
	Item                DonationItem           `json:"item"`
	Batch               DonationBatch          `json:"batch"`
	Reward              *DonationReward        `json:"reward"`
	Events              []DonationEvent        `json:"events,omitempty"`
	PendingReviewAction *DonationReviewAction  `json:"-"`
	LatestTest          *DonationTestAttempt   `json:"-"`
	RecentReviewActions []DonationReviewAction `json:"-"`
	RecentTests         []DonationTestAttempt  `json:"-"`
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
	if err := s.DB.WithContext(ctx).Where("item_id = ?", id).Order("id").Find(&record.Events).Error; err != nil {
		return record, err
	}
	if err := s.DB.WithContext(ctx).Where("item_id = ?", id).Order("created_at_ms DESC, id DESC").Limit(20).Find(&record.RecentReviewActions).Error; err != nil {
		return record, err
	}
	for i := range record.RecentReviewActions {
		if record.RecentReviewActions[i].Status == DonationActionPending {
			record.PendingReviewAction = &record.RecentReviewActions[i]
			break
		}
	}
	if err := s.DB.WithContext(ctx).Where("item_id = ?", id).Order("started_at_ms DESC, id DESC").Limit(20).Find(&record.RecentTests).Error; err != nil {
		return record, err
	}
	if len(record.RecentTests) > 0 {
		record.LatestTest = &record.RecentTests[0]
	}
	return record, nil
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

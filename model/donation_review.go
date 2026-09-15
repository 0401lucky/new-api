package model

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	// A review-only batch is polled at a low rate with GET requests only, so it
	// never competes with the hot recovery queue for work.
	donationColdIntervalMS = 300000
	donationColdLeaseMS    = 30000
	donationHotLeaseMS     = 120000
	// A test that outlives the protocol total timeout plus a generous margin was
	// interrupted; it is never replayed automatically.
	donationTestLeaseMS = (DonationTestTotalTimeout + 60) * 1000
)

// DonationActionReason is the closed set of business reasons the receiver may
// return for a review action. It is not the donation item reason vocabulary.
func DonationActionReason(reason string) string {
	switch reason {
	case "", "revision_mismatch", "mode_conflict", "target_changed", "target_unavailable", "staging_expired",
		"item_state", "already_reviewed", "not_reviewable", "test_running", "resource_busy", "duplicate_pending":
		return reason
	default:
		return "unknown"
	}
}

func DonationTestReason(reason string) string {
	switch reason {
	case "", "timeout", "cancelled", "interrupted", "upstream_error", "rate_limited", "unauthorized", "model_unavailable", "empty_output",
		"response_too_large", "target_changed", "target_unavailable", "staging_expired", "test_unavailable", "incomplete_stream", "internal_error",
		"credential_unavailable", "proxy_unavailable", "request_invalid", "invalid_format", "invalid_stream", "invalid_response",
		"connection_unavailable", "item_not_found", "review_not_found", "integration_unauthorized", "protocol_mismatch", "unknown":
		return reason
	default:
		return "unknown"
	}
}

// DonationActionNoteValid enforces the review-note contract: UTF-8, bounded, and
// non-empty for a rejection.
func DonationActionNoteValid(kind, note string) bool {
	if !utf8.ValidString(note) || len(note) > DonationMaxNoteBytes {
		return false
	}
	if kind == DonationActionReject {
		return strings.TrimSpace(note) != ""
	}
	return true
}

func validDonationRevision(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

// DonationReviewIntent is the locally persisted intent plus the batch it
// belongs to, so the caller can wake recovery without re-reading.
type DonationReviewIntent struct {
	Action  DonationReviewAction
	Batch   DonationBatch
	Created bool
}

// PrepareReviewAction persists the intent with the exact action UUID that will
// be sent to the receiver, then wakes hot recovery for this batch.
func (s *DonationStore) PrepareReviewAction(ctx context.Context, actorID int, itemID, actionID, kind string, expectedRevision int64, targetRevision, note string) (DonationReviewIntent, error) {
	var intent DonationReviewIntent
	if actorID <= 0 || !ValidDonationID(itemID) || !ValidDonationID(actionID) || expectedRevision < 0 || !DonationActionNoteValid(kind, note) {
		return intent, ErrDonationInput
	}
	switch kind {
	case DonationActionEnterReview, DonationActionApprove:
		if !validDonationRevision(targetRevision) {
			return intent, ErrDonationInput
		}
	case DonationActionReject:
		if targetRevision != "" && !validDonationRevision(targetRevision) {
			return intent, ErrDonationInput
		}
	default:
		return intent, ErrDonationInput
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := s.verifySecret(tx); err != nil {
			return err
		}
		var existing DonationReviewAction
		err := tx.Where("action_id = ?", actionID).First(&existing).Error
		if err == nil {
			if existing.ActorID != actorID || existing.ItemID != itemID || existing.Kind != kind || existing.ExpectedItemRevision != expectedRevision ||
				existing.ReviewTargetRevision != targetRevision || existing.Note != note {
				return ErrDonationConflict
			}
			intent.Action = existing
			return tx.Where("id = ?", existing.BatchID).First(&intent.Batch).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var item DonationItem
		if err := lockForUpdate(tx).Where("id = ?", itemID).First(&item).Error; err != nil {
			return err
		}
		if !item.Dispatch || item.ItemRevision != expectedRevision {
			return ErrDonationConflict
		}
		var pending DonationReviewAction
		pendingErr := lockForUpdate(tx).Where("item_id = ? AND status = ?", item.ID, DonationActionPending).First(&pending).Error
		if pendingErr == nil {
			return ErrDonationConflict
		}
		if !errors.Is(pendingErr, gorm.ErrRecordNotFound) {
			return pendingErr
		}
		if err := donationReviewable(item, kind, targetRevision, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err := requireNoRunningDonationTest(tx, item.ID); err != nil {
			return err
		}
		var batch DonationBatch
		if err := lockForUpdate(tx).Where("id = ?", item.BatchID).First(&batch).Error; err != nil {
			return err
		}
		var resource DonationResource
		if err := lockForUpdate(tx).Where("fingerprint = ?", item.Fingerprint).First(&resource).Error; err != nil {
			return err
		}
		if batch.ReceptionState != "confirmed" || resource.Acquired || resource.OwnerItemID != item.ID || resource.Generation != item.Generation {
			return ErrDonationReview
		}
		now := time.Now().UnixMilli()
		intent.Action = DonationReviewAction{ActorID: actorID, ActionID: actionID, BatchID: item.BatchID, ItemID: item.ID,
			Kind: kind, ExpectedItemRevision: expectedRevision, ReviewTargetRevision: targetRevision, Note: note,
			Status: DonationActionPending, CreatedAtMS: now, UpdatedAtMS: now}
		if err := tx.Create(&intent.Action).Error; donationUniqueError(err) {
			return errDonationRace
		} else if err != nil {
			return err
		}
		intent.Batch, intent.Created = batch, true
		// A prepared decision must never wait behind a cold reconciliation task.
		return tx.Model(&DonationBatch{}).Where("id = ?", item.BatchID).Updates(map[string]any{
			"needs_recovery": true, "next_poll_at_ms": now, "lease_until_ms": 0, "lease_token": "", "cold_poll_at_ms": 0, "updated_at_ms": now}).Error
	})
	return intent, err
}

// donationReviewable is the local precondition set for one review kind. The
// receiver re-validates the same facts; a local refusal is a definite local
// decision, not a guess about the receiver.
func donationReviewable(item DonationItem, kind, targetRevision string, now int64) error {
	expired := item.StagingExpiresAtMS != nil && *item.StagingExpiresAtMS > 0 && now >= *item.StagingExpiresAtMS
	switch kind {
	case DonationActionEnterReview:
		if item.EffectiveMode == DonationModeManualReview || item.ReviewState != "" {
			return ErrDonationReview
		}
		if item.State != "retry_pending" || item.ReasonCode != "retry_exhausted" || expired {
			return ErrDonationReview
		}
	case DonationActionApprove:
		if item.EffectiveMode != DonationModeManualReview || item.State != "pending_review" || expired {
			return ErrDonationReview
		}
		if item.ReviewTargetRevision == "" || item.ReviewTargetRevision != targetRevision {
			return ErrDonationReview
		}
	case DonationActionReject:
		// Rejection does not require the target to be online, but still belongs
		// to the current pending item and its original staging lifetime.
		if item.EffectiveMode != DonationModeManualReview || expired {
			return ErrDonationReview
		}
		if item.State != "pending_review" {
			return ErrDonationReview
		}
	}
	return nil
}

func requireNoRunningDonationTest(tx *gorm.DB, itemID string) error {
	// Use a current read after acquiring the item lock. A MySQL REPEATABLE READ
	// snapshot from the idempotency lookup must not hide the winning attempt.
	var running DonationTestAttempt
	err := lockForUpdate(tx).Where("item_id = ? AND state = ?", itemID, DonationTestRunning).First(&running).Error
	if err == nil {
		return ErrDonationTestBusy
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

// ApplyReviewOutcome records the receiver's verdict for one action and, when it
// was applied, advances the local projection of that item.
func (s *DonationStore) ApplyReviewOutcome(ctx context.Context, action DonationReviewAction, outcome, reasonCode string, effectRevision int64, reviewTargetRevision string, appliedAtMS int64) (DonationReviewAction, error) {
	if outcome != DonationActionApplied && outcome != DonationActionRejected {
		return action, ErrDonationReceipt
	}
	if action.ID == 0 || appliedAtMS <= 0 || effectRevision < 0 || reviewTargetRevision != action.ReviewTargetRevision ||
		(outcome == DonationActionApplied && (reasonCode != "" || effectRevision <= action.ExpectedItemRevision)) ||
		(outcome == DonationActionRejected && (reasonCode == "" || DonationActionReason(reasonCode) == "unknown")) {
		return action, ErrDonationReceipt
	}
	var saved DonationReviewAction
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := s.verifySecret(tx); err != nil {
			return err
		}
		var err error
		saved, err = s.applyReviewOutcomeTx(tx, action, outcome, reasonCode, effectRevision, appliedAtMS)
		return err
	})
	if err != nil {
		return action, err
	}
	return saved, nil
}

// applyReviewOutcomeTx is the transaction-scoped half of ApplyReviewOutcome so
// an acceptance receipt can confirm its own approval intent atomically.
func (s *DonationStore) applyReviewOutcomeTx(tx *gorm.DB, action DonationReviewAction, outcome, reasonCode string, effectRevision int64, appliedAtMS int64) (DonationReviewAction, error) {
	var saved DonationReviewAction
	if err := lockForUpdate(tx).Where("id = ? AND actor_id = ? AND action_id = ? AND batch_id = ? AND item_id = ?", action.ID, action.ActorID, action.ActionID, action.BatchID, action.ItemID).First(&saved).Error; err != nil {
		return saved, err
	}
	if saved.Kind != action.Kind || saved.ExpectedItemRevision != action.ExpectedItemRevision || saved.ReviewTargetRevision != action.ReviewTargetRevision || saved.Note != action.Note {
		return saved, ErrDonationReceipt
	}
	if saved.Status != DonationActionPending {
		// The first recorded verdict wins; a repeat observation must agree.
		if saved.Status != outcome || saved.ReasonCode != reasonCode || saved.EffectRevision != effectRevision || saved.AppliedAtMS == nil || *saved.AppliedAtMS != appliedAtMS {
			return saved, ErrDonationConflict
		}
		return saved, nil
	}
	now := time.Now().UnixMilli()
	// An action response is not a complete item snapshot. Only ApplyReceipt
	// advances item state/revision, so delayed action queries cannot roll a
	// newer acceptance back or invent a conflicting snapshot at the same revision.
	updates := map[string]any{"status": outcome, "reason_code": reasonCode, "effect_revision": effectRevision,
		"applied_at_ms": appliedAtMS, "updated_at_ms": now}
	if err := tx.Model(&DonationReviewAction{}).Where("id = ?", saved.ID).Updates(updates).Error; err != nil {
		return saved, err
	}
	saved.Status, saved.ReasonCode, saved.EffectRevision, saved.AppliedAtMS, saved.UpdatedAtMS = outcome, reasonCode, effectRevision, &appliedAtMS, now
	if err := tx.Model(&DonationBatch{}).Where("id = ?", saved.BatchID).Updates(map[string]any{
		"needs_recovery": true, "next_poll_at_ms": now, "cold_poll_at_ms": 0, "updated_at_ms": now}).Error; err != nil {
		return saved, err
	}
	return saved, nil
}

// Item reads one donation item together with its batch.
func (s *DonationStore) Item(ctx context.Context, id string) (DonationItem, DonationBatch, error) {
	var item DonationItem
	if !ValidDonationID(id) {
		return item, DonationBatch{}, ErrDonationInput
	}
	if err := s.DB.WithContext(ctx).Where("id = ?", id).First(&item).Error; err != nil {
		return item, DonationBatch{}, err
	}
	var batch DonationBatch
	err := s.DB.WithContext(ctx).Where("id = ?", item.BatchID).First(&batch).Error
	return item, batch, err
}

func (s *DonationStore) ReviewAction(ctx context.Context, actorID int, actionID string) (DonationReviewAction, error) {
	var action DonationReviewAction
	if actorID <= 0 || !ValidDonationID(actionID) {
		return action, ErrDonationInput
	}
	err := s.DB.WithContext(ctx).Where("actor_id = ? AND action_id = ?", actorID, actionID).First(&action).Error
	return action, err
}

// ReviewActionForItem is an administrator read projection. The HTTP caller
// must hold records.read; another administrator can inspect an existing intent
// without inheriting its actor or permission to send a new decision.
func (s *DonationStore) ReviewActionForItem(ctx context.Context, itemID, actionID string) (DonationReviewAction, error) {
	var action DonationReviewAction
	if !ValidDonationID(itemID) || !ValidDonationID(actionID) {
		return action, ErrDonationInput
	}
	err := s.DB.WithContext(ctx).Where("item_id = ? AND action_id = ?", itemID, actionID).First(&action).Error
	return action, err
}

// PendingReviewActions returns the durable intents that still need the original
// action UUID replayed against the receiver.
func (s *DonationStore) PendingReviewActions(ctx context.Context, batchID string) ([]DonationReviewAction, error) {
	actions := make([]DonationReviewAction, 0)
	err := s.DB.WithContext(ctx).Where("batch_id = ? AND status = ?", batchID, DonationActionPending).Order("created_at_ms, id").Limit(DonationMaxItems).Find(&actions).Error
	return actions, err
}

// ReviewStateForItem projects the donor-visible rejection note from the final
// applied reject action of that item.
func (s *DonationStore) ReviewNotes(ctx context.Context, batchID string) (map[string]string, error) {
	actions := make([]DonationReviewAction, 0)
	if err := s.DB.WithContext(ctx).Where("batch_id = ? AND kind = ? AND status = ?", batchID, DonationActionReject, DonationActionApplied).
		Order("id").Find(&actions).Error; err != nil {
		return nil, err
	}
	notes := make(map[string]string, len(actions))
	for _, action := range actions {
		notes[action.ActionID] = action.Note
	}
	return notes, nil
}

// DonationTestIntent is the persisted test reservation plus its batch.
type DonationTestIntent struct {
	Attempt DonationTestAttempt
	Batch   DonationBatch
	Created bool
}

// TestRequestDigest binds prompt boundaries and the output budget without
// storing chat content or using a public, dictionary-testable hash key.
func (s *DonationStore) TestRequestDigest(prompt, systemPrompt string, maxOutputTokens int) (string, error) {
	encoded, err := common.Marshal(struct {
		Prompt          string
		SystemPrompt    string
		MaxOutputTokens int
	}{prompt, systemPrompt, maxOutputTokens})
	if err != nil {
		return "", err
	}
	return common.GenerateHMACWithKey(s.fingerprintKey, "new-api/donation-test/v1\x00"+string(encoded)), nil
}

// PrepareTestAttempt reserves the test ID before any upstream call. A replay of
// the same ID returns the original metadata and never consumes the upstream
// twice.
func (s *DonationStore) PrepareTestAttempt(ctx context.Context, actorID int, itemID, testID string, startRevision int64, targetRevision, model string, stream bool, promptBytes int, promptDigest string) (DonationTestIntent, error) {
	var intent DonationTestIntent
	if actorID <= 0 || !ValidDonationID(itemID) || !ValidDonationID(testID) || startRevision < 0 || promptBytes <= 0 ||
		promptBytes > DonationMaxPromptBytes || len(model) == 0 || len(model) > 128 || !validDonationRevision(targetRevision) {
		return intent, ErrDonationInput
	}
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := s.verifySecret(tx); err != nil {
			return err
		}
		var existing DonationTestAttempt
		err := tx.Where("test_id = ?", testID).First(&existing).Error
		if err == nil {
			if existing.ActorID != actorID || existing.ItemID != itemID || existing.StartRevision != startRevision || existing.ReviewTargetRevision != targetRevision ||
				existing.Model != model || existing.Stream != stream || existing.PromptDigest != promptDigest {
				return ErrDonationConflict
			}
			intent.Attempt = existing
			return tx.Where("id = ?", existing.BatchID).First(&intent.Batch).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var item DonationItem
		if err := lockForUpdate(tx).Where("id = ?", itemID).First(&item).Error; err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		expired := item.StagingExpiresAtMS != nil && *item.StagingExpiresAtMS > 0 && now >= *item.StagingExpiresAtMS
		if !item.Dispatch || item.EffectiveMode != DonationModeManualReview || item.State != "pending_review" ||
			item.ItemRevision != startRevision || item.ReviewTargetRevision == "" || item.ReviewTargetRevision != targetRevision || expired {
			return ErrDonationReview
		}
		if err := requireNoRunningDonationTest(tx, item.ID); err != nil {
			return err
		}
		var pending DonationReviewAction
		pendingErr := lockForUpdate(tx).Where("item_id = ? AND status = ?", item.ID, DonationActionPending).First(&pending).Error
		if pendingErr == nil {
			return ErrDonationConflict
		}
		if !errors.Is(pendingErr, gorm.ErrRecordNotFound) {
			return pendingErr
		}
		intent.Attempt = DonationTestAttempt{ActorID: actorID, TestID: testID, BatchID: item.BatchID, ItemID: item.ID,
			StartRevision: startRevision, ReviewTargetRevision: targetRevision, Model: model, Stream: stream,
			PromptDigest: promptDigest, PromptBytes: promptBytes, State: DonationTestRunning, StartedAtMS: now}
		if err := tx.Create(&intent.Attempt).Error; donationUniqueError(err) {
			return errDonationRace
		} else if err != nil {
			return err
		}
		intent.Created = true
		return tx.Where("id = ?", item.BatchID).First(&intent.Batch).Error
	})
	return intent, err
}

// FinishTestAttempt persists a terminal test state. A terminal row is never
// overwritten by a different outcome.
func (s *DonationStore) FinishTestAttempt(ctx context.Context, id uint64, state, reasonCode string, statusCode, outputBytes, inputTokens, outputTokens int, finishedAtMS int64) error {
	switch state {
	case DonationTestSucceeded, DonationTestFailed, DonationTestCancelled, DonationTestInterrupted:
	default:
		return ErrDonationInput
	}
	if reasonCode != "" {
		reasonCode = DonationTestReason(reasonCode)
	}
	if finishedAtMS <= 0 {
		finishedAtMS = time.Now().UnixMilli()
	}
	if outputBytes < 0 || inputTokens < 0 || outputTokens < 0 {
		return ErrDonationInput
	}
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var attempt DonationTestAttempt
		if err := lockForUpdate(tx).Where("id = ?", id).First(&attempt).Error; err != nil {
			return err
		}
		if attempt.State != DonationTestRunning {
			if attempt.State == state {
				return nil
			}
			return ErrDonationConflict
		}
		return tx.Model(&DonationTestAttempt{}).Where("id = ?", id).Updates(map[string]any{
			"state": state, "reason_code": reasonCode, "status_code": statusCode, "output_bytes": outputBytes,
			"input_tokens": inputTokens, "output_tokens": outputTokens, "finished_at_ms": finishedAtMS}).Error
	})
}

func (s *DonationStore) TestAttempt(ctx context.Context, actorID int, itemID, testID string) (DonationTestAttempt, error) {
	var attempt DonationTestAttempt
	if actorID <= 0 || !ValidDonationID(itemID) || !ValidDonationID(testID) {
		return attempt, ErrDonationInput
	}
	err := s.DB.WithContext(ctx).Where("actor_id = ? AND item_id = ? AND test_id = ?", actorID, itemID, testID).First(&attempt).Error
	return attempt, err
}

func (s *DonationStore) TestAttemptForItem(ctx context.Context, itemID, testID string) (DonationTestAttempt, error) {
	var attempt DonationTestAttempt
	if !ValidDonationID(itemID) || !ValidDonationID(testID) {
		return attempt, ErrDonationInput
	}
	err := s.DB.WithContext(ctx).Where("item_id = ? AND test_id = ?", itemID, testID).First(&attempt).Error
	return attempt, err
}

// InterruptStaleTests closes attempts whose process died mid-flight. It never
// re-sends a model request.
func (s *DonationStore) InterruptStaleTests(ctx context.Context, now int64) error {
	return s.DB.WithContext(ctx).Model(&DonationTestAttempt{}).
		Where("state = ? AND started_at_ms <= ?", DonationTestRunning, now-donationTestLeaseMS).
		Updates(map[string]any{"state": DonationTestInterrupted, "reason_code": "interrupted", "finished_at_ms": now}).Error
}

// ClaimColdRecovery claims a small quota of GET-only reconciliation work for
// batches that only wait for a human decision.
func (s *DonationStore) ClaimColdRecovery(ctx context.Context, now int64) ([]DonationBatch, error) {
	pending := s.DB.WithContext(ctx).Model(&DonationItem{}).Select("batch_id").Where("state = ?", "pending_review")
	var candidates []DonationBatch
	if err := s.DB.WithContext(ctx).
		Where("needs_recovery = ? AND lease_until_ms <= ? AND (cold_poll_at_ms = 0 OR cold_poll_at_ms <= ?) AND id IN (?)", false, now, now, pending).
		Order("cold_poll_at_ms, id").Limit(1).Find(&candidates).Error; err != nil {
		return nil, err
	}
	claimed := make([]DonationBatch, 0, len(candidates))
	for _, batch := range candidates {
		token := newDonationLeaseToken()
		result := s.DB.WithContext(ctx).Model(&DonationBatch{}).
			Where("id = ? AND needs_recovery = ? AND lease_until_ms <= ?", batch.ID, false, now).
			Updates(map[string]any{"lease_token": token, "lease_until_ms": now + donationColdLeaseMS})
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected == 1 {
			batch.LeaseToken = token
			claimed = append(claimed, batch)
		}
	}
	return claimed, nil
}

// ReleaseColdRecovery reschedules the next low-rate check and re-derives whether
// the batch still belongs to the cold queue at all.
func (s *DonationStore) ReleaseColdRecovery(ctx context.Context, batch DonationBatch) error {
	now := time.Now().UnixMilli()
	return s.DB.WithContext(ctx).Model(&DonationBatch{}).Where("id = ? AND lease_token = ?", batch.ID, batch.LeaseToken).
		Updates(map[string]any{"lease_until_ms": 0, "lease_token": "", "cold_poll_at_ms": now + donationColdIntervalMS}).Error
}

func newDonationLeaseToken() string {
	return uuid.NewString()
}

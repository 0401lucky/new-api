package model

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DonationLine struct {
	Line        int
	Key         string
	Fingerprint string
	ID          string
	Invalid     bool
}

func (s *DonationStore) ParseLines(campaignID int, text string) ([]DonationLine, string, error) {
	if campaignID <= 0 || len(text) > DonationMaxBodyBytes || !utf8.ValidString(text) {
		return nil, "", ErrDonationInput
	}
	lines := make([]DonationLine, 0)
	lineNumber := 0
	proof := make([]struct {
		Line        int
		Fingerprint string
	}, 0)
	for line := range strings.SplitSeq(text, "\n") {
		lineNumber++
		key := strings.TrimSpace(line)
		if key == "" {
			continue
		}
		if len(lines) == DonationMaxItems {
			return nil, "", ErrDonationInput
		}
		invalid := len(key) > DonationMaxKeyBytes || strings.HasPrefix(key, "{") || strings.ContainsFunc(key, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) })
		fp := s.Fingerprint(key)
		lines = append(lines, DonationLine{Line: lineNumber, Key: key, Fingerprint: fp, ID: uuid.NewString(), Invalid: invalid})
		proof = append(proof, struct {
			Line        int
			Fingerprint string
		}{lineNumber, fp})
	}
	if len(lines) == 0 {
		return nil, "", ErrDonationInput
	}
	encoded, err := common.Marshal(struct {
		Campaign int
		Lines    any
	}{campaignID, proof})
	if err != nil {
		return nil, "", err
	}
	return lines, common.GenerateHMACWithKey(s.fingerprintKey, "new-api/donation-request/v1\x00"+string(encoded)), nil
}

func (s *DonationStore) SaveCampaign(ctx context.Context, value DonationCampaign, expectedVersion int) (DonationCampaign, error) {
	value.Name = strings.TrimSpace(value.Name)
	mode, modeOK := NormalizeDonationValidationMode(value.ValidationMode)
	if !modeOK {
		return value, ErrDonationInput
	}
	value.ValidationMode = mode
	if value.Name == "" || utf8.RuneCountInString(value.Name) > 120 || len(value.Description) > 4000 || value.GroupID == 0 || !validDonationQuota(value.RewardQuota) || len(value.TargetRevision) != 64 {
		return value, ErrDonationInput
	}
	var saved DonationCampaign
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := s.verifySecret(tx); err != nil {
			return err
		}
		var conn DonationConnection
		if err := lockForUpdate(tx).Select("id", "instance_id", "source_id").First(&conn, 1).Error; err != nil {
			return ErrDonationUnavailable
		}
		if conn.InstanceID != value.InstanceID || conn.SourceID != value.SourceID {
			return ErrDonationConflict
		}
		saved = value
		now := time.Now().UnixMilli()
		if value.ID != 0 {
			var old DonationCampaign
			if err := lockForUpdate(tx).First(&old, value.ID).Error; err != nil {
				return err
			}
			if old.Version != expectedVersion {
				return ErrDonationConflict
			}
			saved.Version, saved.CreatedAtMS = old.Version+1, old.CreatedAtMS
		} else {
			saved.Version, saved.CreatedAtMS = 1, now
		}
		saved.UpdatedAtMS = now
		if saved.ID == 0 {
			if err := tx.Create(&saved).Error; err != nil {
				return err
			}
		} else if err := tx.Save(&saved).Error; err != nil {
			return err
		}
		snapshot, err := common.Marshal(saved)
		if err != nil {
			return err
		}
		return tx.Create(&DonationCampaignRevision{CampaignID: saved.ID, Version: saved.Version, Snapshot: string(snapshot), CreatedAtMS: now}).Error
	})
	return saved, err
}

func (s *DonationStore) Campaign(ctx context.Context, id int) (DonationCampaign, error) {
	var campaign DonationCampaign
	err := s.DB.WithContext(ctx).First(&campaign, id).Error
	return campaign, err
}

func (s *DonationStore) Campaigns(ctx context.Context) ([]DonationCampaign, error) {
	items := make([]DonationCampaign, 0)
	err := s.DB.WithContext(ctx).Order("id desc").Find(&items).Error
	return items, err
}

func (s *DonationStore) FindBatchRequest(ctx context.Context, userID int, requestKey, digest string) (DonationBatch, error) {
	var batch DonationBatch
	err := s.DB.WithContext(ctx).Where("user_id = ? AND request_key = ?", userID, requestKey).First(&batch).Error
	if err == nil && (batch.RequestDigest != digest || batch.SecretID != s.secretID) {
		return batch, ErrDonationConflict
	}
	return batch, err
}

// PrepareBatch freezes the rules and acquires only undecided resources. Keys
// exist solely in the caller's memory; every persisted comparator is an HMAC.
func (s *DonationStore) PrepareBatch(ctx context.Context, userID int, requestKey, digest string, expected DonationCampaign, lines []DonationLine) (DonationBatch, error) {
	if userID <= 0 || !ValidDonationID(requestKey) || len(lines) == 0 || len(lines) > DonationMaxItems {
		return DonationBatch{}, ErrDonationInput
	}
	batchID := uuid.NewString()
	var saved DonationBatch
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		if err := s.verifySecret(tx); err != nil {
			return err
		}
		var replay DonationBatch
		err := tx.Where("user_id = ? AND request_key = ?", userID, requestKey).First(&replay).Error
		if err == nil {
			if replay.RequestDigest != digest {
				return ErrDonationConflict
			}
			saved = replay
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var campaign DonationCampaign
		if err := lockForUpdate(tx).First(&campaign, expected.ID).Error; err != nil {
			return ErrDonationUnavailable
		}
		if !campaign.Enabled {
			return ErrDonationUnavailable
		}
		if campaign.Version != expected.Version || campaign.TargetRevision != expected.TargetRevision || campaign.InstanceID != expected.InstanceID || campaign.SourceID != expected.SourceID {
			return ErrDonationConflict
		}
		var user User
		if err := tx.Select("id", "username", "linux_do_id", "status").First(&user, userID).Error; err != nil {
			return err
		}
		if user.Status != common.UserStatusEnabled {
			return ErrDonationAccount
		}
		now := time.Now().UnixMilli()
		mode, ok := NormalizeDonationValidationMode(campaign.ValidationMode)
		if !ok {
			return ErrDonationUnavailable
		}
		saved = DonationBatch{ID: batchID, UserID: userID, Username: user.Username, LinuxDOID: user.LinuxDOId,
			RequestKey: requestKey, RequestDigest: digest, SecretID: s.secretID, CampaignID: campaign.ID, CampaignVersion: campaign.Version,
			CampaignName: campaign.Name, GroupID: campaign.GroupID, GroupName: campaign.GroupName, InstanceID: campaign.InstanceID,
			SourceID: campaign.SourceID, TargetRevision: campaign.TargetRevision, RewardQuota: campaign.RewardQuota,
			ValidationMode: mode, ReceptionState: "local_only", CreatedAtMS: now, UpdatedAtMS: now}
		if err := tx.Create(&saved).Error; donationUniqueError(err) {
			return errDonationRace
		} else if err != nil {
			return err
		}
		// Acquire resources in a deterministic order across campaigns and replicas.
		ordered := append([]DonationLine(nil), lines...)
		slices.SortFunc(ordered, func(a, b DonationLine) int {
			if result := strings.Compare(a.Fingerprint, b.Fingerprint); result != 0 {
				return result
			}
			return a.Line - b.Line
		})
		seen := make(map[string]string)
		for _, line := range ordered {
			mask := "••••"
			if !line.Invalid && len(line.Key) > 12 {
				characters := []rune(line.Key)
				if len(characters) > 12 {
					mask += string(characters[len(characters)-4:])
				}
			}
			item := DonationItem{ID: line.ID, BatchID: batchID, Line: line.Line, Fingerprint: line.Fingerprint, KeyMask: mask,
				State: "unconfirmed", RewardState: "none", CreatedAtMS: now, UpdatedAtMS: now,
				EffectiveMode: mode, StagingExpiresAtMS: donationStagingDeadline(now)}
			if first, duplicate := seen[line.Fingerprint]; duplicate {
				item.State, item.ReasonCode, item.DuplicateOf = "duplicate", "duplicate_item", first
			} else if line.Invalid {
				item.State, item.ReasonCode = "invalid", "invalid_format"
			} else {
				var resource DonationResource
				err := lockForUpdate(tx).Where("fingerprint = ?", line.Fingerprint).First(&resource).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					resource = DonationResource{Fingerprint: line.Fingerprint, SecretID: s.secretID}
					if err := tx.Create(&resource).Error; donationUniqueError(err) {
						return errDonationRace
					} else if err != nil {
						return err
					}
				} else if err != nil {
					return err
				}
				if resource.SecretID != s.secretID {
					return ErrDonationSecret
				}
				switch {
				case resource.Acquired:
					item.State, item.ReasonCode, item.DuplicateOf = "duplicate", "already_donated", resource.OwnerItemID
				case resource.OwnerItemID != "":
					item.State, item.ReasonCode, item.DuplicateOf = "duplicate", "resource_processing", resource.OwnerItemID
				default:
					resource.OwnerItemID, resource.Generation, resource.UpdatedAtMS = item.ID, resource.Generation+1, now
					if err := tx.Save(&resource).Error; err != nil {
						return err
					}
					item.Dispatch, item.Generation = true, resource.Generation
					saved.ReceptionState, saved.NeedsRecovery, saved.NextPollAtMS = "unconfirmed", true, now
				}
			}
			seen[line.Fingerprint] = item.ID
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		return tx.Save(&saved).Error
	})
	return saved, err
}

type DonationBatchDetail struct {
	DonationBatch
	Items   []DonationItem  `json:"items"`
	Summary DonationSummary `json:"summary"`
}

type DonationSummary struct {
	Total         int   `json:"total"`
	Accepted      int   `json:"accepted"`
	Invalid       int   `json:"invalid"`
	Duplicate     int   `json:"duplicate"`
	Processing    int   `json:"processing"`
	PendingReview int   `json:"pending_review"`
	Rejected      int   `json:"rejected"`
	Rewarded      int   `json:"rewarded"`
	RewardedQuota int64 `json:"rewarded_quota"`
}

func (s *DonationStore) Batch(ctx context.Context, id string, userID int) (DonationBatchDetail, error) {
	var result DonationBatchDetail
	query := s.DB.WithContext(ctx).Where("id = ?", id)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.First(&result.DonationBatch).Error; err != nil {
		return result, err
	}
	result.Items = make([]DonationItem, 0)
	if err := s.DB.WithContext(ctx).Where("batch_id = ?", id).Order("line").Find(&result.Items).Error; err != nil {
		return result, err
	}
	// Only a confirmed rejection reason is projected to the donor; approval notes
	// and the administrative review/test ledger stay behind the admin boundary.
	notes, err := s.ReviewNotes(ctx, id)
	if err != nil {
		return result, err
	}
	for i := range result.Items {
		item := &result.Items[i]
		result.Summary.Total++
		switch item.State {
		case "accepted":
			result.Summary.Accepted++
		case "invalid":
			result.Summary.Invalid++
		case "existing", "duplicate":
			result.Summary.Duplicate++
		case "pending_review":
			// An item whose approval is already applied is mid-receive, not
			// waiting for a human.
			if item.ReviewState == "approved" {
				result.Summary.Processing++
			} else {
				result.Summary.PendingReview++
			}
		case "rejected":
			result.Summary.Rejected++
		default:
			result.Summary.Processing++
		}
		if note, ok := notes[item.ReviewActionID]; ok && item.State == "rejected" && item.ReviewDecision == DonationDecisionRejected {
			item.ReviewNote = note
		}
		if item.RewardState == "rewarded" {
			result.Summary.Rewarded++
			result.Summary.RewardedQuota += int64(item.RewardedQuota)
		}
	}
	return result, nil
}

func (s *DonationStore) Batches(ctx context.Context, userID, offset, limit int) ([]DonationBatchDetail, int64, error) {
	query := s.DB.WithContext(ctx).Model(&DonationBatch{}).Where("user_id = ?", userID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var ids []string
	if err := query.Order("created_at_ms desc, id desc").Offset(offset).Limit(limit).Pluck("id", &ids).Error; err != nil {
		return nil, 0, err
	}
	result := make([]DonationBatchDetail, 0, len(ids))
	for _, id := range ids {
		item, err := s.Batch(ctx, id, userID)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, item)
	}
	return result, total, nil
}

func (s *DonationStore) MarkBatchError(ctx context.Context, id, code string) error {
	return s.DB.WithContext(ctx).Model(&DonationBatch{}).Where("id = ?", id).Updates(map[string]any{"last_error": code, "updated_at_ms": time.Now().UnixMilli()}).Error
}

func (s *DonationStore) BeginSend(ctx context.Context, batchID string) (int, error) {
	var attempt int
	err := s.transaction(ctx, func(tx *gorm.DB) error {
		var batch DonationBatch
		if err := lockForUpdate(tx).Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		if batch.ReceptionState != "unconfirmed" {
			attempt = 0
			return nil
		}
		attempt = batch.SendAttempts + 1
		return tx.Model(&DonationBatch{}).Where("id = ?", batchID).Update("send_attempts", attempt).Error
	})
	return attempt, err
}

// A definite rejection of the sole first transmission proves it was never
// staged. Once another send began, or any send was uncertain, only a durable
// item receipt may release ownership; a later rejection is insufficient.
func (s *DonationStore) RejectFirstSend(ctx context.Context, batchID, reason string) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var batch DonationBatch
		if err := lockForUpdate(tx).Where("id = ?", batchID).First(&batch).Error; err != nil {
			return err
		}
		if batch.SendAttempts != 1 || batch.ReceptionState != "unconfirmed" {
			return nil
		}
		var items []DonationItem
		if err := tx.Where("batch_id = ? AND dispatch = ?", batchID, true).Order("fingerprint").Find(&items).Error; err != nil {
			return err
		}
		for _, item := range items {
			var resource DonationResource
			if err := lockForUpdate(tx).Where("fingerprint = ?", item.Fingerprint).First(&resource).Error; err != nil {
				return err
			}
			if resource.Acquired || resource.OwnerItemID != item.ID || resource.Generation != item.Generation {
				return ErrDonationConflict
			}
			if err := tx.Model(&DonationResource{}).Where("fingerprint = ?", item.Fingerprint).Update("owner_item_id", "").Error; err != nil {
				return err
			}
			if err := tx.Model(&DonationItem{}).Where("id = ?", item.ID).Updates(map[string]any{"state": "invalid", "reason_code": reason, "updated_at_ms": time.Now().UnixMilli()}).Error; err != nil {
				return err
			}
			if err := tx.Create(&DonationEvent{ItemID: item.ID, State: "invalid", ReasonCode: reason, CreatedAtMS: time.Now().UnixMilli()}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&DonationBatch{}).Where("id = ?", batchID).Updates(map[string]any{"reception_state": "local_only", "needs_recovery": false, "last_error": reason}).Error
	})
}

func (s *DonationStore) PrepareRetry(ctx context.Context, userID int, batchID, requestKey string, itemIDs []string) (DonationRetry, error) {
	if !ValidDonationID(requestKey) || !ValidDonationID(batchID) || len(itemIDs) > DonationMaxItems {
		return DonationRetry{}, ErrDonationInput
	}
	ids := append([]string{}, itemIDs...)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	for _, id := range ids {
		if !ValidDonationID(id) {
			return DonationRetry{}, ErrDonationInput
		}
	}
	encoded, err := common.Marshal(ids)
	if err != nil {
		return DonationRetry{}, err
	}
	digest := common.GenerateHMACWithKey(s.fingerprintKey, batchID+"\x00"+string(encoded))
	var saved DonationRetry
	err = s.transaction(ctx, func(tx *gorm.DB) error {
		var old DonationRetry
		err := tx.Where("user_id = ? AND request_key = ?", userID, requestKey).First(&old).Error
		if err == nil {
			if old.Digest != digest || old.BatchID != batchID {
				return ErrDonationConflict
			}
			saved = old
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var batch DonationBatch
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", batchID, userID).First(&batch).Error; err != nil {
			return err
		}
		if batch.ReceptionState != "confirmed" {
			return ErrDonationUnavailable
		}
		if len(ids) > 0 {
			var count int64
			if err := tx.Model(&DonationItem{}).Where("batch_id = ? AND id IN ? AND dispatch = ?", batchID, ids, true).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(ids)) {
				return ErrDonationInput
			}
		}
		saved = DonationRetry{ID: uuid.NewString(), UserID: userID, BatchID: batchID, RequestKey: requestKey, Digest: digest, ItemIDsJSON: string(encoded), CreatedAtMS: time.Now().UnixMilli()}
		if err := tx.Create(&saved).Error; donationUniqueError(err) {
			return errDonationRace
		} else if err != nil {
			return err
		}
		return tx.Model(&DonationBatch{}).Where("id = ?", batchID).Updates(map[string]any{"needs_recovery": true, "next_poll_at_ms": time.Now().UnixMilli()}).Error
	})
	return saved, err
}

func (s *DonationStore) PendingRetries(ctx context.Context, batchID string) ([]DonationRetry, error) {
	items := make([]DonationRetry, 0)
	err := s.DB.WithContext(ctx).Where("batch_id = ? AND done = ?", batchID, false).Order("created_at_ms, id").Limit(100).Find(&items).Error
	return items, err
}

func (s *DonationStore) FinishRetry(ctx context.Context, id string) error {
	return s.DB.WithContext(ctx).Model(&DonationRetry{}).Where("id = ?", id).Update("done", true).Error
}

func (s *DonationStore) ClaimRecovery(ctx context.Context, now int64) ([]DonationBatch, error) {
	var candidates []DonationBatch
	if err := s.DB.WithContext(ctx).Where("needs_recovery = ? AND next_poll_at_ms <= ? AND lease_until_ms <= ?", true, now, now).
		Order("next_poll_at_ms, id").Limit(1).Find(&candidates).Error; err != nil {
		return nil, err
	}
	claimed := make([]DonationBatch, 0, len(candidates))
	for _, batch := range candidates {
		token := newDonationLeaseToken()
		result := s.DB.WithContext(ctx).Model(&DonationBatch{}).Where("id = ? AND needs_recovery = ? AND next_poll_at_ms <= ? AND lease_until_ms <= ?", batch.ID, true, now, now).
			Updates(map[string]any{"lease_token": token, "lease_until_ms": now + donationHotLeaseMS})
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

// ReleaseRecovery re-derives the schedule from the state the reconciliation
// actually left behind: a batch that still needs recovery backs off, and one
// that went cold stops competing for hot leases immediately.
func (s *DonationStore) ReleaseRecovery(ctx context.Context, batch DonationBatch) error {
	return s.transaction(ctx, func(tx *gorm.DB) error {
		var current DonationBatch
		if err := lockForUpdate(tx).Where("id = ? AND lease_token = ?", batch.ID, batch.LeaseToken).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		updates := map[string]any{"lease_until_ms": 0, "lease_token": "", "poll_attempts": current.PollAttempts + 1}
		if current.NeedsRecovery {
			updates["next_poll_at_ms"] = time.Now().UnixMilli() + int64(min(60, 3*(current.PollAttempts+1)))*1000
		}
		return tx.Model(&DonationBatch{}).Where("id = ?", batch.ID).Updates(updates).Error
	})
}

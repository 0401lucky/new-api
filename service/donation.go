package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type DonationService struct{ Store *model.DonationStore }

func NewDonationService() (*DonationService, error) {
	store, err := model.OpenDonationStore(model.DB)
	if err != nil {
		return nil, err
	}
	return &DonationService{Store: store}, nil
}

type DonationConnectionView struct {
	BaseURL     string `json:"base_url"`
	Configured  bool   `json:"configured"`
	InstanceID  string `json:"instance_id"`
	SourceID    string `json:"source_id"`
	Version     int64  `json:"version"`
	UpdatedAtMS int64  `json:"updated_at_ms"`
}

func (s *DonationService) Connection(ctx context.Context) (DonationConnectionView, error) {
	conn, token, err := s.Store.Connection(ctx)
	return DonationConnectionView{BaseURL: conn.BaseURL, Configured: token != "", InstanceID: conn.InstanceID, SourceID: conn.SourceID, Version: conn.Version, UpdatedAtMS: conn.UpdatedAtMS}, err
}

func (s *DonationService) SaveConnection(ctx context.Context, baseURL string, token *string) (DonationConnectionView, error) {
	old, previousToken, err := s.Store.Connection(ctx)
	if err != nil {
		return DonationConnectionView{}, err
	}
	value := previousToken
	if token != nil && *token != "" {
		value = *token
	}
	client, err := newDonationClient(baseURL, value)
	if err != nil {
		return DonationConnectionView{}, err
	}
	defer client.close()
	caps, err := client.capabilities(ctx)
	if err != nil {
		return DonationConnectionView{}, err
	}
	if old.ID != 0 && (old.InstanceID != caps.InstanceID || old.SourceID != caps.SourceID) {
		return DonationConnectionView{}, &DonationRemoteError{Reason: "instance_mismatch"}
	}
	err = s.Store.SaveConnection(ctx, model.DonationConnection{BaseURL: client.baseURL, InstanceID: caps.InstanceID, SourceID: caps.SourceID}, value, old.Version)
	if err != nil {
		return DonationConnectionView{}, err
	}
	return s.Connection(ctx)
}

func (s *DonationService) checkedClient(ctx context.Context, instanceID, sourceID string) (*donationClient, model.DonationConnection, error) {
	client, conn, _, err := s.checkedClientCaps(ctx, instanceID, sourceID)
	return client, conn, err
}

// checkedClientCaps also returns the authenticated capabilities, which the
// manual-review negotiation needs on the same authenticated connection.
func (s *DonationService) checkedClientCaps(ctx context.Context, instanceID, sourceID string) (*donationClient, model.DonationConnection, DonationCapabilities, error) {
	conn, token, err := s.Store.Connection(ctx)
	if err != nil {
		return nil, conn, DonationCapabilities{}, err
	}
	if token == "" {
		return nil, conn, DonationCapabilities{}, model.ErrDonationUnavailable
	}
	if instanceID != "" && (instanceID != conn.InstanceID || sourceID != conn.SourceID) {
		return nil, conn, DonationCapabilities{}, &DonationRemoteError{Reason: "instance_mismatch"}
	}
	client, err := newDonationClient(conn.BaseURL, token)
	if err != nil {
		return nil, conn, DonationCapabilities{}, err
	}
	caps, err := client.capabilities(ctx)
	if err == nil && (caps.InstanceID != conn.InstanceID || caps.SourceID != conn.SourceID) {
		err = &DonationRemoteError{Reason: "instance_mismatch"}
	}
	if err != nil {
		client.close()
		return nil, conn, DonationCapabilities{}, err
	}
	return client, conn, caps, nil
}

func (s *DonationService) Groups(ctx context.Context) ([]DonationGroup, error) {
	client, _, err := s.checkedClient(ctx, "", "")
	if err != nil {
		return nil, err
	}
	defer client.close()
	return client.groups(ctx)
}

// availableDonationGroup selects the group capability that matches the
// requested validation mode. Manual review never borrows the probe capability.
func availableDonationGroup(groups []DonationGroup, id uint64, mode string) (DonationGroup, error) {
	for _, group := range groups {
		if group.ID != id {
			continue
		}
		if !group.Enabled || group.ConnectionType != "api_key" {
			return group, model.ErrDonationUnavailable
		}
		if mode == model.DonationModeManualReview {
			if !group.CanManualReview {
				return group, model.ErrDonationUnavailable
			}
			return group, nil
		}
		if !group.CanProbe {
			return group, model.ErrDonationUnavailable
		}
		return group, nil
	}
	return DonationGroup{}, model.ErrDonationUnavailable
}

// donationGroupRevision is the frozen target revision of the active mode.
func donationGroupRevision(group DonationGroup, mode string) string {
	if mode == model.DonationModeManualReview {
		return group.ManualTargetRevision
	}
	return group.TargetRevision
}

func (s *DonationService) SaveCampaign(ctx context.Context, value model.DonationCampaign, previous *model.DonationCampaign) (model.DonationCampaign, error) {
	mode, ok := model.NormalizeDonationValidationMode(value.ValidationMode)
	if !ok {
		return value, model.ErrDonationInput
	}
	value.ValidationMode = mode
	expectedVersion := 0
	if previous != nil {
		previousMode, previousOK := model.NormalizeDonationValidationMode(previous.ValidationMode)
		if !previousOK {
			return value, model.ErrDonationConflict
		}
		expectedVersion = previous.Version
		// Closing a campaign remains possible while its remote target is offline.
		if !value.Enabled && value.GroupID == previous.GroupID {
			value.InstanceID, value.SourceID, value.GroupName, value.TargetRevision = previous.InstanceID, previous.SourceID, previous.GroupName, previous.TargetRevision
			// A mode change is a capability claim and is never made offline.
			value.ValidationMode = previousMode
			return s.Store.SaveCampaign(ctx, value, expectedVersion)
		}
	}
	client, conn, caps, err := s.checkedClientCaps(ctx, "", "")
	if err != nil {
		return value, err
	}
	defer client.close()
	if mode == model.DonationModeManualReview && !supportsDonationManualReview(caps) {
		// Never silently downgrade to an unaudited reception.
		return value, &DonationRemoteError{Reason: "manual_review_unsupported"}
	}
	groups, err := client.groups(ctx)
	if err != nil {
		return value, err
	}
	group, err := availableDonationGroup(groups, value.GroupID, mode)
	if err != nil {
		return value, err
	}
	value.InstanceID, value.SourceID, value.GroupName = conn.InstanceID, conn.SourceID, group.Name
	value.TargetRevision = donationGroupRevision(group, mode)
	return s.Store.SaveCampaign(ctx, value, expectedVersion)
}

type DonationCampaignView struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	ValidationMode    string `json:"validation_mode"`
	RewardQuota       int    `json:"reward_quota"`
	Enabled           bool   `json:"enabled"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason"`
}

func (s *DonationService) Campaigns(ctx context.Context) ([]DonationCampaignView, error) {
	campaigns, err := s.Store.Campaigns(ctx)
	if err != nil {
		return nil, err
	}
	groups, groupErr := s.Groups(ctx)
	result := make([]DonationCampaignView, 0, len(campaigns))
	for _, campaign := range campaigns {
		mode, ok := model.NormalizeDonationValidationMode(campaign.ValidationMode)
		if !ok {
			return nil, model.ErrDonationConflict
		}
		view := DonationCampaignView{ID: campaign.ID, Name: campaign.Name, Description: campaign.Description,
			ValidationMode: mode, RewardQuota: campaign.RewardQuota, Enabled: campaign.Enabled}
		switch {
		case !campaign.Enabled:
			view.UnavailableReason = "campaign_closed"
		case groupErr != nil:
			view.UnavailableReason = "connection_unavailable"
		default:
			group, err := availableDonationGroup(groups, campaign.GroupID, mode)
			if err != nil {
				view.UnavailableReason = "target_unavailable"
			} else if donationGroupRevision(group, mode) != campaign.TargetRevision {
				view.UnavailableReason = "target_changed"
			} else {
				view.Available = true
			}
		}
		result = append(result, view)
	}
	return result, nil
}

func (s *DonationService) Submit(ctx context.Context, userID, campaignID int, requestKey, keysText string) (model.DonationBatchDetail, error) {
	if !model.ValidDonationID(requestKey) {
		return model.DonationBatchDetail{}, model.ErrDonationInput
	}
	lines, digest, err := s.Store.ParseLines(campaignID, keysText)
	if err != nil {
		return model.DonationBatchDetail{}, err
	}
	batch, err := s.Store.FindBatchRequest(ctx, userID, requestKey, digest)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		campaign, err := s.Store.Campaign(ctx, campaignID)
		if err != nil || !campaign.Enabled {
			return model.DonationBatchDetail{}, model.ErrDonationUnavailable
		}
		campaignMode, modeOK := model.NormalizeDonationValidationMode(campaign.ValidationMode)
		if !modeOK {
			return model.DonationBatchDetail{}, model.ErrDonationUnavailable
		}
		client, _, caps, err := s.checkedClientCaps(ctx, campaign.InstanceID, campaign.SourceID)
		if err != nil {
			return model.DonationBatchDetail{}, err
		}
		groups, groupErr := client.groups(ctx)
		client.close()
		if groupErr != nil {
			return model.DonationBatchDetail{}, groupErr
		}
		// A manual submission is never accepted from a receiver that stopped
		// announcing the contract: no silent downgrade to an unaudited intake.
		if campaignMode == model.DonationModeManualReview && !supportsDonationManualReview(caps) {
			return model.DonationBatchDetail{}, &DonationRemoteError{Reason: "manual_review_unsupported"}
		}
		group, err := availableDonationGroup(groups, campaign.GroupID, campaignMode)
		if err != nil {
			return model.DonationBatchDetail{}, err
		}
		if donationGroupRevision(group, campaignMode) != campaign.TargetRevision {
			return model.DonationBatchDetail{}, &DonationRemoteError{Reason: "target_changed"}
		}
		// Account for JSON escaping before acquiring any resource ownership. A
		// browser's keys_text byte length can understate the backend wire size.
		preview := donationIntakeBatch{BatchID: "00000000-0000-4000-8000-000000000000", GroupID: campaign.GroupID, TargetRevision: campaign.TargetRevision, ValidationMode: donationWireMode(campaignMode), Items: make([]donationIntakeItem, 0)}
		seen := make(map[string]bool)
		for _, line := range lines {
			if line.Invalid || seen[line.Fingerprint] {
				continue
			}
			seen[line.Fingerprint] = true
			preview.Items = append(preview.Items, donationIntakeItem{ItemID: line.ID, Key: line.Key})
		}
		wire, err := common.Marshal(preview)
		if err != nil || len(wire) > model.DonationMaxBodyBytes {
			return model.DonationBatchDetail{}, model.ErrDonationInput
		}
		batch, err = s.Store.PrepareBatch(ctx, userID, requestKey, digest, campaign, lines)
		if err != nil {
			return model.DonationBatchDetail{}, err
		}
	} else if err != nil {
		return model.DonationBatchDetail{}, err
	}
	if batch.ReceptionState == "local_only" {
		return s.Store.Batch(ctx, batch.ID, userID)
	}
	if batch.ReceptionState == "confirmed" {
		_ = s.ReconcileBatch(ctx, batch)
		return s.Store.Batch(ctx, batch.ID, userID)
	}
	client, _, err := s.checkedClient(ctx, batch.InstanceID, batch.SourceID)
	if err == nil {
		defer client.close()
		// Even the original caller queries first: a previous process may already
		// have staged this exact request and lost its response.
		receipt, getErr := client.batch(ctx, batch)
		var remote *DonationRemoteError
		if errors.As(getErr, &remote) && remote.Status == http.StatusNotFound {
			detail, readErr := s.Store.Batch(ctx, batch.ID, userID)
			if readErr != nil {
				return detail, readErr
			}
			byLine := make(map[int]model.DonationLine, len(lines))
			for _, line := range lines {
				byLine[line.Line] = line
			}
			request := donationIntakeBatch{BatchID: batch.ID, GroupID: batch.GroupID, TargetRevision: batch.TargetRevision,
				ValidationMode: donationWireMode(batch.ValidationMode), Items: make([]donationIntakeItem, 0)}
			for _, item := range detail.Items {
				if !item.Dispatch {
					continue
				}
				line, ok := byLine[item.Line]
				if !ok || line.Fingerprint != item.Fingerprint {
					return detail, model.ErrDonationConflict
				}
				request.Items = append(request.Items, donationIntakeItem{ItemID: item.ID, Key: line.Key})
			}
			attempt, sendErr := s.Store.BeginSend(ctx, batch.ID)
			if sendErr != nil {
				return detail, sendErr
			}
			if attempt == 0 {
				return s.Store.Batch(ctx, batch.ID, userID)
			}
			getErr = client.request(ctx, http.MethodPost, "/batches", batch.ID, request, &receipt)
			var rejection *DonationRemoteError
			if attempt == 1 && errors.As(getErr, &rejection) && rejection.RejectedBeforeStaging {
				if rejectErr := s.Store.RejectFirstSend(ctx, batch.ID, rejection.Reason); rejectErr != nil {
					return detail, rejectErr
				}
			}
			if getErr == nil && receipt.BatchID != batch.ID {
				getErr = model.ErrDonationReceipt
			}
			receipt.InstanceID, receipt.SourceID = batch.InstanceID, batch.SourceID
		}
		err = getErr
		if err == nil {
			err = s.Store.ApplyReceipt(ctx, receipt)
		}
	}
	if err != nil {
		_ = s.Store.MarkBatchError(ctx, batch.ID, donationRemoteReason(err))
	}
	_ = s.Store.SettleBatch(ctx, batch.ID)
	return s.Store.Batch(ctx, batch.ID, userID)
}

func (s *DonationService) Retry(ctx context.Context, userID int, batchID, requestKey string, ids []string) (model.DonationBatchDetail, error) {
	batch, err := s.Store.Batch(ctx, batchID, userID)
	if err != nil {
		return batch, err
	}
	if err := s.ReconcileBatch(ctx, batch.DonationBatch); err != nil {
		return batch, err
	}
	action, err := s.Store.PrepareRetry(ctx, userID, batchID, requestKey, ids)
	if err != nil {
		return batch, err
	}
	if !action.Done {
		_ = s.ReconcileBatch(ctx, batch.DonationBatch)
	}
	return s.Store.Batch(ctx, batchID, userID)
}

func (s *DonationService) ReconcileBatch(ctx context.Context, batch model.DonationBatch) error {
	// Settling an authenticated saved acceptance must not depend on the remote
	// credential still existing, or on the current campaign still being enabled.
	settleErr := s.Store.SettleBatch(ctx, batch.ID)
	detail, err := s.Store.Batch(ctx, batch.ID, 0)
	if err != nil {
		return err
	}
	actions, err := s.Store.PendingRetries(ctx, batch.ID)
	if err != nil {
		return err
	}
	reviews, err := s.Store.PendingReviewActions(ctx, batch.ID)
	if err != nil {
		return err
	}
	if detail.Summary.Processing == 0 && detail.Summary.PendingReview == 0 && len(actions) == 0 && len(reviews) == 0 {
		return settleErr
	}
	client, _, err := s.checkedClient(ctx, batch.InstanceID, batch.SourceID)
	if err != nil {
		_ = s.Store.MarkBatchError(ctx, batch.ID, donationRemoteReason(err))
		return err
	}
	defer client.close()
	receipt, err := client.batch(ctx, batch)
	if err == nil {
		err = s.Store.ApplyReceipt(ctx, receipt)
	}
	if err != nil {
		_ = s.Store.MarkBatchError(ctx, batch.ID, donationRemoteReason(err))
		return err
	}
	// A persisted pending intent is never released and never replaced by the
	// opposite decision: it is reconciled with its original action UUID only.
	for _, action := range reviews {
		if err := s.confirmReviewAction(ctx, client, action); err != nil {
			_ = s.Store.MarkBatchError(ctx, batch.ID, donationRemoteReason(err))
			return err
		}
	}
	for _, action := range actions {
		var ids []string
		if common.UnmarshalJsonStr(action.ItemIDsJSON, &ids) != nil {
			return model.ErrDonationConflict
		}
		var retried model.DonationReceipt
		err := client.request(ctx, http.MethodPost, "/batches/"+batch.ID+"/retry", action.ID, struct {
			ItemIDs []string `json:"item_ids,omitempty"`
		}{ids}, &retried)
		if err != nil {
			_ = s.Store.MarkBatchError(ctx, batch.ID, donationRemoteReason(err))
			return err
		}
		retried.InstanceID, retried.SourceID = batch.InstanceID, batch.SourceID
		if retried.BatchID != batch.ID {
			return model.ErrDonationReceipt
		}
		if err := s.Store.ApplyReceipt(ctx, retried); err != nil {
			return err
		}
		if err := s.Store.FinishRetry(ctx, action.ID); err != nil {
			return err
		}
	}
	return s.Store.SettleBatch(ctx, batch.ID)
}

// confirmReviewAction queries the original action, and only replays the request
// when the receiver never recorded it. A timeout, a 404 for other reasons, and a
// plain conflict all leave the intent pending.
func (s *DonationService) confirmReviewAction(ctx context.Context, client *donationClient, action model.DonationReviewAction) error {
	verified, err := client.reviewActionStatus(ctx, action.ActionID)
	var remote *DonationRemoteError
	if errors.As(err, &remote) && remote.Status == http.StatusNotFound {
		verified, err = client.reviewAction(ctx, action.BatchID, action.ItemID, action.ActionID, action.Kind, action.ExpectedItemRevision, action.ReviewTargetRevision, action.Note, action.ActorID)
	}
	if err != nil {
		return err
	}
	if verified.BatchID != action.BatchID || verified.ItemID != action.ItemID || verified.Kind != action.Kind ||
		verified.ExpectedItemRevision != action.ExpectedItemRevision || verified.ReviewTargetRevision != action.ReviewTargetRevision {
		return model.ErrDonationReceipt
	}
	_, err = s.Store.ApplyReviewOutcome(ctx, action, verified.Outcome, verified.ReasonCode, verified.EffectRevision, verified.ReviewTargetRevision, verified.AppliedAtMS)
	return err
}

func (s *DonationService) Recover(ctx context.Context) error {
	// A process that died mid-test closes its own attempt; it is never replayed.
	if err := s.Store.InterruptStaleTests(ctx, time.Now().UnixMilli()); err != nil {
		return err
	}
	var firstErr error
	// Claim immediately before work, never a whole queue's expiring leases.
	for range 32 {
		batches, err := s.Store.ClaimRecovery(ctx, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		if len(batches) == 0 {
			break
		}
		batch := batches[0]
		workCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		if err := s.ReconcileBatch(workCtx, batch); err != nil && firstErr == nil {
			firstErr = err
		}
		cancel()
		if err := s.Store.ReleaseRecovery(ctx, batch); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	// Pending review waits in a small, low-rate GET-only queue so it can never
	// delay a new approval or exhaust the hot work budget.
	for range 4 {
		cold, err := s.Store.ClaimColdRecovery(ctx, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		if len(cold) == 0 {
			break
		}
		batch := cold[0]
		workCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if err := s.reconcileCold(workCtx, batch); err != nil && firstErr == nil {
			firstErr = err
		}
		cancel()
		if err := s.Store.ReleaseColdRecovery(ctx, batch); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// reconcileCold only reads: no probe, no retry, and no model test.
func (s *DonationService) reconcileCold(ctx context.Context, batch model.DonationBatch) error {
	if err := s.Store.SettleBatch(ctx, batch.ID); err != nil {
		return err
	}
	detail, err := s.Store.Batch(ctx, batch.ID, 0)
	if err != nil {
		return err
	}
	if detail.Summary.Processing == 0 && detail.Summary.PendingReview == 0 {
		return nil
	}
	client, _, err := s.checkedClient(ctx, batch.InstanceID, batch.SourceID)
	if err != nil {
		return err
	}
	defer client.close()
	receipt, err := client.batch(ctx, batch)
	if err != nil {
		return err
	}
	if err := s.Store.ApplyReceipt(ctx, receipt); err != nil {
		return err
	}
	return s.Store.SettleBatch(ctx, batch.ID)
}

var donationRecoveryOnce sync.Once

func StartDonationRecovery() {
	donationRecoveryOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				service, err := NewDonationService()
				if err != nil {
					common.SysError("donation recovery identity unavailable")
					continue
				}
				if err := service.Recover(context.Background()); err != nil {
					common.SysError("donation recovery: " + donationRemoteReason(err))
				}
			}
		}()
	})
}

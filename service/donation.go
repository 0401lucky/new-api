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
	conn, token, err := s.Store.Connection(ctx)
	if err != nil {
		return nil, conn, err
	}
	if token == "" {
		return nil, conn, model.ErrDonationUnavailable
	}
	if instanceID != "" && (instanceID != conn.InstanceID || sourceID != conn.SourceID) {
		return nil, conn, &DonationRemoteError{Reason: "instance_mismatch"}
	}
	client, err := newDonationClient(conn.BaseURL, token)
	if err != nil {
		return nil, conn, err
	}
	caps, err := client.capabilities(ctx)
	if err == nil && (caps.InstanceID != conn.InstanceID || caps.SourceID != conn.SourceID) {
		err = &DonationRemoteError{Reason: "instance_mismatch"}
	}
	if err != nil {
		client.close()
		return nil, conn, err
	}
	return client, conn, nil
}

func (s *DonationService) Groups(ctx context.Context) ([]DonationGroup, error) {
	client, _, err := s.checkedClient(ctx, "", "")
	if err != nil {
		return nil, err
	}
	defer client.close()
	return client.groups(ctx)
}

func availableDonationGroup(groups []DonationGroup, id uint64) (DonationGroup, error) {
	for _, group := range groups {
		if group.ID != id {
			continue
		}
		if !group.Enabled || !group.CanProbe || group.ConnectionType != "api_key" {
			return group, model.ErrDonationUnavailable
		}
		return group, nil
	}
	return DonationGroup{}, model.ErrDonationUnavailable
}

func (s *DonationService) SaveCampaign(ctx context.Context, value model.DonationCampaign, previous *model.DonationCampaign) (model.DonationCampaign, error) {
	expectedVersion := 0
	if previous != nil {
		expectedVersion = previous.Version
		// Closing a campaign remains possible while its remote target is offline.
		if !value.Enabled && value.GroupID == previous.GroupID {
			value.InstanceID, value.SourceID, value.GroupName, value.TargetRevision = previous.InstanceID, previous.SourceID, previous.GroupName, previous.TargetRevision
			return s.Store.SaveCampaign(ctx, value, expectedVersion)
		}
	}
	client, conn, err := s.checkedClient(ctx, "", "")
	if err != nil {
		return value, err
	}
	defer client.close()
	groups, err := client.groups(ctx)
	if err != nil {
		return value, err
	}
	group, err := availableDonationGroup(groups, value.GroupID)
	if err != nil {
		return value, err
	}
	value.InstanceID, value.SourceID, value.GroupName, value.TargetRevision = conn.InstanceID, conn.SourceID, group.Name, group.TargetRevision
	return s.Store.SaveCampaign(ctx, value, expectedVersion)
}

type DonationCampaignView struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
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
		view := DonationCampaignView{ID: campaign.ID, Name: campaign.Name, Description: campaign.Description, RewardQuota: campaign.RewardQuota, Enabled: campaign.Enabled}
		switch {
		case !campaign.Enabled:
			view.UnavailableReason = "campaign_closed"
		case groupErr != nil:
			view.UnavailableReason = "connection_unavailable"
		default:
			group, err := availableDonationGroup(groups, campaign.GroupID)
			if err != nil {
				view.UnavailableReason = "target_unavailable"
			} else if group.TargetRevision != campaign.TargetRevision {
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
		client, _, err := s.checkedClient(ctx, campaign.InstanceID, campaign.SourceID)
		if err != nil {
			return model.DonationBatchDetail{}, err
		}
		groups, groupErr := client.groups(ctx)
		client.close()
		if groupErr != nil {
			return model.DonationBatchDetail{}, groupErr
		}
		group, err := availableDonationGroup(groups, campaign.GroupID)
		if err != nil {
			return model.DonationBatchDetail{}, err
		}
		if group.TargetRevision != campaign.TargetRevision {
			return model.DonationBatchDetail{}, &DonationRemoteError{Reason: "target_changed"}
		}
		// Account for JSON escaping before acquiring any resource ownership. A
		// browser's keys_text byte length can understate the backend wire size.
		preview := donationIntakeBatch{BatchID: "00000000-0000-4000-8000-000000000000", GroupID: campaign.GroupID, TargetRevision: campaign.TargetRevision, Items: make([]donationIntakeItem, 0)}
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
			request := donationIntakeBatch{BatchID: batch.ID, GroupID: batch.GroupID, TargetRevision: batch.TargetRevision, Items: make([]donationIntakeItem, 0)}
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
	if detail.Summary.Processing == 0 && len(actions) == 0 {
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

func (s *DonationService) Recover(ctx context.Context) error {
	batches, err := s.Store.ClaimRecovery(ctx, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	var firstErr error
	for _, batch := range batches {
		workCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		if err := s.ReconcileBatch(workCtx, batch); err != nil && firstErr == nil {
			firstErr = err
		}
		cancel()
		if err := s.Store.ReleaseRecovery(ctx, batch); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
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

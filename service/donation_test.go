package service

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const donationTestToken = "donation-test-token-only-1234567890"

type fakeDonationIntake struct {
	mu           sync.Mutex
	instance     string
	source       string
	server       *httptest.Server
	batches      map[string]model.DonationReceipt
	retries      map[string]bool
	posts        int
	retryPosts   int
	lostPost     bool
	lostRetry    bool
	hideReceipts bool
	rejectPosts  bool

	// Manual review state.
	noManualReview  bool
	manual          bool
	manualTarget    string
	actions         map[string]DonationReviewActionResult
	testPosts       int
	lostAction      bool
	lossBeforeApply bool
	actionPosts     []string
	testModel       string
	testReply       []string
	holdAcceptance  bool
	testEnv         []donationTestEnvironment
}

// donationTestEnvironment records what a controlled test request carried, so a
// test can prove which key, model, and target were actually used.
type donationTestEnvironment struct {
	Actor  string
	ItemID string
	TestID string
	Model  string
	Prompt string
	Target string
	Stream bool
}

func donationTestJSON(w http.ResponseWriter, value any) {
	encoded, err := common.Marshal(value)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(encoded)
}

func newFakeDonationIntake(t *testing.T) *fakeDonationIntake {
	t.Helper()
	f := &fakeDonationIntake{instance: uuid.NewString(), source: uuid.NewString(), batches: make(map[string]model.DonationReceipt),
		retries: make(map[string]bool), actions: make(map[string]DonationReviewActionResult),
		manualTarget: strings.Repeat("b", 64), testModel: "z-ai/glm-5.3-flash", testReply: []string{"ok"}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeDonationIntake) capabilities() DonationCapabilities {
	result := DonationCapabilities{ProtocolVersion: "1", InstanceID: f.instance, SourceID: f.source, InputTypes: []string{"api_key"}}
	result.Limits.MaxItems, result.Limits.MaxKeyBytes, result.Limits.MaxBodyBytes, result.Limits.StagingRetentionSeconds = 100, 4096, 1048576, 604800
	if !f.noManualReview {
		result.Features = []string{model.DonationFeatureManualReview}
		result.ReviewLimits.MaxNoteBytes = model.DonationMaxNoteBytes
	}
	return result
}

func (f *fakeDonationIntake) groups() []DonationGroup {
	groups := []DonationGroup{
		{ID: 1, Name: "Empty group", ChannelID: "gemini", ConnectionType: "api_key", Enabled: true, CanProbe: true, TargetRevision: strings.Repeat("a", 64)},
		{ID: 2, Name: "Other group", ChannelID: "gemini", ConnectionType: "api_key", Enabled: true, CanProbe: true, TargetRevision: strings.Repeat("a", 64)},
		{ID: 3, Name: "Disabled group", ChannelID: "gemini", ConnectionType: "api_key", Enabled: false, CanProbe: false, UnavailableReason: "group_disabled"},
	}
	if !f.noManualReview {
		for i := range groups {
			groups[i].CanManualReview = true
			groups[i].ManualTargetRevision = f.manualTarget
		}
	}
	return groups
}

func (f *fakeDonationIntake) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+donationTestToken {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/integrations/donations/v1")
	switch path {
	case "/capabilities":
		donationTestJSON(w, map[string]any{"code": 0, "data": f.capabilities()})
		return
	case "/groups":
		donationTestJSON(w, map[string]any{"code": 0, "data": f.groups()})
		return
	case "/batches":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		f.posts++
		if f.rejectPosts {
			w.WriteHeader(http.StatusConflict)
			donationTestJSON(w, map[string]any{"code": "DONATION_TARGET_CHANGED"})
			return
		}
		var request donationIntakeBatch
		if common.DecodeJson(r.Body, &request) != nil || r.Header.Get("Idempotency-Key") != request.BatchID {
			http.Error(w, "invalid", http.StatusBadRequest)
			return
		}
		if _, ok := f.batches[request.BatchID]; !ok {
			receipt := model.DonationReceipt{BatchID: request.BatchID, GroupID: request.GroupID, TargetRevision: request.TargetRevision, ValidationMode: request.ValidationMode, CreatedAtMS: time.Now().UnixMilli(), State: "processing"}
			manual := f.manual && request.ValidationMode == model.DonationModeManualReview
			for i, item := range request.Items {
				result := model.DonationReceiptItem{ItemID: item.ItemID, State: "accepted"}
				switch {
				case manual:
					result.State = "pending_review"
					result.ItemRevision = int64PtrValue(1)
					result.EffectiveMode = model.DonationModeManualReview
					result.ReviewTargetRevision = request.TargetRevision
					result.StagingExpiresAtMS = int64PtrValue(receipt.CreatedAtMS + model.DonationStagingRetentionSeconds*1000)
				case strings.HasPrefix(item.Key, "invalid"):
					result.State, result.ReasonCode = "invalid", "invalid_credential"
				case strings.HasPrefix(item.Key, "existing"):
					result.State, result.ReasonCode = "existing", "already_exists"
				case strings.HasPrefix(item.Key, "wait"):
					result.State, result.ReasonCode, result.Retryable = "retry_pending", "retry_exhausted", true
				default:
					id, at := uint64(100+i), int64(1789300000000)
					result.CredentialID, result.AcceptedAtMS = &id, &at
				}
				receipt.Items = append(receipt.Items, result)
			}
			f.batches[request.BatchID] = receipt
		}
		if f.lostPost {
			f.lostPost = false
			http.Error(w, "lost receipt", http.StatusInternalServerError)
			return
		}
		donationTestJSON(w, map[string]any{"code": 0, "data": f.batches[request.BatchID]})
		return
	}
	if !strings.HasPrefix(path, "/batches/") {
		if strings.HasPrefix(path, "/review-actions/") {
			actionID := strings.TrimPrefix(path, "/review-actions/")
			result, ok := f.actions[actionID]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				donationTestJSON(w, map[string]any{"code": "DONATION_REVIEW_NOT_FOUND"})
				return
			}
			donationTestJSON(w, map[string]any{"code": 0, "data": result})
			return
		}
		http.NotFound(w, r)
		return
	}
	// /batches/{id}/items/{item_id}/review-actions and /review-context.
	if rest, ok := strings.CutPrefix(path, "/batches/"); ok {
		if batchID, tail, found := strings.Cut(rest, "/items/"); found {
			itemID, verb, _ := strings.Cut(tail, "/")
			switch verb {
			case "review-context":
				receipt, exists := f.batches[batchID]
				if !exists {
					http.NotFound(w, r)
					return
				}
				for _, item := range receipt.Items {
					if item.ItemID != itemID {
						continue
					}
					mode, _ := model.NormalizeDonationValidationMode(item.EffectiveMode)
					action, target := "", item.ReviewTargetRevision
					if item.State == "pending_review" {
						action = model.DonationActionApprove
					} else if item.State == "retry_pending" && item.ReasonCode == "retry_exhausted" {
						action, target = model.DonationActionEnterReview, f.manualTarget
					}
					expiry := receipt.CreatedAtMS + model.DonationStagingRetentionSeconds*1000
					donationTestJSON(w, map[string]any{"code": 0, "data": DonationReviewContextView{
						BatchID: batchID, ItemID: itemID, GroupID: receipt.GroupID, State: item.State,
						EffectiveMode: mode, ItemRevision: donationRevision(item), ReviewAction: action,
						ReviewTargetRevision: target, ExpiresAtMS: expiry,
						CanReview: action != "", CanReject: item.State == "pending_review", CanTest: item.State == "pending_review",
						TestModels: []string{f.testModel}}})
					return
				}
				http.NotFound(w, r)
				return
			case "review-actions":
				f.handleReviewAction(w, r, batchID, itemID)
				return
			case "tests":
				f.handleTest(w, r, batchID, itemID)
				return
			}
			http.NotFound(w, r)
			return
		}
	}
	id := strings.TrimPrefix(path, "/batches/")
	if batchID, retry := strings.CutSuffix(id, "/retry"); retry {
		receipt, exists := f.batches[batchID]
		if !exists {
			http.NotFound(w, r)
			return
		}
		action := r.Header.Get("Idempotency-Key")
		if !model.ValidDonationID(action) {
			http.Error(w, "invalid action", http.StatusBadRequest)
			return
		}
		if !f.retries[action] {
			f.retries[action] = true
			f.retryPosts++
			for i := range receipt.Items {
				if receipt.Items[i].State != "retry_pending" {
					continue
				}
				id, at := uint64(i+1000), int64(1789300001000)
				receipt.Items[i].State, receipt.Items[i].ReasonCode, receipt.Items[i].Retryable = "accepted", "", false
				receipt.Items[i].CredentialID, receipt.Items[i].AcceptedAtMS = &id, &at
			}
			f.batches[batchID] = receipt
		}
		if f.lostRetry {
			f.lostRetry = false
			http.Error(w, "lost retry receipt", http.StatusInternalServerError)
			return
		}
		donationTestJSON(w, map[string]any{"code": 0, "data": receipt})
		return
	}
	receipt, exists := f.batches[id]
	if !exists || f.hideReceipts {
		http.NotFound(w, r)
		return
	}
	// An applied approval becomes a published acceptance on the next receipt.
	receipt = f.publish(receipt)
	f.batches[id] = receipt
	donationTestJSON(w, map[string]any{"code": 0, "data": receipt})
}

func int64PtrValue(value int64) *int64 { return &value }

func donationRevision(item model.DonationReceiptItem) int64 {
	if item.ItemRevision == nil {
		return 0
	}
	return *item.ItemRevision
}

// publish advances an approved staging key to a published acceptance, which is
// how the receiver reports the result of an applied approval. holdAcceptance
// keeps the acceptance unpublished so a test can prove the approval alone is
// not a payout condition.
func (f *fakeDonationIntake) publish(receipt model.DonationReceipt) model.DonationReceipt {
	if f.holdAcceptance {
		return receipt
	}
	for i := range receipt.Items {
		if receipt.Items[i].State != "committing" {
			continue
		}
		id, at := uint64(900+i), time.Now().UnixMilli()
		receipt.Items[i].State, receipt.Items[i].CredentialID, receipt.Items[i].AcceptedAtMS = "accepted", &id, &at
		receipt.Items[i].ReasonCode = ""
		receipt.Items[i].ItemRevision = int64PtrValue(donationRevision(receipt.Items[i]) + 1)
	}
	return receipt
}

func (f *fakeDonationIntake) handleReviewAction(w http.ResponseWriter, r *http.Request, batchID, itemID string) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	receipt, exists := f.batches[batchID]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		donationTestJSON(w, map[string]any{"code": "DONATION_ITEM_NOT_FOUND"})
		return
	}
	var request donationReviewActionRequest
	if common.DecodeJson(r.Body, &request) != nil || r.Header.Get("Idempotency-Key") != request.ActionID || request.Actor == "" {
		http.Error(w, "invalid", http.StatusBadRequest)
		return
	}
	if result, ok := f.actions[request.ActionID]; ok {
		donationTestJSON(w, map[string]any{"code": 0, "data": result})
		return
	}
	f.actionPosts = append(f.actionPosts, request.ActionID)
	if f.lostAction && f.lossBeforeApply {
		f.lostAction = false
		http.Error(w, "lost action response", http.StatusInternalServerError)
		return
	}
	index := -1
	for i := range receipt.Items {
		if receipt.Items[i].ItemID == itemID {
			index = i
		}
	}
	if index < 0 {
		w.WriteHeader(http.StatusNotFound)
		donationTestJSON(w, map[string]any{"code": "DONATION_ITEM_NOT_FOUND"})
		return
	}
	result := DonationReviewActionResult{ActionID: request.ActionID, BatchID: batchID, ItemID: itemID, Kind: request.Kind,
		ExpectedItemRevision: request.ExpectedItemRevision, ReviewTargetRevision: request.ReviewTargetRevision,
		Outcome: "applied", EffectRevision: donationRevision(receipt.Items[index]) + 1, AppliedAtMS: time.Now().UnixMilli()}
	item := &receipt.Items[index]
	item.ItemRevision = int64PtrValue(result.EffectRevision)
	switch request.Kind {
	case model.DonationActionApprove:
		item.State, item.ReasonCode = "committing", "runtime_pending"
		item.ReviewActionID, item.ReviewDecision = request.ActionID, "approved"
		item.ReviewedAtMS = int64PtrValue(result.AppliedAtMS)
		if request.ReviewTargetRevision != "" {
			item.ReviewTargetRevision = request.ReviewTargetRevision
		}
	case model.DonationActionReject:
		item.State, item.ReasonCode = "rejected", "review_rejected"
		item.ReviewActionID, item.ReviewDecision = request.ActionID, "rejected"
		item.ReviewedAtMS = int64PtrValue(result.AppliedAtMS)
	case model.DonationActionEnterReview:
		// The exhausted item is re-entered under the current manual target.
		item.State, item.ReasonCode, item.Retryable = "pending_review", "", false
		item.EffectiveMode = model.DonationModeManualReview
		item.ReviewTargetRevision = f.manualTarget
		item.EntryActionID = request.ActionID
		result.ReviewTargetRevision = f.manualTarget
	}
	f.batches[batchID] = receipt
	f.actions[request.ActionID] = result
	if f.lostAction {
		f.lostAction = false
		http.Error(w, "lost applied action response", http.StatusInternalServerError)
		return
	}
	donationTestJSON(w, map[string]any{"code": 0, "data": result})
}

func (f *fakeDonationIntake) handleTest(w http.ResponseWriter, r *http.Request, batchID, itemID string) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var request donationTestRequest
	if common.DecodeJson(r.Body, &request) != nil || r.Header.Get("Idempotency-Key") != request.TestID || request.Actor == "" {
		http.Error(w, "invalid", http.StatusBadRequest)
		return
	}
	f.testPosts++
	f.testEnv = append(f.testEnv, donationTestEnvironment{Actor: request.Actor, ItemID: itemID, TestID: request.TestID, Model: request.Model,
		Prompt: request.Prompt, Target: request.ReviewTargetRevision, Stream: request.Stream})
	text := strings.Join(f.testReply, "")
	if request.Stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writer := w.(http.Flusher)
		_, _ = fmt.Fprintf(w, "event: meta\ndata: %s\n\n", mustDonationJSON(donationTestMeta{TestID: request.TestID, BatchID: batchID, ItemID: itemID,
			Model: request.Model, StartRevision: request.ExpectedItemRevision, TargetRevision: request.ReviewTargetRevision, StartedAtMS: time.Now().UnixMilli()}))
		writer.Flush()
		for _, piece := range f.testReply {
			_, _ = fmt.Fprintf(w, "event: delta\ndata: %s\n\n", mustDonationJSON(struct {
				Text string `json:"text"`
			}{piece}))
			writer.Flush()
		}
		_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", mustDonationJSON(donationTestDone{State: model.DonationTestSucceeded,
			StatusCode: http.StatusOK, FinishedAtMS: time.Now().UnixMilli(), OutputBytes: len(text),
			Usage: &donationTestUsage{InputTokens: 12, OutputTokens: 34}}))
		writer.Flush()
		return
	}
	donationTestJSON(w, map[string]any{"code": 0, "data": DonationTestResult{TestID: request.TestID, BatchID: batchID, ItemID: itemID,
		Model: request.Model, State: model.DonationTestSucceeded, StatusCode: http.StatusOK,
		StartRevision: request.ExpectedItemRevision, TargetRevision: request.ReviewTargetRevision,
		StartedAtMS: time.Now().UnixMilli(), FinishedAtMS: time.Now().UnixMilli(), OutputBytes: len(text), Text: text,
		Usage: &donationTestUsage{InputTokens: 12, OutputTokens: 34}}})
}

// donationManualServiceFixture builds the service with a manual-review campaign
// already saved, so a test starts from a submitted pending-review item.
func donationManualServiceFixture(t *testing.T, fake *fakeDonationIntake) (*DonationService, model.User, model.DonationCampaign) {
	t.Helper()
	svc, user, _ := donationServiceFixture(t, fake)
	fake.mu.Lock()
	fake.manual = true
	fake.mu.Unlock()
	campaign, err := svc.SaveCampaign(t.Context(), model.DonationCampaign{Name: "Manual keys", GroupID: 1,
		RewardQuota: 25, Enabled: true, ValidationMode: model.DonationModeManualReview}, nil)
	require.NoError(t, err)
	require.Equal(t, model.DonationModeManualReview, campaign.ValidationMode)
	return svc, user, campaign
}

func submitManualBatch(t *testing.T, svc *DonationService, userID, campaignID int, text string) model.DonationBatchDetail {
	t.Helper()
	batch, err := svc.Submit(t.Context(), userID, campaignID, uuid.NewString(), text)
	require.NoError(t, err)
	require.Equal(t, "confirmed", batch.ReceptionState)
	return batch
}

func donationServiceFixture(t *testing.T, fake *fakeDonationIntake) (*DonationService, model.User, model.DonationCampaign) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "new-api.db")+"?_pragma=busy_timeout(30000)&_txlock=immediate"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	require.NoError(t, model.MigrateDonations(db))
	oldDB, oldLog, oldRedis, oldType := model.DB, model.LOG_DB, common.RedisEnabled, common.MainDatabaseType()
	model.DB, model.LOG_DB, common.RedisEnabled = db, nil, false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = oldDB, oldLog, oldRedis
		common.SetMainDatabaseType(oldType)
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
	user := model.User{Username: "donor", AffCode: "donor", Password: "synthetic", Quota: 100, Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, db.Create(&user).Error)
	svc, err := NewDonationService()
	require.NoError(t, err)
	token := donationTestToken
	view, err := svc.SaveConnection(t.Context(), fake.server.URL, &token)
	require.NoError(t, err)
	encoded, err := common.Marshal(view)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), token)
	var connection model.DonationConnection
	require.NoError(t, db.First(&connection).Error)
	assert.NotContains(t, connection.TokenCipher, token)
	campaign, err := svc.SaveCampaign(t.Context(), model.DonationCampaign{Name: "Community keys", GroupID: 1, RewardQuota: 25, Enabled: true}, nil)
	require.NoError(t, err)
	return svc, user, campaign
}

func TestDonationServiceDurableRecovery(t *testing.T) {
	fake := newFakeDonationIntake(t)
	svc, user, campaign := donationServiceFixture(t, fake)
	fake.lostPost = true
	requestKey := uuid.NewString()
	batch, err := svc.Submit(t.Context(), user.Id, campaign.ID, requestKey, "\n valid-donation-key \r\ninvalid-donation-key\nvalid-donation-key\nexisting-donation-key\nwait-donation-key\na b\n")
	require.NoError(t, err)
	assert.Equal(t, "unconfirmed", batch.ReceptionState)
	assert.Zero(t, batch.Summary.RewardedQuota)
	assert.Equal(t, 1, fake.posts)
	closed := campaign
	closed.Enabled = false
	closed.RewardQuota = 50
	_, err = svc.SaveCampaign(t.Context(), closed, &campaign)
	require.NoError(t, err)
	require.NoError(t, svc.Store.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("status", common.UserStatusDisabled).Error)
	restarted, err := NewDonationService()
	require.NoError(t, err)
	require.NoError(t, restarted.ReconcileBatch(t.Context(), batch.DonationBatch))
	batch, err = restarted.Store.Batch(t.Context(), batch.ID, user.Id)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", batch.ReceptionState)
	assert.Equal(t, "accepted", batch.Items[0].State)
	assert.Equal(t, "paused", batch.Items[0].RewardState)
	assert.Equal(t, "invalid", batch.Items[1].State)
	assert.Equal(t, "duplicate", batch.Items[2].State)
	assert.Equal(t, "existing", batch.Items[3].State)
	assert.Equal(t, "retry_pending", batch.Items[4].State)
	assert.Equal(t, "invalid", batch.Items[5].State)
	require.NoError(t, svc.Store.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("status", common.UserStatusEnabled).Error)
	require.NoError(t, restarted.Recover(t.Context()))
	batch, err = restarted.Store.Batch(t.Context(), batch.ID, user.Id)
	require.NoError(t, err)
	assert.EqualValues(t, 25, batch.Summary.RewardedQuota)
	// A retry action survives its lost response and is replayed using exactly its
	// original remote action UUID; accepted items never receive another reward.
	fake.lostRetry = true
	retryKey := uuid.NewString()
	_, err = restarted.Retry(t.Context(), user.Id, batch.ID, retryKey, nil)
	require.NoError(t, err)
	require.NoError(t, restarted.ReconcileBatch(t.Context(), batch.DonationBatch))
	batch, err = restarted.Retry(t.Context(), user.Id, batch.ID, retryKey, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 50, batch.Summary.RewardedQuota)
	assert.Equal(t, 1, fake.retryPosts)
	assert.Equal(t, 1, fake.posts)
	_, err = restarted.Retry(t.Context(), user.Id, batch.ID, retryKey, []string{batch.Items[4].ID})
	assert.ErrorIs(t, err, model.ErrDonationConflict)
	var saved model.User
	require.NoError(t, svc.Store.DB.First(&saved, user.Id).Error)
	assert.Equal(t, 150, saved.Quota)
	other := model.User{Username: "other", AffCode: "other", Password: "synthetic", Status: common.UserStatusEnabled, AuthVersion: 1}
	require.NoError(t, svc.Store.DB.Create(&other).Error)
	second, err := restarted.SaveCampaign(t.Context(), model.DonationCampaign{Name: "Different display name", GroupID: 2, RewardQuota: 99, Enabled: true}, nil)
	require.NoError(t, err)
	duplicate, err := restarted.Submit(t.Context(), other.Id, second.ID, uuid.NewString(), " valid-donation-key \nexisting-donation-key")
	require.NoError(t, err)
	assert.Equal(t, "local_only", duplicate.ReceptionState)
	assert.Equal(t, 2, duplicate.Summary.Duplicate)
	assert.Zero(t, duplicate.Summary.RewardedQuota)
	// Several distinct keys have no user/day reward count gate; case is retained.
	newKeys, err := restarted.Submit(t.Context(), user.Id, second.ID, uuid.NewString(), "unique-one\nunique-two\nUnique-two")
	require.NoError(t, err)
	assert.Equal(t, 3, newKeys.Summary.Rewarded)
	assert.EqualValues(t, 297, newKeys.Summary.RewardedQuota)
}

func TestDonationUncertainAndRejectedRequests(t *testing.T) {
	fake := newFakeDonationIntake(t)
	svc, user, campaign := donationServiceFixture(t, fake)
	fake.rejectPosts = true
	rejected, err := svc.Submit(t.Context(), user.Id, campaign.ID, uuid.NewString(), "target-race-first-key")
	require.NoError(t, err)
	assert.Equal(t, "local_only", rejected.ReceptionState)
	assert.Equal(t, "target_changed", rejected.Items[0].ReasonCode)
	fake.rejectPosts = false
	accepted, err := svc.Submit(t.Context(), user.Id, campaign.ID, uuid.NewString(), "target-race-first-key")
	require.NoError(t, err)
	assert.Equal(t, 1, accepted.Summary.Rewarded)
	fake.lostPost = true
	requestKey := uuid.NewString()
	unknown, err := svc.Submit(t.Context(), user.Id, campaign.ID, requestKey, "uncertain-remote-key")
	require.NoError(t, err)
	assert.Equal(t, "unconfirmed", unknown.ReceptionState)
	fake.hideReceipts, fake.rejectPosts = true, true
	unknown, err = svc.Submit(t.Context(), user.Id, campaign.ID, requestKey, "uncertain-remote-key")
	require.NoError(t, err)
	assert.Equal(t, "unconfirmed", unknown.ReceptionState)
	require.Equal(t, 2, unknown.SendAttempts)
	duplicate, err := svc.Submit(t.Context(), user.Id, campaign.ID, uuid.NewString(), "uncertain-remote-key")
	require.NoError(t, err)
	assert.Equal(t, "resource_processing", duplicate.Items[0].ReasonCode)
	fake.hideReceipts, fake.rejectPosts = false, false
	require.NoError(t, svc.ReconcileBatch(t.Context(), unknown.DonationBatch))
	unknown, err = svc.Store.Batch(t.Context(), unknown.ID, user.Id)
	require.NoError(t, err)
	assert.Equal(t, 1, unknown.Summary.Rewarded)
	// Oversized encoded JSON is rejected before creating any batch or resource.
	var before int64
	require.NoError(t, svc.Store.DB.Model(&model.DonationBatch{}).Count(&before).Error)
	keys := make([]string, 100)
	for i := range keys {
		keys[i] = strings.Repeat("<", 4000) + strconv.Itoa(i)
	}
	_, err = svc.Submit(t.Context(), user.Id, campaign.ID, uuid.NewString(), strings.Join(keys, "\n"))
	assert.ErrorIs(t, err, model.ErrDonationInput)
	var after int64
	require.NoError(t, svc.Store.DB.Model(&model.DonationBatch{}).Count(&after).Error)
	assert.Equal(t, before, after)
	fake.mu.Lock()
	fake.instance = uuid.NewString()
	fake.mu.Unlock()
	_, err = svc.Groups(t.Context())
	assert.EqualError(t, err, "donation integration: instance_mismatch")
}

func TestDonationClientBoundaries(t *testing.T) {
	for _, baseURL := range []string{"http://10.0.0.1", "http://example.com", "https://user:password@example.com", "https://example.com?token=x", "https://example.com#x", "https://example.com/token", "ftp://example.com", " https://example.com", "https://example.com\\secret"} {
		t.Run(baseURL, func(t *testing.T) {
			_, err := newDonationClient(baseURL, donationTestToken)
			assert.ErrorIs(t, err, model.ErrDonationInput)
		})
	}
	fake := newFakeDonationIntake(t)
	// The donation credential must not inherit the relay's insecure TLS mode.
	untrustedTLS := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		donationTestJSON(w, map[string]any{"code": 0, "data": fake.capabilities()})
	}))
	defer untrustedTLS.Close()
	previousTransport := http.DefaultTransport
	http.DefaultTransport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // synthetic relay setting
	secureClient, err := newDonationClient(untrustedTLS.URL, donationTestToken)
	http.DefaultTransport = previousTransport
	require.NoError(t, err)
	_, err = secureClient.capabilities(t.Context())
	assert.Error(t, err)
	secureClient.close()
	for _, response := range []any{
		map[string]any{"success": true, "data": fake.capabilities()},
		map[string]any{"code": "0", "data": fake.capabilities()},
		map[string]any{"code": 0, "data": map[string]any{"protocol_version": 1}},
		map[string]any{"code": 0, "data": nil},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { donationTestJSON(w, response) }))
		client, err := newDonationClient(server.URL, donationTestToken)
		require.NoError(t, err)
		_, err = client.capabilities(t.Context())
		assert.Error(t, err)
		client.close()
		server.Close()
	}
	var leaked atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client, err := newDonationClient(redirect.URL, donationTestToken)
	require.NoError(t, err)
	_, err = client.capabilities(t.Context())
	assert.Error(t, err)
	assert.Zero(t, leaked.Load())
	client.close()
	// Reuse the capabilities connection, then drop it after reading the POST.
	// Transport's implicit Idempotency-Key replay would submit this body twice.
	var posts atomic.Int32
	dropped := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			donationTestJSON(w, map[string]any{"code": 0, "data": fake.capabilities()})
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		posts.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			panic(err)
		}
		_ = conn.Close()
	}))
	defer dropped.Close()
	client, err = newDonationClient(dropped.URL, donationTestToken)
	require.NoError(t, err)
	_, err = client.capabilities(t.Context())
	require.NoError(t, err)
	var output model.DonationReceipt
	err = client.request(context.Background(), http.MethodPost, "/batches", uuid.NewString(), map[string]string{"key": "synthetic-key"}, &output)
	assert.Error(t, err)
	assert.EqualValues(t, 1, posts.Load(), fmt.Sprintf("one persisted send must be one transport send, observed %d", posts.Load()))
	client.close()
}

func donationUserQuota(t *testing.T, svc *DonationService, userID int) int {
	t.Helper()
	var user model.User
	require.NoError(t, svc.Store.DB.Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func storageCount[T any](t *testing.T, svc *DonationService) int64 {
	t.Helper()
	var count int64
	require.NoError(t, svc.Store.DB.Model(new(T)).Count(&count).Error)
	return count
}

// TestDonationManualReviewFlowAndReward covers the whole manual path: a pending
// item holds no credential and no reward, an applied approval plus the matching
// acceptance pays exactly once, and a replay never repeats either.
func TestDonationManualReviewFlowAndReward(t *testing.T) {
	fake := newFakeDonationIntake(t)
	svc, user, campaign := donationManualServiceFixture(t, fake)
	batch := submitManualBatch(t, svc, user.Id, campaign.ID, "manual-key-0001")
	require.Len(t, batch.Items, 1)
	assert.Equal(t, 1, batch.Summary.PendingReview)
	assert.Zero(t, batch.Summary.Processing)
	assert.Zero(t, batch.Summary.Rewarded)
	assert.Zero(t, batch.Summary.RewardedQuota)
	item := batch.Items[0]

	// A pending item neither holds a formal credential nor consumes a reward.
	approved, err := svc.Store.CreditDonationReward(t.Context(), item.ID)
	require.Error(t, err)
	assert.False(t, approved)

	before := donationUserQuota(t, svc, user.Id)
	view, err := svc.ReviewContext(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, batch.ID, view.BatchID)
	assert.True(t, view.CanReview)
	assert.True(t, view.CanTest)
	assert.Equal(t, []string{fake.testModel}, view.TestModels)

	// The revision and target the receiver reported are recorded locally.
	var stored model.DonationItem
	require.NoError(t, svc.Store.DB.Where("id = ?", item.ID).First(&stored).Error)
	assert.Equal(t, view.ItemRevision, stored.ItemRevision)
	assert.Equal(t, batch.TargetRevision, stored.ReviewTargetRevision)
	assert.Equal(t, model.DonationModeManualReview, stored.EffectiveMode)

	// The receiver has not published the acceptance yet, so the applied approval
	// is the only fact: on its own it must never pay.
	fake.mu.Lock()
	fake.holdAcceptance = true
	fake.mu.Unlock()
	actionID := uuid.NewString()
	input := DonationReviewInput{Kind: model.DonationActionApprove, ExpectedItemRevision: view.ItemRevision,
		ReviewTargetRevision: batch.TargetRevision, Note: "checked the key by hand"}
	saved, err := svc.Review(t.Context(), 7, item.ID, actionID, input)
	require.NoError(t, err)
	assert.Equal(t, model.DonationActionApplied, saved.Status)
	assert.Equal(t, 7, saved.ActorID)
	assert.Equal(t, "checked the key by hand", saved.Note)

	require.NoError(t, svc.ReconcileBatch(t.Context(), batch.DonationBatch))
	detail, err := svc.Store.Batch(t.Context(), batch.ID, user.Id)
	require.NoError(t, err)
	assert.Equal(t, "committing", detail.Items[0].State)
	assert.Zero(t, detail.Summary.RewardedQuota)
	assert.Equal(t, before, donationUserQuota(t, svc, user.Id))

	// The receiver then publishes the acceptance for the approved item.
	fake.mu.Lock()
	fake.holdAcceptance = false
	fake.mu.Unlock()
	require.NoError(t, svc.ReconcileBatch(t.Context(), batch.DonationBatch))
	detail, err = svc.Store.Batch(t.Context(), batch.ID, user.Id)
	require.NoError(t, err)
	assert.Equal(t, "accepted", detail.Items[0].State)
	assert.Equal(t, 1, detail.Summary.Accepted)
	assert.EqualValues(t, 25, detail.Summary.RewardedQuota)
	assert.Equal(t, before+25, donationUserQuota(t, svc, user.Id))
	assert.Equal(t, int64(1), storageCount[model.DonationReward](t, svc))

	// Replaying the same action UUID and reconciling again pays nothing more.
	replayed, err := svc.Review(t.Context(), 7, item.ID, actionID, input)
	require.NoError(t, err)
	assert.Equal(t, model.DonationActionApplied, replayed.Status)
	require.NoError(t, svc.ReconcileBatch(t.Context(), batch.DonationBatch))
	assert.Equal(t, before+25, donationUserQuota(t, svc, user.Id))
	assert.Equal(t, int64(1), storageCount[model.DonationReward](t, svc))

	// A different meaning under the same UUID is refused instead of overwriting.
	_, err = svc.Review(t.Context(), 7, item.ID, actionID, DonationReviewInput{Kind: model.DonationActionReject,
		ExpectedItemRevision: view.ItemRevision, Note: "changed my mind"})
	assert.ErrorIs(t, err, model.ErrDonationConflict)
}

// TestDonationManualModeRequiresReceiverCapability proves the mode is never
// enabled, and never silently downgraded, without the announced feature.
func TestDonationManualModeRequiresReceiverCapability(t *testing.T) {
	fake := newFakeDonationIntake(t)
	svc, _, saved := donationServiceFixture(t, fake)

	fake.mu.Lock()
	fake.noManualReview = true
	fake.mu.Unlock()
	_, err := svc.SaveCampaign(t.Context(), model.DonationCampaign{Name: "Manual keys", GroupID: 1,
		RewardQuota: 25, Enabled: true, ValidationMode: model.DonationModeManualReview}, nil)
	var remote *DonationRemoteError
	require.ErrorAs(t, err, &remote)
	assert.Equal(t, "manual_review_unsupported", remote.Reason)

	// An empty mode is the released representation of auto, and an unknown mode
	// is refused outright.
	automatic, err := svc.SaveCampaign(t.Context(), model.DonationCampaign{Name: "Auto keys", GroupID: 1, RewardQuota: 25, Enabled: true}, nil)
	require.NoError(t, err)
	assert.Equal(t, model.DonationModeAuto, automatic.ValidationMode)
	_, err = svc.SaveCampaign(t.Context(), model.DonationCampaign{Name: "Bad keys", GroupID: 1, RewardQuota: 25,
		Enabled: true, ValidationMode: "occasionally"}, nil)
	assert.ErrorIs(t, err, model.ErrDonationInput)

	// A manual campaign saved while the capability was announced cannot accept a
	// new submission once the receiver stops announcing it.
	fake.mu.Lock()
	fake.noManualReview = false
	fake.mu.Unlock()
	manual, err := svc.SaveCampaign(t.Context(), model.DonationCampaign{Name: "Manual keys", GroupID: 1,
		RewardQuota: 25, Enabled: true, ValidationMode: model.DonationModeManualReview}, nil)
	require.NoError(t, err)
	require.Equal(t, model.DonationModeManualReview, manual.ValidationMode)
	fake.mu.Lock()
	fake.noManualReview = true
	fake.mu.Unlock()
	_, err = svc.Submit(t.Context(), 1, manual.ID, uuid.NewString(), "manual-submit-after-downgrade")
	require.ErrorAs(t, err, &remote)
	assert.Equal(t, "manual_review_unsupported", remote.Reason)
	fake.mu.Lock()
	fake.noManualReview = false
	fake.mu.Unlock()

	// Disabling an existing campaign keeps its stored mode even when the request
	// claims a different one, so the offline close path cannot activate a mode.
	closed := saved
	closed.Enabled = false
	closed.ValidationMode = model.DonationModeManualReview
	kept, err := svc.SaveCampaign(t.Context(), closed, &saved)
	require.NoError(t, err)
	assert.Equal(t, model.DonationModeAuto, kept.ValidationMode)
	assert.False(t, kept.Enabled)
}

// TestDonationTestProxyRedactsAndNeverReplays proves the controlled test uses
// exactly this item's key and model, that a secret split across events never
// reaches the caller, and that a replayed test ID never calls upstream twice.
func TestDonationTestProxyRedactsAndNeverReplays(t *testing.T) {
	fake := newFakeDonationIntake(t)
	svc, user, campaign := donationManualServiceFixture(t, fake)
	batch := submitManualBatch(t, svc, user.Id, campaign.ID, "manual-test-key-0001")
	item := batch.Items[0]
	view, err := svc.ReviewContext(t.Context(), item.ID)
	require.NoError(t, err)

	// The upstream echoes the staging key back, split across two events.
	secret := "sk-" + strings.Repeat("A", 40)
	fake.mu.Lock()
	fake.testReply = []string{"here is ", secret[:20], secret[20:], " done"}
	fake.mu.Unlock()

	input := DonationTestInput{ExpectedItemRevision: view.ItemRevision, ReviewTargetRevision: batch.TargetRevision,
		Model: fake.testModel, Prompt: "ping", MaxOutputTokens: 64}
	testID := uuid.NewString()
	attempt, source, created, err := svc.BeginTest(t.Context(), 7, item.ID, testID, input)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, model.DonationTestRunning, attempt.State)
	result, err := svc.RunTest(t.Context(), attempt, source, input)
	require.NoError(t, err)
	assert.Equal(t, model.DonationTestSucceeded, result.State)
	assert.Contains(t, result.Text, "here is")
	assert.NotContains(t, result.Text, secret)
	assert.NotContains(t, result.Text, secret[:20]+secret[20:])

	// Exactly one request reached the receiver, for this item, model, and target.
	require.Len(t, fake.testEnv, 1)
	assert.Equal(t, item.ID, fake.testEnv[0].ItemID)
	assert.Equal(t, fake.testModel, fake.testEnv[0].Model)
	assert.Equal(t, "ping", fake.testEnv[0].Prompt)
	assert.Equal(t, batch.TargetRevision, fake.testEnv[0].Target)

	// The main ledger keeps metadata only: no prompt and no key.
	var stored model.DonationTestAttempt
	require.NoError(t, svc.Store.DB.Where("test_id = ?", testID).First(&stored).Error)
	assert.Equal(t, model.DonationTestSucceeded, stored.State)
	assert.Equal(t, len("ping"), stored.PromptBytes)
	assert.Equal(t, 12, stored.InputTokens)
	assert.Equal(t, 34, stored.OutputTokens)
	assert.NotContains(t, stored.PromptDigest, "ping")
	assert.NotContains(t, stored.PromptDigest, "manual-test-key-0001")

	// A replayed test ID returns its metadata and never calls upstream again.
	replay, _, created, err := svc.BeginTest(t.Context(), 7, item.ID, testID, input)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, model.DonationTestSucceeded, replay.State)
	assert.Equal(t, 1, fake.testPosts)
	// A different meaning under the same test ID is refused.
	_, _, _, err = svc.BeginTest(t.Context(), 7, item.ID, testID, DonationTestInput{ExpectedItemRevision: view.ItemRevision,
		ReviewTargetRevision: batch.TargetRevision, Model: fake.testModel, Prompt: "different", MaxOutputTokens: 64})
	assert.ErrorIs(t, err, model.ErrDonationConflict)

	// The streaming projection forwards the same allowlisted protocol and applies
	// the same redaction.
	streamInput := input
	streamInput.Stream = true
	streamID := uuid.NewString()
	streamAttempt, streamSource, _, err := svc.BeginTest(t.Context(), 7, item.ID, streamID, streamInput)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	require.NoError(t, svc.StreamTest(t.Context(), streamAttempt, streamSource, streamInput, recorder, nil))
	body := recorder.Body.String()
	assert.Contains(t, body, "event: meta")
	assert.Contains(t, body, "event: delta")
	assert.Contains(t, body, "event: done")
	assert.NotContains(t, body, secret)
	assert.NotContains(t, body, secret[:20]+secret[20:])
	assert.Contains(t, body, "here is")
	var streamed model.DonationTestAttempt
	require.NoError(t, svc.Store.DB.Where("test_id = ?", streamID).First(&streamed).Error)
	assert.Equal(t, model.DonationTestSucceeded, streamed.State)
	// A session cannot start a second test while one is running for this item.
	require.NoError(t, svc.Store.DB.Model(&model.DonationTestAttempt{}).Where("test_id = ?", streamID).Update("state", model.DonationTestRunning).Error)
	_, _, _, err = svc.BeginTest(t.Context(), 7, item.ID, uuid.NewString(), input)
	assert.ErrorIs(t, err, model.ErrDonationTestBusy)
}

// TestDonationReviewReconcilesLostResponse proves a lost action response is
// reconciled with the original action UUID and never releases or reverses the
// decision it already persisted.
func TestDonationReviewReconcilesLostResponse(t *testing.T) {
	for _, test := range []struct {
		name        string
		beforeApply bool
		posts       int
	}{{"after_apply", false, 1}, {"before_apply", true, 2}} {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeDonationIntake(t)
			svc, user, campaign := donationManualServiceFixture(t, fake)
			batch := submitManualBatch(t, svc, user.Id, campaign.ID, "manual-lost-action-0001")
			item := batch.Items[0]
			view, err := svc.ReviewContext(t.Context(), item.ID)
			require.NoError(t, err)
			before := donationUserQuota(t, svc, user.Id)

			fake.mu.Lock()
			fake.lostAction = true
			fake.lossBeforeApply = test.beforeApply
			fake.mu.Unlock()
			actionID := uuid.NewString()
			_, err = svc.Review(t.Context(), 7, item.ID, actionID, DonationReviewInput{Kind: model.DonationActionApprove,
				ExpectedItemRevision: view.ItemRevision, ReviewTargetRevision: batch.TargetRevision, Note: "approve after inspection"})
			require.Error(t, err)

			// The intent survives, the item is untouched, and nothing was paid.
			pending, err := svc.Store.PendingReviewActions(t.Context(), batch.ID)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			assert.Equal(t, actionID, pending[0].ActionID)
			assert.Equal(t, model.DonationActionPending, pending[0].Status)
			detail, err := svc.Store.Batch(t.Context(), batch.ID, user.Id)
			require.NoError(t, err)
			assert.Equal(t, "pending_review", detail.Items[0].State)
			assert.Zero(t, detail.Summary.RewardedQuota)

			// Reconciliation queries the original action and replays its UUID only when
			// no action was recorded. An acceptance may arrive before the action query.
			require.NoError(t, svc.ReconcileBatch(t.Context(), batch.DonationBatch))
			require.NoError(t, svc.ReconcileBatch(t.Context(), batch.DonationBatch))
			fake.mu.Lock()
			posted := append([]string(nil), fake.actionPosts...)
			fake.mu.Unlock()
			require.Len(t, posted, test.posts)
			for _, id := range posted {
				assert.Equal(t, actionID, id, "every replay must reuse the original action UUID")
			}
			settled, err := svc.Store.ReviewAction(t.Context(), 7, actionID)
			require.NoError(t, err)
			assert.Equal(t, model.DonationActionApplied, settled.Status)

			require.NoError(t, svc.ReconcileBatch(t.Context(), batch.DonationBatch))
			detail, err = svc.Store.Batch(t.Context(), batch.ID, user.Id)
			require.NoError(t, err)
			assert.Equal(t, "accepted", detail.Items[0].State)
			assert.EqualValues(t, 25, detail.Summary.RewardedQuota)
			assert.Equal(t, before+25, donationUserQuota(t, svc, user.Id))
			assert.Equal(t, int64(1), storageCount[model.DonationReward](t, svc))
		})
	}
}

// TestDonationPendingReviewLeavesTheHotQueue proves a batch that only waits for a
// human decision is scheduled for the cold queue instead of consuming hot work.
func TestDonationPendingReviewLeavesTheHotQueue(t *testing.T) {
	fake := newFakeDonationIntake(t)
	svc, user, campaign := donationManualServiceFixture(t, fake)
	batch := submitManualBatch(t, svc, user.Id, campaign.ID, "manual-cold-queue-0001")

	var stored model.DonationBatch
	require.NoError(t, svc.Store.DB.Where("id = ?", batch.ID).First(&stored).Error)
	assert.False(t, stored.NeedsRecovery)

	claim, err := svc.Store.ClaimRecovery(t.Context(), time.Now().UnixMilli())
	require.NoError(t, err)
	for _, candidate := range claim {
		assert.NotEqual(t, batch.ID, candidate.ID, "a pending review must not occupy the hot queue")
	}

	// A prepared decision wakes the same batch immediately, even from cold.
	view, err := svc.ReviewContext(t.Context(), batch.Items[0].ID)
	require.NoError(t, err)
	_, err = svc.Store.PrepareReviewAction(t.Context(), 7, batch.Items[0].ID, uuid.NewString(),
		model.DonationActionReject, view.ItemRevision, "", "not usable for chat")
	require.NoError(t, err)
	require.NoError(t, svc.Store.DB.Where("id = ?", batch.ID).First(&stored).Error)
	assert.True(t, stored.NeedsRecovery)
	assert.Zero(t, stored.ColdPollAtMS)
}

// TestDonationEnterReviewForExhaustedItem proves an explicitly re-entered
// exhausted item keeps its batch identity and reward while it becomes a manual
// pending-review item, and that it is the only kind of item that can be.
func TestDonationEnterReviewForExhaustedItem(t *testing.T) {
	fake := newFakeDonationIntake(t)
	svc, user, campaign := donationServiceFixture(t, fake)
	batch, err := svc.Submit(t.Context(), user.Id, campaign.ID, uuid.NewString(), "wait-exhausted-key-0001")
	require.NoError(t, err)
	require.Equal(t, "retry_pending", batch.Items[0].State)
	item := batch.Items[0]
	before := donationUserQuota(t, svc, user.Id)
	view, err := svc.ReviewContext(t.Context(), item.ID)
	require.NoError(t, err)
	assert.Equal(t, model.DonationModeAuto, view.EffectiveMode)
	assert.Equal(t, model.DonationActionEnterReview, view.ReviewAction)
	assert.True(t, view.CanReview)
	assert.False(t, view.CanTest)

	actionID := uuid.NewString()
	saved, err := svc.Review(t.Context(), 7, item.ID, actionID, DonationReviewInput{Kind: model.DonationActionEnterReview,
		ExpectedItemRevision: item.ItemRevision, ReviewTargetRevision: fake.manualTarget, Note: "automatic probes are exhausted upstream"})
	require.NoError(t, err)
	assert.Equal(t, model.DonationActionApplied, saved.Status)
	_, err = svc.Review(t.Context(), 7, item.ID, actionID, DonationReviewInput{Kind: model.DonationActionEnterReview,
		ExpectedItemRevision: item.ItemRevision, ReviewTargetRevision: fake.manualTarget, Note: "automatic probes are exhausted upstream"})
	require.NoError(t, err, "an applied enter_review keeps its original request identity")

	detail, err := svc.Store.Batch(t.Context(), batch.ID, user.Id)
	require.NoError(t, err)
	assert.Equal(t, "pending_review", detail.Items[0].State)
	assert.Equal(t, model.DonationModeManualReview, detail.Items[0].EffectiveMode)
	assert.Equal(t, "pending_review", detail.Items[0].ReviewState)
	assert.Equal(t, actionID, detail.Items[0].EntryActionID)
	// The original batch identity, request, owner, and reward are untouched.
	assert.Equal(t, batch.TargetRevision, detail.TargetRevision)
	assert.Equal(t, batch.RequestDigest, detail.RequestDigest)
	assert.Equal(t, campaign.RewardQuota, detail.RewardQuota)
	assert.Equal(t, batch.CreatedAtMS, detail.CreatedAtMS)
	assert.Equal(t, before, donationUserQuota(t, svc, user.Id))
	fake.mu.Lock()
	manualReceipt := fake.batches[batch.ID]
	manualReceipt.Items = append([]model.DonationReceiptItem(nil), manualReceipt.Items...)
	fake.mu.Unlock()
	manualReceipt.InstanceID, manualReceipt.SourceID = batch.InstanceID, batch.SourceID
	for _, sameVersion := range []bool{true, false} {
		forged := manualReceipt
		forged.Items = append([]model.DonationReceiptItem(nil), manualReceipt.Items...)
		if !sameVersion {
			forged.Items[0].ItemRevision = int64PtrValue(detail.Items[0].ItemRevision + 1)
		}
		forged.Items[0].EffectiveMode = model.DonationModeAuto
		err = svc.Store.ApplyReceipt(t.Context(), forged)
		assert.ErrorIs(t, err, model.ErrDonationReceipt, "manual history cannot be downgraded, including at a newer revision")
	}

	// An automatic item that is not exhausted is not re-enterable, and an item
	// that is already manual cannot enter review twice.
	other, err := svc.Submit(t.Context(), user.Id, campaign.ID, uuid.NewString(), "ordinary-auto-key-0002")
	require.NoError(t, err)
	_, err = svc.Review(t.Context(), 7, other.Items[0].ID, uuid.NewString(), DonationReviewInput{Kind: model.DonationActionEnterReview, ReviewTargetRevision: fake.manualTarget})
	assert.ErrorIs(t, err, model.ErrDonationReview)
	_, err = svc.Review(t.Context(), 7, item.ID, uuid.NewString(), DonationReviewInput{Kind: model.DonationActionEnterReview,
		ExpectedItemRevision: detail.Items[0].ItemRevision, ReviewTargetRevision: fake.manualTarget})
	assert.ErrorIs(t, err, model.ErrDonationReview)
}

func TestDonationColdRecoveryDoesNotReleaseUnconfirmedExpiry(t *testing.T) {
	fake := newFakeDonationIntake(t)
	svc, user, campaign := donationManualServiceFixture(t, fake)
	batch := submitManualBatch(t, svc, user.Id, campaign.ID, "manual-cold-expiry")
	item := batch.Items[0]
	now := time.Now().UnixMilli()
	require.NoError(t, svc.Store.DB.Model(&model.DonationItem{}).Where("id = ?", item.ID).
		Update("staging_expires_at_ms", now-1).Error)
	require.NoError(t, svc.Store.DB.Model(&model.DonationBatch{}).Where("id = ?", batch.ID).
		Update("cold_poll_at_ms", now-1).Error)
	fake.mu.Lock()
	fake.hideReceipts = true
	fake.mu.Unlock()
	assert.Error(t, svc.Recover(t.Context()), "an unavailable receipt must remain unknown")
	var resource model.DonationResource
	require.NoError(t, svc.Store.DB.Where("owner_item_id = ?", item.ID).First(&resource).Error)
	assert.False(t, resource.Acquired)
	detail, err := svc.Store.Batch(t.Context(), batch.ID, user.Id)
	require.NoError(t, err)
	assert.Equal(t, "pending_review", detail.Items[0].State, "local time cannot prove remote non-acceptance")
}

func TestDonationControlledStreamRequiresMetaBeforeOutput(t *testing.T) {
	for _, body := range []string{
		"event: done\ndata: {\"state\":\"succeeded\"}\n\n",
		"event: delta\ndata: {\"text\":\"unbound reply\"}\n\nevent: meta\ndata: {}\n\nevent: done\ndata: {}\n\n",
	} {
		err := readDonationTestStream(strings.NewReader(body), donationLocalReviewLimits(), func(string, []byte) error { return nil })
		assert.ErrorIs(t, err, model.ErrDonationReceipt)
	}

	secret := strings.Repeat("t", 40)
	redactor := newDonationRedactor(secret)
	output := redactor.push(strings.Repeat("p", 500) + secret[:10])
	output += redactor.push(secret[10:] + strings.Repeat("s", 500))
	output += redactor.flush()
	assert.Equal(t, strings.Repeat("p", 500)+"[redacted]"+strings.Repeat("s", 500), output)
}

func TestDonationTestRunningReplayDoesNotAuthorizeAnotherCall(t *testing.T) {
	fake := newFakeDonationIntake(t)
	svc, user, campaign := donationManualServiceFixture(t, fake)
	batch := submitManualBatch(t, svc, user.Id, campaign.ID, "manual-running-replay")
	input := DonationTestInput{ExpectedItemRevision: batch.Items[0].ItemRevision, ReviewTargetRevision: batch.TargetRevision,
		Model: fake.testModel, Prompt: "one call", MaxOutputTokens: 64}
	id := uuid.NewString()
	first, _, created, err := svc.BeginTest(t.Context(), 7, batch.Items[0].ID, id, input)
	require.NoError(t, err)
	require.True(t, created)
	replay, _, created, err := svc.BeginTest(t.Context(), 7, batch.Items[0].ID, id, input)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, replay.ID)
	input.MaxOutputTokens++
	_, _, _, err = svc.BeginTest(t.Context(), 7, batch.Items[0].ID, id, input)
	assert.ErrorIs(t, err, model.ErrDonationConflict, "the output budget is part of the request identity")
}

func TestDonationAdminHistoryIsBoundedAndMetadataOnly(t *testing.T) {
	fake := newFakeDonationIntake(t)
	svc, user, campaign := donationManualServiceFixture(t, fake)
	batch := submitManualBatch(t, svc, user.Id, campaign.ID, "manual-history-key")
	itemID := batch.Items[0].ID
	for i := range 21 {
		now := int64(1000 + i)
		action := model.DonationReviewAction{ActorID: 7, ActionID: uuid.NewString(), BatchID: batch.ID, ItemID: itemID,
			Kind: model.DonationActionApprove, Status: model.DonationActionRejected, ReasonCode: "revision_mismatch", Note: fmt.Sprintf("review-%02d", i),
			ExpectedItemRevision: 1, ReviewTargetRevision: batch.TargetRevision, CreatedAtMS: now, UpdatedAtMS: now, AppliedAtMS: &now}
		test := model.DonationTestAttempt{ActorID: 8, TestID: uuid.NewString(), BatchID: batch.ID, ItemID: itemID, StartRevision: 1,
			ReviewTargetRevision: batch.TargetRevision, Model: fake.testModel, State: model.DonationTestInterrupted, ReasonCode: "interrupted",
			PromptDigest: strings.Repeat("e", 64), PromptBytes: 10, StartedAtMS: now, FinishedAtMS: &now}
		require.NoError(t, svc.Store.DB.Create(&action).Error)
		require.NoError(t, svc.Store.DB.Create(&test).Error)
	}
	record, err := svc.Store.Record(t.Context(), itemID)
	require.NoError(t, err)
	view := DonationRecordViewOf(record)
	require.Len(t, view.RecentReviewActions, 20)
	require.Len(t, view.RecentTests, 20)
	assert.Equal(t, "review-20", view.RecentReviewActions[0].Note)
	assert.Equal(t, "review-01", view.RecentReviewActions[19].Note)
	assert.Equal(t, int64(1020), view.RecentTests[0].StartedAtMS)
	assert.Equal(t, 8, view.RecentTests[0].ActorID)
	assert.Equal(t, view.RecentTests[0].TestID, view.LatestTest.TestID)
	encoded, err := common.Marshal(view)
	require.NoError(t, err)
	for _, private := range []string{"manual-history-key", strings.Repeat("e", 64), "\"prompt\":", "\"prompt_digest\":", "\"text\":"} {
		assert.NotContains(t, string(encoded), private)
	}
	self, err := svc.Store.Batch(t.Context(), batch.ID, user.Id)
	require.NoError(t, err)
	encoded, err = common.Marshal(self)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "review-20")
	assert.NotContains(t, string(encoded), "recent_tests")
}

func TestDonationLongTestEnforcesFirstByteAndIdleBudgets(t *testing.T) {
	for _, firstByte := range []bool{false, true} {
		t.Run(fmt.Sprintf("first_byte_%v", firstByte), func(t *testing.T) {
			cancelled := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if firstByte {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "x")
					w.(http.Flusher).Flush()
				}
				select {
				case <-r.Context().Done():
					close(cancelled)
				case <-release:
				}
			}))
			t.Cleanup(func() { close(release); server.Close() })
			client, err := newDonationClient(server.URL, donationTestToken)
			require.NoError(t, err)
			t.Cleanup(client.close)
			limits := donationLocalReviewLimits()
			limits.FirstByteTimeoutSeconds, limits.IdleTimeoutSeconds = 1, 1
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			request := donationTestRequest{TestID: uuid.NewString(), Actor: "user:7", Model: "test", Stream: true,
				ExpectedItemRevision: 1, ReviewTargetRevision: strings.Repeat("a", 64), Prompt: "bounded", MaxOutputTokens: 64}
			response, err := client.testStreamRequest(ctx, uuid.NewString(), uuid.NewString(), request.TestID, request, limits)
			if firstByte {
				require.NoError(t, err)
				defer response.Body.Close()
				buffer := make([]byte, 1)
				_, err = io.ReadFull(response.Body, buffer)
				require.NoError(t, err)
				_, err = response.Body.Read(buffer)
			}
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			assert.NoError(t, ctx.Err(), "the first/idle budget must cancel before the total budget")
			select {
			case <-cancelled:
			case <-ctx.Done():
				t.Fatal("the timed-out request did not cancel its receiver")
			}
		})
	}
}

func TestDonationNonStreamingTestRejectsMismatchedModel(t *testing.T) {
	batchID, itemID, testID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	target := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		donationTestJSON(w, map[string]any{"code": 0, "data": DonationTestResult{TestID: testID, BatchID: batchID, ItemID: itemID,
			Model: "another-model", StartRevision: 1, TargetRevision: target, State: model.DonationTestSucceeded, StatusCode: 200,
			StartedAtMS: 1000, FinishedAtMS: 1001, Text: "ok", OutputBytes: 2}})
	}))
	t.Cleanup(server.Close)
	client, err := newDonationClient(server.URL, donationTestToken)
	require.NoError(t, err)
	t.Cleanup(client.close)
	_, err = client.test(t.Context(), batchID, itemID, testID, donationTestRequest{TestID: testID, Actor: "user:7", Model: "expected-model",
		ExpectedItemRevision: 1, ReviewTargetRevision: target, Prompt: "test", MaxOutputTokens: 64}, donationLocalReviewLimits())
	var remote *DonationRemoteError
	require.ErrorAs(t, err, &remote)
	assert.Equal(t, "invalid_response", remote.Reason)
}

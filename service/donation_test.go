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
	f := &fakeDonationIntake{instance: uuid.NewString(), source: uuid.NewString(), batches: make(map[string]model.DonationReceipt), retries: make(map[string]bool)}
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeDonationIntake) capabilities() DonationCapabilities {
	result := DonationCapabilities{ProtocolVersion: "1", InstanceID: f.instance, SourceID: f.source, InputTypes: []string{"api_key"}}
	result.Limits.MaxItems, result.Limits.MaxKeyBytes, result.Limits.MaxBodyBytes, result.Limits.StagingRetentionSeconds = 100, 4096, 1048576, 604800
	return result
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
		donationTestJSON(w, map[string]any{"code": 0, "data": []DonationGroup{{ID: 1, Name: "Empty group", ChannelID: "gemini", ConnectionType: "api_key", Enabled: true, CanProbe: true, TargetRevision: strings.Repeat("a", 64)}, {ID: 2, Name: "Other group", ChannelID: "gemini", ConnectionType: "api_key", Enabled: true, CanProbe: true, TargetRevision: strings.Repeat("a", 64)}, {ID: 3, Name: "Disabled group", ChannelID: "gemini", ConnectionType: "api_key", Enabled: false, CanProbe: false, UnavailableReason: "group_disabled"}}})
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
			receipt := model.DonationReceipt{BatchID: request.BatchID, GroupID: request.GroupID, TargetRevision: request.TargetRevision, CreatedAtMS: time.Now().UnixMilli(), State: "processing"}
			for i, item := range request.Items {
				result := model.DonationReceiptItem{ItemID: item.ItemID, State: "accepted"}
				switch {
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
		http.NotFound(w, r)
		return
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
	donationTestJSON(w, map[string]any{"code": 0, "data": receipt})
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

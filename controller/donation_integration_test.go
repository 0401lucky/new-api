/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package controller_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run with freshly built DONATION_TEST_GPT_LOAD_BIN and
// DONATION_TEST_NEW_API_BIN. Both binaries run against new temporary SQLite
// databases; no application code or database globals are replaced by this test.
func TestDonationIntegrationRealServices(t *testing.T) {
	f := newDonationIntegrationFixture(t)
	const reward = int64(37)
	const otherReward = int64(91)
	const validKey = "synthetic-gemini-donation-CaseSensitive"
	const secondKey = "synthetic-gemini-donation-casesensitive"
	const invalidKey = "synthetic-gemini-denied-fixture"
	const inventoryKey = "synthetic-gemini-existing-inventory"
	const limitedKey = "synthetic-gemini-rate-limited-fixture"
	const seedKey = "synthetic-gemini-empty-group-seed"
	const recoveryKey = "synthetic-gemini-lost-receipt-fixture"
	const concurrentKey = "synthetic-gemini-concurrent-fixture"
	const pausedKey = "synthetic-gemini-paused-account-fixture"
	const malformedKey = "synthetic key with spaces"
	f.secrets = append(f.secrets, validKey, secondKey, invalidKey, inventoryKey, limitedKey,
		seedKey, recoveryKey, concurrentKey, pausedKey, malformedKey)
	for _, key := range []string{validKey, secondKey, inventoryKey, seedKey, recoveryKey, concurrentKey, pausedKey} {
		f.upstream.configure(key, http.StatusOK, nil)
	}
	f.upstream.configure(invalidKey, http.StatusUnauthorized, nil)
	f.upstream.configure(limitedKey, http.StatusTooManyRequests, nil)

	t.Log("starting isolated services and configuring real accounts and Gemini groups")
	f.start(t)
	f.api(t, "", http.MethodPost, "/api/setup", map[string]any{
		"username": "donationroot", "password": f.password, "confirmPassword": f.password,
		"SelfUseModeEnabled": false, "DemoSiteEnabled": false,
	}, "", nil)
	admin := f.login(t, "donationroot")
	alice := f.register(t, "donor_one")
	bob := f.register(t, "donor_two")
	initialAlice := f.wallet(t, alice)
	initialBob := f.wallet(t, bob)
	assert.Zero(t, initialAlice.TemporaryQuota)
	assert.Zero(t, initialBob.TemporaryQuota)

	// Enabling the existing status endpoint lets the test observe the entire
	// Checkin history without querying or mutating application database internals.
	f.api(t, admin.Token, http.MethodPut, "/api/option/checkin", map[string]any{
		"enabled": true, "min_quota": 1, "max_quota": 1, "fixed_quota": 23,
		"random_mode": false, "reward_type": "temporary", "available_from_minutes": 0,
	}, "", nil)

	group := f.createGroup(t, "donation-gemini", inventoryKey)
	initialInventory := f.credentials(t, group)
	require.Len(t, initialInventory.Items, 1)
	emptyGroup := f.createGroup(t, "donation-empty", seedKey)
	seedCredentials := f.credentials(t, emptyGroup)
	require.Len(t, seedCredentials.Items, 1)
	f.gpt(t, f.adminKey, http.MethodDelete,
		fmt.Sprintf("/api/groups/%d/credentials/%d", emptyGroup, seedCredentials.Items[0].CredentialID), nil, "", nil)
	assert.Empty(t, f.credentials(t, emptyGroup).Items)

	var identity donationRemoteIdentity
	f.gpt(t, f.integrationToken, http.MethodGet, "/integrations/donations/v1/capabilities", nil, "", &identity)
	assert.Equal(t, "1", identity.ProtocolVersion)
	require.NotEmpty(t, identity.InstanceID)
	require.NotEmpty(t, identity.SourceID)
	f.api(t, admin.Token, http.MethodPut, "/api/donations/admin/connection", map[string]any{
		"base_url": f.bridge.server.URL, "token": f.integrationToken,
	}, "", nil)
	var options []struct {
		ID       uint64 `json:"id"`
		CanProbe bool   `json:"can_probe"`
		Enabled  bool   `json:"enabled"`
	}
	f.api(t, admin.Token, http.MethodGet, "/api/donations/admin/group-options", nil, "", &options)
	assert.True(t, slices.ContainsFunc(options, func(option struct {
		ID       uint64 `json:"id"`
		CanProbe bool   `json:"can_probe"`
		Enabled  bool   `json:"enabled"`
	}) bool {
		return option.ID == emptyGroup && option.Enabled && option.CanProbe
	}), "an empty group must remain a donation target")
	campaign := f.createCampaign(t, admin, group, reward, "gemini aistudio free key")
	otherCampaign := f.createCampaign(t, admin, emptyGroup, otherReward, "another community activity")

	t.Log("checking Authorization, ownership, and server-controlled reward fields")
	for _, path := range []string{"/api/donations/campaigns", "/api/donations/batches"} {
		response := f.request(t, "", http.MethodGet, f.newAPI.url+path, nil, "", nil)
		assert.Equal(t, http.StatusUnauthorized, response.status)
		assert.False(t, response.envelope.Success)
	}
	cookieResponse := f.request(t, "", http.MethodPost, f.newAPI.url+"/api/donations/batches",
		map[string]any{"campaign_id": campaign.ID, "keys_text": validKey}, uuid.NewString(), alice.Cookies)
	assert.Equal(t, http.StatusUnauthorized, cookieResponse.status, "refresh cookies alone must not authorize donation writes")
	for _, path := range []string{"connection", "group-options", "campaigns", "records"} {
		response := f.request(t, alice.Token, http.MethodGet, f.newAPI.url+"/api/donations/admin/"+path, nil, "", nil)
		assert.Equal(t, http.StatusForbidden, response.status, path)
		assert.False(t, response.envelope.Success)
	}
	for _, route := range []struct{ method, path string }{
		{http.MethodPut, "/api/donations/admin/connection"},
		{http.MethodPost, "/api/donations/admin/campaigns"},
		{http.MethodPatch, fmt.Sprintf("/api/donations/admin/campaigns/%d", campaign.ID)},
	} {
		response := f.request(t, alice.Token, route.method, f.newAPI.url+route.path, map[string]any{}, "", nil)
		assert.Equal(t, http.StatusForbidden, response.status, route.path)
		assert.False(t, response.envelope.Success)
	}
	for _, extra := range []string{"user_id", "group_id", "reward_quota"} {
		response := f.request(t, alice.Token, http.MethodPost, f.newAPI.url+"/api/donations/batches",
			map[string]any{"campaign_id": campaign.ID, "keys_text": validKey, extra: bob.ID}, uuid.NewString(), nil)
		assert.Equal(t, http.StatusBadRequest, response.status, extra)
		assert.False(t, response.envelope.Success)
	}
	for _, token := range []string{"", f.adminKey} {
		response := f.request(t, token, http.MethodGet, f.gptLoad.url+"/integrations/donations/v1/capabilities", nil, "", nil)
		assert.Equal(t, http.StatusUnauthorized, response.status)
	}
	response := f.request(t, f.integrationToken, http.MethodGet, f.gptLoad.url+"/api/groups/options", nil, "", nil)
	assert.Equal(t, http.StatusUnauthorized, response.status, "the integration credential must not authorize management")

	t.Log("checking LF/CRLF line identity, mixed outcomes, permanent credit, and explicit retry")
	requestID := uuid.NewString()
	keysText := "  " + validKey + "  \r\n\r\n" + invalidKey + "\n" + inventoryKey + "\r\n\t" + validKey + "\t\n" + secondKey + "\n" + malformedKey + "\n" + limitedKey + "\r\n   \r\n"
	mixed := f.submit(t, alice, campaign.ID, requestID, keysText)
	require.Equal(t, "confirmed", mixed.ReceptionState)
	require.Len(t, mixed.Items, 7)
	itemIDs := mixed.itemIDs()
	assert.Equal(t, []int{1, 3, 4, 5, 6, 7, 8}, mixed.lines())
	mixed = f.awaitBatch(t, alice, mixed.ID, func(batch donationIntegrationBatch) bool {
		return batch.Summary.Rewarded == 2 && batch.itemAt(8).ReasonCode == "retry_exhausted"
	})
	for line, state := range map[int]string{1: "accepted", 3: "invalid", 4: "existing", 5: "duplicate", 6: "accepted", 7: "invalid", 8: "retry_pending"} {
		assert.Equal(t, state, mixed.itemAt(line).State, "input line %d", line)
	}
	assert.Equal(t, donationIntegrationSummary{Total: 7, Accepted: 2, Invalid: 2, Duplicate: 2, Processing: 1, Rewarded: 2, RewardedQuota: 2 * reward}, mixed.Summary)
	assert.Nil(t, mixed.itemAt(4).CredentialID, "inventory must not be attributed to this donor")
	assert.Equal(t, "none", mixed.itemAt(4).RewardState)
	assert.Equal(t, initialAlice.Quota+2*reward, f.wallet(t, alice).Quota)
	assert.Zero(t, f.wallet(t, alice).TemporaryQuota)
	assert.Equal(t, initialBob.Quota, f.wallet(t, bob).Quota)
	assert.Equal(t, 1, f.upstream.count(validKey))
	assert.Equal(t, 1, f.upstream.count(secondKey), "normalization must preserve case")
	assert.Equal(t, 1, f.upstream.count(invalidKey))
	assert.Zero(t, f.upstream.count(inventoryKey), "a probe must not fall back to working inventory")
	assert.Zero(t, f.upstream.count(seedKey))
	assert.Zero(t, f.upstream.count(malformedKey))
	limitedAttempts := f.upstream.count(limitedKey)
	require.Positive(t, limitedAttempts)

	f.upstream.configure(limitedKey, http.StatusOK, nil)
	retryID := uuid.NewString()
	f.api(t, alice.Token, http.MethodPost, "/api/donations/batches/"+mixed.ID+"/retry", map[string]any{}, retryID, nil)
	f.api(t, alice.Token, http.MethodPost, "/api/donations/batches/"+mixed.ID+"/retry", map[string]any{}, retryID, nil)
	mixed = f.awaitBatch(t, alice, mixed.ID, func(batch donationIntegrationBatch) bool { return batch.Summary.Rewarded == 3 })
	assert.Equal(t, itemIDs, mixed.itemIDs())
	assert.Equal(t, int64(3)*reward, mixed.Summary.RewardedQuota)
	assert.Equal(t, initialAlice.Quota+3*reward, f.wallet(t, alice).Quota)
	assert.Equal(t, limitedAttempts+1, f.upstream.count(limitedKey))
	assert.Equal(t, 1, f.upstream.count(validKey), "retry must not probe an accepted item again")
	assert.Equal(t, 1, f.upstream.count(invalidKey), "retry must not probe a definitive rejection again")
	acceptedCredentials := []uint64{initialInventory.Items[0].CredentialID}
	for _, line := range []int{1, 6, 8} {
		item := mixed.itemAt(line)
		require.NotNil(t, item.CredentialID)
		acceptedCredentials = append(acceptedCredentials, *item.CredentialID)
	}
	var actualCredentials []uint64
	for _, credential := range f.credentials(t, group).Items {
		actualCredentials = append(actualCredentials, credential.CredentialID)
	}
	assert.ElementsMatch(t, acceptedCredentials, actualCredentials, "stable receipts must refer to credentials in the selected real group")
	replay := f.submit(t, alice, campaign.ID, requestID, keysText)
	assert.Equal(t, mixed.ID, replay.ID)
	assert.Equal(t, itemIDs, replay.itemIDs())
	conflict := f.request(t, alice.Token, http.MethodPost, f.newAPI.url+"/api/donations/batches",
		map[string]any{"campaign_id": campaign.ID, "keys_text": secondKey}, requestID, nil)
	assert.Equal(t, http.StatusConflict, conflict.status)
	assert.False(t, conflict.envelope.Success)

	for _, attempt := range []struct {
		actor    donationIntegrationActor
		campaign int64
		key      string
	}{
		{alice, campaign.ID, uuid.NewString()},
		{bob, campaign.ID, uuid.NewString()},
		{bob, otherCampaign.ID, requestID},
	} {
		duplicate := f.submit(t, attempt.actor, attempt.campaign, attempt.key, validKey)
		require.Len(t, duplicate.Items, 1)
		assert.Equal(t, "duplicate", duplicate.Items[0].State)
		assert.Zero(t, duplicate.Summary.RewardedQuota)
		assert.NotEqual(t, mixed.ID, duplicate.ID)
	}
	assert.Equal(t, initialAlice.Quota+3*reward, f.wallet(t, alice).Quota)
	assert.Equal(t, initialBob.Quota, f.wallet(t, bob).Quota)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path := f.newAPI.url + "/api/donations/batches/" + mixed.ID
		var body any
		if method == http.MethodPost {
			path += "/retry"
			body = map[string]any{}
		}
		denied := f.request(t, bob.Token, method, path, body, uuid.NewString(), nil)
		assert.Equal(t, http.StatusNotFound, denied.status)
		assert.False(t, denied.envelope.Success)
	}
	var bobBatches struct {
		Items []donationIntegrationBatch `json:"items"`
	}
	f.api(t, bob.Token, http.MethodGet, "/api/donations/batches?p=1&page_size=100", nil, "", &bobBatches)
	for _, batch := range bobBatches.Items {
		assert.Equal(t, bob.ID, batch.UserID)
		assert.NotEqual(t, mixed.ID, batch.ID)
	}
	denied := f.request(t, bob.Token, http.MethodGet, f.newAPI.url+"/api/donations/batches?user_id="+strconv.FormatInt(alice.ID, 10), nil, "", nil)
	assert.Equal(t, http.StatusBadRequest, denied.status)

	t.Log("checking real concurrent submissions across users, campaigns, and groups")
	concurrentGate := newDonationProbeGate()
	f.upstream.configure(concurrentKey, http.StatusOK, concurrentGate)
	concurrentRequestID := uuid.NewString()
	start := make(chan struct{})
	type submitted struct {
		actor donationIntegrationActor
		body  donationIntegrationHTTPResponse
		err   error
	}
	results := make(chan submitted, 2)
	for _, attempt := range []struct {
		actor    donationIntegrationActor
		campaign int64
	}{{alice, campaign.ID}, {bob, otherCampaign.ID}} {
		payload, err := common.Marshal(map[string]any{"campaign_id": attempt.campaign, "keys_text": concurrentKey})
		require.NoError(t, err)
		go func() {
			<-start
			response, err := f.exchange(t.Context(), attempt.actor.Token, http.MethodPost, f.newAPI.url+"/api/donations/batches", payload, concurrentRequestID, nil)
			results <- submitted{attempt.actor, response, err}
		}()
	}
	close(start)
	var competing []submitted
	for range 2 {
		select {
		case result := <-results:
			require.NoError(t, result.err)
			require.True(t, result.body.envelope.Success, f.redact(string(result.body.body)))
			f.assertNoSecrets(t, result.body.body, "concurrent response")
			competing = append(competing, result)
		case <-time.After(30 * time.Second):
			t.Fatal("concurrent donation submission did not return")
		}
	}
	f.awaitProbe(t, concurrentGate)
	concurrentGate.release()
	var winningBatch donationIntegrationBatch
	var winner donationIntegrationActor
	totalAccepted, totalRewarded := 0, 0
	for _, result := range competing {
		var batch donationIntegrationBatch
		require.NoError(t, common.Unmarshal(result.body.envelope.Data, &batch))
		batch = f.awaitBatch(t, result.actor, batch.ID, func(batch donationIntegrationBatch) bool {
			return batch.Summary.Processing == 0 && (batch.Summary.Accepted == 0 || batch.Summary.Rewarded == 1)
		})
		totalAccepted += batch.Summary.Accepted
		totalRewarded += batch.Summary.Rewarded
		if batch.Summary.Rewarded == 1 {
			winningBatch, winner = batch, result.actor
		} else {
			assert.Equal(t, 1, batch.Summary.Duplicate)
		}
	}
	require.Equal(t, 1, totalAccepted)
	require.Equal(t, 1, totalRewarded)
	assert.Equal(t, 1, f.upstream.count(concurrentKey))
	expectedAlice, expectedBob := initialAlice.Quota+3*reward, initialBob.Quota
	if winner.ID == alice.ID {
		assert.Equal(t, reward, winningBatch.Summary.RewardedQuota)
		expectedAlice += reward
	} else {
		assert.Equal(t, otherReward, winningBatch.Summary.RewardedQuota)
		expectedBob += otherReward
	}
	assert.Equal(t, expectedAlice, f.wallet(t, alice).Quota)
	assert.Equal(t, expectedBob, f.wallet(t, bob).Quota)

	t.Log("dropping a real persisted receipt and restarting both services before reconciliation")
	f.bridge.loseNextBatchResponse()
	recoveryRequestID := uuid.NewString()
	recovering := f.submit(t, alice, campaign.ID, recoveryRequestID, recoveryKey)
	require.Equal(t, "unconfirmed", recovering.ReceptionState)
	require.Len(t, recovering.Items, 1)
	assert.Equal(t, expectedAlice, f.wallet(t, alice).Quota, "an unconfirmed receipt must not credit the wallet")
	remote := f.awaitRemoteAccepted(t, recovering.ID)
	require.Len(t, remote.Items, 1)
	assert.Equal(t, recovering.Items[0].ID, remote.Items[0].ItemID)
	assert.Equal(t, 1, f.bridge.postCount(recovering.ID))
	f.newAPI.stop(t)
	f.gptLoad.stop(t)
	f.startProcess(t, f.gptLoad, "/integrations/donations/v1/capabilities", f.integrationToken)
	var restartedIdentity donationRemoteIdentity
	f.gpt(t, f.integrationToken, http.MethodGet, "/integrations/donations/v1/capabilities", nil, "", &restartedIdentity)
	assert.Equal(t, identity, restartedIdentity)
	restartedRemote := f.awaitRemoteAccepted(t, recovering.ID)
	assert.Equal(t, remote, restartedRemote, "restart must preserve receipt, credential, and acceptance time")
	f.startProcess(t, f.newAPI, "/api/setup", "")
	f.bridge.resumeBatchReads()
	recovered := f.awaitBatch(t, alice, recovering.ID, func(batch donationIntegrationBatch) bool { return batch.Summary.Rewarded == 1 })
	assert.Equal(t, "confirmed", recovered.ReceptionState)
	assert.Equal(t, recovering.itemIDs(), recovered.itemIDs())
	assert.Equal(t, remote.Items[0].CredentialID, recovered.Items[0].CredentialID)
	assert.Equal(t, remote.Items[0].AcceptedAtMS, recovered.Items[0].AcceptedAtMS)
	expectedAlice += reward
	assert.Equal(t, expectedAlice, f.wallet(t, alice).Quota)
	assert.Equal(t, 1, f.bridge.postCount(recovered.ID), "recovery must query the saved batch instead of resending keys")
	assert.Equal(t, 1, f.upstream.count(recoveryKey))
	recoveryReplay := f.submit(t, alice, campaign.ID, recoveryRequestID, recoveryKey)
	assert.Equal(t, recovered.ID, recoveryReplay.ID)
	assert.Equal(t, expectedAlice, f.wallet(t, alice).Quota)

	t.Log("freezing campaign rewards, pausing a disabled account, and resuming its original item")
	pausedGate := newDonationProbeGate()
	f.upstream.configure(pausedKey, http.StatusOK, pausedGate)
	paused := f.submit(t, alice, campaign.ID, uuid.NewString(), pausedKey)
	require.Len(t, paused.Items, 1)
	f.awaitProbe(t, pausedGate)
	f.api(t, admin.Token, http.MethodPost, "/api/user/manage", map[string]any{"id": alice.ID, "action": "disable"}, "", nil)
	f.api(t, admin.Token, http.MethodPatch, fmt.Sprintf("/api/donations/admin/campaigns/%d", campaign.ID),
		map[string]any{"reward_quota": reward + 1000, "enabled": false}, "", nil)
	closed := f.request(t, bob.Token, http.MethodPost, f.newAPI.url+"/api/donations/batches",
		map[string]any{"campaign_id": campaign.ID, "keys_text": secondKey}, uuid.NewString(), nil)
	assert.Equal(t, http.StatusConflict, closed.status)
	assert.False(t, closed.envelope.Success)
	pausedGate.release()
	var pausedRecord donationIntegrationRecord
	f.await(t, "disabled account reward pause", func() (bool, string) {
		f.api(t, admin.Token, http.MethodGet, "/api/donations/admin/records/"+paused.Items[0].ID, nil, "", &pausedRecord)
		return pausedRecord.Item.State == "accepted" && pausedRecord.Item.RewardState == "paused", f.snapshot(pausedRecord)
	})
	assert.Nil(t, pausedRecord.Reward)
	assert.Equal(t, alice.ID, pausedRecord.Batch.UserID)
	assert.Equal(t, reward, pausedRecord.Batch.RewardQuota)
	assert.Equal(t, campaign.Version, pausedRecord.Batch.CampaignVersion)
	var disabledWallet donationIntegrationWallet
	f.api(t, admin.Token, http.MethodGet, fmt.Sprintf("/api/user/%d", alice.ID), nil, "", &disabledWallet)
	assert.Equal(t, expectedAlice, disabledWallet.Quota)
	denied = f.request(t, alice.Token, http.MethodGet, f.newAPI.url+"/api/donations/batches/"+paused.ID, nil, "", nil)
	assert.Equal(t, http.StatusUnauthorized, denied.status)
	f.api(t, admin.Token, http.MethodPost, "/api/user/manage", map[string]any{"id": alice.ID, "action": "enable"}, "", nil)
	alice = f.login(t, "donor_one")
	paused = f.awaitBatch(t, alice, paused.ID, func(batch donationIntegrationBatch) bool { return batch.Summary.Rewarded == 1 })
	assert.Equal(t, reward, paused.Summary.RewardedQuota)
	expectedAlice += reward
	assert.Equal(t, expectedAlice, f.wallet(t, alice).Quota)
	assert.Equal(t, 1, f.upstream.count(pausedKey))

	t.Log("retaining accepted history and eligibility after credential disable/delete and group deletion")
	original := f.record(t, admin, mixed.itemAt(1).ID)
	require.NotNil(t, original.Reward)
	require.NotNil(t, original.Item.CredentialID)
	assert.Equal(t, alice.ID, original.Batch.UserID)
	assert.Equal(t, group, original.Batch.GroupID)
	assert.Equal(t, reward, original.Reward.Quota)
	assert.Equal(t, original.Item.ID, original.Reward.ItemID)
	credentialPath := fmt.Sprintf("/api/groups/%d/credentials/%d", group, *original.Item.CredentialID)
	f.gpt(t, f.adminKey, http.MethodPut, credentialPath, map[string]any{"status": "disabled"}, "", nil)
	assert.Equal(t, expectedAlice, f.wallet(t, alice).Quota)
	f.gpt(t, f.adminKey, http.MethodDelete, credentialPath, nil, "", nil)
	f.gpt(t, f.adminKey, http.MethodDelete, fmt.Sprintf("/api/groups/%d", group), nil, "", nil)
	afterDeletion := f.record(t, admin, original.Item.ID)
	assert.Equal(t, original.Item, afterDeletion.Item)
	assert.Equal(t, original.Batch, afterDeletion.Batch)
	assert.Equal(t, original.Reward, afterDeletion.Reward)
	duplicateAfterDeletion := f.submit(t, bob, otherCampaign.ID, uuid.NewString(), validKey)
	require.Len(t, duplicateAfterDeletion.Items, 1)
	assert.Equal(t, "duplicate", duplicateAfterDeletion.Items[0].State)
	assert.Zero(t, duplicateAfterDeletion.Summary.RewardedQuota)
	assert.Equal(t, expectedAlice, f.wallet(t, alice).Quota)
	assert.Equal(t, expectedBob, f.wallet(t, bob).Quota)
	assert.Equal(t, 1, f.upstream.count(validKey))
	var records struct {
		Total int64                       `json:"total"`
		Items []donationIntegrationRecord `json:"items"`
	}
	f.api(t, admin.Token, http.MethodGet, fmt.Sprintf("/api/donations/admin/records?user_id=%d&group_id=%d&credential_id=%d&item_id=%s&state=accepted&reward_state=rewarded&p=1&page_size=1", alice.ID, group, *original.Item.CredentialID, original.Item.ID), nil, "", &records)
	require.Equal(t, int64(1), records.Total)
	require.Len(t, records.Items, 1)
	assert.Equal(t, original.Reward, records.Items[0].Reward)
	denied = f.request(t, bob.Token, http.MethodGet, f.newAPI.url+"/api/donations/admin/records/"+original.Item.ID, nil, "", nil)
	assert.Equal(t, http.StatusForbidden, denied.status)
	for _, actor := range []donationIntegrationActor{alice, bob} {
		wallet := f.wallet(t, actor)
		assert.Zero(t, wallet.TemporaryQuota)
		var checkin struct {
			Stats struct {
				TotalCheckins int64 `json:"total_checkins"`
				TotalQuota    int64 `json:"total_quota"`
			} `json:"stats"`
		}
		f.api(t, actor.Token, http.MethodGet, "/api/user/checkin", nil, "", &checkin)
		assert.Zero(t, checkin.Stats.TotalCheckins, "donations must not create Checkin rows")
		assert.Zero(t, checkin.Stats.TotalQuota)
	}
	// Audit and ordinary logs are also public API response surfaces. Process log
	// files are checked after both children have stopped in the fixture cleanup.
	f.api(t, admin.Token, http.MethodGet, "/api/audit?p=1&page_size=100", nil, "", nil)
	f.api(t, admin.Token, http.MethodGet, "/api/log/?p=1&page_size=100", nil, "", nil)
	assert.Empty(t, f.upstream.problems(), "Gemini validation must use the expected generation endpoint and submitted credential")
}

func TestDonationManualReviewIntegrationRealServices(t *testing.T) {
	f := newDonationIntegrationFixture(t)
	const reward = int64(41)
	const goodKey = "synthetic-manual-review-good"
	const rejectedKey = "synthetic-manual-review-reject"
	const legacyKey = "synthetic-manual-review-legacy"
	const slowKey = "synthetic-manual-review-cancel"
	const inventoryKey = "synthetic-manual-review-inventory"
	f.secrets = append(f.secrets, goodKey, rejectedKey, legacyKey, slowKey, inventoryKey)
	gate := newDonationProbeGate()
	f.upstream.configure(inventoryKey, http.StatusOK, nil)
	f.upstream.configureChat(goodKey, http.StatusOK, nil)
	f.upstream.configureChat(rejectedKey, http.StatusUnauthorized, nil)
	f.upstream.configureChat(legacyKey, http.StatusOK, nil)
	f.upstream.configureChat(slowKey, http.StatusOK, gate)
	f.start(t)
	f.api(t, "", http.MethodPost, "/api/setup", map[string]any{
		"username": "manualroot", "password": f.password, "confirmPassword": f.password,
		"SelfUseModeEnabled": false, "DemoSiteEnabled": false,
	}, "", nil)
	admin := f.login(t, "manualroot")
	alice := f.register(t, "manualdonor")
	reader := f.createDonationReader(t, admin, "manualreader")
	initialWallet := f.wallet(t, alice)
	initialAdminWallet := f.wallet(t, admin)
	group := f.createGroup(t, "manual-review-gemini", inventoryKey)
	f.api(t, admin.Token, http.MethodPut, "/api/donations/admin/connection", map[string]any{
		"base_url": f.bridge.server.URL, "token": f.integrationToken,
	}, "", nil)
	var campaign donationIntegrationCampaign
	f.api(t, admin.Token, http.MethodPost, "/api/donations/admin/campaigns", map[string]any{
		"name": "Manual review fixture", "group_id": group, "reward_quota": reward,
		"enabled": true, "validation_mode": "manual_review",
	}, "", &campaign)
	batch := f.submit(t, alice, campaign.ID, uuid.NewString(), goodKey+"\n"+rejectedKey+"\n"+slowKey+"\n"+inventoryKey)
	batch = f.awaitBatch(t, alice, batch.ID, func(batch donationIntegrationBatch) bool {
		return batch.Summary.PendingReview == 3 && batch.Summary.Duplicate == 1
	})
	require.Equal(t, "manual_review", batch.ValidationMode)
	assert.Len(t, f.credentials(t, group).Items, 1, "pending donations must not enter the serving pool")
	assert.Equal(t, initialWallet, f.wallet(t, alice))
	assert.Zero(t, f.upstream.count(goodKey), "manual intake must not issue an automatic probe")
	good := batch.itemAt(1)
	contextView := f.reviewContext(t, admin, good.ID)
	require.True(t, contextView.CanReview)
	require.True(t, contextView.CanReject)
	require.True(t, contextView.CanTest)
	require.Equal(t, "manual_review", contextView.EffectiveMode)
	require.Equal(t, "approve", contextView.ReviewAction)
	f.record(t, reader, good.ID)
	for _, actor := range []donationIntegrationActor{alice, reader} {
		for _, suffix := range []string{"/review-actions", "/tests"} {
			response := f.request(t, actor.Token, http.MethodPost,
				f.newAPI.url+"/api/donations/admin/records/"+good.ID+suffix, map[string]any{}, uuid.NewString(), nil)
			assert.Equal(t, http.StatusForbidden, response.status)
		}
	}
	testPath := "/api/donations/admin/records/" + good.ID + "/tests"
	testInput := map[string]any{
		"expected_item_revision": contextView.ItemRevision, "review_target_revision": contextView.ReviewTargetRevision,
		"model": "gemini-donation-test", "prompt": "Explain why this manual review request uses the submitted key.",
		"system_prompt": "Answer briefly.", "max_output_tokens": 1024, "stream": false,
	}
	var result donationIntegrationTestResult
	testID := uuid.NewString()
	f.api(t, admin.Token, http.MethodPost, testPath, testInput, testID, &result)
	require.Equal(t, "succeeded", result.State)
	assert.Contains(t, result.Text, "manual reply")
	assert.Equal(t, 1, f.upstream.count(goodKey))
	assert.Equal(t, testInput["prompt"], f.upstream.chatPrompt(goodKey))
	var metadata donationIntegrationTestResult
	f.api(t, reader.Token, http.MethodGet, testPath+"/"+testID, nil, "", &metadata)
	assert.Equal(t, "succeeded", metadata.State)
	assert.Empty(t, metadata.Text, "metadata queries must not return saved conversation text")
	f.api(t, admin.Token, http.MethodPost, testPath, testInput, testID, &metadata)
	assert.Equal(t, 1, f.upstream.count(goodKey), "same test ID must not call the upstream again")
	changedInput := make(map[string]any, len(testInput))
	for name, value := range testInput {
		changedInput[name] = value
	}
	changedInput["max_output_tokens"] = 2048
	conflict := f.request(t, admin.Token, http.MethodPost, f.newAPI.url+testPath, changedInput, testID, nil)
	assert.Equal(t, http.StatusConflict, conflict.status, "the same test ID must bind the token limit")

	contextView = f.reviewContext(t, admin, good.ID)
	testInput["expected_item_revision"] = contextView.ItemRevision
	testInput["review_target_revision"] = contextView.ReviewTargetRevision
	testInput["stream"] = true
	streamID := uuid.NewString()
	streamResult := f.streamTest(t, admin, good.ID, streamID, testInput)
	require.Equal(t, "succeeded", streamResult.State, "%s", f.snapshot(streamResult))
	assert.Contains(t, streamResult.Text, "manual reply")
	assert.Equal(t, 2, f.upstream.count(goodKey))
	f.api(t, admin.Token, http.MethodPost, testPath, testInput, streamID, &metadata)
	assert.Equal(t, "succeeded", metadata.State)
	assert.Empty(t, metadata.Text)
	assert.Equal(t, 2, f.upstream.count(goodKey))
	assert.Equal(t, initialWallet, f.wallet(t, alice), "tests must not reward or bill the donor")
	assert.Equal(t, initialAdminWallet, f.wallet(t, admin), "tests must not debit the administrator")

	contextView = f.reviewContext(t, admin, good.ID)
	approveInput := map[string]any{
		"kind": "approve", "expected_item_revision": contextView.ItemRevision,
		"review_target_revision": contextView.ReviewTargetRevision, "note": "Verified with the submitted credential",
	}
	approveID := uuid.NewString()
	actionPath := "/api/donations/admin/records/" + good.ID + "/review-actions"
	var action donationIntegrationReviewAction
	f.api(t, admin.Token, http.MethodPost, actionPath, approveInput, approveID, &action)
	require.Equal(t, "applied", action.Status)
	batch = f.awaitBatch(t, alice, batch.ID, func(batch donationIntegrationBatch) bool {
		if batch.LastError == "invalid_receipt" {
			receiver := f.request(t, f.integrationToken, http.MethodGet,
				f.gptLoad.url+"/integrations/donations/v1/batches/"+batch.ID, nil, "", nil)
			review := f.request(t, admin.Token, http.MethodGet,
				f.newAPI.url+actionPath+"/"+approveID, nil, "", nil)
			t.Fatalf("approved receipt rejected: receiver=%s action=%s", f.redact(string(receiver.envelope.Data)), f.redact(string(review.envelope.Data)))
		}
		return batch.itemAt(1).RewardState == "rewarded"
	})
	assert.Equal(t, initialWallet.Quota+reward, f.wallet(t, alice).Quota)
	f.api(t, admin.Token, http.MethodPost, actionPath, approveInput, approveID, &action)
	assert.Equal(t, initialWallet.Quota+reward, f.wallet(t, alice).Quota)

	slow := batch.itemAt(3)
	slowContext := f.reviewContext(t, admin, slow.ID)
	cancelID := uuid.NewString()
	cancelInput, err := common.Marshal(map[string]any{
		"expected_item_revision": slowContext.ItemRevision, "review_target_revision": slowContext.ReviewTargetRevision,
		"model": "gemini-donation-test", "prompt": "Cancel this pending request.", "max_output_tokens": 1024, "stream": false,
	})
	require.NoError(t, err)
	callContext, cancelCall := context.WithCancel(t.Context())
	t.Cleanup(cancelCall)
	cancelDone := make(chan error, 1)
	go func() {
		_, callErr := f.exchange(callContext, admin.Token, http.MethodPost,
			f.newAPI.url+"/api/donations/admin/records/"+slow.ID+"/tests", cancelInput, cancelID, nil)
		cancelDone <- callErr
	}()
	f.awaitProbe(t, gate)
	cancelCall()
	select {
	case callErr := <-cancelDone:
		require.Error(t, callErr)
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled test did not release the requesting client")
	}
	f.await(t, "cancelled manual test metadata", func() (bool, string) {
		f.api(t, admin.Token, http.MethodGet, "/api/donations/admin/records/"+slow.ID+"/tests/"+cancelID, nil, "", &metadata)
		return metadata.State != "running", f.snapshot(metadata)
	})
	assert.Contains(t, []string{"cancelled", "interrupted"}, metadata.State)
	assert.Equal(t, 1, f.upstream.count(slowKey))

	legacyCampaign := f.createCampaign(t, admin, group, reward, "Legacy probe fixture")
	legacy := f.submit(t, alice, legacyCampaign.ID, uuid.NewString(), legacyKey)
	legacy = f.awaitBatch(t, alice, legacy.ID, func(batch donationIntegrationBatch) bool {
		return batch.itemAt(1).ReasonCode == "retry_exhausted"
	})
	legacyItem := legacy.itemAt(1)
	legacyContext := f.reviewContext(t, admin, legacyItem.ID)
	require.Equal(t, "auto", legacyContext.EffectiveMode, "review context must report the actual item mode")
	require.Equal(t, "enter_review", legacyContext.ReviewAction)
	require.False(t, legacyContext.CanTest)
	legacyActionPath := "/api/donations/admin/records/" + legacyItem.ID + "/review-actions"
	enterInput := map[string]any{"kind": "enter_review", "expected_item_revision": legacyContext.ItemRevision,
		"review_target_revision": legacyContext.ReviewTargetRevision, "note": "Probe is incompatible"}
	enterID := uuid.NewString()
	f.api(t, admin.Token, http.MethodPost, legacyActionPath, enterInput, enterID, &action)
	require.Equal(t, "applied", action.Status)
	f.api(t, admin.Token, http.MethodPost, legacyActionPath, enterInput, enterID, &action)
	legacy = f.awaitBatch(t, alice, legacy.ID, func(batch donationIntegrationBatch) bool {
		return batch.itemAt(1).State == "pending_review"
	})
	assert.Equal(t, "auto", legacy.ValidationMode)
	assert.Equal(t, "manual_review", legacy.itemAt(1).EffectiveMode)
	assert.Equal(t, legacyContext.ExpiresAtMS, legacy.itemAt(1).StagingExpiresAtMS)
	legacyContext = f.reviewContext(t, admin, legacyItem.ID)
	legacyApproveID := uuid.NewString()
	f.bridge.loseNextReviewResponse()
	lost := f.request(t, admin.Token, http.MethodPost, f.newAPI.url+legacyActionPath,
		map[string]any{"kind": "approve", "expected_item_revision": legacyContext.ItemRevision,
			"review_target_revision": legacyContext.ReviewTargetRevision}, legacyApproveID, nil)
	require.False(t, lost.envelope.Success, "the fixture must actually lose the command acknowledgement")
	f.awaitRemoteAccepted(t, legacy.ID)
	pending := f.record(t, admin, legacyItem.ID)
	require.NotNil(t, pending.PendingReviewAction)
	assert.Equal(t, legacyApproveID, pending.PendingReviewAction.ActionID)
	assert.Equal(t, initialWallet.Quota+reward, f.wallet(t, alice).Quota)
	f.newAPI.stop(t)
	f.bridge.resumeBatchReads()
	f.startProcess(t, f.newAPI, "/api/setup", "")
	admin, alice = f.login(t, "manualroot"), f.login(t, "manualdonor")
	legacy = f.awaitBatch(t, alice, legacy.ID, func(batch donationIntegrationBatch) bool {
		return batch.itemAt(1).RewardState == "rewarded"
	})
	assert.Equal(t, initialWallet.Quota+2*reward, f.wallet(t, alice).Quota)
	assert.Equal(t, reward, legacy.RewardQuota)
	assert.Equal(t, 5, f.upstream.count(legacyKey), "manual approval must not start a sixth probe")

	f.gpt(t, f.adminKey, http.MethodDelete, fmt.Sprintf("/api/groups/%d", group), nil, "", nil)
	rejectItem := batch.itemAt(2)
	rejectContext := f.reviewContext(t, admin, rejectItem.ID)
	require.False(t, rejectContext.CanReview)
	require.True(t, rejectContext.CanReject, "a removed target must not prevent rejection")
	const rejectNote = "Unable to verify this donation; please check the credential."
	f.api(t, admin.Token, http.MethodPost, "/api/donations/admin/records/"+rejectItem.ID+"/review-actions",
		map[string]any{"kind": "reject", "expected_item_revision": rejectContext.ItemRevision, "note": rejectNote},
		uuid.NewString(), &action)
	require.Equal(t, "applied", action.Status)
	batch = f.awaitBatch(t, alice, batch.ID, func(batch donationIntegrationBatch) bool {
		return batch.itemAt(2).State == "rejected"
	})
	assert.Equal(t, rejectNote, batch.itemAt(2).ReviewNote)
	assert.Equal(t, initialWallet.Quota+2*reward, f.wallet(t, alice).Quota)
	assert.Empty(t, f.upstream.problems())
}

// This explicit opt-in fixture keeps the real services available for a browser.
// Use a freshly built new-api binary containing the UI under review. Only a new
// directory is accepted; its private metadata describes synthetic credentials
// and the stop file. No application data survives an orderly fixture shutdown.
func TestDonationBrowserFixture(t *testing.T) {
	if os.Getenv("DONATION_BROWSER_FIXTURE") != "1" {
		t.Skip("set DONATION_BROWSER_FIXTURE=1 and DONATION_BROWSER_FIXTURE_DIR to hold a local browser fixture")
	}
	require.NotEmpty(t, os.Getenv("DONATION_TEST_GPT_LOAD_BIN"))
	require.NotEmpty(t, os.Getenv("DONATION_TEST_NEW_API_BIN"))
	requestedDir := os.Getenv("DONATION_BROWSER_FIXTURE_DIR")
	require.True(t, filepath.IsAbs(requestedDir), "DONATION_BROWSER_FIXTURE_DIR must be an absolute new directory")
	requestedDir = filepath.Clean(requestedDir)
	parent, err := filepath.EvalSymlinks(filepath.Dir(requestedDir))
	require.NoError(t, err, "the fixture parent directory must already exist")
	directory := filepath.Join(parent, filepath.Base(requestedDir))
	require.NoError(t, os.Mkdir(directory, 0700), "refusing to reuse an existing directory")
	owner := uuid.NewString()
	marker := filepath.Join(directory, ".donation-browser-fixture")
	require.NoError(t, os.WriteFile(marker, []byte(owner), 0600))
	// Registered before the shared fixture: its process shutdown and log checks
	// finish first. Verify the resolved path and ownership before recursive removal.
	t.Cleanup(func() {
		resolved, err := filepath.EvalSymlinks(directory)
		if !assert.NoError(t, err) || !assert.Equal(t, directory, resolved, "fixture path changed; refusing removal") {
			return
		}
		storedOwner, err := os.ReadFile(marker)
		if !assert.NoError(t, err) || !assert.Equal(t, owner, string(storedOwner), "fixture ownership changed; refusing removal") {
			return
		}
		assert.NoError(t, os.RemoveAll(resolved))
	})
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)

	f := newDonationIntegrationFixtureIn(t, directory)
	const desktopKey = "synthetic-browser-valid-desktop"
	const mobileKey = "synthetic-browser-valid-mobile"
	const otherUserKey = "synthetic-browser-valid-other-user"
	const invalidKey = "synthetic-browser-invalid-denied"
	const limitedKey = "synthetic-browser-limited-retry"
	const inventoryKey = "synthetic-browser-existing-stock"
	const seedKey = "synthetic-browser-empty-group-seed"
	const manualKey = "synthetic-browser-manual-good"
	const manualRejectKey = "synthetic-browser-manual-reject"
	const manualStopKey = "synthetic-browser-manual-stop"
	for _, key := range []string{desktopKey, mobileKey, otherUserKey, inventoryKey, seedKey} {
		f.upstream.configure(key, http.StatusOK, nil)
	}
	f.upstream.configure(invalidKey, http.StatusUnauthorized, nil)
	f.upstream.configure(limitedKey, http.StatusTooManyRequests, nil)
	f.upstream.configureChat(manualKey, http.StatusOK, nil)
	f.upstream.configureChat(manualRejectKey, http.StatusUnauthorized, nil)
	f.upstream.configureChat(manualStopKey, http.StatusOK, newDonationProbeGate())
	f.secrets = append(f.secrets, desktopKey, mobileKey, otherUserKey, invalidKey, limitedKey, inventoryKey, seedKey,
		manualKey, manualRejectKey, manualStopKey)
	f.start(t)
	f.api(t, "", http.MethodPost, "/api/setup", map[string]any{
		"username": "browserroot", "password": f.password, "confirmPassword": f.password,
		"SelfUseModeEnabled": false, "DemoSiteEnabled": false,
	}, "", nil)
	admin := f.login(t, "browserroot")
	reader := f.createDonationReader(t, admin, "browser_reader")
	firstUser := f.register(t, "browser_one")
	secondUser := f.register(t, "browser_two")
	group := f.createGroup(t, "browser-gemini", inventoryKey)
	emptyGroup := f.createGroup(t, "browser-empty", seedKey)
	seed := f.credentials(t, emptyGroup)
	require.Len(t, seed.Items, 1)
	f.gpt(t, f.adminKey, http.MethodDelete, fmt.Sprintf("/api/groups/%d/credentials/%d", emptyGroup, seed.Items[0].CredentialID), nil, "", nil)
	f.api(t, admin.Token, http.MethodPut, "/api/donations/admin/connection", map[string]any{
		"base_url": f.bridge.server.URL, "token": f.integrationToken,
	}, "", nil)
	const campaignName = "Gemini community browser fixture"
	const rewardQuota = int64(500000)
	campaign := f.createCampaign(t, admin, group, rewardQuota, campaignName)
	var available []struct {
		ID        int64 `json:"id"`
		Available bool  `json:"available"`
	}
	f.api(t, firstUser.Token, http.MethodGet, "/api/donations/campaigns", nil, "", &available)
	require.Len(t, available, 1)
	require.Equal(t, campaign.ID, available[0].ID)
	require.True(t, available[0].Available)

	var manualCampaign donationIntegrationCampaign
	f.api(t, admin.Token, http.MethodPost, "/api/donations/admin/campaigns", map[string]any{
		"name": "Manual review browser fixture", "group_id": group, "reward_quota": rewardQuota,
		"validation_mode": "manual_review", "enabled": true,
	}, "", &manualCampaign)

	stopFile := filepath.Join(directory, "stop")
	resolveLimitFile := filepath.Join(directory, "resolve-rate-limit")
	metadataPath := filepath.Join(directory, "metadata.json")
	metadata := map[string]any{
		"kind": "new-api-donation-browser-fixture-v1", "ready_at_ms": time.Now().UnixMilli(),
		"base_url": f.newAPI.url, "fixture_dir": directory,
		"accounts": []map[string]any{
			{"username": "browserroot", "password": f.password, "role": "root", "id": admin.ID},
			{"username": "browser_one", "password": f.password, "role": "user", "id": firstUser.ID},
			{"username": "browser_two", "password": f.password, "role": "user", "id": secondUser.ID},
			{"username": "browser_reader", "password": f.password, "role": "read-only-admin", "id": reader.ID},
		},
		"campaign":        map[string]any{"id": campaign.ID, "name": campaignName, "reward_quota": rewardQuota, "group_id": group},
		"manual_campaign": map[string]any{"id": manualCampaign.ID, "name": "Manual review browser fixture", "reward_quota": rewardQuota, "group_id": group},
		"empty_group_id":  emptyGroup,
		"gpt_load": map[string]any{
			"base_url": f.gptLoad.url, "integration_base_url": f.bridge.server.URL,
			"admin_key": f.adminKey, "integration_token": f.integrationToken,
		},
		"keys": map[string]any{
			"valid_desktop": desktopKey, "valid_mobile": mobileKey, "valid_other_user": otherUserKey,
			"invalid": invalidKey, "rate_limited": limitedKey, "inventory": inventoryKey,
			"mixed_desktop_text": desktopKey + "\r\n" + invalidKey + "\n" + inventoryKey + "\r\n" + desktopKey + "\n" + limitedKey,
			"manual_valid":       manualKey, "manual_reject": manualRejectKey, "manual_stop": manualStopKey,
		},
		"controls":  map[string]any{"stop_file": stopFile, "resolve_rate_limit_file": resolveLimitFile},
		"processes": map[string]any{"runner_pid": os.Getpid(), "gpt_load_pid": f.gptLoad.command.Process.Pid, "new_api_pid": f.newAPI.command.Process.Pid},
	}
	// Exit through cleanup before Go's own timeout panic can orphan a child.
	// With -timeout=0, only a stop file, signal, or unexpected child exit ends it.
	var deadline <-chan time.Time
	if at, limited := t.Deadline(); limited {
		shutdownAt := at.Add(-30 * time.Second)
		require.True(t, time.Now().Before(shutdownAt), "allow at least 30 seconds for fixture cleanup")
		metadata["shutdown_deadline_at_ms"] = shutdownAt.UnixMilli()
		timer := time.NewTimer(time.Until(shutdownAt))
		defer timer.Stop()
		deadline = timer.C
	}
	encoded, err := common.Marshal(metadata)
	require.NoError(t, err)
	// Rename publishes only complete JSON to the browser runner's readiness poll.
	metadataTemporary := metadataPath + ".tmp"
	require.NoError(t, os.WriteFile(metadataTemporary, append(encoded, '\n'), 0600))
	require.NoError(t, os.Rename(metadataTemporary, metadataPath))
	t.Logf("Donation browser fixture ready; private metadata: %s", metadataPath)

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	resolvedLimit := false
	for {
		select {
		case <-interrupts:
			t.Log("Donation browser fixture received a shutdown signal")
			return
		case <-deadline:
			t.Fatal("browser fixture reached its test deadline; stopping owned services")
		case <-f.gptLoad.done:
			t.Fatalf("fixture gpt-load exited unexpectedly: %v", f.gptLoad.waitError)
		case <-f.newAPI.done:
			t.Fatalf("fixture new-api exited unexpectedly: %v", f.newAPI.waitError)
		case <-ticker.C:
			if _, err := os.Stat(stopFile); err == nil {
				t.Log("Donation browser fixture stop file received")
				return
			} else if !os.IsNotExist(err) {
				require.NoError(t, err)
			}
			if !resolvedLimit {
				if _, err := os.Stat(resolveLimitFile); err == nil {
					f.upstream.configure(limitedKey, http.StatusOK, nil)
					resolvedLimit = true
					t.Log("Donation browser fixture rate-limited key is now available")
				} else if !os.IsNotExist(err) {
					require.NoError(t, err)
				}
			}
		}
	}
}

type donationIntegrationActor struct {
	ID      int64
	Token   string
	Cookies []*http.Cookie
}

type donationIntegrationWallet struct {
	ID             int64 `json:"id"`
	Quota          int64 `json:"quota"`
	TemporaryQuota int64 `json:"temporary_quota"`
}

type donationIntegrationCampaign struct {
	ID      int64 `json:"id"`
	Version int   `json:"version"`
}

type donationIntegrationItem struct {
	ID                 string  `json:"id"`
	BatchID            string  `json:"batch_id"`
	Line               int     `json:"line"`
	KeyMask            string  `json:"key_mask"`
	State              string  `json:"state"`
	ReasonCode         string  `json:"reason_code"`
	CredentialID       *uint64 `json:"credential_id"`
	AcceptedAtMS       *int64  `json:"accepted_at_ms"`
	RewardState        string  `json:"reward_state"`
	RewardReason       string  `json:"reward_reason"`
	RewardedQuota      int64   `json:"rewarded_quota"`
	RewardedAtMS       *int64  `json:"rewarded_at_ms"`
	EffectiveMode      string  `json:"effective_mode"`
	ItemRevision       int64   `json:"item_revision"`
	ReviewNote         string  `json:"review_note"`
	StagingExpiresAtMS int64   `json:"staging_expires_at_ms"`
}

type donationIntegrationSummary struct {
	Total         int   `json:"total"`
	Accepted      int   `json:"accepted"`
	Invalid       int   `json:"invalid"`
	Duplicate     int   `json:"duplicate"`
	Processing    int   `json:"processing"`
	Rewarded      int   `json:"rewarded"`
	RewardedQuota int64 `json:"rewarded_quota"`
	PendingReview int   `json:"pending_review"`
	Rejected      int   `json:"rejected"`
}

type donationIntegrationBatch struct {
	ID              string                     `json:"id"`
	UserID          int64                      `json:"user_id"`
	CampaignID      int64                      `json:"campaign_id"`
	CampaignVersion int                        `json:"campaign_version"`
	GroupID         uint64                     `json:"group_id"`
	RewardQuota     int64                      `json:"reward_quota"`
	ReceptionState  string                     `json:"reception_state"`
	ValidationMode  string                     `json:"validation_mode"`
	LastError       string                     `json:"last_error"`
	Items           []donationIntegrationItem  `json:"items"`
	Summary         donationIntegrationSummary `json:"summary"`
}

func (batch donationIntegrationBatch) itemAt(line int) donationIntegrationItem {
	for _, item := range batch.Items {
		if item.Line == line {
			return item
		}
	}
	return donationIntegrationItem{}
}

func (batch donationIntegrationBatch) itemIDs() []string {
	ids := make([]string, 0, len(batch.Items))
	for _, item := range batch.Items {
		ids = append(ids, item.ID)
	}
	return ids
}

func (batch donationIntegrationBatch) lines() []int {
	lines := make([]int, 0, len(batch.Items))
	for _, item := range batch.Items {
		lines = append(lines, item.Line)
	}
	return lines
}

type donationIntegrationRecord struct {
	Item                donationIntegrationItem          `json:"item"`
	Batch               donationIntegrationBatch         `json:"batch"`
	PendingReviewAction *donationIntegrationReviewAction `json:"pending_review_action"`
	Reward              *struct {
		ID           string `json:"id"`
		ItemID       string `json:"item_id"`
		UserID       int64  `json:"user_id"`
		Quota        int64  `json:"quota"`
		CreditedAtMS int64  `json:"credited_at_ms"`
	} `json:"reward"`
}

type donationIntegrationReviewContext struct {
	State                string `json:"state"`
	EffectiveMode        string `json:"effective_mode"`
	ReviewAction         string `json:"review_action"`
	ItemRevision         int64  `json:"item_revision"`
	ReviewTargetRevision string `json:"review_target_revision"`
	ExpiresAtMS          int64  `json:"expires_at_ms"`
	CanReview            bool   `json:"can_review"`
	CanReject            bool   `json:"can_reject"`
	CanTest              bool   `json:"can_test"`
}

type donationIntegrationReviewAction struct {
	ActionID string `json:"action_id"`
	Status   string `json:"status"`
}

type donationIntegrationTestResult struct {
	TestID       string `json:"test_id"`
	State        string `json:"state"`
	ReasonCode   string `json:"reason_code"`
	StatusCode   int    `json:"status_code"`
	Text         string `json:"text"`
	FinishedAtMS int64  `json:"finished_at_ms"`
}

type donationRemoteIdentity struct {
	ProtocolVersion string `json:"protocol_version"`
	InstanceID      string `json:"instance_id"`
	SourceID        string `json:"source_id"`
}

type donationRemoteBatch struct {
	BatchID string `json:"batch_id"`
	Items   []struct {
		ItemID       string  `json:"item_id"`
		State        string  `json:"state"`
		CredentialID *uint64 `json:"credential_id"`
		AcceptedAtMS *int64  `json:"accepted_at_ms"`
	} `json:"items"`
}

type donationCredentialCollection struct {
	Items []struct {
		CredentialID uint64 `json:"credential_id"`
	} `json:"items"`
}

type donationIntegrationHTTPResponse struct {
	status   int
	body     []byte
	headers  http.Header
	cookies  []*http.Cookie
	envelope struct {
		Success bool            `json:"success"`
		Code    json.RawMessage `json:"code"`
		Data    json.RawMessage `json:"data"`
	}
}

type donationIntegrationProcess struct {
	binary     string
	dir        string
	url        string
	port       string
	args       []string
	env        []string
	command    *exec.Cmd
	done       chan struct{}
	waitError  error
	generation int
}

func (process *donationIntegrationProcess) stop(t *testing.T) {
	t.Helper()
	if process == nil || process.command == nil {
		return
	}
	select {
	case <-process.done:
	default:
		// Kill is intentional: the recovery case models an abrupt process loss.
		err := process.command.Process.Kill()
		assert.True(t, err == nil || err == os.ErrProcessDone, "stop fixture child: %v", err)
		select {
		case <-process.done:
		case <-time.After(10 * time.Second):
			t.Error("fixture child did not exit after being killed")
		}
	}
	process.command = nil
}

type donationIntegrationFixture struct {
	dir              string
	client           *http.Client
	password         string
	adminKey         string
	integrationToken string
	secrets          []string
	gptLoad          *donationIntegrationProcess
	newAPI           *donationIntegrationProcess
	upstream         *donationGeminiUpstream
	bridge           *donationReceiptBridge
	blockedEgress    *httptest.Server
	blockedMu        sync.Mutex
	blockedCount     int
}

func newDonationIntegrationFixture(t *testing.T) *donationIntegrationFixture {
	t.Helper()
	return newDonationIntegrationFixtureIn(t, "")
}

func newDonationIntegrationFixtureIn(t *testing.T, directory string) *donationIntegrationFixture {
	t.Helper()
	gptBinary, newAPIBinary := os.Getenv("DONATION_TEST_GPT_LOAD_BIN"), os.Getenv("DONATION_TEST_NEW_API_BIN")
	if gptBinary == "" || newAPIBinary == "" {
		t.Skip("set DONATION_TEST_GPT_LOAD_BIN and DONATION_TEST_NEW_API_BIN to run the real-service donation integration test")
	}
	for _, binary := range []string{gptBinary, newAPIBinary} {
		require.True(t, filepath.IsAbs(binary), "integration binary paths must be absolute")
		info, err := os.Stat(binary)
		require.NoError(t, err)
		require.False(t, info.IsDir())
	}
	if directory == "" {
		directory = t.TempDir()
	}
	transport := &http.Transport{DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil || !net.ParseIP(host).IsLoopback() {
			return nil, fmt.Errorf("fixture refused a non-loopback destination")
		}
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, address)
	}}
	f := &donationIntegrationFixture{
		dir: directory, password: "Donation-fixture-only-123!",
		adminKey: "fixture-admin-" + uuid.NewString(), integrationToken: "fixture-integration-" + uuid.NewString(),
		client: &http.Client{Transport: transport, Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	f.secrets = []string{f.password, f.adminKey, f.integrationToken}
	f.upstream = &donationGeminiUpstream{modes: make(map[string]donationGeminiMode), calls: make(map[string]int)}
	f.upstream.server = httptest.NewServer(f.upstream)
	f.blockedEgress = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		f.blockedMu.Lock()
		f.blockedCount++
		f.blockedMu.Unlock()
		writer.WriteHeader(http.StatusBadGateway)
	}))
	f.gptLoad = &donationIntegrationProcess{binary: gptBinary, dir: filepath.Join(f.dir, "gpt-load")}
	f.newAPI = &donationIntegrationProcess{binary: newAPIBinary, dir: filepath.Join(f.dir, "new-api")}
	t.Cleanup(func() {
		f.newAPI.stop(t)
		f.gptLoad.stop(t)
		if f.bridge != nil {
			f.bridge.server.Close()
		}
		f.upstream.close()
		f.blockedEgress.Close()
		transport.CloseIdleConnections()
		f.blockedMu.Lock()
		assert.Zero(t, f.blockedCount, "a child attempted outbound traffic through the fixture's deny proxy")
		f.blockedMu.Unlock()
		require.NoError(t, filepath.WalkDir(f.dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || filepath.Ext(path) != ".log" {
				return err
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relativePath, err := filepath.Rel(f.dir, path)
			if err != nil {
				return err
			}
			f.assertNoSecrets(t, body, "process log "+relativePath)
			if t.Failed() {
				text := f.redact(string(body))
				if len(text) > 4096 {
					text = text[len(text)-4096:]
				}
				t.Logf("%s (redacted tail):\n%s", relativePath, text)
			}
			return nil
		}))
	})
	return f
}

func (f *donationIntegrationFixture) start(t *testing.T) {
	t.Helper()
	for _, process := range []*donationIntegrationProcess{f.gptLoad, f.newAPI} {
		require.NoError(t, os.MkdirAll(filepath.Join(process.dir, "tmp"), 0700))
		require.NoError(t, os.MkdirAll(filepath.Join(process.dir, "logs"), 0700))
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		require.NoError(t, err)
		process.port = strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
		require.NoError(t, listener.Close())
		process.url = "http://127.0.0.1:" + process.port
		// A minimal allowlist prevents inherited production DB, cache, OAuth,
		// provider, tracing, proxy, or secret configuration reaching a child.
		for _, key := range []string{"SystemRoot", "WINDIR", "COMSPEC", "PATH", "PATHEXT"} {
			if value, ok := os.LookupEnv(key); ok {
				process.env = append(process.env, key+"="+value)
			}
		}
		process.env = append(process.env,
			"GOMAXPROCS=2", "GIN_MODE=release", "TZ=UTC", "TEMP="+filepath.Join(process.dir, "tmp"), "TMP="+filepath.Join(process.dir, "tmp"), "TMPDIR="+filepath.Join(process.dir, "tmp"),
			"PORT="+process.port, "HTTP_PROXY="+f.blockedEgress.URL, "HTTPS_PROXY="+f.blockedEgress.URL, "ALL_PROXY="+f.blockedEgress.URL, "NO_PROXY=127.0.0.1,::1,localhost")
		if process == f.gptLoad {
			process.env = append(process.env, "HOST=127.0.0.1", "DATABASE_DSN=", "DATA_DIR="+filepath.Join(process.dir, "data"),
				"AUTH_KEY="+f.adminKey, "DONATION_INTEGRATION_TOKEN="+f.integrationToken, "ENCRYPTION_KEY=", "MODELS_DEV_AUTO_SYNC_ENABLED=false", "LOG_LEVEL=info", "LOG_FORMAT=json")
			f.startProcess(t, process, "/integrations/donations/v1/capabilities", f.integrationToken)
		} else {
			process.env = append(process.env, "HTTP_LISTEN_HOST=127.0.0.1", "SQL_DSN=", "LOG_SQL_DSN=", "SQLITE_PATH="+filepath.Join(process.dir, "new-api.db"),
				"REDIS_CONN_STRING=", "SESSION_SECRET=fixture-stable-dashboard-session-material", "CRYPTO_SECRET=fixture-independent-local-crypto-material",
				"PASSWORD_LOGIN_ENCRYPTION_ENABLED=false", "SQL_MAX_OPEN_CONNS=1", "SQL_MAX_IDLE_CONNS=1", "NODE_TYPE=master", "DEBUG=false", "MEMORY_CACHE_ENABLED=false", "BATCH_UPDATE_ENABLED=false",
				"GLOBAL_API_RATE_LIMIT_ENABLE=false", "GLOBAL_WEB_RATE_LIMIT_ENABLE=false", "CRITICAL_RATE_LIMIT_ENABLE=false", "SEARCH_RATE_LIMIT_ENABLE=false",
				"TRUSTED_PROXIES=none", "TASK_PLUGIN_ENABLED=false", "UPDATE_TASK=false", "ENABLE_PPROF=false", "PYROSCOPE_URL=", "SHUTDOWN_TIMEOUT_SECONDS=1")
			process.args = []string{"--log-dir", filepath.Join(process.dir, "logs")}
			f.startProcess(t, process, "/api/setup", "")
		}
	}
	f.bridge = &donationReceiptBridge{target: f.gptLoad.url, client: f.client, posts: make(map[string]int), blocked: make(map[string]bool)}
	f.bridge.server = httptest.NewServer(f.bridge)
}

func (f *donationIntegrationFixture) startProcess(t *testing.T, process *donationIntegrationProcess, readyPath, token string) {
	t.Helper()
	process.generation++
	output, err := os.OpenFile(filepath.Join(process.dir, fmt.Sprintf("process-%d.log", process.generation)), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	require.NoError(t, err)
	process.command = exec.Command(process.binary, process.args...)
	process.command.Dir = process.dir
	process.command.Env = process.env
	process.command.Stdout, process.command.Stderr = output, output
	process.done = make(chan struct{})
	process.waitError = nil
	if err := process.command.Start(); err != nil {
		_ = output.Close()
		close(process.done)
		require.NoError(t, err)
	}
	go func() {
		process.waitError = process.command.Wait()
		_ = output.Close()
		close(process.done)
	}()
	f.await(t, filepath.Base(process.dir)+" readiness", func() (bool, string) {
		select {
		case <-process.done:
			t.Fatalf("%s exited before readiness: %v", filepath.Base(process.dir), process.waitError)
		default:
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		response, err := f.exchange(ctx, token, http.MethodGet, process.url+readyPath, nil, "", nil)
		if err != nil {
			return false, "waiting for loopback HTTP readiness"
		}
		return response.status == http.StatusOK, f.redact(string(response.body))
	})
	// A wildcard listener would also claim this other loopback address. This
	// verifies the real child honored the loopback-only bind configuration.
	probe, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.2", process.port))
	require.NoError(t, err, "the child must listen only on 127.0.0.1")
	require.NoError(t, probe.Close())
}

func (f *donationIntegrationFixture) exchange(ctx context.Context, token, method, address string, body []byte, requestID string, cookies []*http.Cookie) (donationIntegrationHTTPResponse, error) {
	var result donationIntegrationHTTPResponse
	request, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", "en")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if requestID != "" {
		request.Header.Set("Idempotency-Key", requestID)
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response, err := f.client.Do(request)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	result.status, result.headers, result.cookies = response.StatusCode, response.Header.Clone(), response.Cookies()
	result.body, err = io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return result, err
	}
	if err := common.Unmarshal(result.body, &result.envelope); err != nil {
		return result, fmt.Errorf("HTTP %d returned a non-JSON envelope", result.status)
	}
	return result, nil
}

func (f *donationIntegrationFixture) request(t *testing.T, token, method, address string, body any, requestID string, cookies []*http.Cookie) donationIntegrationHTTPResponse {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = common.Marshal(body)
		require.NoError(t, err)
	}
	response, err := f.exchange(t.Context(), token, method, address, payload, requestID, cookies)
	require.NoError(t, err)
	f.assertNoSecrets(t, response.body, method+" HTTP response")
	return response
}

func (f *donationIntegrationFixture) api(t *testing.T, token, method, path string, body any, requestID string, target any) donationIntegrationHTTPResponse {
	t.Helper()
	response := f.request(t, token, method, f.newAPI.url+path, body, requestID, nil)
	require.Equal(t, http.StatusOK, response.status, "%s %s: %s", method, path, f.redact(string(response.body)))
	require.True(t, response.envelope.Success, "%s %s: %s", method, path, f.redact(string(response.body)))
	if strings.HasPrefix(path, "/api/donations/") {
		assert.Contains(t, response.headers.Get("Cache-Control"), "no-store")
	}
	if target != nil {
		require.NoError(t, common.Unmarshal(response.envelope.Data, target), "decode %s", path)
	}
	return response
}

func (f *donationIntegrationFixture) gpt(t *testing.T, token, method, path string, body any, requestID string, target any) {
	t.Helper()
	response := f.request(t, token, method, f.gptLoad.url+path, body, requestID, nil)
	require.Equal(t, http.StatusOK, response.status, "%s %s: %s", method, path, f.redact(string(response.body)))
	require.Equal(t, "0", string(response.envelope.Code), "%s %s: %s", method, path, f.redact(string(response.body)))
	if target != nil {
		require.NoError(t, common.Unmarshal(response.envelope.Data, target))
	}
}

func (f *donationIntegrationFixture) login(t *testing.T, username string) donationIntegrationActor {
	t.Helper()
	var data struct {
		Token string                    `json:"access_token"`
		User  donationIntegrationWallet `json:"user"`
	}
	response := f.api(t, "", http.MethodPost, "/api/user/login", map[string]any{"username": username, "password": f.password}, "", &data)
	require.NotEmpty(t, data.Token)
	require.Positive(t, data.User.ID)
	// Login is the intended token transport; subsequent responses and logs must
	// not echo any usable session credential.
	f.secrets = append(f.secrets, data.Token)
	for _, cookie := range response.cookies {
		if cookie.HttpOnly && cookie.Value != "" {
			f.secrets = append(f.secrets, cookie.Value)
		}
	}
	return donationIntegrationActor{ID: data.User.ID, Token: data.Token, Cookies: response.cookies}
}

func (f *donationIntegrationFixture) register(t *testing.T, username string) donationIntegrationActor {
	t.Helper()
	f.api(t, "", http.MethodPost, "/api/user/register", map[string]any{"username": username, "password": f.password}, "", nil)
	return f.login(t, username)
}

func (f *donationIntegrationFixture) wallet(t *testing.T, actor donationIntegrationActor) donationIntegrationWallet {
	t.Helper()
	var result donationIntegrationWallet
	f.api(t, actor.Token, http.MethodGet, "/api/user/self", nil, "", &result)
	return result
}

func (f *donationIntegrationFixture) createDonationReader(t *testing.T, admin donationIntegrationActor, username string) donationIntegrationActor {
	t.Helper()
	f.api(t, admin.Token, http.MethodPost, "/api/user/", map[string]any{
		"username": username, "password": f.password, "role": 10,
		"admin_permissions": map[string]map[string]bool{
			"donation_records": {"read": true, "review": false, "test": false},
			"donation_config":  {"read": false, "write": false},
		},
	}, "", nil)
	return f.login(t, username)
}

func (f *donationIntegrationFixture) reviewContext(t *testing.T, actor donationIntegrationActor, itemID string) donationIntegrationReviewContext {
	t.Helper()
	var result donationIntegrationReviewContext
	f.api(t, actor.Token, http.MethodGet, "/api/donations/admin/records/"+itemID+"/review-context", nil, "", &result)
	return result
}

func (f *donationIntegrationFixture) streamTest(t *testing.T, actor donationIntegrationActor, itemID, testID string, input map[string]any) donationIntegrationTestResult {
	t.Helper()
	payload, err := common.Marshal(input)
	require.NoError(t, err)
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
		f.newAPI.url+"/api/donations/admin/records/"+itemID+"/tests", bytes.NewReader(payload))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+actor.Token)
	request.Header.Set("Idempotency-Key", testID)
	response, err := f.client.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	require.NoError(t, err)
	f.assertNoSecrets(t, body, "manual review stream")
	require.Equal(t, http.StatusOK, response.StatusCode, "%s", f.redact(string(body)))
	require.Contains(t, response.Header.Get("Content-Type"), "text/event-stream")
	var events []string
	var output strings.Builder
	var result donationIntegrationTestResult
	for _, frame := range strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n\n") {
		var event, data string
		for line := range strings.SplitSeq(frame, "\n") {
			if value, ok := strings.CutPrefix(line, "event:"); ok {
				event = strings.TrimSpace(value)
			}
			if value, ok := strings.CutPrefix(line, "data:"); ok {
				data = strings.TrimSpace(value)
			}
		}
		if event == "" {
			continue
		}
		events = append(events, event)
		switch event {
		case "meta":
			var meta struct {
				ItemID string `json:"item_id"`
				TestID string `json:"test_id"`
			}
			require.NoError(t, common.UnmarshalJsonStr(data, &meta))
			assert.Equal(t, itemID, meta.ItemID)
			assert.Equal(t, testID, meta.TestID)
		case "delta":
			var delta struct {
				Text string `json:"text"`
			}
			require.NoError(t, common.UnmarshalJsonStr(data, &delta))
			output.WriteString(delta.Text)
		case "done":
			require.NoError(t, common.UnmarshalJsonStr(data, &result))
		default:
			t.Fatalf("unexpected test stream event: %s", event)
		}
	}
	require.GreaterOrEqual(t, len(events), 3)
	assert.Equal(t, "meta", events[0])
	assert.Equal(t, "done", events[len(events)-1])
	for _, event := range events[1 : len(events)-1] {
		assert.Equal(t, "delta", event)
	}
	assert.Positive(t, result.FinishedAtMS)
	result.Text = output.String()
	return result
}

func (f *donationIntegrationFixture) createGroup(t *testing.T, name, inventory string) uint64 {
	t.Helper()
	var result struct {
		GroupID uint64 `json:"group_id"`
	}
	f.gpt(t, f.adminKey, http.MethodPost, "/api/groups", map[string]any{
		"name": name, "channel_id": "gemini", "connection_type": "api_key", "params": map[string]any{"base_url": f.upstream.server.URL + "/v1beta"},
		"models": []map[string]any{{"id": "gemini-donation-test", "alias_enabled": false}}, "credentials": inventory, "confirm_same_target": true,
		"proxy": map[string]any{"mode": "direct"},
	}, uuid.NewString(), &result)
	require.Positive(t, result.GroupID)
	return result.GroupID
}

func (f *donationIntegrationFixture) credentials(t *testing.T, groupID uint64) donationCredentialCollection {
	t.Helper()
	var result donationCredentialCollection
	f.gpt(t, f.adminKey, http.MethodGet, fmt.Sprintf("/api/groups/%d/credentials", groupID), nil, "", &result)
	return result
}

func (f *donationIntegrationFixture) createCampaign(t *testing.T, admin donationIntegrationActor, group uint64, reward int64, name string) donationIntegrationCampaign {
	t.Helper()
	var result donationIntegrationCampaign
	f.api(t, admin.Token, http.MethodPost, "/api/donations/admin/campaigns", map[string]any{
		"name": name, "description": "Local integration fixture", "group_id": group, "reward_quota": reward, "enabled": true,
	}, "", &result)
	require.Positive(t, result.ID)
	return result
}

func (f *donationIntegrationFixture) submit(t *testing.T, actor donationIntegrationActor, campaign int64, requestID, keys string) donationIntegrationBatch {
	t.Helper()
	var result donationIntegrationBatch
	f.api(t, actor.Token, http.MethodPost, "/api/donations/batches", map[string]any{"campaign_id": campaign, "keys_text": keys}, requestID, &result)
	require.NotEmpty(t, result.ID)
	assert.Equal(t, actor.ID, result.UserID)
	return result
}

func (f *donationIntegrationFixture) record(t *testing.T, actor donationIntegrationActor, itemID string) donationIntegrationRecord {
	t.Helper()
	var result donationIntegrationRecord
	f.api(t, actor.Token, http.MethodGet, "/api/donations/admin/records/"+itemID, nil, "", &result)
	return result
}

func (f *donationIntegrationFixture) await(t *testing.T, description string, poll func() (bool, string)) {
	t.Helper()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready, state := poll()
		if ready {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s; last state: %s", description, f.redact(state))
		case <-t.Context().Done():
			t.Fatalf("test canceled while waiting for %s", description)
		case <-ticker.C:
		}
	}
}

func (f *donationIntegrationFixture) awaitBatch(t *testing.T, actor donationIntegrationActor, batchID string, done func(donationIntegrationBatch) bool) donationIntegrationBatch {
	t.Helper()
	var result donationIntegrationBatch
	f.await(t, "donation batch "+batchID, func() (bool, string) {
		f.api(t, actor.Token, http.MethodGet, "/api/donations/batches/"+batchID, nil, "", &result)
		return done(result), f.snapshot(result)
	})
	return result
}

func (f *donationIntegrationFixture) awaitRemoteAccepted(t *testing.T, batchID string) donationRemoteBatch {
	t.Helper()
	var result donationRemoteBatch
	f.await(t, "persisted gpt-load receipt "+batchID, func() (bool, string) {
		f.gpt(t, f.integrationToken, http.MethodGet, "/integrations/donations/v1/batches/"+batchID, nil, "", &result)
		return len(result.Items) == 1 && result.Items[0].State == "accepted", f.snapshot(result)
	})
	return result
}

func (f *donationIntegrationFixture) awaitProbe(t *testing.T, gate *donationProbeGate) {
	t.Helper()
	t.Cleanup(gate.release)
	select {
	case <-gate.started:
	case <-time.After(20 * time.Second):
		t.Fatal("the submitted credential did not reach the Gemini upstream")
	}
}

func (f *donationIntegrationFixture) snapshot(value any) string {
	data, err := common.Marshal(value)
	if err != nil {
		return "unable to encode observed state"
	}
	return f.redact(string(data))
}

func (f *donationIntegrationFixture) redact(value string) string {
	for _, secret := range f.secrets {
		value = strings.ReplaceAll(value, secret, "[redacted]")
	}
	return value
}

func (f *donationIntegrationFixture) assertNoSecrets(t *testing.T, data []byte, surface string) {
	t.Helper()
	for _, secret := range f.secrets {
		assert.False(t, bytes.Contains(data, []byte(secret)), "%s exposed a fixture credential", surface)
	}
}

type donationProbeGate struct {
	started     chan struct{}
	released    chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func newDonationProbeGate() *donationProbeGate {
	return &donationProbeGate{started: make(chan struct{}), released: make(chan struct{})}
}

func (gate *donationProbeGate) release() {
	gate.releaseOnce.Do(func() { close(gate.released) })
}

type donationGeminiMode struct {
	status int
	gate   *donationProbeGate
	chat   bool
}

type donationGeminiUpstream struct {
	server      *httptest.Server
	mu          sync.Mutex
	modes       map[string]donationGeminiMode
	calls       map[string]int
	issues      []string
	chatPrompts map[string]string
}

func (upstream *donationGeminiUpstream) configure(key string, status int, gate *donationProbeGate) {
	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	upstream.modes[key] = donationGeminiMode{status: status, gate: gate}
}

func (upstream *donationGeminiUpstream) configureChat(key string, status int, gate *donationProbeGate) {
	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	upstream.modes[key] = donationGeminiMode{status: status, gate: gate, chat: true}
	if upstream.chatPrompts == nil {
		upstream.chatPrompts = make(map[string]string)
	}
}

func (upstream *donationGeminiUpstream) chatPrompt(key string) string {
	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	return upstream.chatPrompts[key]
}

func (upstream *donationGeminiUpstream) count(key string) int {
	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	return upstream.calls[key]
}

func (upstream *donationGeminiUpstream) problems() []string {
	upstream.mu.Lock()
	defer upstream.mu.Unlock()
	return slices.Clone(upstream.issues)
}

func (upstream *donationGeminiUpstream) close() {
	upstream.mu.Lock()
	for _, mode := range upstream.modes {
		if mode.gate != nil {
			mode.gate.release()
		}
	}
	upstream.mu.Unlock()
	upstream.server.Close()
}

func (upstream *donationGeminiUpstream) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	key := request.Header.Get("X-Goog-Api-Key")
	if key == "" {
		key = request.URL.Query().Get("key")
	}
	upstream.mu.Lock()
	upstream.calls[key]++
	mode, known := upstream.modes[key]
	if !known {
		upstream.issues = append(upstream.issues, "probe used an unconfigured credential")
		mode.status = http.StatusUnauthorized
	}
	stream := request.URL.Path == "/v1beta/models/gemini-donation-test:streamGenerateContent"
	if request.Method != http.MethodPost || (request.URL.Path != "/v1beta/models/gemini-donation-test:generateContent" && !(mode.chat && stream)) {
		upstream.issues = append(upstream.issues, "probe did not use the configured Gemini generateContent endpoint")
	}
	upstream.mu.Unlock()
	if mode.chat {
		var body struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
			GenerationConfig struct {
				MaxOutputTokens int `json:"maxOutputTokens"`
			} `json:"generationConfig"`
		}
		err := common.DecodeJson(io.LimitReader(request.Body, 65536), &body)
		var textParts []string
		for _, content := range body.Contents {
			for _, part := range content.Parts {
				textParts = append(textParts, part.Text)
			}
		}
		prompt := strings.Join(textParts, "")
		if err != nil || prompt == "" {
			upstream.mu.Lock()
			upstream.issues = append(upstream.issues, "manual chat omitted the user's text")
			upstream.mu.Unlock()
			mode.status = http.StatusBadRequest
		} else if prompt == "ping" || body.GenerationConfig.MaxOutputTokens == 1 {
			// This upstream accepts normal chat but deliberately rejects the
			// existing generic probe shape, modelling the reported provider.
			mode.status, mode.gate = http.StatusServiceUnavailable, nil
		} else {
			upstream.mu.Lock()
			upstream.chatPrompts[key] = prompt
			upstream.mu.Unlock()
		}
	}
	if mode.gate != nil {
		mode.gate.startedOnce.Do(func() { close(mode.gate.started) })
		select {
		case <-mode.gate.released:
		case <-request.Context().Done():
			return
		}
	}
	if mode.chat && mode.status == http.StatusOK {
		if stream {
			writer.Header().Set("Content-Type", "text/event-stream")
			writer.WriteHeader(http.StatusOK)
			for _, text := range []string{"manual reply " + key[:len(key)/2], key[len(key)/2:] + " done"} {
				candidate := map[string]any{"index": 0, "content": map[string]any{"role": "model", "parts": []map[string]any{{"text": text}}}}
				data, _ := common.Marshal(map[string]any{"candidates": []map[string]any{candidate}, "modelVersion": "gemini-donation-test"})
				_, _ = fmt.Fprintf(writer, "data: %s\n\n", data)
				if flush, ok := writer.(http.Flusher); ok {
					flush.Flush()
				}
			}
			// Gemini's SDK completes the stream with its final usage-bearing
			// STOP frame, separate from the body deltas under test.
			terminal, _ := common.Marshal(map[string]any{
				"candidates":    []map[string]any{{"index": 0, "content": map[string]any{"role": "model", "parts": []any{}}, "finishReason": "STOP"}},
				"usageMetadata": map[string]any{"promptTokenCount": 7, "candidatesTokenCount": 11, "totalTokenCount": 18},
				"modelVersion":  "gemini-donation-test",
			})
			_, _ = fmt.Fprintf(writer, "data: %s\n\n", terminal)
			return
		}
		data, _ := common.Marshal(map[string]any{
			"candidates":   []map[string]any{{"content": map[string]any{"role": "model", "parts": []map[string]any{{"text": "manual reply " + key + " done"}}}, "finishReason": "STOP"}},
			"modelVersion": "gemini-donation-test",
		})
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(data)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(mode.status)
	if mode.status == http.StatusOK {
		_, _ = io.WriteString(writer, `{"candidates":[{"content":{"role":"model","parts":[{"text":"pong"}]},"finishReason":"STOP"}],"modelVersion":"gemini-donation-test"}`)
		return
	}
	// Deliberately echo the synthetic key upstream. Neither application's API,
	// ordinary logs, nor administrative audit responses may propagate it.
	errorStatus := "UNAUTHENTICATED"
	if mode.status == http.StatusTooManyRequests {
		errorStatus = "RESOURCE_EXHAUSTED"
	}
	body, _ := common.Marshal(map[string]any{"error": map[string]any{"code": mode.status, "status": errorStatus, "message": key}})
	_, _ = writer.Write(body)
}

// This transport fault injector forwards to the real gpt-load process before
// discarding one receipt. It never fabricates an acceptance or reward outcome.
type donationReceiptBridge struct {
	server         *httptest.Server
	target         string
	client         *http.Client
	mu             sync.Mutex
	loseNext       bool
	posts          map[string]int
	blocked        map[string]bool
	loseReview     bool
	blockedActions map[string]bool
}

func (bridge *donationReceiptBridge) loseNextBatchResponse() {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	bridge.loseNext = true
}

func (bridge *donationReceiptBridge) resumeBatchReads() {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	clear(bridge.blocked)
	clear(bridge.blockedActions)
}

func (bridge *donationReceiptBridge) loseNextReviewResponse() {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	bridge.loseReview = true
	if bridge.blockedActions == nil {
		bridge.blockedActions = make(map[string]bool)
	}
}

func (bridge *donationReceiptBridge) postCount(batchID string) int {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	return bridge.posts[batchID]
}

func (bridge *donationReceiptBridge) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	const prefix = "/integrations/donations/v1/batches"
	body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
	if err != nil {
		writer.WriteHeader(http.StatusBadGateway)
		return
	}
	drop := false
	actionID := ""
	if request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/review-actions") {
		var submitted struct {
			ActionID string `json:"action_id"`
		}
		if common.Unmarshal(body, &submitted) == nil {
			actionID = submitted.ActionID
		}
	}
	bridge.mu.Lock()
	blockedReviewRead := request.Method == http.MethodGet &&
		bridge.blockedActions[strings.TrimPrefix(request.URL.Path, "/integrations/donations/v1/review-actions/")]
	if (request.Method == http.MethodGet && bridge.blocked[strings.TrimPrefix(request.URL.Path, prefix+"/")]) ||
		blockedReviewRead || (actionID != "" && bridge.blockedActions[actionID]) {
		bridge.mu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(writer, `{"code":"FIXTURE_RECEIPT_UNAVAILABLE","message":"receipt transport unavailable"}`)
		return
	}
	if request.Method == http.MethodPost && request.URL.Path == prefix {
		var submitted struct {
			BatchID string `json:"batch_id"`
		}
		if common.Unmarshal(body, &submitted) == nil {
			bridge.posts[submitted.BatchID]++
			drop = bridge.loseNext
			if drop {
				bridge.loseNext = false
				bridge.blocked[submitted.BatchID] = true
			}
		}
	}
	if actionID != "" && bridge.loseReview {
		drop, bridge.loseReview = true, false
		bridge.blockedActions[actionID] = true
		parts := strings.Split(strings.TrimPrefix(request.URL.Path, prefix+"/"), "/")
		if len(parts) == 4 {
			bridge.blocked[parts[0]] = true
		}
	}
	bridge.mu.Unlock()
	forward, err := http.NewRequestWithContext(request.Context(), request.Method, bridge.target+request.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		writer.WriteHeader(http.StatusBadGateway)
		return
	}
	forward.Header = request.Header.Clone()
	response, err := bridge.client.Do(forward)
	if err != nil {
		writer.WriteHeader(http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	if !drop && strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		for key, values := range response.Header {
			writer.Header()[key] = values
		}
		writer.WriteHeader(response.StatusCode)
		buffer := make([]byte, 4096)
		for {
			n, readErr := response.Body.Read(buffer)
			if n > 0 {
				if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
					return
				}
				if flush, ok := writer.(http.Flusher); ok {
					flush.Flush()
				}
			}
			if readErr != nil {
				return
			}
		}
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		writer.WriteHeader(http.StatusBadGateway)
		return
	}
	if drop {
		if hijacker, ok := writer.(http.Hijacker); ok {
			connection, _, err := hijacker.Hijack()
			if err == nil {
				_ = connection.Close()
				return
			}
		}
		writer.WriteHeader(http.StatusBadGateway)
		return
	}
	for key, values := range response.Header {
		writer.Header()[key] = values
	}
	writer.WriteHeader(response.StatusCode)
	_, _ = writer.Write(responseBody)
}

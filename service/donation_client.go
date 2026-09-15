package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type DonationCapabilities struct {
	ProtocolVersion string   `json:"protocol_version"`
	InstanceID      string   `json:"instance_id"`
	SourceID        string   `json:"source_id"`
	InputTypes      []string `json:"input_types"`
	// Features negotiates the optional manual-review contract. A released
	// receiver omits it, which keeps every automatic path unchanged.
	Features []string `json:"features"`
	Limits   struct {
		MaxItems                int   `json:"max_items"`
		MaxKeyBytes             int   `json:"max_key_bytes"`
		MaxBodyBytes            int   `json:"max_body_bytes"`
		StagingRetentionSeconds int64 `json:"staging_retention_seconds"`
	} `json:"limits"`
	ReviewLimits DonationReviewLimits `json:"review_limits"`
}

// DonationReviewLimits is the receiver-declared bound for one controlled chat
// test. Local bounds are only ever tightened by it, never loosened.
type DonationReviewLimits struct {
	MaxPromptBytes          int `json:"max_prompt_bytes"`
	MaxRequestBytes         int `json:"max_request_bytes"`
	DefaultOutputTokens     int `json:"default_output_tokens"`
	MaxOutputTokens         int `json:"max_output_tokens"`
	MaxResponseBytes        int `json:"max_response_bytes"`
	MaxEventBytes           int `json:"max_event_bytes"`
	TotalTimeoutSeconds     int `json:"total_timeout_seconds"`
	FirstByteTimeoutSeconds int `json:"first_byte_timeout_seconds"`
	IdleTimeoutSeconds      int `json:"idle_timeout_seconds"`
	MaxNoteBytes            int `json:"max_note_bytes"`
}

func donationLocalReviewLimits() DonationReviewLimits {
	return DonationReviewLimits{MaxPromptBytes: model.DonationMaxPromptBytes, MaxRequestBytes: model.DonationMaxTestRequestBytes,
		DefaultOutputTokens: model.DonationDefaultOutputTokens, MaxOutputTokens: model.DonationMaxOutputTokens,
		MaxResponseBytes: model.DonationMaxTestResponseBytes, MaxEventBytes: model.DonationMaxTestEventBytes,
		TotalTimeoutSeconds: model.DonationTestTotalTimeout, FirstByteTimeoutSeconds: 30, IdleTimeoutSeconds: 20,
		MaxNoteBytes: model.DonationMaxNoteBytes}
}

// donationReviewLimits tightens the local protocol bounds with whatever the
// receiver declares, so both ends enforce the same smaller limit.
func donationReviewLimits(caps DonationCapabilities) DonationReviewLimits {
	limits := donationLocalReviewLimits()
	remote := caps.ReviewLimits
	for _, bound := range []struct {
		remote int
		local  *int
	}{
		{remote.MaxPromptBytes, &limits.MaxPromptBytes},
		{remote.MaxRequestBytes, &limits.MaxRequestBytes},
		{remote.DefaultOutputTokens, &limits.DefaultOutputTokens},
		{remote.MaxOutputTokens, &limits.MaxOutputTokens},
		{remote.MaxResponseBytes, &limits.MaxResponseBytes},
		{remote.MaxEventBytes, &limits.MaxEventBytes},
		{remote.TotalTimeoutSeconds, &limits.TotalTimeoutSeconds},
		{remote.FirstByteTimeoutSeconds, &limits.FirstByteTimeoutSeconds},
		{remote.IdleTimeoutSeconds, &limits.IdleTimeoutSeconds},
		{remote.MaxNoteBytes, &limits.MaxNoteBytes},
	} {
		if bound.remote > 0 && bound.remote < *bound.local {
			*bound.local = bound.remote
		}
	}
	limits.DefaultOutputTokens = min(limits.DefaultOutputTokens, limits.MaxOutputTokens)
	return limits
}

func supportsDonationManualReview(caps DonationCapabilities) bool {
	return slices.Contains(caps.Features, model.DonationFeatureManualReview)
}

type DonationGroup struct {
	ID                uint64 `json:"id"`
	Name              string `json:"name"`
	ChannelID         string `json:"channel_id"`
	ConnectionType    string `json:"connection_type"`
	Enabled           bool   `json:"enabled"`
	CanProbe          bool   `json:"can_probe"`
	TargetRevision    string `json:"target_revision"`
	UnavailableReason string `json:"unavailable_reason"`
	// Manual-review capability is independent from the probe capability.
	CanManualReview         bool   `json:"can_manual_review"`
	ManualTargetRevision    string `json:"manual_target_revision"`
	ManualUnavailableReason string `json:"manual_unavailable_reason"`
}

type donationIntakeItem struct {
	ItemID string `json:"item_id"`
	Key    string `json:"key"`
}

type donationIntakeBatch struct {
	BatchID        string `json:"batch_id"`
	GroupID        uint64 `json:"group_id"`
	TargetRevision string `json:"target_revision"`
	// ValidationMode must stay omitted for automatic batches: the original
	// frozen request digest is an HMAC over this serialized body.
	ValidationMode string               `json:"validation_mode,omitempty"`
	Items          []donationIntakeItem `json:"items"`
}

// donationWireMode keeps the automatic request byte-identical to the released
// representation: only an explicit manual batch carries the new key.
func donationWireMode(mode string) string {
	if mode == model.DonationModeManualReview {
		return model.DonationModeManualReview
	}
	return ""
}

type DonationRemoteError struct {
	Reason                string
	Status                int
	RejectedBeforeStaging bool
	Cause                 error
}

func (e *DonationRemoteError) Error() string { return "donation integration: " + e.Reason }
func (e *DonationRemoteError) Unwrap() error { return e.Cause }

type donationClient struct {
	baseURL string
	token   string
	http    *http.Client
	// long has no global timeout: a controlled chat test is bounded by its
	// context and by the receiver, not by the 12-second JSON budget.
	long *http.Client
}

func newDonationClient(baseURL, token string) (*donationClient, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || len(baseURL) > 512 || strings.TrimSpace(baseURL) != baseURL || strings.ContainsAny(baseURL, "\\\r\n\t") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" || parsed.Opaque != "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawPath != "" {
		return nil, model.ErrDonationInput
	}
	if parsed.Scheme != "https" {
		ip := net.ParseIP(parsed.Hostname())
		if parsed.Scheme != "http" || (parsed.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return nil, model.ErrDonationInput
		}
	}
	if len(token) < 32 || len(token) > 256 {
		return nil, model.ErrDonationInput
	}
	for _, b := range []byte(token) {
		if b < 33 || b > 126 {
			return nil, model.ErrDonationInput
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	// Relay TLS overrides and environment proxies do not apply to this privileged
	// integration credential. Verify the peer using the system trust store.
	newTransport := func(responseHeaderTimeout time.Duration) *http.Transport {
		transport := &http.Transport{
			DialContext: dialer.DialContext, ForceAttemptHTTP2: true,
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 10 * time.Second,
			MaxResponseHeaderBytes: 32 << 10, ResponseHeaderTimeout: responseHeaderTimeout,
			MaxIdleConns: 4, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second,
		}
		// Loopback development HTTP must stay on loopback even if localhost DNS is
		// overridden. HTTPS relies on the configured host's ordinary TLS verification.
		if parsed.Scheme == "http" && parsed.Hostname() == "localhost" {
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				_, port, err := net.SplitHostPort(address)
				if err != nil {
					return nil, err
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
			}
		}
		return transport
	}
	// One timeout policy, two budgets: the JSON control plane stays at 12 seconds
	// and a controlled test is only bounded by its own context.
	return &donationClient{baseURL: strings.TrimSuffix(baseURL, "/"), token: token,
		http: &http.Client{Timeout: 12 * time.Second, Transport: newTransport(10 * time.Second),
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		long: &http.Client{Timeout: 0, Transport: newTransport(30 * time.Second),
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func (c *donationClient) close() {
	c.http.CloseIdleConnections()
	c.long.CloseIdleConnections()
}

func (c *donationClient) request(ctx context.Context, method, path, requestKey string, input, output any) error {
	return c.do(ctx, c.http, method, path, requestKey, input, output)
}

func (c *donationClient) do(ctx context.Context, client *http.Client, method, path, requestKey string, input, output any) error {
	var body []byte
	var err error
	if input != nil {
		body, err = common.Marshal(input)
		if err != nil || len(body) > model.DonationMaxBodyBytes {
			return model.ErrDonationInput
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/integrations/donations/v1"+path, bytes.NewReader(body))
	if err != nil {
		return model.ErrDonationInput
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
		// All POST replays must increment durable send/action state. Go otherwise
		// transparently replays Idempotency-Key requests on a broken reused socket.
		req.GetBody = nil
	}
	if requestKey != "" {
		req.Header.Set("Idempotency-Key", requestKey)
	}
	response, err := client.Do(req)
	if err != nil {
		return &DonationRemoteError{Reason: "connection_unavailable"}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		reason := "integration_error"
		switch response.StatusCode {
		case http.StatusNotFound:
			reason = "receipt_not_found"
		case http.StatusUnauthorized, http.StatusForbidden:
			reason = "integration_unauthorized"
		case http.StatusConflict:
			reason = "target_or_request_conflict"
		}
		failure := &DonationRemoteError{Status: response.StatusCode, Reason: reason}
		if response.StatusCode == http.StatusConflict || response.StatusCode == http.StatusNotFound {
			envelope, ok := readDonationErrorCode(response)
			if ok {
				switch envelope {
				case "DONATION_TARGET_CHANGED":
					failure.Reason, failure.RejectedBeforeStaging = "target_changed", true
				case "DONATION_TARGET_UNAVAILABLE":
					failure.Reason, failure.RejectedBeforeStaging = "target_unavailable", true
				case "DONATION_REVIEW_NOT_FOUND":
					failure.Reason = "review_not_found"
				case "DONATION_REVIEW_CONFLICT":
					failure.Reason = "review_conflict"
				case "DONATION_TEST_UNAVAILABLE", "DONATION_TEST_CONFLICT", "DONATION_TEST_BUSY":
					failure.Reason = "test_unavailable"
				case "DONATION_ITEM_NOT_FOUND":
					failure.Reason = "item_not_found"
				}
			}
		}
		return failure
	}
	encoded, err := io.ReadAll(io.LimitReader(response.Body, model.DonationMaxBodyBytes+1))
	if err != nil || len(encoded) > model.DonationMaxBodyBytes {
		return &DonationRemoteError{Reason: "invalid_response"}
	}
	var envelope struct {
		Code json.RawMessage `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if common.Unmarshal(encoded, &envelope) != nil || strings.TrimSpace(string(envelope.Code)) != "0" || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return &DonationRemoteError{Reason: "invalid_response"}
	}
	if err := common.Unmarshal(envelope.Data, output); err != nil {
		return &DonationRemoteError{Reason: "invalid_response"}
	}
	return nil
}

func readDonationErrorCode(response *http.Response) (string, bool) {
	body, err := io.ReadAll(io.LimitReader(response.Body, model.DonationMaxBodyBytes+1))
	if err != nil || len(body) > model.DonationMaxBodyBytes {
		return "", false
	}
	var envelope struct {
		Code string `json:"code"`
	}
	if common.Unmarshal(body, &envelope) != nil {
		return "", false
	}
	return envelope.Code, envelope.Code != ""
}

func (c *donationClient) capabilities(ctx context.Context) (DonationCapabilities, error) {
	var result DonationCapabilities
	if err := c.request(ctx, http.MethodGet, "/capabilities", "", nil, &result); err != nil {
		return result, err
	}
	if result.ProtocolVersion != "1" || !model.ValidDonationID(result.InstanceID) || !model.ValidDonationID(result.SourceID) || !slices.Contains(result.InputTypes, "api_key") || result.Limits.MaxItems != model.DonationMaxItems || result.Limits.MaxKeyBytes != model.DonationMaxKeyBytes || result.Limits.MaxBodyBytes != model.DonationMaxBodyBytes || result.Limits.StagingRetentionSeconds != model.DonationStagingRetentionSeconds {
		return result, &DonationRemoteError{Reason: "protocol_mismatch"}
	}
	if supportsDonationManualReview(result) && result.ReviewLimits.MaxNoteBytes <= 0 {
		return result, &DonationRemoteError{Reason: "protocol_mismatch"}
	}
	return result, nil
}

func (c *donationClient) groups(ctx context.Context) ([]DonationGroup, error) {
	result := make([]DonationGroup, 0)
	if err := c.request(ctx, http.MethodGet, "/groups", "", nil, &result); err != nil {
		return nil, err
	}
	seen := make(map[uint64]bool)
	for i := range result {
		group := &result[i]
		if group.ID == 0 || group.ID > uint64(common.MaxWalletQuota) || seen[group.ID] || len(group.Name) > 1024 || len(group.ChannelID) > 128 ||
			(group.CanProbe && len(group.TargetRevision) != 64) || (!group.CanProbe && group.TargetRevision != "" && len(group.TargetRevision) != 64) ||
			(group.CanManualReview && len(group.ManualTargetRevision) != 64) || (!group.CanManualReview && group.ManualTargetRevision != "" && len(group.ManualTargetRevision) != 64) {
			return nil, &DonationRemoteError{Reason: "invalid_response"}
		}
		seen[group.ID] = true
		group.UnavailableReason = model.DonationReason(group.UnavailableReason)
		group.ManualUnavailableReason = model.DonationReason(group.ManualUnavailableReason)
	}
	return result, nil
}

func (c *donationClient) batch(ctx context.Context, batch model.DonationBatch) (model.DonationReceipt, error) {
	var receipt model.DonationReceipt
	if !model.ValidDonationID(batch.ID) {
		return receipt, model.ErrDonationInput
	}
	err := c.request(ctx, http.MethodGet, "/batches/"+batch.ID, "", nil, &receipt)
	if err == nil && receipt.BatchID != batch.ID {
		err = model.ErrDonationReceipt
	}
	receipt.InstanceID, receipt.SourceID = batch.InstanceID, batch.SourceID
	return receipt, err
}

// DonationReviewContextView is the allowlisted review context. The receiver's
// private execution configuration never reaches this projection.
type DonationReviewContextView struct {
	BatchID              string               `json:"batch_id"`
	ItemID               string               `json:"item_id"`
	GroupID              uint64               `json:"group_id"`
	State                string               `json:"state"`
	EffectiveMode        string               `json:"effective_mode"`
	ItemRevision         int64                `json:"item_revision"`
	ReviewTargetRevision string               `json:"review_target_revision"`
	ExpiresAtMS          int64                `json:"expires_at_ms"`
	CanReview            bool                 `json:"can_review"`
	CanReject            bool                 `json:"can_reject"`
	ReviewAction         string               `json:"review_action"`
	CanTest              bool                 `json:"can_test"`
	UnavailableReason    string               `json:"unavailable_reason"`
	TestModels           []string             `json:"test_models"`
	ReviewLimits         DonationReviewLimits `json:"review_limits"`
}

type donationReviewActionRequest struct {
	ActionID             string `json:"action_id"`
	Actor                string `json:"actor"`
	Kind                 string `json:"kind"`
	ExpectedItemRevision int64  `json:"expected_item_revision"`
	ReviewTargetRevision string `json:"review_target_revision,omitempty"`
	Note                 string `json:"note,omitempty"`
}

// DonationReviewActionResult is the receiver's verdict. outcome=rejected means
// the command was not applied; it is not a rejection of the donation.
type DonationReviewActionResult struct {
	ActionID             string `json:"action_id"`
	BatchID              string `json:"batch_id"`
	ItemID               string `json:"item_id"`
	Kind                 string `json:"kind"`
	ExpectedItemRevision int64  `json:"expected_item_revision"`
	ReviewTargetRevision string `json:"review_target_revision"`
	Outcome              string `json:"outcome"`
	ReasonCode           string `json:"reason_code"`
	EffectRevision       int64  `json:"effect_revision"`
	AppliedAtMS          int64  `json:"applied_at_ms"`
}

type donationTestRequest struct {
	TestID               string `json:"test_id"`
	Actor                string `json:"actor"`
	ExpectedItemRevision int64  `json:"expected_item_revision"`
	ReviewTargetRevision string `json:"review_target_revision"`
	Model                string `json:"model"`
	Prompt               string `json:"prompt"`
	SystemPrompt         string `json:"system_prompt,omitempty"`
	MaxOutputTokens      int    `json:"max_output_tokens"`
	Stream               bool   `json:"stream"`
}

// DonationTestResult is the non-streaming projection. It never carries upstream
// headers, raw error bodies, or unsanitized text.
type DonationTestResult struct {
	TestID         string             `json:"test_id"`
	BatchID        string             `json:"batch_id"`
	ItemID         string             `json:"item_id"`
	Model          string             `json:"model"`
	Stream         bool               `json:"stream"`
	State          string             `json:"state"`
	ReasonCode     string             `json:"reason_code"`
	StatusCode     int                `json:"status_code"`
	StartRevision  int64              `json:"start_revision"`
	TargetRevision string             `json:"target_revision"`
	StartedAtMS    int64              `json:"started_at_ms"`
	FinishedAtMS   int64              `json:"finished_at_ms"`
	OutputBytes    int                `json:"output_bytes"`
	Text           string             `json:"text,omitempty"`
	Usage          *donationTestUsage `json:"usage,omitempty"`
	InputTokens    int                `json:"-"`
	OutputTokens   int                `json:"-"`
}

func (r *DonationTestResult) recordUsage() {
	if r.Usage == nil {
		return
	}
	r.InputTokens, r.OutputTokens = r.Usage.InputTokens, r.Usage.OutputTokens
}

type donationTestDone struct {
	State        string             `json:"state"`
	ReasonCode   string             `json:"reason_code"`
	StatusCode   int                `json:"status_code"`
	FinishedAtMS int64              `json:"finished_at_ms"`
	OutputBytes  int                `json:"output_bytes"`
	Usage        *donationTestUsage `json:"usage"`
}

type donationTestUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type donationTestMeta struct {
	TestID         string `json:"test_id"`
	BatchID        string `json:"batch_id"`
	ItemID         string `json:"item_id"`
	Model          string `json:"model"`
	StartRevision  int64  `json:"start_revision"`
	TargetRevision string `json:"target_revision"`
	StartedAtMS    int64  `json:"started_at_ms"`
}

func donationItemStateAllowed(state string) bool {
	switch state {
	case "queued", "validating", "pending_review", "rejected", "committing", "accepted", "existing", "invalid", "retry_pending":
		return true
	default:
		return false
	}
}

func donationUnavailableReasonAllowed(reason string) bool {
	switch reason {
	case "", "staging_expired", "item_state", "target_changed", "target_unavailable", "group_deleted", "test_running", "resource_busy", "model_unavailable":
		return true
	default:
		return false
	}
}

func (c *donationClient) reviewContext(ctx context.Context, batchID, itemID string) (DonationReviewContextView, error) {
	var result DonationReviewContextView
	if !model.ValidDonationID(batchID) || !model.ValidDonationID(itemID) {
		return result, model.ErrDonationInput
	}
	if err := c.request(ctx, http.MethodGet, "/batches/"+batchID+"/items/"+itemID+"/review-context", "", nil, &result); err != nil {
		return result, err
	}
	groupID := result.GroupID
	if result.BatchID != batchID || result.ItemID != itemID || groupID == 0 || groupID > uint64(common.MaxWalletQuota) ||
		!donationItemStateAllowed(result.State) || result.ItemRevision < 0 || result.ExpiresAtMS < 0 ||
		!donationUnavailableReasonAllowed(result.UnavailableReason) || len(result.TestModels) > 64 {
		return result, &DonationRemoteError{Reason: "invalid_response"}
	}
	if result.EffectiveMode != model.DonationModeManualReview && result.EffectiveMode != model.DonationModeAuto {
		return result, &DonationRemoteError{Reason: "invalid_response"}
	}
	if result.ReviewTargetRevision != "" && !donationTargetRevisionValid(result.ReviewTargetRevision) {
		return result, &DonationRemoteError{Reason: "invalid_response"}
	}
	if result.CanReview {
		if !donationTargetRevisionValid(result.ReviewTargetRevision) ||
			(result.ReviewAction == model.DonationActionEnterReview && (result.EffectiveMode != model.DonationModeAuto || result.State != "retry_pending")) ||
			(result.ReviewAction == model.DonationActionApprove && (result.EffectiveMode != model.DonationModeManualReview || result.State != "pending_review")) ||
			(result.ReviewAction != model.DonationActionEnterReview && result.ReviewAction != model.DonationActionApprove) {
			return result, &DonationRemoteError{Reason: "invalid_response"}
		}
	} else if result.ReviewAction != "" {
		return result, &DonationRemoteError{Reason: "invalid_response"}
	}
	if (result.CanTest && (!result.CanReview || result.EffectiveMode != model.DonationModeManualReview || len(result.TestModels) == 0)) ||
		(result.CanReject && (result.EffectiveMode != model.DonationModeManualReview || result.State != "pending_review")) {
		return result, &DonationRemoteError{Reason: "invalid_response"}
	}
	for _, name := range result.TestModels {
		if len(name) == 0 || len(name) > 128 {
			return result, &DonationRemoteError{Reason: "invalid_response"}
		}
	}
	return result, nil
}

func (c *donationClient) reviewAction(ctx context.Context, batchID, itemID, actionID, kind string, expectedRevision int64, targetRevision, note string, actorID int) (DonationReviewActionResult, error) {
	var result DonationReviewActionResult
	if !model.ValidDonationID(batchID) || !model.ValidDonationID(itemID) || !model.ValidDonationID(actionID) {
		return result, model.ErrDonationInput
	}
	request := donationReviewActionRequest{ActionID: actionID, Actor: "user:" + strconv.Itoa(actorID), Kind: kind, ExpectedItemRevision: expectedRevision,
		ReviewTargetRevision: targetRevision, Note: note}
	err := c.request(ctx, http.MethodPost, "/batches/"+batchID+"/items/"+itemID+"/review-actions", actionID, request, &result)
	if err != nil {
		return result, err
	}
	if result.ActionID != actionID || result.BatchID != batchID || result.ItemID != itemID || result.Kind != kind ||
		result.ExpectedItemRevision != expectedRevision || result.ReviewTargetRevision != targetRevision {
		return result, &DonationRemoteError{Reason: "invalid_response"}
	}
	if !validDonationReviewResult(result) {
		return result, &DonationRemoteError{Reason: "invalid_response"}
	}
	return result, nil
}

// reviewActionStatus queries one action by its original UUID. It never re-sends
// a model test.
func (c *donationClient) reviewActionStatus(ctx context.Context, actionID string) (DonationReviewActionResult, error) {
	var result DonationReviewActionResult
	if !model.ValidDonationID(actionID) {
		return result, model.ErrDonationInput
	}
	err := c.request(ctx, http.MethodGet, "/review-actions/"+actionID, "", nil, &result)
	if err != nil {
		return result, err
	}
	if result.ActionID != actionID || !validDonationReviewResult(result) {
		return result, &DonationRemoteError{Reason: "invalid_response"}
	}
	return result, nil
}

func donationTargetRevisionValid(revision string) bool {
	return len(revision) == 64 && strings.Trim(revision, "0123456789abcdef") == ""
}

func validDonationReviewResult(result DonationReviewActionResult) bool {
	if !model.ValidDonationID(result.BatchID) || !model.ValidDonationID(result.ItemID) || result.ExpectedItemRevision < 0 || result.EffectRevision < 0 || result.AppliedAtMS <= 0 {
		return false
	}
	switch result.Kind {
	case model.DonationActionEnterReview, model.DonationActionApprove:
		if !donationTargetRevisionValid(result.ReviewTargetRevision) {
			return false
		}
	case model.DonationActionReject:
		if result.ReviewTargetRevision != "" && !donationTargetRevisionValid(result.ReviewTargetRevision) {
			return false
		}
	default:
		return false
	}
	return (result.Outcome == model.DonationActionApplied && result.EffectRevision > result.ExpectedItemRevision && result.ReasonCode == "") ||
		(result.Outcome == model.DonationActionRejected && result.ReasonCode != "" && model.DonationActionReason(result.ReasonCode) != "unknown")
}

func (c *donationClient) test(ctx context.Context, batchID, itemID, testID string, request donationTestRequest, limits DonationReviewLimits) (DonationTestResult, error) {
	var result DonationTestResult
	response, err := c.testStreamRequest(ctx, batchID, itemID, testID, request, limits)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	result, err = readDonationTestResult(response.Body, limits)
	if err == nil {
		err = validateDonationTestResult(result, batchID, itemID, request, limits)
	}
	return result, err
}

func readDonationTestResult(body io.Reader, limits DonationReviewLimits) (DonationTestResult, error) {
	var envelope struct {
		Code json.RawMessage    `json:"code"`
		Data DonationTestResult `json:"data"`
	}
	// Text may be JSON-escaped to six bytes per input byte. Bounds on actual
	// text and metadata are checked again after decoding.
	maxBytes := limits.MaxResponseBytes*6 + limits.MaxEventBytes
	encoded, err := io.ReadAll(io.LimitReader(body, int64(maxBytes+1)))
	if err != nil {
		return envelope.Data, err
	}
	if len(encoded) > maxBytes || common.Unmarshal(encoded, &envelope) != nil || string(envelope.Code) != "0" {
		return envelope.Data, &DonationRemoteError{Reason: "invalid_response"}
	}
	return envelope.Data, nil
}

func validateDonationTestResult(result DonationTestResult, batchID, itemID string, request donationTestRequest, limits DonationReviewLimits) error {
	if result.TestID != request.TestID || result.BatchID != batchID || result.ItemID != itemID || result.Model != request.Model ||
		result.Stream != request.Stream || result.StartRevision != request.ExpectedItemRevision || result.TargetRevision != request.ReviewTargetRevision ||
		!donationTestStateAllowed(result.State) || result.StartedAtMS <= 0 || !utf8.ValidString(result.Text) || len(result.Text) > limits.MaxResponseBytes ||
		result.OutputBytes < 0 || result.OutputBytes > limits.MaxResponseBytes || result.StatusCode < 0 || result.StatusCode > 599 ||
		model.DonationTestReason(result.ReasonCode) != result.ReasonCode ||
		(result.Usage != nil && (result.Usage.InputTokens < 0 || result.Usage.OutputTokens < 0)) {
		return &DonationRemoteError{Reason: "invalid_response"}
	}
	if result.State == model.DonationTestRunning {
		if result.FinishedAtMS != 0 || result.Text != "" {
			return &DonationRemoteError{Reason: "invalid_response"}
		}
		return nil
	}
	if result.FinishedAtMS < result.StartedAtMS || (result.State == model.DonationTestSucceeded &&
		(result.ReasonCode != "" || result.StatusCode < 200 || result.StatusCode >= 300 || result.OutputBytes == 0)) {
		return &DonationRemoteError{Reason: "invalid_response"}
	}
	if result.State != model.DonationTestSucceeded && result.Text != "" {
		return &DonationRemoteError{Reason: "invalid_response"}
	}
	return nil
}

func (c *donationClient) testStatus(ctx context.Context, attempt model.DonationTestAttempt) (DonationTestResult, error) {
	var result DonationTestResult
	if err := c.request(ctx, http.MethodGet, "/tests/"+attempt.TestID, "", nil, &result); err != nil {
		return result, err
	}
	if result.Text != "" {
		return result, &DonationRemoteError{Reason: "invalid_response"}
	}
	request := donationTestRequest{TestID: attempt.TestID, Model: attempt.Model, Stream: attempt.Stream,
		ExpectedItemRevision: attempt.StartRevision, ReviewTargetRevision: attempt.ReviewTargetRevision}
	return result, validateDonationTestResult(result, attempt.BatchID, attempt.ItemID, request, donationLocalReviewLimits())
}

func donationTestStateAllowed(state string) bool {
	switch state {
	case model.DonationTestRunning, model.DonationTestSucceeded, model.DonationTestFailed, model.DonationTestCancelled, model.DonationTestInterrupted:
		return true
	default:
		return false
	}
}

// testStreamRequest opens the controlled SSE call and hands the raw body to the
// caller. The caller owns the response body.
func (c *donationClient) testStreamRequest(ctx context.Context, batchID, itemID, testID string, request donationTestRequest, limits DonationReviewLimits) (*http.Response, error) {
	if !model.ValidDonationID(batchID) || !model.ValidDonationID(itemID) || !model.ValidDonationID(testID) {
		return nil, model.ErrDonationInput
	}
	body, err := common.Marshal(request)
	if err != nil || len(body) > limits.MaxRequestBytes {
		return nil, model.ErrDonationInput
	}
	bounded, cancel := context.WithCancelCause(ctx)
	timer := time.AfterFunc(time.Duration(limits.FirstByteTimeoutSeconds)*time.Second, func() { cancel(context.DeadlineExceeded) })
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/integrations/donations/v1/batches/"+batchID+"/items/"+itemID+"/tests", bytes.NewReader(body))
	if err != nil {
		timer.Stop()
		cancel(nil)
		return nil, model.ErrDonationInput
	}
	req = req.WithContext(bounded)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if request.Stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", testID)
	req.GetBody = nil
	response, err := c.long.Do(req)
	if err != nil {
		timer.Stop()
		cause := context.Cause(bounded)
		cancel(nil)
		return nil, &DonationRemoteError{Reason: donationStreamReason(cause), Cause: cause}
	}
	response.Body = &donationTestBody{ReadCloser: response.Body, timer: timer, cancel: cancel,
		ctx: bounded, idle: time.Duration(limits.IdleTimeoutSeconds) * time.Second}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer response.Body.Close()
		reason := "integration_error"
		if response.StatusCode == http.StatusNotFound {
			reason = "item_not_found"
		} else if response.StatusCode == http.StatusConflict {
			reason = "test_unavailable"
		}
		if code, ok := readDonationErrorCode(response); ok {
			switch code {
			case "DONATION_TEST_UNAVAILABLE", "DONATION_TEST_CONFLICT", "DONATION_TEST_BUSY":
				reason = "test_unavailable"
			case "DONATION_ITEM_NOT_FOUND":
				reason = "item_not_found"
			case "DONATION_REVIEW_NOT_FOUND":
				reason = "review_not_found"
			}
		}
		return nil, &DonationRemoteError{Status: response.StatusCode, Reason: reason}
	}
	return response, nil
}

// donationTestBody closes the whole request when the first byte or an idle
// read exceeds its budget. Closing downstream also closes the upstream body.
type donationTestBody struct {
	io.ReadCloser
	timer  *time.Timer
	cancel context.CancelCauseFunc
	ctx    context.Context
	idle   time.Duration
}

func (body *donationTestBody) Read(buffer []byte) (int, error) {
	n, err := body.ReadCloser.Read(buffer)
	if n > 0 {
		body.timer.Reset(body.idle)
	}
	if cause := context.Cause(body.ctx); cause != nil {
		return n, cause
	}
	return n, err
}

func (body *donationTestBody) Close() error {
	body.timer.Stop()
	body.cancel(nil)
	return body.ReadCloser.Close()
}

func donationRemoteReason(err error) string {
	var remote *DonationRemoteError
	if errors.As(err, &remote) {
		return remote.Reason
	}
	if errors.Is(err, model.ErrDonationReceipt) {
		return "invalid_receipt"
	}
	if errors.Is(err, model.ErrDonationSecret) {
		return "identity_unavailable"
	}
	if errors.Is(err, model.ErrDonationTestBusy) {
		return "test_running"
	}
	if errors.Is(err, model.ErrDonationReview) {
		return "not_reviewable"
	}
	return "recovery_pending"
}

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
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type DonationCapabilities struct {
	ProtocolVersion string   `json:"protocol_version"`
	InstanceID      string   `json:"instance_id"`
	SourceID        string   `json:"source_id"`
	InputTypes      []string `json:"input_types"`
	Limits          struct {
		MaxItems                int   `json:"max_items"`
		MaxKeyBytes             int   `json:"max_key_bytes"`
		MaxBodyBytes            int   `json:"max_body_bytes"`
		StagingRetentionSeconds int64 `json:"staging_retention_seconds"`
	} `json:"limits"`
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
}

type donationIntakeItem struct {
	ItemID string `json:"item_id"`
	Key    string `json:"key"`
}

type donationIntakeBatch struct {
	BatchID        string               `json:"batch_id"`
	GroupID        uint64               `json:"group_id"`
	TargetRevision string               `json:"target_revision"`
	Items          []donationIntakeItem `json:"items"`
}

type DonationRemoteError struct {
	Reason                string
	Status                int
	RejectedBeforeStaging bool
}

func (e *DonationRemoteError) Error() string { return "donation integration: " + e.Reason }

type donationClient struct {
	baseURL string
	token   string
	http    *http.Client
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
	transport := &http.Transport{
		DialContext: dialer.DialContext, ForceAttemptHTTP2: true,
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 10 * time.Second,
		MaxResponseHeaderBytes: 32 << 10, ResponseHeaderTimeout: 10 * time.Second,
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
	return &donationClient{baseURL: strings.TrimSuffix(baseURL, "/"), token: token, http: &http.Client{
		Timeout: 12 * time.Second, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *donationClient) close() { c.http.CloseIdleConnections() }

func (c *donationClient) request(ctx context.Context, method, path, requestKey string, input, output any) error {
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
	response, err := c.http.Do(req)
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
		if response.StatusCode == http.StatusConflict {
			body, readErr := io.ReadAll(io.LimitReader(response.Body, model.DonationMaxBodyBytes+1))
			var envelope struct {
				Code string `json:"code"`
			}
			if readErr == nil && len(body) <= model.DonationMaxBodyBytes && common.Unmarshal(body, &envelope) == nil {
				switch envelope.Code {
				case "DONATION_TARGET_CHANGED":
					failure.Reason, failure.RejectedBeforeStaging = "target_changed", true
				case "DONATION_TARGET_UNAVAILABLE":
					failure.Reason, failure.RejectedBeforeStaging = "target_unavailable", true
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

func (c *donationClient) capabilities(ctx context.Context) (DonationCapabilities, error) {
	var result DonationCapabilities
	if err := c.request(ctx, http.MethodGet, "/capabilities", "", nil, &result); err != nil {
		return result, err
	}
	if result.ProtocolVersion != "1" || !model.ValidDonationID(result.InstanceID) || !model.ValidDonationID(result.SourceID) || !slices.Contains(result.InputTypes, "api_key") || result.Limits.MaxItems != model.DonationMaxItems || result.Limits.MaxKeyBytes != model.DonationMaxKeyBytes || result.Limits.MaxBodyBytes != model.DonationMaxBodyBytes || result.Limits.StagingRetentionSeconds != 604800 {
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
		if group.ID == 0 || group.ID > uint64(common.MaxWalletQuota) || seen[group.ID] || len(group.Name) > 1024 || len(group.ChannelID) > 128 || (group.CanProbe && len(group.TargetRevision) != 64) || (!group.CanProbe && group.TargetRevision != "" && len(group.TargetRevision) != 64) {
			return nil, &DonationRemoteError{Reason: "invalid_response"}
		}
		seen[group.ID] = true
		group.UnavailableReason = model.DonationReason(group.UnavailableReason)
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
	return "recovery_pending"
}

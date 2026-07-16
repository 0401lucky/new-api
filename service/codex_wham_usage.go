package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
)

func FetchCodexWham(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
	method string,
	endpoint string,
) (statusCode int, body []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	bu := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if bu == "" {
		return 0, nil, fmt.Errorf("empty baseURL")
	}
	at := strings.TrimSpace(accessToken)
	aid := strings.TrimSpace(accountID)
	if at == "" {
		return 0, nil, fmt.Errorf("empty accessToken")
	}
	if aid == "" {
		return 0, nil, fmt.Errorf("empty accountID")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	endpoint = strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return 0, nil, fmt.Errorf("empty endpoint")
	}

	var requestBody io.Reader
	if method != http.MethodGet {
		requestBody = strings.NewReader("{}")
	}
	req, err := http.NewRequestWithContext(ctx, method, bu+"/backend-api/wham/"+endpoint, requestBody)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+at)
	req.Header.Set("chatgpt-account-id", aid)
	req.Header.Set("Accept", "application/json")
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}
	if req.Header.Get("originator") == "" {
		req.Header.Set("originator", "codex_cli_rs")
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func FetchCodexWhamUsage(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	return FetchCodexWham(ctx, client, baseURL, accessToken, accountID, http.MethodGet, "usage")
}

func FetchCodexWhamRateLimitResetCredits(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	return fetchCodexWhamEndpoint(ctx, client, baseURL, accessToken, accountID, http.MethodGet, "rate-limit-reset-credits", nil)
}

func ConsumeCodexWhamRateLimitResetCredit(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
) (statusCode int, body []byte, err error) {
	requestBody, err := common.Marshal(map[string]string{"redeem_request_id": uuid.NewString()})
	if err != nil {
		return 0, nil, err
	}
	return fetchCodexWhamEndpoint(ctx, client, baseURL, accessToken, accountID, http.MethodPost, "rate-limit-reset-credits/consume", requestBody)
}

func fetchCodexWhamEndpoint(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	accessToken string,
	accountID string,
	method string,
	endpoint string,
	body []byte,
) (statusCode int, responseBody []byte, err error) {
	if client == nil {
		return 0, nil, fmt.Errorf("nil http client")
	}
	bu := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if bu == "" || strings.TrimSpace(accessToken) == "" || strings.TrimSpace(accountID) == "" {
		return 0, nil, fmt.Errorf("invalid Codex credentials or baseURL")
	}
	var requestBody io.Reader
	if len(body) > 0 {
		requestBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, bu+"/backend-api/wham/"+strings.TrimLeft(endpoint, "/"), requestBody)
	if err != nil {
		return 0, nil, err
	}
	setCodexWhamRequestHeaders(req, accessToken, accountID)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	responseBody, err = io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, responseBody, nil
}

func setCodexWhamRequestHeaders(req *http.Request, accessToken string, accountID string) {
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("chatgpt-account-id", strings.TrimSpace(accountID))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("originator", "codex_cli_rs")
}

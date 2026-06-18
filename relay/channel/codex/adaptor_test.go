package codex

import (
	"net/http"
	"net/http/httptest"
	"testing"

	appconstant "github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newTestGinContext(method string, target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, target, nil)
	return c
}

func newTestRelayInfo(relayMode int, baseURL string, apiKey string, stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode: relayMode,
		IsStream:  stream,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    appconstant.ChannelTypeCodex,
			ChannelBaseUrl: baseURL,
			ApiKey:         apiKey,
		},
	}
}

func TestCodexAdaptorOAuthJSONUsesNativeChatGPTEndpoint(t *testing.T) {
	adaptor := &Adaptor{}
	info := newTestRelayInfo(
		relayconstant.RelayModeResponses,
		"https://chatgpt.com",
		`{"access_token":"access-token","account_id":"account-id"}`,
		true,
	)

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://chatgpt.com/backend-api/codex/responses", requestURL)

	c := newTestGinContext(http.MethodPost, "/v1/responses")
	c.Request.Header.Set("Content-Type", "application/json; charset=utf-8")
	headers := http.Header{}
	err = adaptor.SetupRequestHeader(c, &headers, info)
	require.NoError(t, err)
	require.Equal(t, "Bearer access-token", headers.Get("Authorization"))
	require.Equal(t, "account-id", headers.Get("chatgpt-account-id"))
	require.Equal(t, "responses=experimental", headers.Get("OpenAI-Beta"))
	require.Equal(t, "codex_cli_rs", headers.Get("originator"))
	require.Equal(t, "application/json", headers.Get("Content-Type"))
	require.Equal(t, "text/event-stream", headers.Get("Accept"))
}

func TestCodexAdaptorProxyKeyUsesResponsesEndpointAndPassesCodexHeaders(t *testing.T) {
	adaptor := &Adaptor{}
	info := newTestRelayInfo(
		relayconstant.RelayModeResponses,
		"http://127.0.0.1:8317",
		"cpa-client-key",
		true,
	)

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "http://127.0.0.1:8317/v1/responses", requestURL)

	c := newTestGinContext(http.MethodPost, "/v1/responses")
	c.Request.Header.Set("Content-Type", "application/json; charset=utf-8")
	c.Request.Header.Set("User-Agent", "codex-cli-test")
	c.Request.Header.Set("Originator", "Codex CLI")
	c.Request.Header.Set("Session_id", "session-123")
	c.Request.Header.Set("X-Codex-Turn-Metadata", `{"turn_id":"turn-1"}`)
	c.Request.Header.Set("X-Codex-Window-Id", "session-123:0")
	c.Request.Header.Set("X-Client-Request-Id", "request-123")

	headers := http.Header{}
	err = adaptor.SetupRequestHeader(c, &headers, info)
	require.NoError(t, err)
	require.Equal(t, "Bearer cpa-client-key", headers.Get("Authorization"))
	require.Empty(t, headers.Get("chatgpt-account-id"))
	require.Equal(t, "codex-cli-test", headers.Get("User-Agent"))
	require.Equal(t, "Codex CLI", headers.Get("Originator"))
	require.Equal(t, "session-123", headers.Get("Session_id"))
	require.Equal(t, `{"turn_id":"turn-1"}`, headers.Get("X-Codex-Turn-Metadata"))
	require.Equal(t, "session-123:0", headers.Get("X-Codex-Window-Id"))
	require.Equal(t, "request-123", headers.Get("X-Client-Request-Id"))
	require.Equal(t, "responses=experimental", headers.Get("OpenAI-Beta"))
	require.Equal(t, "application/json", headers.Get("Content-Type"))
	require.Equal(t, "text/event-stream", headers.Get("Accept"))
}

func TestCodexAdaptorProxyKeyDoesNotDuplicateV1BasePath(t *testing.T) {
	adaptor := &Adaptor{}
	info := newTestRelayInfo(
		relayconstant.RelayModeResponsesCompact,
		"https://cpa.example.com/v1",
		"cpa-client-key",
		false,
	)

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://cpa.example.com/v1/responses/compact", requestURL)
}

func TestCodexAdaptorRejectsProxyKeyWithDefaultChatGPTBaseURL(t *testing.T) {
	adaptor := &Adaptor{}
	info := newTestRelayInfo(
		relayconstant.RelayModeResponses,
		"https://chatgpt.com",
		"cpa-client-key",
		false,
	)

	_, err := adaptor.GetRequestURL(info)
	require.ErrorContains(t, err, "non-JSON key requires a Codex-compatible proxy base_url")

	c := newTestGinContext(http.MethodPost, "/v1/responses")
	headers := http.Header{}
	err = adaptor.SetupRequestHeader(c, &headers, info)
	require.ErrorContains(t, err, "non-JSON key requires a Codex-compatible proxy base_url")
}

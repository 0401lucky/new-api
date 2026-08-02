package deepseek

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDeepSeekTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	return c
}

func newDeepSeekTestRelayInfo(relayFormat types.RelayFormat, relayMode int, baseURL string, upstreamModel string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayFormat: relayFormat,
		RelayMode:   relayMode,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    baseURL,
			UpstreamModelName: upstreamModel,
		},
	}
}

func TestGetRequestURL(t *testing.T) {
	adaptor := &Adaptor{}
	cases := []struct {
		name        string
		relayFormat types.RelayFormat
		relayMode   int
		baseURL     string
		want        string
	}{
		{
			name:        "claude messages",
			relayFormat: types.RelayFormatClaude,
			relayMode:   constant.RelayModeChatCompletions,
			baseURL:     "https://api.deepseek.com",
			want:        "https://api.deepseek.com/anthropic/v1/messages",
		},
		{
			name:        "chat completions",
			relayFormat: types.RelayFormatOpenAI,
			relayMode:   constant.RelayModeChatCompletions,
			baseURL:     "https://api.deepseek.com",
			want:        "https://api.deepseek.com/v1/chat/completions",
		},
		{
			name:        "fim completions appends beta",
			relayFormat: types.RelayFormatOpenAI,
			relayMode:   constant.RelayModeCompletions,
			baseURL:     "https://api.deepseek.com",
			want:        "https://api.deepseek.com/beta/completions",
		},
		{
			name:        "fim completions keeps existing beta suffix",
			relayFormat: types.RelayFormatOpenAI,
			relayMode:   constant.RelayModeCompletions,
			baseURL:     "https://api.deepseek.com/beta",
			want:        "https://api.deepseek.com/beta/completions",
		},
		{
			name:        "responses appends v1",
			relayFormat: types.RelayFormatOpenAIResponses,
			relayMode:   constant.RelayModeResponses,
			baseURL:     "https://api.deepseek.com",
			want:        "https://api.deepseek.com/v1/responses",
		},
		{
			name:        "responses keeps existing v1 suffix",
			relayFormat: types.RelayFormatOpenAIResponses,
			relayMode:   constant.RelayModeResponses,
			baseURL:     "https://proxy.example.com/v1",
			want:        "https://proxy.example.com/v1/responses",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := newDeepSeekTestRelayInfo(tc.relayFormat, tc.relayMode, tc.baseURL, "")
			got, err := adaptor.GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestConvertOpenAIRequestThinkingSuffix(t *testing.T) {
	adaptor := &Adaptor{}
	cases := []struct {
		name          string
		upstreamModel string
		wantModel     string
		wantThinking  string
		wantEffort    string
	}{
		{
			name:          "max suffix enables thinking",
			upstreamModel: "deepseek-v4-pro-max",
			wantModel:     "deepseek-v4-pro",
			wantThinking:  `{"type":"enabled"}`,
			wantEffort:    "max",
		},
		{
			name:          "none suffix disables thinking",
			upstreamModel: "deepseek-v4-flash-none",
			wantModel:     "deepseek-v4-flash",
			wantThinking:  `{"type":"disabled"}`,
			wantEffort:    "",
		},
		{
			name:          "model without suffix is untouched",
			upstreamModel: "deepseek-v4-pro",
			wantModel:     "deepseek-v4-pro",
			wantThinking:  "",
			wantEffort:    "",
		},
		{
			name:          "non v4 model is untouched",
			upstreamModel: "deepseek-chat",
			wantModel:     "deepseek-chat",
			wantThinking:  "",
			wantEffort:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := newDeepSeekTestRelayInfo(types.RelayFormatOpenAI, constant.RelayModeChatCompletions, "https://api.deepseek.com", tc.upstreamModel)
			request := &dto.GeneralOpenAIRequest{Model: tc.upstreamModel}

			converted, err := adaptor.ConvertOpenAIRequest(newDeepSeekTestContext(t), info, request)
			require.NoError(t, err)
			openAIRequest, ok := converted.(*dto.GeneralOpenAIRequest)
			require.True(t, ok)

			assert.Equal(t, tc.wantModel, openAIRequest.Model)
			assert.Equal(t, tc.wantModel, info.UpstreamModelName)
			assert.Equal(t, tc.wantEffort, openAIRequest.ReasoningEffort)
			if tc.wantThinking == "" {
				assert.Empty(t, openAIRequest.THINKING)
			} else {
				assert.JSONEq(t, tc.wantThinking, string(openAIRequest.THINKING))
			}
		})
	}
}

func TestConvertOpenAIResponsesRequestThinkingSuffix(t *testing.T) {
	adaptor := &Adaptor{}
	cases := []struct {
		name           string
		upstreamModel  string
		inputReasoning *dto.Reasoning
		wantModel      string
		wantReasoning  *dto.Reasoning
	}{
		{
			name:          "max suffix sets max effort",
			upstreamModel: "deepseek-v4-pro-max",
			wantModel:     "deepseek-v4-pro",
			wantReasoning: &dto.Reasoning{Effort: "max"},
		},
		{
			name:          "none suffix sets none effort",
			upstreamModel: "deepseek-v4-flash-none",
			wantModel:     "deepseek-v4-flash",
			wantReasoning: &dto.Reasoning{Effort: "none"},
		},
		{
			name:           "suffix overrides client effort and keeps summary",
			upstreamModel:  "deepseek-v4-pro-max",
			inputReasoning: &dto.Reasoning{Effort: "low", Summary: "auto"},
			wantModel:      "deepseek-v4-pro",
			wantReasoning:  &dto.Reasoning{Effort: "max", Summary: "auto"},
		},
		{
			name:           "non v4 model keeps client reasoning",
			upstreamModel:  "deepseek-chat",
			inputReasoning: &dto.Reasoning{Effort: "low"},
			wantModel:      "deepseek-chat",
			wantReasoning:  &dto.Reasoning{Effort: "low"},
		},
		{
			name:          "model without suffix keeps nil reasoning",
			upstreamModel: "deepseek-v4-pro",
			wantModel:     "deepseek-v4-pro",
			wantReasoning: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := newDeepSeekTestRelayInfo(types.RelayFormatOpenAIResponses, constant.RelayModeResponses, "https://api.deepseek.com", tc.upstreamModel)
			request := dto.OpenAIResponsesRequest{Model: tc.upstreamModel, Reasoning: tc.inputReasoning}

			converted, err := adaptor.ConvertOpenAIResponsesRequest(newDeepSeekTestContext(t), info, request)
			require.NoError(t, err)
			responsesRequest, ok := converted.(dto.OpenAIResponsesRequest)
			require.True(t, ok)

			assert.Equal(t, tc.wantModel, responsesRequest.Model)
			assert.Equal(t, tc.wantModel, info.UpstreamModelName)
			assert.Equal(t, tc.wantReasoning, responsesRequest.Reasoning)
		})
	}
}

func TestConvertClaudeRequestThinkingSuffix(t *testing.T) {
	adaptor := &Adaptor{}
	cases := []struct {
		name             string
		upstreamModel    string
		wantModel        string
		wantThinking     *dto.Thinking
		wantOutputConfig string
	}{
		{
			name:             "max suffix enables thinking with effort",
			upstreamModel:    "deepseek-v4-pro-max",
			wantModel:        "deepseek-v4-pro",
			wantThinking:     &dto.Thinking{Type: "enabled"},
			wantOutputConfig: `{"effort":"max"}`,
		},
		{
			name:          "none suffix disables thinking and clears output config",
			upstreamModel: "deepseek-v4-flash-none",
			wantModel:     "deepseek-v4-flash",
			wantThinking:  &dto.Thinking{Type: "disabled"},
		},
		{
			name:          "non v4 model is untouched",
			upstreamModel: "deepseek-chat",
			wantModel:     "deepseek-chat",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := newDeepSeekTestRelayInfo(types.RelayFormatClaude, constant.RelayModeChatCompletions, "https://api.deepseek.com", tc.upstreamModel)
			request := &dto.ClaudeRequest{Model: tc.upstreamModel}

			converted, err := adaptor.ConvertClaudeRequest(newDeepSeekTestContext(t), info, request)
			require.NoError(t, err)
			claudeRequest, ok := converted.(*dto.ClaudeRequest)
			require.True(t, ok)

			assert.Equal(t, tc.wantModel, claudeRequest.Model)
			assert.Equal(t, tc.wantModel, info.UpstreamModelName)
			assert.Equal(t, tc.wantThinking, claudeRequest.Thinking)
			if tc.wantOutputConfig == "" {
				assert.Empty(t, claudeRequest.OutputConfig)
			} else {
				assert.JSONEq(t, tc.wantOutputConfig, string(claudeRequest.OutputConfig))
			}
		})
	}
}

// 模型重定向后，后缀解析必须以映射后的上游模型名为准，客户端原始模型名不参与。
func TestThinkingSuffixUsesMappedUpstreamModel(t *testing.T) {
	adaptor := &Adaptor{}
	info := newDeepSeekTestRelayInfo(types.RelayFormatOpenAI, constant.RelayModeChatCompletions, "https://api.deepseek.com", "deepseek-v4-pro-max")
	request := &dto.GeneralOpenAIRequest{Model: "my-model-alias"}

	converted, err := adaptor.ConvertOpenAIRequest(newDeepSeekTestContext(t), info, request)
	require.NoError(t, err)
	openAIRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)

	assert.Equal(t, "deepseek-v4-pro", openAIRequest.Model)
	assert.Equal(t, "deepseek-v4-pro", info.UpstreamModelName)
	assert.Equal(t, "max", openAIRequest.ReasoningEffort)
	assert.JSONEq(t, `{"type":"enabled"}`, string(openAIRequest.THINKING))
}

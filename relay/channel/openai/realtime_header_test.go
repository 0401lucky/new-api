package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupRequestHeaderRealtimeBetaCompatibility(t *testing.T) {
	tests := []struct {
		name          string
		channelType   int
		originModel   string
		upstreamModel string
		websocket     bool
		wantBeta      bool
	}{
		{
			name:          "OpenAI GA HTTP 不携带 Beta Header",
			channelType:   constant.ChannelTypeOpenAI,
			originModel:   "gpt-realtime-2",
			upstreamModel: "gpt-realtime-2",
		},
		{
			name:          "OpenAI GA WebSocket 不携带 Beta 协议",
			channelType:   constant.ChannelTypeOpenAI,
			originModel:   "gpt-realtime-mini",
			upstreamModel: "gpt-realtime-mini",
			websocket:     true,
		},
		{
			name:          "OpenAI preview 保留 Beta Header",
			channelType:   constant.ChannelTypeOpenAI,
			originModel:   "gpt-4o-realtime-preview",
			upstreamModel: "gpt-4o-realtime-preview",
			wantBeta:      true,
		},
		{
			name:          "OpenAI 未知模型保留 Beta Header",
			channelType:   constant.ChannelTypeOpenAI,
			originModel:   "gpt-realtime-future",
			upstreamModel: "gpt-realtime-future",
			wantBeta:      true,
		},
		{
			name:          "第三方兼容渠道保留 Beta Header",
			channelType:   constant.ChannelTypeOpenAIMax,
			originModel:   "gpt-realtime-2",
			upstreamModel: "gpt-realtime-2",
			wantBeta:      true,
		},
		{
			name:          "自定义渠道保留 Beta WebSocket 协议",
			channelType:   constant.ChannelTypeCustom,
			originModel:   "gpt-realtime-2.1",
			upstreamModel: "gpt-realtime-2.1",
			websocket:     true,
			wantBeta:      true,
		},
		{
			name:          "其他复用 adaptor 的渠道保留 Beta Header",
			channelType:   constant.ChannelTypeOpenRouter,
			originModel:   "gpt-realtime-whisper",
			upstreamModel: "gpt-realtime-whisper",
			wantBeta:      true,
		},
		{
			name:          "映射到 OpenAI GA 模型时移除 Beta Header",
			channelType:   constant.ChannelTypeOpenAI,
			originModel:   "gpt-4o-realtime-preview",
			upstreamModel: "gpt-realtime-2.1-mini",
		},
		{
			name:          "映射到 OpenAI preview 模型时保留 Beta Header",
			channelType:   constant.ChannelTypeOpenAI,
			originModel:   "gpt-realtime-2",
			upstreamModel: "gpt-4o-mini-realtime-preview",
			websocket:     true,
			wantBeta:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
			if test.websocket {
				context.Request.Header.Set("Sec-WebSocket-Protocol", "realtime")
			}
			header := http.Header{}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       test.channelType,
					ApiKey:            "sk-test",
					UpstreamModelName: test.upstreamModel,
					IsModelMapped:     test.originModel != test.upstreamModel,
				},
				RelayMode:       relayconstant.RelayModeRealtime,
				OriginModelName: test.originModel,
			}

			err := (&Adaptor{}).SetupRequestHeader(context, &header, info)

			require.NoError(t, err)
			if test.websocket {
				protocols := header.Get("Sec-WebSocket-Protocol")
				assert.Contains(t, protocols, "openai-insecure-api-key.sk-test")
				assert.Equal(t, test.wantBeta, strings.Contains(protocols, "openai-beta.realtime-v1"))
				return
			}
			assert.Equal(t, test.wantBeta, header.Get("openai-beta") == "realtime=v1")
			assert.Equal(t, "Bearer sk-test", header.Get("Authorization"))
		})
	}
}

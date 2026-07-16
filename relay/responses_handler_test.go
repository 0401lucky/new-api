package relay

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesRequestFromCompactionForwardsCompactionFields(t *testing.T) {
	req := &dto.OpenAIResponsesCompactionRequest{
		Model:             "gpt-test",
		Tools:             []byte(`[{"type":"function","name":"lookup"}]`),
		Reasoning:         &dto.Reasoning{Effort: "high", Summary: "auto"},
		PromptCacheKey:    []byte(`"session-1"`),
		Text:              []byte(`{"format":{"type":"text"}}`),
		ParallelToolCalls: []byte(`true`),
	}

	converted := responsesRequestFromCompaction(req)
	require.NotNil(t, converted)
	require.Equal(t, req.Model, converted.Model)
	require.JSONEq(t, string(req.Tools), string(converted.Tools))
	require.Equal(t, req.Reasoning, converted.Reasoning)
	require.JSONEq(t, string(req.PromptCacheKey), string(converted.PromptCacheKey))
	require.JSONEq(t, string(req.Text), string(converted.Text))
	require.JSONEq(t, string(req.ParallelToolCalls), string(converted.ParallelToolCalls))
}

func TestResponsesHelperAllowsAdvancedCustomCompactionEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeAdvancedCustom)

	apiErr := ResponsesHelper(ctx, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponsesCompact,
		Request:   &dto.BaseRequest{},
	})

	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "invalid request type")
	assert.False(t, strings.Contains(apiErr.Error(), "unsupported endpoint"))
}

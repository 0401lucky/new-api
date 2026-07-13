package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestResponsesRequestFromCompactionForwardsPromptCacheKey(t *testing.T) {
	req := &dto.OpenAIResponsesCompactionRequest{
		Model:          "gpt-test",
		PromptCacheKey: []byte(`"session-1"`),
	}

	converted := responsesRequestFromCompaction(req)
	require.Equal(t, req.Model, converted.Model)
	require.JSONEq(t, string(req.PromptCacheKey), string(converted.PromptCacheKey))
}

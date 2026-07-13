package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestInputTokenDetailsCacheWriteJSONAndTotal(t *testing.T) {
	var details InputTokenDetails
	require.NoError(t, common.UnmarshalJsonStr(`{"cached_creation_tokens":12,"cache_write_tokens":34}`, &details))
	require.Equal(t, 34, details.CacheWriteTokens)
	require.Equal(t, 34, details.CacheCreationTokensTotal())

	details = InputTokenDetails{CachedCreationTokens: 56, CacheWriteTokens: 34}
	require.Equal(t, 56, details.CacheCreationTokensTotal())
	details = InputTokenDetails{CachedCreationTokens: -1, CacheWriteTokens: -2}
	require.Zero(t, details.CacheCreationTokensTotal())
}

func TestCompactionRequestPromptCacheKeyJSON(t *testing.T) {
	var req OpenAIResponsesCompactionRequest
	require.NoError(t, common.UnmarshalJsonStr(`{"model":"gpt-test","prompt_cache_key":"session-1"}`, &req))
	require.JSONEq(t, `"session-1"`, string(req.PromptCacheKey))
}

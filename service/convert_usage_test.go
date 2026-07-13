package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestBuildClaudeUsageFromOpenAIUsageCacheWriteSemantics(t *testing.T) {
	t.Run("legacy cache creation keeps existing input semantics", func(t *testing.T) {
		usage := buildClaudeUsageFromOpenAIUsage(&dto.Usage{
			PromptTokens:     100,
			CompletionTokens: 7,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         30,
				CachedCreationTokens: 20,
			},
		})

		require.NotNil(t, usage)
		require.Equal(t, 100, usage.InputTokens)
		require.Equal(t, 30, usage.CacheReadInputTokens)
		require.Equal(t, 20, usage.CacheCreationInputTokens)
		require.Equal(t, 7, usage.OutputTokens)
	})

	t.Run("native cache write subtracts cached prefixes", func(t *testing.T) {
		usage := buildClaudeUsageFromOpenAIUsage(&dto.Usage{
			PromptTokens:     100,
			CompletionTokens: 7,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:     30,
				CacheWriteTokens: 20,
			},
		})

		require.NotNil(t, usage)
		require.Equal(t, 50, usage.InputTokens)
		require.Equal(t, 30, usage.CacheReadInputTokens)
		require.Equal(t, 20, usage.CacheCreationInputTokens)
	})

	t.Run("native overlapping prefixes clamp input remainder", func(t *testing.T) {
		usage := buildClaudeUsageFromOpenAIUsage(&dto.Usage{
			PromptTokens: 40,
			PromptTokensDetails: dto.InputTokenDetails{
				CachedTokens:         30,
				CachedCreationTokens: 10,
				CacheWriteTokens:     25,
			},
		})

		require.NotNil(t, usage)
		require.Equal(t, 0, usage.InputTokens)
		require.Equal(t, 25, usage.CacheCreationInputTokens)
	})
}

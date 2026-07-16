package geminichat

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageFromGeminiMetadataClampsInvalidNegativeCounts(t *testing.T) {
	metadata := &dto.GeminiUsageMetadata{
		PromptTokenCount:        10,
		ToolUsePromptTokenCount: -3,
		CandidatesTokenCount:    -2,
		ThoughtsTokenCount:      -4,
		TotalTokenCount:         5,
		CachedContentTokenCount: -6,
		PromptTokensDetails: []dto.GeminiPromptTokensDetails{
			{Modality: "TEXT", TokenCount: -8},
		},
		CandidatesTokensDetails: []dto.GeminiPromptTokensDetails{
			{Modality: "TEXT", TokenCount: -9},
		},
	}

	usage := UsageFromGeminiMetadata(metadata, 0)
	require.NotNil(t, usage)
	assert.Equal(t, 10, usage.PromptTokens)
	assert.Zero(t, usage.CompletionTokens)
	assert.Equal(t, 5, usage.TotalTokens)
	assert.Zero(t, usage.PromptTokensDetails.CachedTokens)
	assert.Zero(t, usage.CompletionTokenDetails.ReasoningTokens)
	assert.Equal(t, 10, usage.PromptTokensDetails.TextTokens)
	assert.Zero(t, usage.CompletionTokenDetails.TextTokens)
}

package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
)

// TestFormatUserLogsStripsQuotaSaturation verifies the admin-only quota
// saturation marker (nested under other.admin_info) is removed for non-admin
// log views, since formatUserLogs strips the whole admin_info object.
func TestFormatUserLogsStripsQuotaSaturation(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}

// TestFormatUserLogsStripsPromptCheckFullText verifies that the full prompt
// stored for admin review is removed from non-admin self log views.
func TestFormatUserLogsStripsPromptCheckFullText(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"prompt_check": map[string]interface{}{
			"action":    "block",
			"score":     75,
			"threshold": 50,
			"preview":   "short preview...",
			"full_text": "complete multi-line\nprompt for human review",
		},
		"reject_reason": "prompt_check",
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	promptCheck, ok := parsed["prompt_check"].(map[string]interface{})
	require.True(t, ok)
	_, hasFullText := promptCheck["full_text"]
	require.False(t, hasFullText, "full_text must be stripped for non-admin views")
	require.Equal(t, "short preview...", promptCheck["preview"])
	require.Equal(t, "prompt_check", parsed["reject_reason"])
}

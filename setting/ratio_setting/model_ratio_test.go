package ratio_setting

import "testing"

func TestGetCompletionRatio_CustomConfigOverridesHardcodedLockedRatio(t *testing.T) {
	completionRatioMap.Clear()
	t.Cleanup(func() {
		completionRatioMap.Clear()
	})

	const modelName = "gpt-5.5"

	if got := GetCompletionRatio(modelName); got != 6 {
		t.Fatalf("default completion ratio = %v, want 6", got)
	}
	if info := GetCompletionRatioInfo(modelName); info.Ratio != 6 || !info.Locked {
		t.Fatalf("default completion info = %+v, want ratio 6 locked true", info)
	}

	completionRatioMap.Set(modelName, 8)

	if got := GetCompletionRatio(modelName); got != 8 {
		t.Fatalf("custom completion ratio = %v, want 8", got)
	}
	if info := GetCompletionRatioInfo(modelName); info.Ratio != 8 || info.Locked {
		t.Fatalf("custom completion info = %+v, want ratio 8 locked false", info)
	}
}

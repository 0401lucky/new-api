package ratio_setting

import (
	"math"
	"testing"
)

func assertFloatEqual(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestDefaultZhipuLatestPricing(t *testing.T) {
	assertFloatEqual(t, "glm-5.2 model ratio", defaultModelRatio["glm-5.2"], 1.4/2)
	assertFloatEqual(t, "glm-5.2 completion ratio", defaultCompletionRatio["glm-5.2"], 4.4/1.4)
	assertFloatEqual(t, "glm-5.2 cache ratio", defaultCacheRatio["glm-5.2"], 0.26/1.4)
	assertFloatEqual(t, "glm-4.7-flash model ratio", defaultModelRatio["glm-4.7-flash"], 0)
	assertFloatEqual(t, "glm-image price", defaultModelPrice["glm-image"], 0.015)
	assertFloatEqual(t, "cogview-4 price", defaultModelPrice["cogview-4"], 0.01)
}

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

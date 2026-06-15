package model

import (
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func dynamicRatioTestRules(rules []DynamicRatioRule) []parsedDynamicRatioRule {
	return parseDynamicRatioRules(rules)
}

func dynamicRatioInt64(v int64) *int64 {
	return &v
}

func dynamicRatioTime(hour, minute int, weekday time.Weekday) time.Time {
	base := time.Date(2026, time.June, 14, hour, minute, 0, 0, time.Local)
	delta := (int(weekday) - int(base.Weekday()) + 7) % 7
	return base.AddDate(0, 0, delta)
}

func TestMatchDynamicRatioBasicGroupOnly(t *testing.T) {
	rules := []DynamicRatioRule{{Id: 1, Enable: true, Group: "vip", Ratio: 1.5}}

	ratio := matchDynamicRatio(dynamicRatioTestRules(rules), "vip", "gpt-4", 0, time.Now())
	if ratio != 1.5 {
		t.Fatalf("ratio = %v, want 1.5", ratio)
	}
}

func TestMatchDynamicRatioGroupNotMatch(t *testing.T) {
	rules := []DynamicRatioRule{{Id: 1, Enable: true, Group: "vip", Ratio: 1.5}}

	ratio := matchDynamicRatio(dynamicRatioTestRules(rules), "default", "gpt-4", 0, time.Now())
	if ratio != 0 {
		t.Fatalf("ratio = %v, want 0", ratio)
	}
}

func TestMatchDynamicRatioConcurrencyStrictlyGreater(t *testing.T) {
	rules := []DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", Concurrency: dynamicRatioInt64(10), Ratio: 1.8},
	}

	if ratio := matchDynamicRatio(dynamicRatioTestRules(rules), "vip", "gpt-4", 10, time.Now()); ratio != 0 {
		t.Fatalf("ratio at threshold = %v, want 0", ratio)
	}
	if ratio := matchDynamicRatio(dynamicRatioTestRules(rules), "vip", "gpt-4", 11, time.Now()); ratio != 1.8 {
		t.Fatalf("ratio over threshold = %v, want 1.8", ratio)
	}
}

func TestMatchDynamicRatioTimeRangeAndWeekday(t *testing.T) {
	rules := []DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", Weekdays: "[1]", StartTime: "09:00", EndTime: "18:00", Ratio: 1.2},
	}

	ratio := matchDynamicRatio(dynamicRatioTestRules(rules), "vip", "gpt-4", 0, dynamicRatioTime(10, 0, time.Monday))
	if ratio != 1.2 {
		t.Fatalf("ratio = %v, want 1.2", ratio)
	}

	ratio = matchDynamicRatio(dynamicRatioTestRules(rules), "vip", "gpt-4", 0, dynamicRatioTime(10, 0, time.Tuesday))
	if ratio != 0 {
		t.Fatalf("ratio = %v, want 0", ratio)
	}
}

func TestMatchDynamicRatioCrossDayUsesPreviousWeekdayAfterMidnight(t *testing.T) {
	rules := []DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", Weekdays: "[1]", StartTime: "22:00", EndTime: "06:00", Ratio: 2},
	}

	ratio := matchDynamicRatio(dynamicRatioTestRules(rules), "vip", "gpt-4", 0, dynamicRatioTime(3, 0, time.Tuesday))
	if ratio != 2 {
		t.Fatalf("ratio = %v, want 2", ratio)
	}
}

func TestMatchDynamicRatioModelPatterns(t *testing.T) {
	rules := []DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", Models: `["gpt-4*","*-preview"]`, Ratio: 1.4},
	}
	parsed := dynamicRatioTestRules(rules)

	for _, modelName := range []string{"gpt-4o", "claude-preview"} {
		if ratio := matchDynamicRatio(parsed, "vip", modelName, 0, time.Now()); ratio != 1.4 {
			t.Fatalf("model %s ratio = %v, want 1.4", modelName, ratio)
		}
	}
	if ratio := matchDynamicRatio(parsed, "vip", "gemini-pro", 0, time.Now()); ratio != 0 {
		t.Fatalf("non-matching model ratio = %v, want 0", ratio)
	}
}

func TestMatchDynamicRatioModelRulePriority(t *testing.T) {
	rules := []DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", Ratio: 1.1, Priority: 0},
		{Id: 2, Enable: true, Group: "vip", Models: `["gpt-4*"]`, Ratio: 1.8, Priority: 10},
	}

	ratio := matchDynamicRatio(dynamicRatioTestRules(rules), "vip", "gpt-4o", 0, time.Now())
	if ratio != 1.8 {
		t.Fatalf("ratio = %v, want model-specific ratio 1.8", ratio)
	}
}

func TestMatchDynamicRatioConcurrencyPriority(t *testing.T) {
	rules := []DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", Concurrency: dynamicRatioInt64(10), Ratio: 1.5, Priority: 10},
		{Id: 2, Enable: true, Group: "vip", Concurrency: dynamicRatioInt64(14), Ratio: 2.0, Priority: 20},
	}

	ratio := matchDynamicRatio(dynamicRatioTestRules(rules), "vip", "gpt-4", 15, time.Now())
	if ratio != 2.0 {
		t.Fatalf("ratio = %v, want nearest concurrency threshold ratio 2.0", ratio)
	}
}

func TestGetMatchedDynamicRatioEnabledSwitch(t *testing.T) {
	originalEnabled := common.DynamicRatioEnabled
	originalGetter := common.GetActiveConnectionsFunc
	t.Cleanup(func() {
		common.DynamicRatioEnabled = originalEnabled
		common.GetActiveConnectionsFunc = originalGetter
		SetDynamicRatioRulesForTest(nil)
	})

	common.GetActiveConnectionsFunc = func() int64 { return 15 }
	SetDynamicRatioRulesForTest([]DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", Concurrency: dynamicRatioInt64(10), Ratio: 1.7},
	})

	common.DynamicRatioEnabled = false
	if ratio := GetMatchedDynamicRatio("vip", "gpt-4"); ratio != 0 {
		t.Fatalf("disabled ratio = %v, want 0", ratio)
	}

	common.DynamicRatioEnabled = true
	if ratio := GetMatchedDynamicRatio("vip", "gpt-4"); ratio != 1.7 {
		t.Fatalf("enabled ratio = %v, want 1.7", ratio)
	}
}

func TestDynamicRatioStatusDoesNotExposeRuleModels(t *testing.T) {
	originalEnabled := common.DynamicRatioEnabled
	t.Cleanup(func() {
		common.DynamicRatioEnabled = originalEnabled
		SetDynamicRatioRulesForTest(nil)
	})

	common.DynamicRatioEnabled = true
	SetDynamicRatioRulesForTest([]DynamicRatioRule{
		{Id: 1, Enable: true, Group: "default", Models: `["secret-model"]`, Ratio: 1.3},
	})

	status := GetDynamicRatioStatus("default")
	payload, err := common.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "secret-model") {
		t.Fatalf("dynamic ratio status exposed model filters: %s", payload)
	}
}

func TestDynamicRatioRuleValidateModelList(t *testing.T) {
	rule := DynamicRatioRule{
		Group:  "default",
		Models: `["gpt-4*",""]`,
		Ratio:  1.2,
	}

	if err := rule.Validate(); err == nil {
		t.Fatal("expected empty model name validation error")
	}
}

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

func TestMatchDynamicRatioBalanceOldRuleCompatibility(t *testing.T) {
	rules := []DynamicRatioRule{{Id: 1, Enable: true, Group: "vip", Ratio: 1.5}}
	parsed := dynamicRatioTestRules(rules)

	for _, balance := range []int64{0, 100, 1000} {
		match := matchDynamicRatioMatch(parsed, "vip", "gpt-4", 0, time.Now(), balance, true)
		if match.Ratio != 1.5 {
			t.Fatalf("balance %d ratio = %v, want 1.5", balance, match.Ratio)
		}
		if match.RuleId != 1 {
			t.Fatalf("balance %d rule id = %d, want 1", balance, match.RuleId)
		}
	}
}

func TestMatchDynamicRatioBalanceRangeBoundaries(t *testing.T) {
	rules := []DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", BalanceMinQuota: dynamicRatioInt64(10), BalanceMaxQuota: dynamicRatioInt64(200), Ratio: 1.2},
	}
	parsed := dynamicRatioTestRules(rules)

	cases := []struct {
		name    string
		balance int64
		want    float64
	}{
		{name: "below min", balance: 9, want: 0},
		{name: "at min", balance: 10, want: 1.2},
		{name: "below max", balance: 199, want: 1.2},
		{name: "at max", balance: 200, want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match := matchDynamicRatioMatch(parsed, "vip", "gpt-4", 0, time.Now(), tc.balance, true)
			if match.Ratio != tc.want {
				t.Fatalf("ratio = %v, want %v", match.Ratio, tc.want)
			}
		})
	}
}

func TestMatchDynamicRatioBalanceContinuousRanges(t *testing.T) {
	rules := []DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", BalanceMinQuota: dynamicRatioInt64(10), BalanceMaxQuota: dynamicRatioInt64(200), Ratio: 1.0},
		{Id: 2, Enable: true, Group: "vip", BalanceMinQuota: dynamicRatioInt64(200), BalanceMaxQuota: dynamicRatioInt64(500), Ratio: 1.5},
	}

	match := matchDynamicRatioMatch(dynamicRatioTestRules(rules), "vip", "gpt-4", 0, time.Now(), 200, true)
	if match.Ratio != 1.5 || match.RuleId != 2 {
		t.Fatalf("match = %+v, want rule 2 ratio 1.5", match)
	}
}

func TestMatchDynamicRatioBalanceSingleSidedRanges(t *testing.T) {
	rules := []DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", BalanceMaxQuota: dynamicRatioInt64(200), Ratio: 1.1},
		{Id: 2, Enable: true, Group: "vip", BalanceMinQuota: dynamicRatioInt64(500), Ratio: 1.8},
	}
	parsed := dynamicRatioTestRules(rules)

	lowMatch := matchDynamicRatioMatch(parsed, "vip", "gpt-4", 0, time.Now(), 199, true)
	if lowMatch.Ratio != 1.1 || lowMatch.RuleId != 1 {
		t.Fatalf("low match = %+v, want rule 1 ratio 1.1", lowMatch)
	}

	highMatch := matchDynamicRatioMatch(parsed, "vip", "gpt-4", 0, time.Now(), 500, true)
	if highMatch.Ratio != 1.8 || highMatch.RuleId != 2 {
		t.Fatalf("high match = %+v, want rule 2 ratio 1.8", highMatch)
	}
}

func TestMatchDynamicRatioBalanceWithModelAndConcurrency(t *testing.T) {
	rules := []DynamicRatioRule{
		{
			Id:              1,
			Enable:          true,
			Group:           "vip",
			Models:          `["gpt-4*"]`,
			Concurrency:     dynamicRatioInt64(10),
			BalanceMinQuota: dynamicRatioInt64(100),
			BalanceMaxQuota: dynamicRatioInt64(300),
			Ratio:           1.6,
		},
	}
	parsed := dynamicRatioTestRules(rules)
	now := time.Now()

	if ratio := matchDynamicRatioMatch(parsed, "vip", "gpt-3.5", 11, now, 150, true).Ratio; ratio != 0 {
		t.Fatalf("model mismatch ratio = %v, want 0", ratio)
	}
	if ratio := matchDynamicRatioMatch(parsed, "vip", "gpt-4o", 10, now, 150, true).Ratio; ratio != 0 {
		t.Fatalf("concurrency at threshold ratio = %v, want 0", ratio)
	}
	if ratio := matchDynamicRatioMatch(parsed, "vip", "gpt-4o", 11, now, 150, true).Ratio; ratio != 1.6 {
		t.Fatalf("matched ratio = %v, want 1.6", ratio)
	}
}

func TestMatchDynamicRatioBalanceSpecificRulePriority(t *testing.T) {
	rules := []DynamicRatioRule{
		{Id: 1, Enable: true, Group: "vip", Ratio: 1.1, Priority: 0},
		{Id: 2, Enable: true, Group: "vip", BalanceMinQuota: dynamicRatioInt64(100), BalanceMaxQuota: dynamicRatioInt64(300), Ratio: 1.5, Priority: 10},
	}

	match := matchDynamicRatioMatch(dynamicRatioTestRules(rules), "vip", "gpt-4", 0, time.Now(), 150, true)
	if match.Ratio != 1.5 || match.RuleId != 2 {
		t.Fatalf("match = %+v, want balance rule 2 ratio 1.5", match)
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

func TestDynamicRatioRuleValidateBalanceRange(t *testing.T) {
	cases := []struct {
		name string
		rule DynamicRatioRule
		want string
	}{
		{
			name: "negative min",
			rule: DynamicRatioRule{Group: "default", Ratio: 1.2, BalanceMinQuota: dynamicRatioInt64(-1)},
			want: "余额下限不能为负数",
		},
		{
			name: "zero max",
			rule: DynamicRatioRule{Group: "default", Ratio: 1.2, BalanceMaxQuota: dynamicRatioInt64(0)},
			want: "余额上限必须大于 0",
		},
		{
			name: "max equals min",
			rule: DynamicRatioRule{Group: "default", Ratio: 1.2, BalanceMinQuota: dynamicRatioInt64(100), BalanceMaxQuota: dynamicRatioInt64(100)},
			want: "余额上限必须大于余额下限",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Validate()
			if err == nil {
				t.Fatalf("expected error %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tc.want)
			}
		})
	}
}

package operation_setting

import (
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type BlackroomRule struct {
	IPCount       int  `json:"ip_count"`
	DurationHours int  `json:"duration_hours"`
	Permanent     bool `json:"permanent"`
}

type BlackroomSetting struct {
	Enabled                     bool            `json:"enabled"`
	AutoBanEnabled              bool            `json:"auto_ban_enabled"`
	LookbackHours               int             `json:"lookback_hours"`
	CheckIntervalMinutes        int             `json:"check_interval_minutes"`
	MinRequests                 int             `json:"min_requests"`
	Rules                       []BlackroomRule `json:"rules"`
	EscalationWindowDays        int             `json:"escalation_window_days"`
	EscalationTemporaryBanCount int             `json:"escalation_temporary_ban_count"`
	ExemptUserIDs               []int           `json:"exempt_user_ids"`
	ExemptGroups                []string        `json:"exempt_groups"`
}

var defaultBlackroomRules = []BlackroomRule{
	{IPCount: 8, DurationHours: 6},
	{IPCount: 13, DurationHours: 72},
	{IPCount: 17, Permanent: true},
}

var blackroomSetting = BlackroomSetting{
	Enabled:                     true,
	AutoBanEnabled:              false,
	LookbackHours:               24,
	CheckIntervalMinutes:        10,
	MinRequests:                 0,
	Rules:                       defaultBlackroomRules,
	EscalationWindowDays:        30,
	EscalationTemporaryBanCount: 3,
	ExemptUserIDs:               []int{},
	ExemptGroups:                []string{},
}

func init() {
	config.GlobalConfig.Register("blackroom_setting", &blackroomSetting)
}

func GetBlackroomSetting() *BlackroomSetting {
	NormalizeBlackroomSetting(&blackroomSetting)
	return &blackroomSetting
}

func NormalizeBlackroomSetting(setting *BlackroomSetting) {
	if setting == nil {
		return
	}
	if setting.LookbackHours <= 0 {
		setting.LookbackHours = 24
	}
	if setting.CheckIntervalMinutes <= 0 {
		setting.CheckIntervalMinutes = 10
	}
	if setting.MinRequests < 0 {
		setting.MinRequests = 0
	}
	if setting.EscalationWindowDays <= 0 {
		setting.EscalationWindowDays = 30
	}
	if setting.EscalationTemporaryBanCount < 0 {
		setting.EscalationTemporaryBanCount = 0
	}
	if len(setting.Rules) == 0 {
		setting.Rules = append([]BlackroomRule(nil), defaultBlackroomRules...)
	}

	cleanRules := make([]BlackroomRule, 0, len(setting.Rules))
	for _, rule := range setting.Rules {
		if rule.IPCount <= 0 {
			continue
		}
		if !rule.Permanent && rule.DurationHours <= 0 {
			continue
		}
		if rule.Permanent {
			rule.DurationHours = 0
		}
		cleanRules = append(cleanRules, rule)
	}
	if len(cleanRules) == 0 {
		cleanRules = append([]BlackroomRule(nil), defaultBlackroomRules...)
	}
	sort.Slice(cleanRules, func(i, j int) bool {
		return cleanRules[i].IPCount < cleanRules[j].IPCount
	})
	setting.Rules = cleanRules

	exemptGroups := make([]string, 0, len(setting.ExemptGroups))
	seenGroups := make(map[string]struct{}, len(setting.ExemptGroups))
	for _, group := range setting.ExemptGroups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := seenGroups[group]; ok {
			continue
		}
		seenGroups[group] = struct{}{}
		exemptGroups = append(exemptGroups, group)
	}
	setting.ExemptGroups = exemptGroups

	exemptIDs := make([]int, 0, len(setting.ExemptUserIDs))
	seenIDs := make(map[int]struct{}, len(setting.ExemptUserIDs))
	for _, userID := range setting.ExemptUserIDs {
		if userID <= 0 {
			continue
		}
		if _, ok := seenIDs[userID]; ok {
			continue
		}
		seenIDs[userID] = struct{}{}
		exemptIDs = append(exemptIDs, userID)
	}
	sort.Ints(exemptIDs)
	setting.ExemptUserIDs = exemptIDs
}

func MatchBlackroomRule(setting *BlackroomSetting, ipCount int) (BlackroomRule, bool) {
	NormalizeBlackroomSetting(setting)
	var matched BlackroomRule
	ok := false
	for _, rule := range setting.Rules {
		if ipCount >= rule.IPCount {
			matched = rule
			ok = true
		}
	}
	return matched, ok
}

func MinBlackroomRuleIPCount(setting *BlackroomSetting) int {
	NormalizeBlackroomSetting(setting)
	if len(setting.Rules) == 0 {
		return 0
	}
	return setting.Rules[0].IPCount
}

func IsBlackroomUserExempt(setting *BlackroomSetting, userID int, group string) bool {
	NormalizeBlackroomSetting(setting)
	for _, exemptUserID := range setting.ExemptUserIDs {
		if exemptUserID == userID {
			return true
		}
	}
	for _, exemptGroup := range setting.ExemptGroups {
		if exemptGroup == group {
			return true
		}
	}
	return false
}

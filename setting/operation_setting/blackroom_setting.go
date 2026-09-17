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

	// ShadowMode 让自动判定只记录命中事件而不真正封禁，用于在开启前先观察
	// 规则会命中谁。仅在 AutoBanEnabled 为 true 时有意义。
	ShadowMode bool `json:"shadow_mode"`
	// RealtimeEnabled 在 relay 请求分发前做一次实时判定，命中即刻封禁，
	// 不必等下一轮定时扫描。定时扫描始终作为兜底保留。
	RealtimeEnabled bool `json:"realtime_enabled"`

	// 地理/ASN 判定：需要配置 MMDB 才生效，未配置时自动降级为不使用。
	// 命中条件为三维同时满足，用于识别「短时间内跨越多个国家与运营商」的
	// 代理池行为 —— 真实用户几乎不可能命中，误伤率极低。
	GeoEnabled       bool   `json:"geo_enabled"`
	GeoCountryCount  int    `json:"geo_country_count"`
	GeoASNCount      int    `json:"geo_asn_count"`
	GeoMinGapSeconds int    `json:"geo_min_gap_seconds"`
	GeoDurationHours int    `json:"geo_duration_hours"`
	CountryMMDBPath  string `json:"country_mmdb_path"`
	ASNMMDBPath      string `json:"asn_mmdb_path"`
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
	ShadowMode:                  false,
	RealtimeEnabled:             true,
	GeoEnabled:                  false,
	GeoCountryCount:             3,
	GeoASNCount:                 3,
	GeoMinGapSeconds:            180,
	GeoDurationHours:            72,
	CountryMMDBPath:             "",
	ASNMMDBPath:                 "",
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
	if setting.GeoCountryCount <= 0 {
		setting.GeoCountryCount = 3
	}
	if setting.GeoASNCount <= 0 {
		setting.GeoASNCount = 3
	}
	if setting.GeoMinGapSeconds <= 0 {
		setting.GeoMinGapSeconds = 180
	}
	if setting.GeoDurationHours <= 0 {
		setting.GeoDurationHours = 72
	}
	setting.CountryMMDBPath = strings.TrimSpace(setting.CountryMMDBPath)
	setting.ASNMMDBPath = strings.TrimSpace(setting.ASNMMDBPath)

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

// MatchBlackroomGeoRule 判断一份地理证据是否命中地理判定规则。三个条件
// 必须同时满足：国家数达标、ASN 数达标、存在短于阈值的 IP 切换间隔。
//
// minGapSeconds 为负表示窗口内无法计算切换间隔（不足两个合格 IP），
// 此时不构成命中。
func MatchBlackroomGeoRule(setting *BlackroomSetting, countryCount int, asnCount int, minGapSeconds int64) bool {
	NormalizeBlackroomSetting(setting)
	if setting == nil || !setting.GeoEnabled {
		return false
	}
	if countryCount < setting.GeoCountryCount || asnCount < setting.GeoASNCount {
		return false
	}
	if minGapSeconds < 0 || minGapSeconds >= int64(setting.GeoMinGapSeconds) {
		return false
	}
	return true
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

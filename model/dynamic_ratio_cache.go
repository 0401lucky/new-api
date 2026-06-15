package model

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type parsedDynamicRatioRule struct {
	DynamicRatioRule
	ParsedWeekdays []int
	ParsedModels   []string
	ParsedStartMin int
	ParsedEndMin   int
	HasTimeRange   bool
}

var (
	dynamicRatioRules     []parsedDynamicRatioRule
	dynamicRatioCacheLock sync.RWMutex
)

func parseDynamicRatioRules(rules []DynamicRatioRule) []parsedDynamicRatioRule {
	result := make([]parsedDynamicRatioRule, 0, len(rules))
	for _, r := range rules {
		parsed := parsedDynamicRatioRule{
			DynamicRatioRule: r,
			ParsedStartMin:   -1,
			ParsedEndMin:     -1,
		}

		if r.Weekdays != "" {
			var days []int
			if err := common.UnmarshalJsonStr(r.Weekdays, &days); err == nil && len(days) > 0 {
				parsed.ParsedWeekdays = days
			}
		}

		if r.Models != "" {
			var models []string
			if err := common.UnmarshalJsonStr(r.Models, &models); err == nil && len(models) > 0 {
				parsed.ParsedModels = models
			}
		}

		if r.StartTime != "" && r.EndTime != "" {
			startParts := strings.Split(r.StartTime, ":")
			endParts := strings.Split(r.EndTime, ":")
			if len(startParts) == 2 && len(endParts) == 2 {
				sh, _ := strconv.Atoi(startParts[0])
				sm, _ := strconv.Atoi(startParts[1])
				eh, _ := strconv.Atoi(endParts[0])
				em, _ := strconv.Atoi(endParts[1])
				parsed.ParsedStartMin = sh*60 + sm
				parsed.ParsedEndMin = eh*60 + em
				parsed.HasTimeRange = parsed.ParsedStartMin != parsed.ParsedEndMin
			}
		}

		result = append(result, parsed)
	}
	return result
}

func InitDynamicRatioCache() {
	var rules []DynamicRatioRule
	err := DB.Where("enable = ?", true).Order("priority ASC, id ASC").Find(&rules).Error
	if err != nil {
		common.SysError("failed to load dynamic ratio rules: " + err.Error())
		return
	}

	parsed := parseDynamicRatioRules(rules)

	dynamicRatioCacheLock.Lock()
	dynamicRatioRules = parsed
	dynamicRatioCacheLock.Unlock()

	common.SysLog("dynamic ratio rules synced from database")
}

func SyncDynamicRatioCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing dynamic ratio rules from database")
		InitDynamicRatioCache()
	}
}

func RefreshDynamicRatioCache() {
	InitDynamicRatioCache()
}

func SetDynamicRatioRulesForTest(rules []DynamicRatioRule) {
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		return rules[i].Id < rules[j].Id
	})

	parsed := parseDynamicRatioRules(rules)

	dynamicRatioCacheLock.Lock()
	dynamicRatioRules = parsed
	dynamicRatioCacheLock.Unlock()
}

func GetDynamicRatioRulesFromCache() []parsedDynamicRatioRule {
	dynamicRatioCacheLock.RLock()
	defer dynamicRatioCacheLock.RUnlock()
	result := make([]parsedDynamicRatioRule, len(dynamicRatioRules))
	copy(result, dynamicRatioRules)
	return result
}

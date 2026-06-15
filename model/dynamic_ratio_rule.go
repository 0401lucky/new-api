package model

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

type DynamicRatioRule struct {
	Id          int64   `json:"id" gorm:"primaryKey"`
	Enable      bool    `json:"enable" gorm:"default:true"`
	Group       string  `json:"group" gorm:"not null;index"`
	Models      string  `json:"models" gorm:"default:''"`
	Concurrency *int64  `json:"concurrency" gorm:""`
	Weekdays    string  `json:"weekdays" gorm:"default:''"`
	StartTime   string  `json:"start_time" gorm:"default:''"`
	EndTime     string  `json:"end_time" gorm:"default:''"`
	Ratio       float64 `json:"ratio" gorm:"not null"`
	Priority    int     `json:"priority" gorm:"default:0;index"`
	CreatedAt   int64   `json:"created_at" gorm:"not null"`
	UpdatedAt   int64   `json:"updated_at" gorm:"not null"`
}

func (DynamicRatioRule) TableName() string {
	return "dynamic_ratio_rules"
}

func (r *DynamicRatioRule) Validate() error {
	r.Group = strings.TrimSpace(r.Group)
	if r.Group == "" {
		return fmt.Errorf("分组不能为空")
	}
	if !ratio_setting.ContainsGroupRatio(r.Group) {
		return fmt.Errorf("分组 %s 不存在", r.Group)
	}
	if r.Models != "" {
		var models []string
		if err := common.UnmarshalJsonStr(r.Models, &models); err != nil {
			return fmt.Errorf("模型列表格式错误，应为 JSON 字符串数组")
		}
		for _, m := range models {
			if strings.TrimSpace(m) == "" {
				return fmt.Errorf("模型名称不能为空")
			}
		}
	}
	if r.Ratio <= 0 {
		return fmt.Errorf("倍率必须大于 0")
	}
	if r.Concurrency != nil && *r.Concurrency <= 0 {
		return fmt.Errorf("并发阈值必须大于 0")
	}
	if (r.StartTime == "") != (r.EndTime == "") {
		return fmt.Errorf("开始时间和结束时间必须同时设置或同时留空")
	}
	if r.StartTime != "" {
		if _, err := time.Parse("15:04", r.StartTime); err != nil {
			return fmt.Errorf("开始时间格式错误，应为 HH:MM")
		}
	}
	if r.EndTime != "" {
		if _, err := time.Parse("15:04", r.EndTime); err != nil {
			return fmt.Errorf("结束时间格式错误，应为 HH:MM")
		}
	}
	if r.Weekdays != "" {
		var days []int
		if err := common.UnmarshalJsonStr(r.Weekdays, &days); err != nil {
			return fmt.Errorf("星期格式错误，应为 JSON 数组")
		}
		for _, d := range days {
			if d < 0 || d > 6 {
				return fmt.Errorf("星期值必须在 0-6 范围内")
			}
		}
	}
	return nil
}

func GetDynamicRatioRules() ([]*DynamicRatioRule, error) {
	var rules []*DynamicRatioRule
	err := DB.Order("priority ASC, id ASC").Find(&rules).Error
	return rules, err
}

func GetDynamicRatioRuleById(id int64) (*DynamicRatioRule, error) {
	var rule DynamicRatioRule
	err := DB.Where("id = ?", id).First(&rule).Error
	return &rule, err
}

func CreateDynamicRatioRule(rule *DynamicRatioRule) error {
	now := time.Now().Unix()
	rule.CreatedAt = now
	rule.UpdatedAt = now
	return DB.Create(rule).Error
}

func UpdateDynamicRatioRule(rule *DynamicRatioRule) error {
	rule.UpdatedAt = time.Now().Unix()
	result := DB.Model(&DynamicRatioRule{}).Where("id = ?", rule.Id).Updates(map[string]interface{}{
		"enable":      rule.Enable,
		"group":       rule.Group,
		"models":      rule.Models,
		"concurrency": rule.Concurrency,
		"weekdays":    rule.Weekdays,
		"start_time":  rule.StartTime,
		"end_time":    rule.EndTime,
		"ratio":       rule.Ratio,
		"priority":    rule.Priority,
		"updated_at":  rule.UpdatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func DeleteDynamicRatioRule(id int64) error {
	return DB.Where("id = ?", id).Delete(&DynamicRatioRule{}).Error
}

func ReorderDynamicRatioRules(ids []int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if err := tx.Model(&DynamicRatioRule{}).Where("id = ?", id).Update("priority", i).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

type DynamicRatioStatus struct {
	Enabled     bool                  `json:"enabled"`
	ActiveRatio float64               `json:"active_ratio"`
	ActiveGroup string                `json:"active_group,omitempty"`
	Timezone    string                `json:"timezone"`
	RulesCount  int                   `json:"rules_count"`
	Rules       []DynamicRatioSummary `json:"rules"`
}

type DynamicRatioSummary struct {
	Group       string  `json:"group"`
	Concurrency *int64  `json:"concurrency"`
	Weekdays    string  `json:"weekdays"`
	StartTime   string  `json:"start_time"`
	EndTime     string  `json:"end_time"`
	Ratio       float64 `json:"ratio"`
	Priority    int     `json:"priority"`
}

func GetDynamicRatioStatus(group string) DynamicRatioStatus {
	return GetDynamicRatioStatusForGroups([]string{group})
}

func GetDynamicRatioStatusForGroups(groups []string) DynamicRatioStatus {
	status := DynamicRatioStatus{
		Enabled:     common.DynamicRatioEnabled,
		ActiveRatio: 1.0,
		Timezone:    common.StartupTimezoneName(),
	}

	if !common.DynamicRatioEnabled {
		return status
	}

	groups = normalizeDynamicRatioGroups(groups)
	if len(groups) == 0 {
		return status
	}
	groupSet := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		groupSet[group] = struct{}{}
	}

	dynamicRatioCacheLock.RLock()
	rules := make([]parsedDynamicRatioRule, len(dynamicRatioRules))
	copy(rules, dynamicRatioRules)
	dynamicRatioCacheLock.RUnlock()

	var groupRules []parsedDynamicRatioRule
	for _, r := range rules {
		if _, ok := groupSet[r.Group]; ok {
			groupRules = append(groupRules, r)
		}
	}

	status.RulesCount = len(groupRules)
	status.Rules = make([]DynamicRatioSummary, 0, len(groupRules))
	for _, r := range groupRules {
		status.Rules = append(status.Rules, DynamicRatioSummary{
			Group:       r.Group,
			Concurrency: r.Concurrency,
			Weekdays:    r.Weekdays,
			StartTime:   r.StartTime,
			EndTime:     r.EndTime,
			Ratio:       r.Ratio,
			Priority:    r.Priority,
		})
	}

	concurrency := getActiveConnections()
	now := common.NowInStartupTimezone()
	hasActiveRatio := false
	for _, group := range groups {
		activeRatio := matchDynamicRatioIgnoreModel(rules, group, concurrency, now)
		if activeRatio <= 0 {
			continue
		}
		if !hasActiveRatio || math.Abs(activeRatio-1) > math.Abs(status.ActiveRatio-1) {
			status.ActiveRatio = activeRatio
			status.ActiveGroup = group
			hasActiveRatio = true
		}
	}

	return status
}

func normalizeDynamicRatioGroups(groups []string) []string {
	groupSet := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		groupSet[group] = struct{}{}
	}

	result := make([]string, 0, len(groupSet))
	for group := range groupSet {
		result = append(result, group)
	}
	sort.Strings(result)
	return result
}

func GetMatchedDynamicRatio(group string, modelName string) float64 {
	if !common.DynamicRatioEnabled {
		return 0
	}

	dynamicRatioCacheLock.RLock()
	rules := dynamicRatioRules
	dynamicRatioCacheLock.RUnlock()

	return matchDynamicRatio(rules, group, modelName, getActiveConnections(), common.NowInStartupTimezone())
}

func matchDynamicRatio(rules []parsedDynamicRatioRule, group string, modelName string, concurrency int64, now time.Time) float64 {
	type scoredRule struct {
		rule           parsedDynamicRatioRule
		hasConcurrency bool
		concurrencyGap int64
		hasModel       bool
	}

	var matched []scoredRule
	currentMinutes := now.Hour()*60 + now.Minute()

	for _, r := range rules {
		if !r.Enable {
			continue
		}
		if r.Group != group {
			continue
		}

		modelMatched := false
		if r.ParsedModels != nil {
			for _, pattern := range r.ParsedModels {
				if matchModelPattern(modelName, pattern) {
					modelMatched = true
					break
				}
			}
			if !modelMatched {
				continue
			}
		}

		effectiveWeekday := int(now.Weekday())
		if r.HasTimeRange {
			startMinutes := r.ParsedStartMin
			endMinutes := r.ParsedEndMin

			if startMinutes <= endMinutes {
				if currentMinutes < startMinutes || currentMinutes >= endMinutes {
					continue
				}
			} else {
				if currentMinutes < startMinutes && currentMinutes >= endMinutes {
					continue
				}
				if currentMinutes < endMinutes {
					effectiveWeekday = int(now.AddDate(0, 0, -1).Weekday())
				}
			}
		}

		if r.Concurrency != nil && concurrency <= *r.Concurrency {
			continue
		}

		if r.ParsedWeekdays != nil {
			found := false
			for _, d := range r.ParsedWeekdays {
				if d == effectiveWeekday {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		sr := scoredRule{
			rule:           r,
			hasConcurrency: r.Concurrency != nil,
			hasModel:       r.ParsedModels != nil,
		}
		if r.Concurrency != nil {
			sr.concurrencyGap = concurrency - *r.Concurrency
		}
		matched = append(matched, sr)
	}

	if len(matched) == 0 {
		return 0
	}

	best := matched[0]
	for _, m := range matched[1:] {
		if m.hasModel && !best.hasModel {
			best = m
		} else if !m.hasModel && best.hasModel {
			continue
		} else if m.hasConcurrency && !best.hasConcurrency {
			best = m
		} else if !m.hasConcurrency && best.hasConcurrency {
			continue
		} else if m.hasConcurrency && best.hasConcurrency {
			if m.concurrencyGap < best.concurrencyGap {
				best = m
			} else if m.concurrencyGap == best.concurrencyGap {
				if m.rule.Priority < best.rule.Priority {
					best = m
				} else if m.rule.Priority == best.rule.Priority && m.rule.Id < best.rule.Id {
					best = m
				}
			}
		} else {
			if m.rule.Priority < best.rule.Priority {
				best = m
			} else if m.rule.Priority == best.rule.Priority && m.rule.Id < best.rule.Id {
				best = m
			}
		}
	}

	return best.rule.Ratio
}

func matchModelPattern(modelName string, pattern string) bool {
	if modelName == "" {
		return false
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "*" {
		return true
	}
	if pattern == modelName {
		return true
	}
	if strings.HasSuffix(pattern, "*") && !strings.Contains(pattern[:len(pattern)-1], "*") {
		prefix := pattern[:len(pattern)-1]
		return strings.HasPrefix(modelName, prefix)
	}
	if strings.HasPrefix(pattern, "*") && !strings.Contains(pattern[1:], "*") {
		suffix := pattern[1:]
		return strings.HasSuffix(modelName, suffix)
	}
	return false
}

func matchDynamicRatioIgnoreModel(rules []parsedDynamicRatioRule, group string, concurrency int64, now time.Time) float64 {
	type scoredRule struct {
		rule           parsedDynamicRatioRule
		hasConcurrency bool
		concurrencyGap int64
	}

	var matched []scoredRule
	currentMinutes := now.Hour()*60 + now.Minute()

	for _, r := range rules {
		if !r.Enable {
			continue
		}
		if r.Group != group {
			continue
		}

		effectiveWeekday := int(now.Weekday())
		if r.HasTimeRange {
			startMinutes := r.ParsedStartMin
			endMinutes := r.ParsedEndMin

			if startMinutes <= endMinutes {
				if currentMinutes < startMinutes || currentMinutes >= endMinutes {
					continue
				}
			} else {
				if currentMinutes < startMinutes && currentMinutes >= endMinutes {
					continue
				}
				if currentMinutes < endMinutes {
					effectiveWeekday = int(now.AddDate(0, 0, -1).Weekday())
				}
			}
		}

		if r.Concurrency != nil && concurrency <= *r.Concurrency {
			continue
		}

		if r.ParsedWeekdays != nil {
			found := false
			for _, d := range r.ParsedWeekdays {
				if d == effectiveWeekday {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		sr := scoredRule{
			rule:           r,
			hasConcurrency: r.Concurrency != nil,
		}
		if r.Concurrency != nil {
			sr.concurrencyGap = concurrency - *r.Concurrency
		}
		matched = append(matched, sr)
	}

	if len(matched) == 0 {
		return 0
	}

	best := matched[0]
	for _, m := range matched[1:] {
		if m.hasConcurrency && !best.hasConcurrency {
			best = m
		} else if !m.hasConcurrency && best.hasConcurrency {
			continue
		} else if m.hasConcurrency && best.hasConcurrency {
			if m.concurrencyGap < best.concurrencyGap {
				best = m
			} else if m.concurrencyGap == best.concurrencyGap {
				if m.rule.Priority < best.rule.Priority {
					best = m
				} else if m.rule.Priority == best.rule.Priority && m.rule.Id < best.rule.Id {
					best = m
				}
			}
		} else {
			if m.rule.Priority < best.rule.Priority {
				best = m
			} else if m.rule.Priority == best.rule.Priority && m.rule.Id < best.rule.Id {
				best = m
			}
		}
	}

	return best.rule.Ratio
}

func getActiveConnections() int64 {
	if common.GetActiveConnectionsFunc != nil {
		return common.GetActiveConnectionsFunc()
	}
	return 0
}

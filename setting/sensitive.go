package setting

import (
	"path"
	"strconv"
	"strings"
)

var CheckSensitiveEnabled = true
var CheckSensitiveOnPromptEnabled = true

//var CheckSensitiveOnCompletionEnabled = true

// StopOnSensitiveEnabled 如果检测到敏感词，是否立刻停止生成，否则替换敏感词
var StopOnSensitiveEnabled = true

// StreamCacheQueueLength 流模式缓存队列长度，0表示无缓存
var StreamCacheQueueLength = 0

const (
	PromptCheckModeMonitor = "monitor"
	PromptCheckModeWarn    = "warn"
	PromptCheckModeBlock   = "block"
)

var PromptCheckMode = PromptCheckModeBlock
var PromptCheckThreshold = 50
var PromptCheckStrictThreshold = 90
var PromptCheckLogMatchesEnabled = true
var PromptCheckMaxTextLength = 81920
var PromptCheckModelScope = "gpt*\no*\nchatgpt*"
var PromptCheckGroupWhitelist = ""
var PromptCheckChannelWhitelist = ""
var PromptCheckDisabledRules = ""
var PromptCheckAPIReviewEnabled = false
var PromptCheckAPIReviewModel = "omni-moderation-latest"
var PromptCheckAPIReviewBaseURL = "https://api.openai.com"
var PromptCheckAPIReviewKey = ""
var PromptCheckAPIReviewTimeoutMS = 3000
var PromptCheckAPIReviewFailClosedEnabled = false

// SensitiveWords 敏感词
// var SensitiveWords []string
var SensitiveWords = []string{
	"test_sensitive",
}

func SensitiveWordsToString() string {
	return strings.Join(SensitiveWords, "\n")
}

func SensitiveWordsFromString(s string) {
	SensitiveWords = []string{}
	sw := strings.Split(s, "\n")
	for _, w := range sw {
		w = strings.TrimSpace(w)
		if w != "" {
			SensitiveWords = append(SensitiveWords, w)
		}
	}
}

func ShouldCheckPromptSensitive() bool {
	return CheckSensitiveEnabled && CheckSensitiveOnPromptEnabled
}

func NormalizePromptCheckMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case PromptCheckModeMonitor:
		return PromptCheckModeMonitor
	case PromptCheckModeWarn:
		return PromptCheckModeWarn
	case PromptCheckModeBlock:
		return PromptCheckModeBlock
	default:
		return PromptCheckModeBlock
	}
}

func PromptCheckEffectiveThreshold() int {
	if PromptCheckThreshold <= 0 {
		return 50
	}
	if PromptCheckThreshold > 500 {
		return 500
	}
	return PromptCheckThreshold
}

func PromptCheckEffectiveStrictThreshold() int {
	threshold := PromptCheckEffectiveThreshold()
	if PromptCheckStrictThreshold <= 0 {
		return 90
	}
	if PromptCheckStrictThreshold < threshold {
		return threshold
	}
	if PromptCheckStrictThreshold > 1000 {
		return 1000
	}
	return PromptCheckStrictThreshold
}

func PromptCheckEffectiveMaxTextLength() int {
	if PromptCheckMaxTextLength <= 0 {
		return 81920
	}
	if PromptCheckMaxTextLength > 1024*1024 {
		return 1024 * 1024
	}
	return PromptCheckMaxTextLength
}

func PromptCheckEffectiveReviewTimeoutMS() int {
	if PromptCheckAPIReviewTimeoutMS <= 0 {
		return 3000
	}
	if PromptCheckAPIReviewTimeoutMS > 30000 {
		return 30000
	}
	return PromptCheckAPIReviewTimeoutMS
}

func ShouldCheckPromptForRequest(model string, group string, channelID int) bool {
	if !ShouldCheckPromptSensitive() {
		return false
	}
	if matchListValue(PromptCheckGroupWhitelist, group) {
		return false
	}
	if matchChannelListValue(PromptCheckChannelWhitelist, channelID) {
		return false
	}
	return matchModelScope(PromptCheckModelScope, model)
}

func PromptCheckAPIReviewReady() bool {
	return PromptCheckAPIReviewEnabled &&
		strings.TrimSpace(PromptCheckAPIReviewKey) != "" &&
		strings.TrimSpace(PromptCheckAPIReviewModel) != ""
}

func PromptCheckDisabledRuleSet() map[string]bool {
	result := make(map[string]bool)
	for _, item := range promptCheckList(PromptCheckDisabledRules) {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			result[item] = true
		}
	}
	return result
}

func IsPromptCheckRuleDisabled(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	return PromptCheckDisabledRuleSet()[name]
}

func promptCheckList(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			result = append(result, field)
		}
	}
	return result
}

func matchListValue(raw string, value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, item := range promptCheckList(raw) {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "*" || item == value {
			return true
		}
	}
	return false
}

func matchChannelListValue(raw string, channelID int) bool {
	if channelID <= 0 {
		return false
	}
	for _, item := range promptCheckList(raw) {
		id, err := strconv.Atoi(strings.TrimSpace(item))
		if err == nil && id == channelID {
			return true
		}
	}
	return false
}

func matchModelScope(raw string, model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	patterns := promptCheckList(raw)
	if len(patterns) == 0 {
		return true
	}
	for _, item := range patterns {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if item == "*" || item == "all" {
			return true
		}
		if strings.ContainsAny(item, "*?") {
			if ok, err := path.Match(item, model); err == nil && ok {
				return true
			}
			continue
		}
		if item == model {
			return true
		}
	}
	return false
}

//func ShouldCheckCompletionSensitive() bool {
//	return CheckSensitiveEnabled && CheckSensitiveOnCompletionEnabled
//}

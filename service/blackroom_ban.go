package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// blackroomBanDecision 表示根据阶梯规则与升级逻辑得出的封禁时长决策。
type blackroomBanDecision struct {
	Matched         bool
	Permanent       bool
	DurationSeconds int64
	BannedUntil     int64
	Escalated       bool
	Rule            operation_setting.BlackroomRule
}

// resolveBlackroomBanDecision 用 ipCount 匹配阶梯规则并应用临时封禁升级。
// allowEscalation 仅在「新建封禁（当前无生效记录）」时为 true，与自动扫描一致。
// Matched=false 表示 ipCount 未命中任何规则。
func resolveBlackroomBanDecision(setting *operation_setting.BlackroomSetting, userID, ipCount int, now int64, allowEscalation bool) (blackroomBanDecision, error) {
	rule, ok := operation_setting.MatchBlackroomRule(setting, ipCount)
	if !ok {
		return blackroomBanDecision{Matched: false}, nil
	}

	decision := blackroomBanDecision{
		Matched:         true,
		Permanent:       rule.Permanent,
		DurationSeconds: int64(rule.DurationHours * 3600),
		Rule:            rule,
	}
	if decision.Permanent {
		decision.DurationSeconds = 0
		return decision, nil
	}

	if allowEscalation && setting.EscalationTemporaryBanCount > 0 {
		since := now - int64(setting.EscalationWindowDays*24*3600)
		recentTemporaryCount, err := model.CountRecentTemporaryBlackroomBans(userID, since)
		if err != nil {
			return blackroomBanDecision{}, err
		}
		if recentTemporaryCount+1 >= int64(setting.EscalationTemporaryBanCount) {
			decision.Permanent = true
			decision.DurationSeconds = 0
			decision.Escalated = true
			return decision, nil
		}
	}

	decision.BannedUntil = now + decision.DurationSeconds
	return decision, nil
}

package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
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

// CreateExternalBlackroomBan 由外部风控工具经 HTTP 接口调用，将用户纳入小黑屋
// （来源 external）并联动 users.status=禁用。时长优先级：
// permanent > durationHours > 阶梯规则(ipCount)+升级 > 永久兜底。
func CreateExternalBlackroomBan(userID, ipCount int, reason, evidence string, permanent bool, durationHours int) (*model.BlackroomBan, error) {
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return nil, err
	}
	if user.Role >= common.RoleAdminUser {
		return nil, errors.New("不能将管理员加入小黑屋")
	}

	existing, existingErr := model.GetActiveBlackroomBan(user.Id)
	if existingErr != nil && !errors.Is(existingErr, gorm.ErrRecordNotFound) {
		return nil, existingErr
	}
	if existingErr == nil && existing != nil && existing.Source == model.BlackroomBanSourceManual {
		return existing, nil
	}

	setting := operation_setting.GetBlackroomSetting()
	now := common.GetTimestamp()

	durationSeconds := int64(0)
	bannedUntil := int64(0)
	escalated := false

	switch {
	case permanent:
		// 永久封禁：durationSeconds/bannedUntil 保持 0。
	case durationHours > 0:
		durationSeconds = int64(durationHours * 3600)
		bannedUntil = now + durationSeconds
	default:
		decision, derr := resolveBlackroomBanDecision(setting, user.Id, ipCount, now, existingErr != nil)
		if derr != nil {
			return nil, derr
		}
		if decision.Matched {
			durationSeconds = decision.DurationSeconds
			bannedUntil = decision.BannedUntil
			escalated = decision.Escalated
		}
		// 未命中规则时保持 durationSeconds=0、bannedUntil=0，作为永久兜底。
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		if ipCount > 0 {
			reason = fmt.Sprintf("外部风控检测到 %d 个不同 IP", ipCount)
		} else {
			reason = "外部 AI 风控封禁"
		}
		if escalated {
			reason += "，已触发多次封禁升级"
		}
	}

	if strings.TrimSpace(evidence) == "" {
		evidenceBytes, merr := common.Marshal(map[string]any{
			"source":    model.BlackroomBanSourceExternal,
			"ip_count":  ipCount,
			"permanent": bannedUntil == 0,
			"escalated": escalated,
		})
		if merr != nil {
			return nil, merr
		}
		evidence = string(evidenceBytes)
	}

	ban, _, err := model.UpsertActiveBlackroomBan(model.BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             model.BlackroomBanSourceExternal,
		Reason:             reason,
		Evidence:           evidence,
		IpCount:            ipCount,
		WindowStart:        now,
		WindowEnd:          now,
		BanDurationSeconds: durationSeconds,
		BannedUntil:        bannedUntil,
	})
	if err != nil {
		return nil, err
	}

	if err := model.SetBlackroomUserStatus(user.Id, common.UserStatusDisabled); err != nil {
		return nil, err
	}
	model.InvalidateBlackroomUserAuthCache(user.Id)

	return ban, nil
}

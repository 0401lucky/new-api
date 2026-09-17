package service

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/ipgeo"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// BlackroomDecision 是一次判定的结果。Ban 非空表示该用户当前处于生效封禁
// 中；Skipped 表示因账号状态、角色或豁免名单而不参与判定。
type BlackroomDecision struct {
	Ban           *model.BlackroomBan
	NewlyApplied  bool
	ShadowMatched bool
	Skipped       bool
	IpCount       int
}

// evaluateBlackroomUser 对单个用户执行完整判定，实时判定与定时扫描共用：
// 先做账号状态、角色与豁免检查，再计算回看窗口、匹配 IP 数与地理规则，
// 命中后取更严格的一档决定时长，最后写入封禁或影子事件。
//
// 返回 (nil, nil) 表示未命中任何规则。调用方负责处理「已有生效封禁」的
// 前置分支，因为实时判定与定时扫描对此的处理不同。
func evaluateBlackroomUser(
	setting *operation_setting.BlackroomSetting,
	userId int,
	username string,
	now int64,
	allowEscalation bool,
) (*BlackroomDecision, error) {
	if userId <= 0 {
		return &BlackroomDecision{Skipped: true}, nil
	}

	user, err := model.GetUserById(userId, false)
	if err != nil {
		return nil, err
	}
	if user.Role >= common.RoleAdminUser || user.Status != common.UserStatusEnabled {
		return &BlackroomDecision{Skipped: true}, nil
	}
	if operation_setting.IsBlackroomUserExempt(setting, user.Id, user.Group) {
		return &BlackroomDecision{Skipped: true}, nil
	}

	// 上次封禁窗口已覆盖的观测不再参与定罪：旧数据会在回看窗口内反复命中，
	// 导致到期后被重新封禁、生效中被误续期、升级计数被重复累加。
	windowStart := now - int64(setting.LookbackHours*3600)
	trimmedToLastBan := false
	lastWindowEnd, err := model.GetLatestBlackroomBanWindowEnd(user.Id)
	if err != nil {
		return nil, err
	}
	if lastWindowEnd >= windowStart {
		windowStart = lastWindowEnd + 1
		trimmedToLastBan = true
	}

	summary, err := model.SummarizeBlackroomIPAudit(user.Id, windowStart, now)
	if err != nil {
		return nil, err
	}

	rule, ipRuleMatched := operation_setting.MatchBlackroomRule(setting, summary.IPCount)

	var geoEvidence model.BlackroomIPEvidence
	geoMatched := false
	if setting.GeoEnabled {
		rows, err := model.GetBlackroomIPRowsForEvaluation(user.Id, windowStart, now, model.BlackroomIPAuditMaxEvaluationRows)
		if err != nil {
			return nil, err
		}
		geoEvidence = model.BuildBlackroomIPEvidence(rows, summary, windowStart, now, ipgeo.DefaultStatus().Version)
		geoMatched = operation_setting.MatchBlackroomGeoRule(
			setting, geoEvidence.CountryCount, geoEvidence.ASNCount, geoEvidence.MinimumGapSeconds,
		)
	}
	if !ipRuleMatched && !geoMatched {
		return nil, nil
	}

	// 两条规则可以同时命中，取更严格的一档：永久封禁优先，其次取更长的时长。
	permanent := false
	durationSeconds := int64(0)
	bannedUntil := int64(0)
	escalated := false
	reasons := make([]string, 0, 2)

	// 只有真正产出时长决策才允许继续，避免在规则判定与时长解析不一致时
	// 落到「无理由且永久」的封禁。
	decided := false

	if ipRuleMatched {
		decision, err := resolveBlackroomBanDecision(setting, user.Id, summary.IPCount, now, allowEscalation)
		if err != nil {
			return nil, err
		}
		if decision.Matched {
			decided = true
			permanent = decision.Permanent
			durationSeconds = decision.DurationSeconds
			bannedUntil = decision.BannedUntil
			escalated = decision.Escalated
			if trimmedToLastBan {
				reasons = append(reasons, fmt.Sprintf("上次封禁后使用了 %d 个不同 IP", summary.IPCount))
			} else {
				reasons = append(reasons, fmt.Sprintf("%d 小时内使用了 %d 个不同 IP", setting.LookbackHours, summary.IPCount))
			}
			if decision.Escalated {
				reasons = append(reasons, "已触发多次封禁升级")
			}
		}
	}
	if geoMatched {
		decided = true
		geoSeconds := int64(setting.GeoDurationHours) * 3600
		if !permanent && geoSeconds > durationSeconds {
			durationSeconds = geoSeconds
			bannedUntil = now + geoSeconds
		}
		reasons = append(reasons, fmt.Sprintf(
			"%d 秒内跨越 %d 个国家 / %d 个 ASN",
			geoEvidence.MinimumGapSeconds, geoEvidence.CountryCount, geoEvidence.ASNCount,
		))
	}
	if !decided {
		return nil, nil
	}

	evidence, err := buildBlackroomEvidence(setting, summary, geoEvidence, windowStart, now, rule, ipRuleMatched, geoMatched, escalated)
	if err != nil {
		return nil, err
	}
	reason := strings.Join(reasons, "，")

	if setting.ShadowMode {
		if err := model.RecordBlackroomShadowMatch(user.Id, reason, evidence); err != nil {
			return nil, err
		}
		return &BlackroomDecision{ShadowMatched: true, IpCount: summary.IPCount}, nil
	}

	ips, err := model.ListBlackroomIPsForUser(user.Id, windowStart, now, 200)
	if err != nil {
		return nil, err
	}
	ipListBytes, err := common.Marshal(ips)
	if err != nil {
		return nil, err
	}

	ban, created, err := model.UpsertActiveBlackroomBan(model.BlackroomBanInput{
		UserId:             user.Id,
		Username:           username,
		Source:             model.BlackroomBanSourceAuto,
		Reason:             reason,
		Evidence:           evidence,
		IpCount:            summary.IPCount,
		IpList:             string(ipListBytes),
		WindowStart:        windowStart,
		WindowEnd:          now,
		BanDurationSeconds: durationSeconds,
		BannedUntil:        bannedUntil,
	})
	if err != nil {
		return nil, err
	}
	if ban == nil {
		return &BlackroomDecision{Skipped: true}, nil
	}
	InvalidateBlackroomRealtimeCache(user.Id)
	return &BlackroomDecision{Ban: ban, NewlyApplied: created, IpCount: summary.IPCount}, nil
}

func buildBlackroomEvidence(
	setting *operation_setting.BlackroomSetting,
	summary *model.BlackroomIPAuditSummary,
	geoEvidence model.BlackroomIPEvidence,
	windowStart int64,
	windowEnd int64,
	rule operation_setting.BlackroomRule,
	ipRuleMatched bool,
	geoMatched bool,
	escalated bool,
) (string, error) {
	payload := map[string]any{
		"window_start":     windowStart,
		"window_end":       windowEnd,
		"lookback_hours":   setting.LookbackHours,
		"ip_count":         summary.IPCount,
		"request_count":    summary.RequestCount,
		"ip_rule_matched":  ipRuleMatched,
		"geo_rule_matched": geoMatched,
		"escalated":        escalated,
		"shadow_mode":      setting.ShadowMode,
	}
	if ipRuleMatched {
		payload["rule"] = rule
	}
	if setting.GeoEnabled {
		payload["geo"] = geoEvidence
	}
	bytes, err := common.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

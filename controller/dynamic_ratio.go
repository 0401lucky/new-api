package controller

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/groupload"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

type groupMultiplierStatus struct {
	Group            string                      `json:"group"`
	Description      string                      `json:"description"`
	Policy           model.GroupMultiplierPolicy `json:"policy"`
	Version          string                      `json:"version"`
	BaseRatio        float64                     `json:"base_ratio"`
	Concurrency      *int64                      `json:"concurrency"`
	Factor           *float64                    `json:"factor"`
	EffectiveRatio   *float64                    `json:"effective_ratio"`
	NextRequestRatio *float64                    `json:"next_request_ratio"`
	NextTier         *model.ConcurrencyTier      `json:"next_tier"`
}

func GetGroupMultiplierPolicies(c *gin.Context) {
	groups := map[string]string{}
	for group := range ratio_setting.GetGroupRatioCopy() {
		groups[group] = ""
	}
	writeGroupMultiplierStatuses(c, groups, "")
}

func GetGroupMultiplierStatuses(c *gin.Context) {
	user, err := model.GetUserById(c.GetInt("id"), false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	writeGroupMultiplierStatuses(c, service.GetUserUsableGroups(user.Group), user.Group)
}

func writeGroupMultiplierStatuses(c *gin.Context, groups map[string]string, userGroup string) {
	names := make([]string, 0, len(groups))
	for name := range groups {
		if ratio_setting.ContainsGroupRatio(name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	statuses := make([]groupMultiplierStatus, 0, len(names))
	for _, group := range names {
		policy, version := model.GetGroupMultiplierPolicy(group)
		base := ratio_setting.GetGroupRatio(group)
		if userGroup != "" {
			if special, ok := ratio_setting.GetGroupGroupRatio(userGroup, group); ok {
				base = special
			}
		}
		status := groupMultiplierStatus{Group: group, Description: groups[group], Policy: policy, Version: version, BaseRatio: base}
		count, err := groupload.Count(group)
		if err == nil {
			status.Concurrency = &count
		}
		if policy.Mode == model.MultiplierFixed || base == 0 {
			factor := 1.0
			status.Factor, status.EffectiveRatio, status.NextRequestRatio = &factor, &base, &base
		} else if policy.Mode == model.MultiplierConcurrency && err == nil {
			tier := policy.TierAt(count)
			effective, next := base*tier.Multiplier, base*policy.TierAt(count+1).Multiplier
			status.Factor, status.EffectiveRatio, status.NextRequestRatio = &tier.Multiplier, &effective, &next
			for _, candidate := range policy.Tiers {
				if candidate.Minimum > count {
					status.NextTier = &candidate
					break
				}
			}
		}
		statuses = append(statuses, status)
	}
	common.ApiSuccess(c, statuses)
}

func UpdateGroupMultiplierPolicy(c *gin.Context) {
	var request struct {
		Group           string                      `json:"group"`
		Policy          model.GroupMultiplierPolicy `json:"policy"`
		ExpectedVersion string                      `json:"expected_version"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if !ratio_setting.ContainsGroupRatio(request.Group) {
		common.ApiErrorMsg(c, "分组不存在")
		return
	}
	if err := model.SaveGroupMultiplierPolicy(request.Group, request.Policy, request.ExpectedVersion); err != nil {
		if errors.Is(err, model.ErrGroupMultiplierConflict) {
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetDynamicRatioRules(c *gin.Context) {
	rules, err := model.GetDynamicRatioRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rules)
}

func CreateDynamicRatioRule(c *gin.Context) {
	var rule model.DynamicRatioRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := rule.Validate(); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.CreateDynamicRatioRule(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshDynamicRatioCache()
	common.ApiSuccess(c, rule)
}

func UpdateDynamicRatioRule(c *gin.Context) {
	var rule model.DynamicRatioRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	if rule.Id == 0 {
		common.ApiErrorMsg(c, "规则 ID 不能为空")
		return
	}
	if err := rule.Validate(); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateDynamicRatioRule(&rule); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshDynamicRatioCache()
	common.ApiSuccess(c, rule)
}

func DeleteDynamicRatioRule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		common.ApiErrorMsg(c, "无效的规则 ID")
		return
	}
	if err := model.DeleteDynamicRatioRule(id); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshDynamicRatioCache()
	common.ApiSuccess(c, nil)
}

func ReorderDynamicRatioRules(c *gin.Context) {
	var req struct {
		Ids []int64 `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(req.Ids) == 0 {
		common.ApiErrorMsg(c, "ID 列表不能为空")
		return
	}
	if err := model.ReorderDynamicRatioRules(req.Ids); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshDynamicRatioCache()
	common.ApiSuccess(c, nil)
}

func SetDynamicRatioEnabled(c *gin.Context) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateOption("DynamicRatioEnabled", strconv.FormatBool(req.Enabled)); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func GetDynamicRatioStatus(c *gin.Context) {
	group := strings.TrimSpace(c.Query("group"))
	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	if group != "" {
		if !service.GroupInUserUsableGroups(user.Group, group) {
			common.ApiErrorMsg(c, "无权访问该分组")
			return
		}
		common.ApiSuccess(c, model.GetDynamicRatioStatusWithBalance(group, int64(user.Quota)))
		return
	}

	usableGroups := service.GetUserUsableGroups(user.Group)
	groups := make([]string, 0, len(usableGroups)+1)
	for usableGroup := range usableGroups {
		groups = append(groups, usableGroup)
	}
	if user.Group != "" {
		groups = append(groups, user.Group)
	}

	common.ApiSuccess(c, model.GetDynamicRatioStatusForGroupsWithBalance(groups, int64(user.Quota)))
}

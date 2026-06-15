package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

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
		common.ApiSuccess(c, model.GetDynamicRatioStatus(group))
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

	common.ApiSuccess(c, model.GetDynamicRatioStatusForGroups(groups))
}

package controller

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// GetCheckinStatus 获取用户签到状态和历史记录
func GetCheckinStatus(c *gin.Context) {
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "签到功能未启用")
		return
	}
	userId := c.GetInt("id")
	// 获取月份参数，默认为当前月份
	month := c.DefaultQuery("month", common.NowInStartupTimezone().Format("2006-01"))

	stats, err := model.GetUserCheckinStats(userId, month)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 签到时间领域数据（后端是唯一权威来源）
	timeInfo, err := service.ComputeCheckinTimeInfo(userId, stats["checked_in_today"].(bool))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 奖励类型：今天已签到则展示当天记录的奖励类型，而不是当前配置（配置可能中途切换）
	rewardType := setting.RewardType
	if stats["checked_in_today"].(bool) {
		if todayCheckin, err := model.GetTodayCheckin(userId); err == nil && todayCheckin != nil {
			rewardType = "permanent"
			if todayCheckin.IsTemporary() {
				rewardType = "temporary"
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"enabled":                 setting.Enabled,
			"min_quota":               setting.MinQuota,
			"max_quota":               setting.MaxQuota,
			"fixed_quota":             setting.FixedQuota,
			"random_mode":             setting.RandomMode,
			"reward_type":             rewardType,
			"available_from_minutes":  setting.AvailableFromMinutes,
			"timezone":                timeInfo.Timezone,
			"current_date":            timeInfo.CurrentDate,
			"server_time":             timeInfo.ServerTime,
			"state":                   timeInfo.State,
			"available_from":          timeInfo.AvailableFrom,
			"expires_at":              timeInfo.ExpiresAt,
			"next_transition_at":      timeInfo.NextTransitionAt,
			"temporary_quota":         timeInfo.TemporaryQuota,
			"available_from_display":  timeInfo.AvailableFromDisplay,
			"expires_at_display":      timeInfo.ExpiresAtDisplay,
			"stats":                   stats,
		},
	})
}

// DoCheckin 执行用户签到
func DoCheckin(c *gin.Context) {
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		common.ApiErrorMsg(c, "签到功能未启用")
		return
	}

	userId := c.GetInt("id")

	checkin, err := model.UserCheckin(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	quotaType := "permanent"
	if checkin.IsTemporary() {
		quotaType = "temporary"
	}
	model.RecordLog(userId, model.LogTypeSystem, fmt.Sprintf("用户签到，获得额度 %s", logger.LogQuota(checkin.QuotaAwarded)))

	data := gin.H{
		"quota_awarded": checkin.QuotaAwarded,
		"checkin_date":  checkin.CheckinDate,
		"quota_type":    quotaType,
	}
	if checkin.IsTemporary() {
		data["temporary_quota"] = checkin.QuotaRemaining
		data["temporary_quota_expires_at"] = checkin.QuotaExpiresAt
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "签到成功",
		"data":    data,
	})
}

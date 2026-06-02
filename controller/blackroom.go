package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

type blackroomManualBanRequest struct {
	UserID        int    `json:"user_id"`
	DurationHours int    `json:"duration_hours"`
	Permanent     bool   `json:"permanent"`
	Reason        string `json:"reason"`
}

type blackroomReleaseRequest struct {
	Reason string `json:"reason"`
}

type blackroomExternalBanRequest struct {
	UserID        int    `json:"user_id"`
	IpCount       int    `json:"ip_count"`
	Reason        string `json:"reason"`
	Evidence      string `json:"evidence"`
	Permanent     bool   `json:"permanent"`
	DurationHours int    `json:"duration_hours"`
}

func GetBlackroomBans(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userID, _ := strconv.Atoi(c.Query("user_id"))
	bans, total, err := model.ListBlackroomBans(
		c.Query("filter"),
		c.Query("status"),
		c.Query("source"),
		userID,
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(bans)
	common.ApiSuccess(c, pageInfo)
}

func GetBlackroomSetting(c *gin.Context) {
	common.ApiSuccess(c, operation_setting.GetBlackroomSetting())
}

func UpdateBlackroomSetting(c *gin.Context) {
	var req operation_setting.BlackroomSetting
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	operation_setting.NormalizeBlackroomSetting(&req)

	rulesBytes, err := common.Marshal(req.Rules)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	exemptUserIDsBytes, err := common.Marshal(req.ExemptUserIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	exemptGroupsBytes, err := common.Marshal(req.ExemptGroups)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	values := map[string]string{
		"blackroom_setting.enabled":                        strconv.FormatBool(req.Enabled),
		"blackroom_setting.auto_ban_enabled":               strconv.FormatBool(req.AutoBanEnabled),
		"blackroom_setting.lookback_hours":                 strconv.Itoa(req.LookbackHours),
		"blackroom_setting.check_interval_minutes":         strconv.Itoa(req.CheckIntervalMinutes),
		"blackroom_setting.min_requests":                   strconv.Itoa(req.MinRequests),
		"blackroom_setting.rules":                          string(rulesBytes),
		"blackroom_setting.escalation_window_days":         strconv.Itoa(req.EscalationWindowDays),
		"blackroom_setting.escalation_temporary_ban_count": strconv.Itoa(req.EscalationTemporaryBanCount),
		"blackroom_setting.exempt_user_ids":                string(exemptUserIDsBytes),
		"blackroom_setting.exempt_groups":                  string(exemptGroupsBytes),
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	if cfg := config.GlobalConfig.Get("blackroom_setting"); cfg != nil {
		_ = config.UpdateConfigFromMap(cfg, map[string]string{
			"rules":                          string(rulesBytes),
			"exempt_user_ids":                string(exemptUserIDsBytes),
			"exempt_groups":                  string(exemptGroupsBytes),
			"enabled":                        strconv.FormatBool(req.Enabled),
			"auto_ban_enabled":               strconv.FormatBool(req.AutoBanEnabled),
			"lookback_hours":                 strconv.Itoa(req.LookbackHours),
			"check_interval_minutes":         strconv.Itoa(req.CheckIntervalMinutes),
			"min_requests":                   strconv.Itoa(req.MinRequests),
			"escalation_window_days":         strconv.Itoa(req.EscalationWindowDays),
			"escalation_temporary_ban_count": strconv.Itoa(req.EscalationTemporaryBanCount),
		})
	}
	common.ApiSuccess(c, operation_setting.GetBlackroomSetting())
}

func ManualBanBlackroomUser(c *gin.Context) {
	var req blackroomManualBanRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	ban, err := service.CreateManualBlackroomBan(req.UserID, req.DurationHours, req.Permanent, req.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, ban)
}

func ExternalBanBlackroomUser(c *gin.Context) {
	var req blackroomExternalBanRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	if req.UserID <= 0 {
		common.ApiErrorMsg(c, "无效的用户 ID")
		return
	}
	ban, err := service.CreateExternalBlackroomBan(req.UserID, req.IpCount, req.Reason, req.Evidence, req.Permanent, req.DurationHours)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, ban)
}

func ReleaseBlackroomBan(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的小黑屋记录 ID")
		return
	}
	var req blackroomReleaseRequest
	if c.Request.Body != nil {
		_ = common.DecodeJson(c.Request.Body, &req)
	}
	ban, err := model.ReleaseBlackroomBan(id, c.GetInt("id"), req.Reason)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, ban)
}

func RunBlackroomScan(c *gin.Context) {
	summary, err := service.RunBlackroomScanOnce()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, summary)
}

package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/ipgeo"
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
		"blackroom_setting.shadow_mode":                    strconv.FormatBool(req.ShadowMode),
		"blackroom_setting.realtime_enabled":               strconv.FormatBool(req.RealtimeEnabled),
		"blackroom_setting.geo_enabled":                    strconv.FormatBool(req.GeoEnabled),
		"blackroom_setting.geo_country_count":              strconv.Itoa(req.GeoCountryCount),
		"blackroom_setting.geo_asn_count":                  strconv.Itoa(req.GeoASNCount),
		"blackroom_setting.geo_min_gap_seconds":            strconv.Itoa(req.GeoMinGapSeconds),
		"blackroom_setting.geo_duration_hours":             strconv.Itoa(req.GeoDurationHours),
		"blackroom_setting.country_mmdb_path":              req.CountryMMDBPath,
		"blackroom_setting.asn_mmdb_path":                  req.ASNMMDBPath,
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
			"shadow_mode":                    strconv.FormatBool(req.ShadowMode),
			"realtime_enabled":               strconv.FormatBool(req.RealtimeEnabled),
			"geo_enabled":                    strconv.FormatBool(req.GeoEnabled),
			"geo_country_count":              strconv.Itoa(req.GeoCountryCount),
			"geo_asn_count":                  strconv.Itoa(req.GeoASNCount),
			"geo_min_gap_seconds":            strconv.Itoa(req.GeoMinGapSeconds),
			"geo_duration_hours":             strconv.Itoa(req.GeoDurationHours),
			"country_mmdb_path":              req.CountryMMDBPath,
			"asn_mmdb_path":                  req.ASNMMDBPath,
		})
	}
	// MMDB 路径可能已变更，按新配置重新加载解析器；失败会自动降级为
	// 未就绪，地理判定停用但不影响其余功能，因此不作为请求错误返回。
	if err := service.ReloadBlackroomGeoResolver(); err != nil {
		common.SysError("failed to reload blackroom IP geo resolver: " + err.Error())
	}
	common.ApiSuccess(c, operation_setting.GetBlackroomSetting())
}

// GetBlackroomIPAudit 返回按 IP 聚合的观测视图，用于发现「一个 IP 被大量
// 账号共用」这类用户维度看不到的信号。
func GetBlackroomIPAudit(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	startAt, _ := strconv.ParseInt(c.Query("start_at"), 10, 64)
	endAt, _ := strconv.ParseInt(c.Query("end_at"), 10, 64)

	result, err := model.ListBlackroomIPAudit(model.BlackroomIPAuditQuery{
		StartAt:  startAt,
		EndAt:    endAt,
		Keyword:  c.Query("filter"),
		Page:     pageInfo.GetPage(),
		PageSize: pageInfo.GetPageSize(),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

// GetBlackroomBanEvents 返回某个用户的封禁事件时间线。
func GetBlackroomBanEvents(c *gin.Context) {
	userID, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || userID <= 0 {
		common.ApiErrorMsg(c, "无效的用户 ID")
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	events, err := model.ListBlackroomBanEvents(userID, limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, events)
}

// GetBlackroomStatus 汇总判定链路的就绪情况，供管理界面说明「为什么没有
// 自动封禁」。
func GetBlackroomStatus(c *gin.Context) {
	setting := operation_setting.GetBlackroomSetting()
	resolver := ipgeo.DefaultStatus()

	blocking := make([]string, 0, 3)
	if !setting.Enabled {
		blocking = append(blocking, "blackroom_disabled")
	}
	if !setting.AutoBanEnabled {
		blocking = append(blocking, "auto_ban_disabled")
	}
	if setting.GeoEnabled && !resolver.Ready {
		blocking = append(blocking, "geo_resolver_not_ready")
	}

	common.ApiSuccess(c, gin.H{
		"enabled":          setting.Enabled,
		"auto_ban_enabled": setting.AutoBanEnabled,
		"shadow_mode":      setting.ShadowMode,
		"realtime_enabled": setting.RealtimeEnabled,
		"geo_enabled":      setting.GeoEnabled,
		"geo_effective":    setting.GeoEnabled && resolver.Ready,
		"resolver":         resolver,
		"blocking":         blocking,
	})
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

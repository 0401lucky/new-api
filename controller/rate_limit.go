package controller

import (
	"context"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

func GetSelfRateLimitUsage(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		c.Status(http.StatusUnauthorized)
		return
	}
	userGroup := c.GetString("group")
	groups := make([]string, 0)
	for group := range service.GetUserUsableGroups(userGroup) {
		if group == "auto" || ratio_setting.ContainsGroupRatio(group) {
			groups = append(groups, group)
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	usage, err := middleware.GetModelRateLimitUsage(ctx, userID, groups)
	if err != nil {
		common.SysLog("Failed to read rate limit usage: " + err.Error())
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Failed to load rate limit usage"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": usage})
}

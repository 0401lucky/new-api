package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type channelStatusRequest struct {
	Status int `json:"status"`
}

type channelBatchStatusRequest struct {
	Ids    []int `json:"ids"`
	Status int   `json:"status"`
}

func isManualChannelStatus(status int) bool {
	return status == common.ChannelStatusEnabled || status == common.ChannelStatusManuallyDisabled
}

func updateSingleChannelStatus(id int, status int) (bool, string, error) {
	channel, err := model.GetChannelById(id, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, "渠道不存在", nil
	}
	if err != nil {
		return false, "", err
	}
	if channel.Status == status {
		return true, "", nil
	}

	reason := ""
	if status == common.ChannelStatusManuallyDisabled {
		reason = "手动禁用"
	}
	if !model.UpdateChannelStatus(id, "", status, reason) {
		return false, "更新渠道状态失败", nil
	}
	return true, "", nil
}

func GetChannelOps(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"retry_times": common.RetryTimes,
		},
	})
}

func UpdateChannelStatus(c *gin.Context) {
	channelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || channelID <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	req := channelStatusRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || !isManualChannelStatus(req.Status) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	updated, msg, err := updateSingleChannelStatus(channelID, req.Status)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": updated,
		"message": msg,
		"data":    updated,
	})
}

func BatchUpdateChannelStatus(c *gin.Context) {
	req := channelBatchStatusRequest{}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Ids) == 0 || !isManualChannelStatus(req.Status) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
			"data":    0,
		})
		return
	}

	successCount := 0
	attempted := 0
	for _, id := range req.Ids {
		if id <= 0 {
			continue
		}
		attempted++
		updated, _, err := updateSingleChannelStatus(id, req.Status)
		if err != nil {
			common.SysError("failed to update channel status: " + err.Error())
			continue
		}
		if updated {
			successCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": attempted > 0 && successCount == attempted,
		"message": "",
		"data":    successCount,
	})
}

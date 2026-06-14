package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type RecordFingerprintRequest struct {
	VisitorId string `json:"visitor_id" binding:"required"`
}

func RecordFingerprint(c *gin.Context) {
	var req RecordFingerprintRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	req.VisitorId = strings.TrimSpace(req.VisitorId)
	if req.VisitorId == "" || len(req.VisitorId) > 64 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "visitor_id 不合法",
		})
		return
	}

	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "用户未登录",
		})
		return
	}

	if err := model.RecordFingerprint(userId, req.VisitorId, c.GetHeader("User-Agent"), c.ClientIP()); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func GetUserFingerprints(c *gin.Context) {
	fingerprints, err := model.GetUserFingerprints(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, fingerprints)
}

func GetAllFingerprints(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	fingerprints, total, err := model.GetAllFingerprints(pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(fingerprints)
	common.ApiSuccess(c, pageInfo)
}

func SearchFingerprints(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	fingerprints, total, err := model.SearchFingerprints(strings.TrimSpace(c.Query("keyword")), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(fingerprints)
	common.ApiSuccess(c, pageInfo)
}

func FindUsersByVisitorId(c *gin.Context) {
	visitorId := strings.TrimSpace(c.Query("visitor_id"))
	if visitorId == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "visitor_id 不能为空",
		})
		return
	}

	pageInfo := common.GetPageQuery(c)
	users, total, err := model.FindUsersByVisitorId(visitorId, strings.TrimSpace(c.Query("ip")), pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(users)
	common.ApiSuccess(c, pageInfo)
}

func FindUsersByIP(c *gin.Context) {
	ip := strings.TrimSpace(c.Query("ip"))
	if ip == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "ip 不能为空",
		})
		return
	}

	pageInfo := common.GetPageQuery(c)
	users, total, err := model.FindUsersByIP(ip, pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(users)
	common.ApiSuccess(c, pageInfo)
}

func GetDuplicateVisitorIds(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	duplicates, total, err := model.GetDuplicateVisitorIds(pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(duplicates)
	common.ApiSuccess(c, pageInfo)
}

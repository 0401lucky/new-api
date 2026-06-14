package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetActiveTaskRankAPI(c *gin.Context) {
	windowSeconds, _ := strconv.ParseInt(c.Query("window"), 10, 64)
	if windowSeconds <= 0 {
		windowSeconds = model.ActiveWindowSeconds
	}
	if windowSeconds > 3600 {
		windowSeconds = 3600
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	rank := model.GetActiveTaskSlotManager().GetActiveTaskRank(windowSeconds)
	if len(rank) > limit {
		rank = rank[:limit]
	}

	common.ApiSuccess(c, gin.H{
		"rank":           rank,
		"window_seconds": windowSeconds,
	})
}

func GetActiveTaskStatsAPI(c *gin.Context) {
	common.ApiSuccess(c, model.GetActiveTaskSlotManager().GetStats())
}

func GetHighActiveTaskHistoryAPI(c *gin.Context) {
	startTime, _ := strconv.ParseInt(c.Query("start_time"), 10, 64)
	endTime, _ := strconv.ParseInt(c.Query("end_time"), 10, 64)
	userId, _ := strconv.Atoi(c.Query("user_id"))
	limit, _ := strconv.Atoi(c.Query("limit"))

	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	records, err := model.GetHighActiveTaskHistory(startTime, endTime, userId, limit)
	if err != nil {
		common.ApiErrorMsg(c, "获取历史记录失败: "+err.Error())
		return
	}

	common.ApiSuccess(c, gin.H{
		"records": records,
		"total":   len(records),
	})
}

func GetUserTokenUsage24hAPI(c *gin.Context) {
	userId, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "无效的用户ID")
		return
	}

	now := common.GetTimestamp()
	startTimestamp := now - 24*60*60
	results, err := model.GetUserTokenUsageByModel(userId, startTimestamp, now)
	if err != nil {
		common.ApiErrorMsg(c, "获取 token 消耗失败: "+err.Error())
		return
	}

	var totalTokens, totalRequests int64
	for _, result := range results {
		totalTokens += result.TotalTokens
		totalRequests += result.RequestCount
	}

	common.ApiSuccess(c, gin.H{
		"user_id":         userId,
		"start_timestamp": startTimestamp,
		"end_timestamp":   now,
		"models":          results,
		"total_tokens":    totalTokens,
		"total_requests":  totalRequests,
	})
}

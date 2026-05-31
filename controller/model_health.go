package controller

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const (
	publicModelHealthCacheKey = "public_model_health:hourly_last24h:v3"
	publicModelHealthCacheTTL = 30 * time.Second
)

var (
	publicModelHealthMemCache     *publicModelHealthCacheData
	publicModelHealthMemCacheLock sync.RWMutex
)

type publicModelHealthCacheData struct {
	Data     publicModelHealthPayload
	ExpireAt time.Time
}

type modelHealthHourlyRespItem struct {
	ModelName       string  `json:"model_name"`
	HourStartTs     int64   `json:"hour_start_ts"`
	SuccessSlices   int64   `json:"success_slices"`
	TotalSlices     int64   `json:"total_slices"`
	SuccessRate     float64 `json:"success_rate"`
	TotalRequests   int64   `json:"total_requests"`
	ErrorRequests   int64   `json:"error_requests"`
	SuccessRequests int64   `json:"success_requests"`
	SuccessTokens   int64   `json:"success_tokens"`
}

type publicModelsHealthHourlyLast24hRespItem struct {
	ModelName       string  `json:"model_name"`
	HourStartTs     int64   `json:"hour_start_ts"`
	SuccessSlices   int64   `json:"success_slices"`
	TotalSlices     int64   `json:"total_slices"`
	SuccessRate     float64 `json:"success_rate"`
	TotalRequests   int64   `json:"total_requests"`
	ErrorRequests   int64   `json:"error_requests"`
	SuccessRequests int64   `json:"success_requests"`
	SuccessTokens   int64   `json:"success_tokens"`
}

type publicModelHealthPayload struct {
	StartHour int64                                     `json:"start_hour"`
	EndHour   int64                                     `json:"end_hour"`
	Rows      []publicModelsHealthHourlyLast24hRespItem `json:"rows"`
}

func GetModelHealthHourlyStatsAPI(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model_name"))
	if modelName == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "model_name is required"})
		return
	}

	hours, hasHours, err := parseHourListParam(c.Query("hours"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var startHourTs int64
	var endHourTs int64
	if hasHours {
		for _, h := range hours {
			if !isAlignedHour(h) {
				c.JSON(http.StatusOK, gin.H{"success": false, "message": "hours must be aligned to hour (ts % 3600 == 0)"})
				return
			}
		}
		startHourTs = hours[0]
		endHourTs = hours[len(hours)-1] + 3600
	} else {
		startHourTs, _ = strconv.ParseInt(c.Query("start_hour"), 10, 64)
		endHourTs, _ = strconv.ParseInt(c.Query("end_hour"), 10, 64)
		if !isAlignedHour(startHourTs) || !isAlignedHour(endHourTs) || endHourTs <= startHourTs {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid hour range, require start_hour/end_hour aligned to hour and end_hour > start_hour"})
			return
		}
		if endHourTs-startHourTs > 31*24*3600 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "hour range too large (max 31 days)"})
			return
		}
	}

	if err := model.BackfillModelHealthSlicesFromLogs(context.Background(), model.DB, model.LOG_DB, startHourTs, endHourTs); err != nil {
		common.SysLog("model health log backfill failed: " + err.Error())
	}

	rows, err := model.GetModelHealthHourlyStats(model.DB, modelName, startHourTs, endHourTs)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	rowMap := make(map[int64]model.ModelHealthHourlyStat, len(rows))
	for _, r := range rows {
		rowMap[r.HourStartTs] = r
	}

	var wantHours []int64
	if hasHours {
		wantHours = hours
	} else {
		count := int((endHourTs - startHourTs) / 3600)
		wantHours = make([]int64, 0, count)
		for h := startHourTs; h < endHourTs; h += 3600 {
			wantHours = append(wantHours, h)
		}
	}

	resp := make([]modelHealthHourlyRespItem, 0, len(wantHours))
	for _, h := range wantHours {
		if stat, ok := rowMap[h]; ok {
			resp = append(resp, modelHealthHourlyRespItem{
				ModelName:       stat.ModelName,
				HourStartTs:     stat.HourStartTs,
				SuccessSlices:   stat.SuccessSlices,
				TotalSlices:     stat.TotalSlices,
				SuccessRate:     stat.SuccessRate,
				TotalRequests:   stat.TotalRequests,
				ErrorRequests:   stat.ErrorRequests,
				SuccessRequests: stat.SuccessRequests,
				SuccessTokens:   stat.SuccessTokens,
			})
			continue
		}
		resp = append(resp, modelHealthHourlyRespItem{
			ModelName:       modelName,
			HourStartTs:     h,
			SuccessSlices:   0,
			TotalSlices:     0,
			SuccessRate:     0,
			TotalRequests:   0,
			ErrorRequests:   0,
			SuccessRequests: 0,
		})
	}

	common.ApiSuccess(c, resp)
}

func GetPublicModelsHealthHourlyLast24hAPI(c *gin.Context) {
	if cachedData, ok := getPublicModelHealthCache(); ok {
		common.ApiSuccess(c, cachedData)
		return
	}

	now := time.Now().Unix()
	endHourTs := now - (now % 3600) + 3600
	startHourTs := endHourTs - 24*3600

	if err := model.BackfillModelHealthSlicesFromLogs(context.Background(), model.DB, model.LOG_DB, startHourTs, endHourTs); err != nil {
		common.SysLog("model health log backfill failed: " + err.Error())
	}

	rows, err := model.GetAllModelsHealthHourlyStats(model.DB, startHourTs, endHourTs)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	type quotaAggRow struct {
		ModelName       string `gorm:"column:model_name"`
		HourStartTs     int64  `gorm:"column:hour_start_ts"`
		SuccessRequests int64  `gorm:"column:success_requests"`
		SuccessTokens   int64  `gorm:"column:success_tokens"`
	}
	var quotaRows []quotaAggRow
	_ = model.DB.Table("quota_data").
		Select("model_name, created_at as hour_start_ts, SUM(count) as success_requests, SUM(token_used) as success_tokens").
		Where("created_at >= ? AND created_at < ?", startHourTs, endHourTs).
		Group("model_name, created_at").
		Scan(&quotaRows).Error

	quotaMap := make(map[string]map[int64]quotaAggRow, 128)
	for _, r := range quotaRows {
		if r.ModelName == "" {
			continue
		}
		if _, ok := quotaMap[r.ModelName]; !ok {
			quotaMap[r.ModelName] = make(map[int64]quotaAggRow, 32)
		}
		quotaMap[r.ModelName][r.HourStartTs] = r
	}

	wantHours := make([]int64, 0, 24)
	for h := startHourTs; h < endHourTs; h += 3600 {
		wantHours = append(wantHours, h)
	}

	grouped := make(map[string]map[int64]model.ModelHealthHourlyStat)
	modelOrder := make([]string, 0)
	for _, r := range rows {
		if _, ok := grouped[r.ModelName]; !ok {
			grouped[r.ModelName] = make(map[int64]model.ModelHealthHourlyStat)
			modelOrder = append(modelOrder, r.ModelName)
		}
		grouped[r.ModelName][r.HourStartTs] = r
	}

	quotaOnlyModels := make([]string, 0)
	for modelName := range quotaMap {
		if _, ok := grouped[modelName]; ok {
			continue
		}
		grouped[modelName] = make(map[int64]model.ModelHealthHourlyStat)
		quotaOnlyModels = append(quotaOnlyModels, modelName)
	}
	sort.Strings(quotaOnlyModels)
	modelOrder = append(modelOrder, quotaOnlyModels...)

	resp := make([]publicModelsHealthHourlyLast24hRespItem, 0, len(modelOrder)*len(wantHours))
	for _, modelName := range modelOrder {
		hourMap := grouped[modelName]
		modelQuota := quotaMap[modelName]
		for _, h := range wantHours {
			fallbackSuccessTokens := int64(0)
			if modelQuota != nil {
				if q, ok := modelQuota[h]; ok {
					fallbackSuccessTokens = q.SuccessTokens
					if _, hasHealthStat := hourMap[h]; !hasHealthStat && (q.SuccessRequests > 0 || q.SuccessTokens > 0) {
						successRequests := q.SuccessRequests
						if successRequests <= 0 {
							successRequests = 1
						}
						resp = append(resp, publicModelsHealthHourlyLast24hRespItem{
							ModelName:       modelName,
							HourStartTs:     h,
							SuccessSlices:   1,
							TotalSlices:     1,
							SuccessRate:     1,
							TotalRequests:   successRequests,
							ErrorRequests:   0,
							SuccessRequests: successRequests,
							SuccessTokens:   q.SuccessTokens,
						})
						continue
					}
				}
			}

			if stat, ok := hourMap[h]; ok {
				successTokens := stat.SuccessTokens
				if successTokens == 0 {
					successTokens = fallbackSuccessTokens
				}
				resp = append(resp, publicModelsHealthHourlyLast24hRespItem{
					ModelName:       stat.ModelName,
					HourStartTs:     stat.HourStartTs,
					SuccessSlices:   stat.SuccessSlices,
					TotalSlices:     stat.TotalSlices,
					SuccessRate:     stat.SuccessRate,
					TotalRequests:   stat.TotalRequests,
					ErrorRequests:   stat.ErrorRequests,
					SuccessRequests: stat.SuccessRequests,
					SuccessTokens:   successTokens,
				})
				continue
			}
			resp = append(resp, publicModelsHealthHourlyLast24hRespItem{
				ModelName:       modelName,
				HourStartTs:     h,
				SuccessSlices:   0,
				TotalSlices:     0,
				SuccessRate:     0,
				TotalRequests:   0,
				ErrorRequests:   0,
				SuccessRequests: 0,
				SuccessTokens:   fallbackSuccessTokens,
			})
		}
	}

	result := publicModelHealthPayload{
		StartHour: startHourTs,
		EndHour:   endHourTs,
		Rows:      resp,
	}
	setPublicModelHealthCache(result)
	common.ApiSuccess(c, result)
}

func getPublicModelHealthCache() (publicModelHealthPayload, bool) {
	if common.RedisEnabled {
		cached, err := common.RedisGet(publicModelHealthCacheKey)
		if err == nil && cached != "" {
			var data publicModelHealthPayload
			if err := common.UnmarshalJsonStr(cached, &data); err == nil {
				return data, true
			}
		}
	}

	publicModelHealthMemCacheLock.RLock()
	defer publicModelHealthMemCacheLock.RUnlock()

	if publicModelHealthMemCache != nil && time.Now().Before(publicModelHealthMemCache.ExpireAt) {
		return publicModelHealthMemCache.Data, true
	}

	return publicModelHealthPayload{}, false
}

func setPublicModelHealthCache(data publicModelHealthPayload) {
	if common.RedisEnabled {
		jsonData, err := common.Marshal(data)
		if err == nil {
			_ = common.RedisSet(publicModelHealthCacheKey, string(jsonData), publicModelHealthCacheTTL)
		}
	}

	publicModelHealthMemCacheLock.Lock()
	defer publicModelHealthMemCacheLock.Unlock()

	publicModelHealthMemCache = &publicModelHealthCacheData{
		Data:     data,
		ExpireAt: time.Now().Add(publicModelHealthCacheTTL),
	}
}

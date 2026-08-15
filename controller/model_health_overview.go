package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"

	"github.com/gin-gonic/gin"
)

const (
	modelHealthStatusOperational = "operational"
	modelHealthStatusDegraded    = "degraded"
	modelHealthStatusOutage      = "outage"
	modelHealthStatusNoData      = "no_data"

	modelHealthOverviewCacheKeyPrefix = "public_model_health:overview:v1:"
	modelHealthOverviewCacheTTL       = 30 * time.Second
)

var (
	modelHealthOverviewMemCache     = map[string]*modelHealthOverviewCacheEntry{}
	modelHealthOverviewMemCacheLock sync.RWMutex
)

type modelHealthOverviewCacheEntry struct {
	Data     modelHealthOverviewPayload
	ExpireAt time.Time
}

type modelHealthOverviewTimelineItem struct {
	HourStartTs   int64   `json:"hour_start_ts"`
	SuccessRate   float64 `json:"success_rate"`
	TotalRequests int64   `json:"total_requests"`
	ErrorRequests int64   `json:"error_requests"`
	SuccessTokens int64   `json:"success_tokens"`
}

type modelHealthOverviewModel struct {
	ModelName           string                            `json:"model_name"`
	Status              string                            `json:"status"`
	Availability        *float64                          `json:"availability"`
	AvailabilitySuccess int64                             `json:"availability_success"`
	AvailabilityTotal   int64                             `json:"availability_total"`
	AvgLatencyMs        *int64                            `json:"avg_latency_ms"`
	AvgTtftMs           *int64                            `json:"avg_ttft_ms"`
	SuccessTokens24h    int64                             `json:"success_tokens_24h"`
	Timeline            []modelHealthOverviewTimelineItem `json:"timeline"`
}

type modelHealthOverviewStats struct {
	TotalModels    int     `json:"total_models"`
	HealthyModels  int     `json:"healthy_models"`
	OverallRate24h float64 `json:"overall_rate_24h"`
	TotalTokens24h int64   `json:"total_tokens_24h"`
}

type modelHealthOverviewPayload struct {
	UpdatedAt    int64                      `json:"updated_at"`
	Period       string                     `json:"period"`
	GlobalStatus string                     `json:"global_status"`
	Stats        modelHealthOverviewStats   `json:"stats"`
	Models       []modelHealthOverviewModel `json:"models"`
}

type modelHealthLatency struct {
	AvgLatencyMs int64
	AvgTtftMs    int64
}

// modelHealthOverviewInput carries every pre-fetched data source needed to
// assemble the overview payload, keeping the assembly itself a pure function.
type modelHealthOverviewInput struct {
	Period       string
	UpdatedAt    int64
	WantHours    []int64
	HourlyRows   []model.ModelHealthHourlyStat
	PeriodTotals []model.ModelHealthTotals
	RecentTotals []model.ModelHealthTotals
	PerfLatency  map[string]modelHealthLatency
	QuotaRows    []modelHealthQuotaAggRow
}

func parseOverviewPeriodDays(raw string) (int, string, error) {
	switch strings.TrimSpace(raw) {
	case "", "7d":
		return 7, "7d", nil
	case "15d":
		return 15, "15d", nil
	case "30d":
		return 30, "30d", nil
	default:
		return 0, "", fmt.Errorf("invalid period, allowed values: 7d, 15d, 30d")
	}
}

func modelHealthStatusFromCounts(totalRequests int64, qualifiedRequests int64) string {
	if totalRequests <= 0 {
		return modelHealthStatusNoData
	}
	rate := float64(qualifiedRequests) / float64(totalRequests)
	if rate >= 0.95 {
		return modelHealthStatusOperational
	}
	if rate >= 0.8 {
		return modelHealthStatusDegraded
	}
	return modelHealthStatusOutage
}

func globalModelHealthStatus(models []modelHealthOverviewModel) string {
	result := modelHealthStatusOperational
	for _, m := range models {
		if m.Status == modelHealthStatusOutage {
			return modelHealthStatusOutage
		}
		if m.Status == modelHealthStatusDegraded {
			result = modelHealthStatusDegraded
		}
	}
	return result
}

func buildModelHealthOverview(in modelHealthOverviewInput) modelHealthOverviewPayload {
	hourlyByModel := make(map[string]map[int64]model.ModelHealthHourlyStat)
	for _, row := range in.HourlyRows {
		if row.ModelName == "" {
			continue
		}
		if _, ok := hourlyByModel[row.ModelName]; !ok {
			hourlyByModel[row.ModelName] = make(map[int64]model.ModelHealthHourlyStat, len(in.WantHours))
		}
		hourlyByModel[row.ModelName][row.HourStartTs] = row
	}

	quotaByModel := make(map[string]map[int64]modelHealthQuotaAggRow)
	for _, row := range in.QuotaRows {
		if row.ModelName == "" {
			continue
		}
		if _, ok := quotaByModel[row.ModelName]; !ok {
			quotaByModel[row.ModelName] = make(map[int64]modelHealthQuotaAggRow, len(in.WantHours))
		}
		quotaByModel[row.ModelName][row.HourStartTs] = row
	}

	periodByModel := make(map[string]model.ModelHealthTotals, len(in.PeriodTotals))
	for _, row := range in.PeriodTotals {
		periodByModel[row.ModelName] = row
	}
	recentByModel := make(map[string]model.ModelHealthTotals, len(in.RecentTotals))
	for _, row := range in.RecentTotals {
		recentByModel[row.ModelName] = row
	}

	modelNames := make([]string, 0, len(hourlyByModel)+len(quotaByModel))
	seen := make(map[string]struct{}, len(hourlyByModel)+len(quotaByModel))
	for name := range hourlyByModel {
		seen[name] = struct{}{}
		modelNames = append(modelNames, name)
	}
	for name := range quotaByModel {
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			modelNames = append(modelNames, name)
		}
	}

	stats := modelHealthOverviewStats{}
	var overallTotal24h int64
	var overallQualified24h int64

	models := make([]modelHealthOverviewModel, 0, len(modelNames))
	for _, name := range modelNames {
		hourMap := hourlyByModel[name]
		quotaMap := quotaByModel[name]

		var tokens24h int64
		timeline := make([]modelHealthOverviewTimelineItem, 0, len(in.WantHours))
		for _, h := range in.WantHours {
			item := modelHealthOverviewTimelineItem{HourStartTs: h}
			if stat, ok := hourMap[h]; ok {
				item.SuccessRate = stat.SuccessRate
				item.TotalRequests = stat.TotalRequests
				item.ErrorRequests = stat.ErrorRequests
				item.SuccessTokens = stat.SuccessTokens
				overallTotal24h += stat.TotalRequests
				overallQualified24h += stat.QualifiedSuccessRequests
			}
			if item.SuccessTokens == 0 && quotaMap != nil {
				if q, ok := quotaMap[h]; ok {
					item.SuccessTokens = q.SuccessTokens
				}
			}
			tokens24h += item.SuccessTokens
			timeline = append(timeline, item)
		}

		recent := recentByModel[name]
		status := modelHealthStatusFromCounts(recent.TotalRequests, recent.QualifiedSuccessRequests)

		entry := modelHealthOverviewModel{
			ModelName:        name,
			Status:           status,
			SuccessTokens24h: tokens24h,
			Timeline:         timeline,
		}

		if period, ok := periodByModel[name]; ok && period.TotalRequests > 0 {
			availability := float64(period.QualifiedSuccessRequests) / float64(period.TotalRequests)
			entry.Availability = &availability
			entry.AvailabilitySuccess = period.QualifiedSuccessRequests
			entry.AvailabilityTotal = period.TotalRequests
		}

		if latency, ok := in.PerfLatency[name]; ok {
			if latency.AvgLatencyMs > 0 {
				value := latency.AvgLatencyMs
				entry.AvgLatencyMs = &value
			}
			if latency.AvgTtftMs > 0 {
				value := latency.AvgTtftMs
				entry.AvgTtftMs = &value
			}
		}

		if status == modelHealthStatusOperational {
			stats.HealthyModels++
		}
		models = append(models, entry)
	}

	sort.SliceStable(models, func(i, j int) bool {
		if models[i].SuccessTokens24h != models[j].SuccessTokens24h {
			return models[i].SuccessTokens24h > models[j].SuccessTokens24h
		}
		return models[i].ModelName < models[j].ModelName
	})

	stats.TotalModels = len(models)
	if overallTotal24h > 0 {
		stats.OverallRate24h = float64(overallQualified24h) / float64(overallTotal24h)
	}
	for _, m := range models {
		stats.TotalTokens24h += m.SuccessTokens24h
	}

	return modelHealthOverviewPayload{
		UpdatedAt:    in.UpdatedAt,
		Period:       in.Period,
		GlobalStatus: globalModelHealthStatus(models),
		Stats:        stats,
		Models:       models,
	}
}

func GetPublicModelHealthOverviewAPI(c *gin.Context) {
	days, period, err := parseOverviewPeriodDays(c.Query("period"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}

	if cached, ok := getModelHealthOverviewCache(period); ok {
		common.ApiSuccess(c, cached)
		return
	}

	now := time.Now().Unix()
	endHourTs := now - (now % 3600) + 3600
	start24hTs := endHourTs - 24*3600
	periodStartTs := endHourTs - int64(days)*24*3600
	recentStartTs := now - 3600

	wantHours := make([]int64, 0, 24)
	for h := start24hTs; h < endHourTs; h += 3600 {
		wantHours = append(wantHours, h)
	}

	hourlyRows, err := model.GetAllModelsHealthHourlyStats(model.DB, start24hTs, endHourTs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	periodTotals, err := model.GetAllModelsHealthTotals(model.DB, periodStartTs, endHourTs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recentTotals, err := model.GetAllModelsHealthTotals(model.DB, recentStartTs, endHourTs)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	perfLatency := map[string]modelHealthLatency{}
	if summary, perfErr := perfmetrics.QuerySummaryAll(24, nil); perfErr != nil {
		common.SysLog("model health overview perf summary failed: " + perfErr.Error())
	} else {
		for _, m := range summary.Models {
			perfLatency[m.ModelName] = modelHealthLatency{
				AvgLatencyMs: m.AvgLatencyMs,
				AvgTtftMs:    m.AvgTtftMs,
			}
		}
	}

	payload := buildModelHealthOverview(modelHealthOverviewInput{
		Period:       period,
		UpdatedAt:    now,
		WantHours:    wantHours,
		HourlyRows:   hourlyRows,
		PeriodTotals: periodTotals,
		RecentTotals: recentTotals,
		PerfLatency:  perfLatency,
		QuotaRows:    getModelHealthQuotaAggRows(start24hTs, endHourTs, ""),
	})

	setModelHealthOverviewCache(period, payload)
	common.ApiSuccess(c, payload)
}

func getModelHealthOverviewCache(period string) (modelHealthOverviewPayload, bool) {
	if common.RedisEnabled {
		cached, err := common.RedisGet(modelHealthOverviewCacheKeyPrefix + period)
		if err == nil && cached != "" {
			var data modelHealthOverviewPayload
			if err := common.UnmarshalJsonStr(cached, &data); err == nil {
				return data, true
			}
		}
	}

	modelHealthOverviewMemCacheLock.RLock()
	defer modelHealthOverviewMemCacheLock.RUnlock()
	entry, ok := modelHealthOverviewMemCache[period]
	if ok && time.Now().Before(entry.ExpireAt) {
		return entry.Data, true
	}
	return modelHealthOverviewPayload{}, false
}

func setModelHealthOverviewCache(period string, data modelHealthOverviewPayload) {
	if common.RedisEnabled {
		jsonData, err := common.Marshal(data)
		if err == nil {
			_ = common.RedisSet(modelHealthOverviewCacheKeyPrefix+period, string(jsonData), modelHealthOverviewCacheTTL)
		}
	}

	modelHealthOverviewMemCacheLock.Lock()
	defer modelHealthOverviewMemCacheLock.Unlock()
	modelHealthOverviewMemCache[period] = &modelHealthOverviewCacheEntry{
		Data:     data,
		ExpireAt: time.Now().Add(modelHealthOverviewCacheTTL),
	}
}

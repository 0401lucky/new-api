package controller

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelHealthOverviewAPIReportsAvailableHistory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.ModelHealthSlice5m{}, &model.PerfMetric{}))
	previousDB, previousRedis := model.DB, common.RedisEnabled
	model.DB, common.RedisEnabled = db, false
	modelHealthOverviewMemCacheLock.Lock()
	previousCache := modelHealthOverviewMemCache
	modelHealthOverviewMemCache = map[string]*modelHealthOverviewCacheEntry{}
	modelHealthOverviewMemCacheLock.Unlock()
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = previousDB, previousRedis
		modelHealthOverviewMemCacheLock.Lock()
		modelHealthOverviewMemCache = previousCache
		modelHealthOverviewMemCacheLock.Unlock()
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	engine := gin.New()
	engine.GET("/health", GetPublicModelHealthOverviewAPI)
	for _, populated := range []bool{false, true} {
		period := "7d"
		start := model.AlignSliceStartTs(time.Now().Unix())
		if populated {
			period = "15d"
			require.NoError(t, db.Create(&model.ModelHealthSlice5m{SliceStartTs: start, ModelName: "short-replies", TotalRequests: 22, SuccessQualifiedRequests: 19, SuccessTokens: 409007}).Error)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest("GET", "/health?period="+period, nil))
		require.Equal(t, 200, response.Code)
		var body struct {
			Success bool
			Data    modelHealthOverviewPayload
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
		require.True(t, body.Success, response.Body.String())
		if !populated {
			assert.Nil(t, body.Data.ObservedSince)
			assert.Empty(t, body.Data.Models)
			assert.Equal(t, modelHealthStatusNoData, body.Data.GlobalStatus)
			continue
		}
		require.NotNil(t, body.Data.ObservedSince)
		assert.Equal(t, start, *body.Data.ObservedSince)
		require.Len(t, body.Data.Models, 1)
		assert.Equal(t, modelHealthStatusOperational, body.Data.GlobalStatus)
		assert.Equal(t, 1.0, body.Data.Stats.OverallRate24h)
		require.NotNil(t, body.Data.Models[0].Availability)
		assert.Equal(t, 1.0, *body.Data.Models[0].Availability)
		assert.Equal(t, int64(22), body.Data.Models[0].AvailabilitySuccess)
	}
}

func TestParseOverviewPeriodDays(t *testing.T) {
	tests := []struct {
		raw        string
		wantDays   int
		wantPeriod string
		wantErr    bool
	}{
		{raw: "", wantDays: 7, wantPeriod: "7d"},
		{raw: "7d", wantDays: 7, wantPeriod: "7d"},
		{raw: "15d", wantDays: 15, wantPeriod: "15d"},
		{raw: "30d", wantDays: 30, wantPeriod: "30d"},
		{raw: "1d", wantErr: true},
		{raw: "abc", wantErr: true},
	}
	for _, tt := range tests {
		days, period, err := parseOverviewPeriodDays(tt.raw)
		if tt.wantErr {
			require.Error(t, err, "raw=%q", tt.raw)
			continue
		}
		require.NoError(t, err, "raw=%q", tt.raw)
		assert.Equal(t, tt.wantDays, days, "raw=%q", tt.raw)
		assert.Equal(t, tt.wantPeriod, period, "raw=%q", tt.raw)
	}
}

func TestModelHealthStatusFromCounts(t *testing.T) {
	tests := []struct {
		name      string
		total     int64
		qualified int64
		want      string
	}{
		{name: "no data", total: 0, qualified: 0, want: modelHealthStatusNoData},
		{name: "exactly 95 pct", total: 100, qualified: 95, want: modelHealthStatusOperational},
		{name: "just below 95 pct", total: 10000, qualified: 9499, want: modelHealthStatusDegraded},
		{name: "exactly 80 pct", total: 100, qualified: 80, want: modelHealthStatusDegraded},
		{name: "just below 80 pct", total: 10000, qualified: 7999, want: modelHealthStatusOutage},
		{name: "all failed", total: 5, qualified: 0, want: modelHealthStatusOutage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, modelHealthStatusFromCounts(tt.total, tt.qualified))
		})
	}
}

func TestBuildModelHealthOverview(t *testing.T) {
	wantHours := []int64{3600, 7200}
	in := modelHealthOverviewInput{
		Period:    "7d",
		UpdatedAt: 10000,
		WantHours: wantHours,
		HourlyRows: []model.ModelHealthHourlyStat{
			{ModelName: "gpt-a", HourStartTs: 3600, SuccessRate: 1, TotalRequests: 10, ErrorRequests: 0, SuccessRequests: 10, QualifiedSuccessRequests: 7, SuccessTokens: 100},
			{ModelName: "gpt-b", HourStartTs: 7200, SuccessRate: 0.5, TotalRequests: 4, ErrorRequests: 2, SuccessRequests: 2, QualifiedSuccessRequests: 2, SuccessTokens: 40},
		},
		PeriodTotals: []model.ModelHealthTotals{
			{ModelName: "gpt-a", TotalRequests: 100, SuccessRequests: 99, QualifiedSuccessRequests: 50},
			{ModelName: "gpt-b", TotalRequests: 50, SuccessRequests: 20, QualifiedSuccessRequests: 10},
		},
		RecentTotals: []model.ModelHealthTotals{
			{ModelName: "gpt-a", TotalRequests: 10, SuccessRequests: 10, QualifiedSuccessRequests: 7},
			{ModelName: "gpt-b", TotalRequests: 4, SuccessRequests: 2, QualifiedSuccessRequests: 1},
		},
		PerfLatency: map[string]modelHealthLatency{
			"gpt-a": {AvgLatencyMs: 1200, AvgTtftMs: 300},
		},
	}

	payload := buildModelHealthOverview(in)

	assert.Equal(t, "7d", payload.Period)
	assert.Equal(t, int64(10000), payload.UpdatedAt)
	// gpt-b 最近 60 分钟成功率 0.5 → outage，全局取最差
	assert.Equal(t, modelHealthStatusOutage, payload.GlobalStatus)

	require.Len(t, payload.Models, 2)
	// Sort by tokens from successful final requests.
	assert.Equal(t, "gpt-a", payload.Models[0].ModelName)
	assert.Equal(t, "gpt-b", payload.Models[1].ModelName)

	a := payload.Models[0]
	assert.Equal(t, modelHealthStatusOperational, a.Status)
	require.NotNil(t, a.Availability)
	assert.InDelta(t, 0.99, *a.Availability, 1e-9)
	assert.Equal(t, int64(99), a.AvailabilitySuccess)
	assert.Equal(t, int64(100), a.AvailabilityTotal)
	require.NotNil(t, a.AvgLatencyMs)
	assert.Equal(t, int64(1200), *a.AvgLatencyMs)
	require.NotNil(t, a.AvgTtftMs)
	assert.Equal(t, int64(300), *a.AvgTtftMs)
	assert.Equal(t, int64(100), a.SuccessTokens24h)
	// 时间线补零到 wantHours 长度，旧 → 新
	require.Len(t, a.Timeline, 2)
	assert.Equal(t, int64(3600), a.Timeline[0].HourStartTs)
	assert.Equal(t, int64(10), a.Timeline[0].TotalRequests)
	assert.Equal(t, int64(7200), a.Timeline[1].HourStartTs)
	assert.Equal(t, int64(0), a.Timeline[1].TotalRequests)

	b := payload.Models[1]
	assert.Equal(t, modelHealthStatusOutage, b.Status)
	// 无 perf 数据 → 延迟为 null
	assert.Nil(t, b.AvgLatencyMs)
	assert.Nil(t, b.AvgTtftMs)

	assert.Equal(t, 2, payload.Stats.TotalModels)
	assert.Equal(t, 1, payload.Stats.HealthyModels)
	assert.InDelta(t, float64(12)/float64(14), payload.Stats.OverallRate24h, 1e-9)
	assert.Equal(t, int64(140), payload.Stats.TotalTokens24h)
}

func TestGlobalModelHealthStatusPriority(t *testing.T) {
	mk := func(statuses ...string) []modelHealthOverviewModel {
		models := make([]modelHealthOverviewModel, 0, len(statuses))
		for _, s := range statuses {
			models = append(models, modelHealthOverviewModel{Status: s})
		}
		return models
	}
	assert.Equal(t, modelHealthStatusNoData, globalModelHealthStatus(mk()))
	assert.Equal(t, modelHealthStatusNoData, globalModelHealthStatus(mk(modelHealthStatusNoData)))
	assert.Equal(t, modelHealthStatusOperational, globalModelHealthStatus(mk(modelHealthStatusOperational, modelHealthStatusNoData)))
	assert.Equal(t, modelHealthStatusDegraded, globalModelHealthStatus(mk(modelHealthStatusOperational, modelHealthStatusDegraded)))
	assert.Equal(t, modelHealthStatusOutage, globalModelHealthStatus(mk(modelHealthStatusDegraded, modelHealthStatusOutage, modelHealthStatusOperational)))
}

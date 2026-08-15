package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			{ModelName: "gpt-a", HourStartTs: 3600, SuccessRate: 1, TotalRequests: 10, ErrorRequests: 0, QualifiedSuccessRequests: 10, SuccessTokens: 100},
			{ModelName: "gpt-b", HourStartTs: 7200, SuccessRate: 0.5, TotalRequests: 4, ErrorRequests: 2, QualifiedSuccessRequests: 2, SuccessTokens: 40},
		},
		PeriodTotals: []model.ModelHealthTotals{
			{ModelName: "gpt-a", TotalRequests: 100, QualifiedSuccessRequests: 99},
			{ModelName: "gpt-b", TotalRequests: 50, QualifiedSuccessRequests: 20},
		},
		RecentTotals: []model.ModelHealthTotals{
			{ModelName: "gpt-a", TotalRequests: 10, QualifiedSuccessRequests: 10},
			{ModelName: "gpt-b", TotalRequests: 4, QualifiedSuccessRequests: 2},
		},
		PerfLatency: map[string]modelHealthLatency{
			"gpt-a": {AvgLatencyMs: 1200, AvgTtftMs: 300},
		},
		QuotaRows: []modelHealthQuotaAggRow{
			{ModelName: "gpt-c", HourStartTs: 3600, SuccessRequests: 3, SuccessTokens: 30},
		},
	}

	payload := buildModelHealthOverview(in)

	assert.Equal(t, "7d", payload.Period)
	assert.Equal(t, int64(10000), payload.UpdatedAt)
	// gpt-b 最近 60 分钟成功率 0.5 → outage，全局取最差
	assert.Equal(t, modelHealthStatusOutage, payload.GlobalStatus)

	require.Len(t, payload.Models, 3)
	// 24h Token 降序：gpt-a(100) > gpt-b(40) > gpt-c(30)
	assert.Equal(t, "gpt-a", payload.Models[0].ModelName)
	assert.Equal(t, "gpt-b", payload.Models[1].ModelName)
	assert.Equal(t, "gpt-c", payload.Models[2].ModelName)

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

	// gpt-c 仅出现在 quota_data：无健康数据 → no_data + 可用性 null + token 兜底
	cModel := payload.Models[2]
	assert.Equal(t, modelHealthStatusNoData, cModel.Status)
	assert.Nil(t, cModel.Availability)
	assert.Equal(t, int64(30), cModel.SuccessTokens24h)
	assert.Equal(t, int64(30), cModel.Timeline[0].SuccessTokens)

	// stats：gpt-c 无健康请求不计入 overall 分母
	assert.Equal(t, 3, payload.Stats.TotalModels)
	assert.Equal(t, 1, payload.Stats.HealthyModels)
	assert.InDelta(t, float64(12)/float64(14), payload.Stats.OverallRate24h, 1e-9)
	assert.Equal(t, int64(170), payload.Stats.TotalTokens24h)
}

func TestGlobalModelHealthStatusPriority(t *testing.T) {
	mk := func(statuses ...string) []modelHealthOverviewModel {
		models := make([]modelHealthOverviewModel, 0, len(statuses))
		for _, s := range statuses {
			models = append(models, modelHealthOverviewModel{Status: s})
		}
		return models
	}
	assert.Equal(t, modelHealthStatusOperational, globalModelHealthStatus(mk()))
	assert.Equal(t, modelHealthStatusOperational, globalModelHealthStatus(mk(modelHealthStatusOperational, modelHealthStatusNoData)))
	assert.Equal(t, modelHealthStatusDegraded, globalModelHealthStatus(mk(modelHealthStatusOperational, modelHealthStatusDegraded)))
	assert.Equal(t, modelHealthStatusOutage, globalModelHealthStatus(mk(modelHealthStatusDegraded, modelHealthStatusOutage, modelHealthStatusOperational)))
}

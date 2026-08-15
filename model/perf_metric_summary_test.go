package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetPerfMetricsSummaryBucketsAllAggregatesTtft(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PerfMetric{}))

	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	rows := []PerfMetric{
		{ModelName: "gpt-test", Group: "default", BucketTs: 3600, RequestCount: 2, SuccessCount: 2, TotalLatencyMs: 3000, TtftSumMs: 500, TtftCount: 1, OutputTokens: 100, GenerationMs: 2000},
		{ModelName: "gpt-test", Group: "vip", BucketTs: 3600, RequestCount: 1, SuccessCount: 1, TotalLatencyMs: 1000, TtftSumMs: 300, TtftCount: 1, OutputTokens: 50, GenerationMs: 800},
		{ModelName: "gpt-test", Group: "default", BucketTs: 7200, RequestCount: 1, SuccessCount: 0, TotalLatencyMs: 2000, TtftSumMs: 0, TtftCount: 0, OutputTokens: 0, GenerationMs: 0},
	}
	require.NoError(t, db.Create(&rows).Error)

	summaries, err := GetPerfMetricsSummaryBucketsAll(0, 10000, nil)
	require.NoError(t, err)
	require.Len(t, summaries, 2)

	byBucket := map[int64]PerfMetricSummaryBucket{}
	for _, s := range summaries {
		byBucket[s.BucketTs] = s
	}
	first := byBucket[3600]
	assert.Equal(t, int64(3), first.RequestCount)
	assert.Equal(t, int64(800), first.TtftSumMs)
	assert.Equal(t, int64(2), first.TtftCount)
	second := byBucket[7200]
	assert.Equal(t, int64(0), second.TtftSumMs)
	assert.Equal(t, int64(0), second.TtftCount)
}

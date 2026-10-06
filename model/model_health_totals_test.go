package model

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newModelHealthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&ModelHealthSlice5m{}))
	return db
}

func TestGetAllModelsHealthTotals(t *testing.T) {
	db := newModelHealthTestDB(t)

	events := []*ModelHealthEvent{
		{ModelName: "gpt-a", CreatedAt: 3601, ResponseBytes: 2048, CompletionTokens: 8, SuccessTokens: 10},
		{ModelName: "gpt-a", CreatedAt: 3910, IsError: true},
		{ModelName: "gpt-a", CreatedAt: 90000, ResponseBytes: 2048, CompletionTokens: 8, SuccessTokens: 5},
		{ModelName: "gpt-b", CreatedAt: 3620, ResponseBytes: 10, CompletionTokens: 1, AssistantChars: 1},
	}
	for _, e := range events {
		require.NoError(t, UpsertModelHealthSlice5m(context.Background(), db, e))
	}

	rows, err := GetAllModelsHealthTotals(db, 3600, 7200)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	byModel := map[string]ModelHealthTotals{}
	for _, r := range rows {
		byModel[r.ModelName] = r
	}
	assert.Equal(t, int64(2), byModel["gpt-a"].TotalRequests)
	assert.Equal(t, int64(1), byModel["gpt-a"].QualifiedSuccessRequests)
	assert.Equal(t, int64(1), byModel["gpt-b"].TotalRequests)
	assert.Equal(t, int64(0), byModel["gpt-b"].QualifiedSuccessRequests)

	_, err = GetAllModelsHealthTotals(db, 7200, 3600)
	require.Error(t, err)
	_, err = GetAllModelsHealthTotals(nil, 3600, 7200)
	require.Error(t, err)
}

func TestModelHealthSuccessRateIncludesShortResponses(t *testing.T) {
	db := newModelHealthTestDB(t)
	for _, event := range []*ModelHealthEvent{
		{ModelName: "short-answer", CreatedAt: 3601, CompletionTokens: 1, SuccessTokens: 11},
		{ModelName: "short-answer", CreatedAt: 3902, CompletionTokens: 20, SuccessTokens: 30},
	} {
		require.NoError(t, UpsertModelHealthSlice5m(context.Background(), db, event))
	}
	rows, err := GetAllModelsHealthHourlyStats(db, 3600, 7200)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(2), rows[0].SuccessRequests)
	assert.Equal(t, int64(0), rows[0].ErrorRequests)
	assert.Equal(t, int64(1), rows[0].QualifiedSuccessRequests)
	assert.Equal(t, int64(2), rows[0].SuccessSlices)
	assert.Equal(t, int64(2), rows[0].TotalSlices)
	assert.Equal(t, 1.0, rows[0].SuccessRate, "a valid short answer is a successful request")
}

func TestModelHealthFlushCancellationPreservesAcceptedEvent(t *testing.T) {
	db := newModelHealthTestDB(t)
	previousDB := DB
	DB = db
	entered, release := make(chan struct{}), make(chan struct{})
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("health_test_block_write", func(*gorm.DB) {
		close(entered)
		<-release
	}))
	t.Cleanup(func() {
		require.NoError(t, FlushModelHealthEvents(context.Background()))
		DB = previousDB
	})
	defer close(release)
	event := &ModelHealthEvent{ModelName: "accepted", CreatedAt: 3601, CompletionTokens: 1, SuccessTokens: 11}
	RecordModelHealthEventAsync(event)
	event.ModelName = "reused"
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("queued write did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- FlushModelHealthEvents(ctx) }()
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown wait ignored cancellation")
	}
	// Verify after the deferred release, before the fixture restores DB.
	t.Cleanup(func() {
		require.NoError(t, FlushModelHealthEvents(context.Background()))
		rows, err := GetAllModelsHealthHourlyStats(db, 3600, 7200)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, "accepted", rows[0].ModelName)
		assert.Equal(t, int64(11), rows[0].SuccessTokens)
	})
}

func TestDeleteModelHealthSlicesBefore(t *testing.T) {
	db := newModelHealthTestDB(t)

	slices := []ModelHealthSlice5m{
		{SliceStartTs: 300, ModelName: "gpt-a", TotalRequests: 1},
		{SliceStartTs: 600, ModelName: "gpt-a", TotalRequests: 1},
		{SliceStartTs: 900, ModelName: "gpt-a", TotalRequests: 1},
	}
	require.NoError(t, db.Create(&slices).Error)

	deleted, err := DeleteModelHealthSlicesBefore(db, 600)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	var remaining []ModelHealthSlice5m
	require.NoError(t, db.Order("slice_start_ts ASC").Find(&remaining).Error)
	require.Len(t, remaining, 2)
	assert.Equal(t, int64(600), remaining[0].SliceStartTs)

	deleted, err = DeleteModelHealthSlicesBefore(db, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(0), deleted)

	_, err = DeleteModelHealthSlicesBefore(nil, 600)
	require.Error(t, err)
}

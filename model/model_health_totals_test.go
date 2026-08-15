package model

import (
	"context"
	"testing"

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

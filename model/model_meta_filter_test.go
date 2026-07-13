package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchModelsCombinesStatusSyncAndPagination(t *testing.T) {
	truncateTables(t)
	models := []Model{
		{ModelName: "filter-enabled-sync-a", Status: 1, SyncOfficial: 1},
		{ModelName: "filter-enabled-sync-b", Status: 1, SyncOfficial: 1},
		{ModelName: "filter-enabled-manual", Status: 1, SyncOfficial: 0},
		{ModelName: "filter-disabled-sync", Status: 0, SyncOfficial: 1},
		{ModelName: "filter-disabled-manual", Status: 0, SyncOfficial: 0},
	}
	for i := range models {
		require.NoError(t, models[i].Insert())
	}

	firstPage, total, err := SearchModels("filter", "", "enabled", "yes", 0, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, firstPage, 1)

	secondPage, total, err := SearchModels("filter", "", "1", "1", 1, 1)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, secondPage, 1)
	require.NotEqual(t, firstPage[0].Id, secondPage[0].Id)
}

func TestSearchModelsDisabledAndNoSyncIncludeZeroValues(t *testing.T) {
	truncateTables(t)
	models := []Model{
		{ModelName: "zero-disabled-manual", Status: 0, SyncOfficial: 0},
		{ModelName: "zero-disabled-sync", Status: 0, SyncOfficial: 1},
		{ModelName: "zero-enabled-manual", Status: 1, SyncOfficial: 0},
	}
	for i := range models {
		require.NoError(t, models[i].Insert())
	}

	matched, total, err := SearchModels("zero", "", "disabled", "no", 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, matched, 1)
	require.Equal(t, "zero-disabled-manual", matched[0].ModelName)
}

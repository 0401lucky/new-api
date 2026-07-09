package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetAllChannelModelsIncludesDisabled(t *testing.T) {
	truncateTables(t)

	abilities := []Ability{
		{Group: "default", Model: "gpt-5.5", ChannelId: 1, Enabled: true},
		{Group: "default", Model: "glm-4.7", ChannelId: 2, Enabled: false},
		{Group: "vip", Model: "gpt-5.5", ChannelId: 3, Enabled: true},
	}
	for i := range abilities {
		require.NoError(t, DB.Create(&abilities[i]).Error)
	}

	all := GetAllChannelModels()
	require.ElementsMatch(t, []string{"gpt-5.5", "glm-4.7"}, all)

	enabled := GetEnabledModels()
	require.ElementsMatch(t, []string{"gpt-5.5"}, enabled)
}

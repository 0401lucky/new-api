package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLogUserFilterKeepsNumericUsername(t *testing.T) {
	username, userId := parseLogUserFilter("3", 0)

	require.Equal(t, "3", username)
	require.Equal(t, 0, userId)
}

func TestParseLogUserFilterParsesExplicitUserID(t *testing.T) {
	cases := []string{"#3", "# 3", "id:3", "ID:3", "uid：3"}

	for _, value := range cases {
		username, userId := parseLogUserFilter(value, 0)
		require.Empty(t, username)
		require.Equal(t, 3, userId)
	}
}

func TestParseLogUserFilterPrefersUserIDQueryParam(t *testing.T) {
	username, userId := parseLogUserFilter("3", 42)

	require.Empty(t, username)
	require.Equal(t, 42, userId)
}

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProxyURLStrict(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		want      string
		wantError string
	}{
		{name: "empty"},
		{name: "http", raw: " HTTP://proxy.example:8080/ ", want: "http://proxy.example:8080"},
		{name: "socks default port", raw: "socks5://user:pass@proxy.example", want: "socks5://user:pass@proxy.example:1080"},
		{name: "socks ipv6", raw: "socks5h://[2001:db8::1]", want: "socks5h://[2001:db8::1]:1080"},
		{name: "unsupported", raw: "ftp://proxy.example", wantError: "must use"},
		{name: "missing host", raw: "http:///path", wantError: "include a host"},
		{name: "invalid port", raw: "http://proxy.example:0", wantError: "valid port"},
		{name: "path", raw: "socks5://proxy.example/path", wantError: "must not include a path"},
		{name: "query", raw: "http://proxy.example?x=1", wantError: "must not include a query"},
		{name: "fragment", raw: "http://proxy.example#x", wantError: "must not include a fragment"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsedURL, err := ParseProxyURLStrict(test.raw)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			if test.want == "" {
				assert.Nil(t, parsedURL)
				return
			}
			assert.Equal(t, test.want, parsedURL.String())
		})
	}
}

func TestParseProxyURLRuntimeStripsLegacySuffix(t *testing.T) {
	parsedURL, stripped, err := ParseProxyURLRuntime("socks5://proxy.example/legacy/path?x=1#fragment")

	require.NoError(t, err)
	assert.True(t, stripped)
	assert.Equal(t, "socks5://proxy.example:1080", parsedURL.String())
}

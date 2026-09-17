package ipgeo

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeClassifiesAddresses(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ip   string
		kind string
	}{
		{"公网 IPv4", "203.0.113.9", "203.0.113.9", KindReserved},
		{"公网 IPv4 常规", "8.8.8.8", "8.8.8.8", KindPublic},
		{"私有地址", "10.1.2.3", "10.1.2.3", KindPrivate},
		{"回环地址", "127.0.0.1", "127.0.0.1", KindLoopback},
		{"链路本地", "169.254.10.1", "169.254.10.1", KindLinkLocal},
		{"运营商级 NAT", "100.64.1.1", "100.64.1.1", KindReserved},
		{"保留段", "240.0.0.1", "240.0.0.1", KindReserved},
		{"组播", "224.0.0.1", "224.0.0.1", KindMulticast},
		{"未指定", "0.0.0.0", "0.0.0.0", KindUnspecified},
		{"IPv6 公网", "2606:4700:4700::1111", "2606:4700:4700::1111", KindPublic},
		{"IPv6 唯一本地", "fd00::1", "fd00::1", KindPrivate},
		// IPv4-mapped IPv6 必须还原成 IPv4，否则同一地址会以两种文本形式
		// 各存一行，让 IP 计数虚高。
		{"IPv4-mapped", "::ffff:8.8.8.8", "8.8.8.8", KindPublic},
		{"非法输入", "not-an-ip", "unknown", KindMalformed},
		{"空字符串", "", "unknown", KindMalformed},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			ip, kind := Normalize(testCase.raw)
			assert.Equal(t, testCase.ip, ip)
			assert.Equal(t, testCase.kind, kind)
		})
	}
}

func TestInitializeDefaultWithEmptyPathsIsNotAnError(t *testing.T) {
	previous := setDefaultResolver(nil, Status{})
	t.Cleanup(func() { setDefaultResolver(previous, Status{}) })

	// 未配置 MMDB 是受支持的部署形态，必须静默降级而不是让启动失败。
	require.NoError(t, InitializeDefault("", "", 0))

	status := DefaultStatus()
	assert.False(t, status.Ready)
	assert.Equal(t, "not_configured", status.ErrorCode)
}

func TestInitializeDefaultWithMissingFileReportsOpenFailure(t *testing.T) {
	previous := setDefaultResolver(nil, Status{})
	t.Cleanup(func() { setDefaultResolver(previous, Status{}) })

	missing := filepath.Join(t.TempDir(), "missing.mmdb")
	err := InitializeDefault(missing, missing, 0)
	require.Error(t, err)

	status := DefaultStatus()
	assert.False(t, status.Ready)
	assert.Equal(t, "open_failed", status.ErrorCode)
}

func TestInitializeDefaultWithOnlyOnePathFails(t *testing.T) {
	previous := setDefaultResolver(nil, Status{})
	t.Cleanup(func() { setDefaultResolver(previous, Status{}) })

	// 只配一半属于配置错误，要明确报错而不是静默当作未配置。
	require.Error(t, InitializeDefault("/tmp/country.mmdb", "", 0))
	assert.False(t, DefaultStatus().Ready)
}

func TestLookupWithoutResolverStillNormalizes(t *testing.T) {
	previous := setDefaultResolver(nil, Status{ErrorCode: "not_configured"})
	t.Cleanup(func() { setDefaultResolver(previous, Status{}) })

	result := LookupDefault("::ffff:8.8.8.8")
	assert.Equal(t, "8.8.8.8", result.Ip)
	assert.Equal(t, KindPublic, result.Kind)
	// 没有解析器时绝不能把观测当作地理证据。
	assert.False(t, result.EvidenceEligible)
	assert.Empty(t, result.CountryISO)
	assert.Zero(t, result.AsnNumber)
}

func TestLookupPrivateAddressSkipsGeoLookup(t *testing.T) {
	previous := setDefaultResolver(nil, Status{})
	t.Cleanup(func() { setDefaultResolver(previous, Status{}) })

	// 私有地址即使解析器可用也不查库，也不构成证据。
	result := LookupDefault("192.168.1.10")
	assert.Equal(t, "192.168.1.10", result.Ip)
	assert.Equal(t, KindPrivate, result.Kind)
	assert.False(t, result.EvidenceEligible)
}

func TestResolverStatusReportsNotInitialized(t *testing.T) {
	var resolver *Resolver
	status := resolver.Status()
	assert.False(t, status.Ready)
	assert.Equal(t, "not_initialized", status.ErrorCode)
}

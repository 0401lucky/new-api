// Package ipgeo 提供 IP 规范化与可选的 MaxMind MMDB 国家/ASN 解析。
//
// 地理信息是风控判定的可降级增强项：未配置 MMDB、文件缺失或解析失败时，
// 解析器保持「未就绪」状态并如实返回，调用方应继续记录观测，只是不把
// 这些观测当作地理判定证据。任何情况下都不应因为 MMDB 不可用而让请求失败。
package ipgeo

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"

	"github.com/oschwald/maxminddb-golang"
)

// IP 类别。只有 KindPublic 且解析完整的观测才能作为风控证据。
const (
	KindPublic      = "public"
	KindPrivate     = "private"
	KindLoopback    = "loopback"
	KindLinkLocal   = "link_local"
	KindMulticast   = "multicast"
	KindUnspecified = "unspecified"
	KindReserved    = "reserved"
	KindMalformed   = "malformed"
)

// reservedPrefixes 是 IsGlobalUnicast 仍会判为真、但不适合作为风控证据的
// 特殊网段（运营商级 NAT、文档示例段、基准测试段等）。
var reservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// Result 是一次解析的完整结果。
type Result struct {
	Ip               string `json:"ip"`
	Kind             string `json:"kind"`
	CountryISO       string `json:"country_iso"`
	AsnNumber        int64  `json:"asn_number"`
	AsnOrganization  string `json:"asn_organization"`
	ResolverVersion  string `json:"resolver_version"`
	EvidenceEligible bool   `json:"evidence_eligible"`
}

// Status 描述解析器的就绪情况，供管理界面展示降级原因。
type Status struct {
	Ready        bool   `json:"ready"`
	CountryReady bool   `json:"country_ready"`
	AsnReady     bool   `json:"asn_ready"`
	Version      string `json:"version"`
	ErrorCode    string `json:"error_code,omitempty"`
}

type countryRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

type asnRecord struct {
	AutonomousSystemNumber       uint32 `maxminddb:"autonomous_system_number"`
	AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
	Traits                       struct {
		AutonomousSystemNumber       uint32 `maxminddb:"autonomous_system_number"`
		AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
	} `maxminddb:"traits"`
}

// Resolver 并发安全地查询两个 MMDB。缓存满了之后不再写入新条目，避免为
// 淘汰策略引入额外的复杂度；缓存容量默认足够覆盖一个部署的活跃 IP 规模。
type Resolver struct {
	country *maxminddb.Reader
	asn     *maxminddb.Reader
	status  Status

	cacheMu   sync.RWMutex
	cache     map[string]Result
	cacheSize int
}

// Open 打开两个 MMDB 文件。任一路径为空或打不开都返回错误，由调用方决定
// 是否降级；已打开的句柄会在失败时关闭。
func Open(countryPath string, asnPath string, cacheSize int) (*Resolver, error) {
	countryPath = strings.TrimSpace(countryPath)
	asnPath = strings.TrimSpace(asnPath)
	if countryPath == "" || asnPath == "" {
		return nil, errors.New("国家与 ASN 的 MMDB 路径都必须配置")
	}

	country, err := maxminddb.Open(countryPath)
	if err != nil {
		return nil, fmt.Errorf("国家 MMDB 无法打开: %w", err)
	}
	asn, err := maxminddb.Open(asnPath)
	if err != nil {
		_ = country.Close()
		return nil, fmt.Errorf("ASN MMDB 无法打开: %w", err)
	}
	if cacheSize <= 0 {
		cacheSize = 100_000
	}

	return &Resolver{
		country: country,
		asn:     asn,
		status: Status{
			Ready:        true,
			CountryReady: true,
			AsnReady:     true,
			Version:      fmt.Sprintf("country-%d/asn-%d", country.Metadata.BuildEpoch, asn.Metadata.BuildEpoch),
		},
		cache:     make(map[string]Result),
		cacheSize: cacheSize,
	}, nil
}

func (r *Resolver) Close() error {
	if r == nil {
		return nil
	}
	var countryErr, asnErr error
	if r.country != nil {
		countryErr = r.country.Close()
	}
	if r.asn != nil {
		asnErr = r.asn.Close()
	}
	return errors.Join(countryErr, asnErr)
}

func (r *Resolver) Status() Status {
	if r == nil {
		return Status{ErrorCode: "not_initialized"}
	}
	return r.status
}

// Lookup 规范化 IP 并（在解析器就绪且地址为公网时）补充国家与 ASN。
// 解析器未就绪或地址非公网时只返回规范化结果，不视为错误。
func (r *Resolver) Lookup(rawIP string) Result {
	address, kind := Normalize(rawIP)
	result := Result{Ip: address, Kind: kind}
	if r == nil {
		return result
	}
	result.ResolverVersion = r.status.Version
	if kind != KindPublic || !r.status.Ready {
		return result
	}

	r.cacheMu.RLock()
	cached, ok := r.cache[address]
	r.cacheMu.RUnlock()
	if ok {
		return cached
	}

	parsed, err := netip.ParseAddr(address)
	if err != nil {
		result.Kind = KindMalformed
		return result
	}
	ip := net.IP(parsed.AsSlice())

	var country countryRecord
	if err := r.country.Lookup(ip, &country); err != nil {
		return result
	}
	var asn asnRecord
	if err := r.asn.Lookup(ip, &asn); err != nil {
		return result
	}
	// 部分 ASN 库把字段放在 traits 下，两种布局都兼容。
	if asn.AutonomousSystemNumber == 0 {
		asn.AutonomousSystemNumber = asn.Traits.AutonomousSystemNumber
		asn.AutonomousSystemOrganization = asn.Traits.AutonomousSystemOrganization
	}

	result.CountryISO = strings.ToUpper(strings.TrimSpace(country.Country.ISOCode))
	result.AsnNumber = int64(asn.AutonomousSystemNumber)
	result.AsnOrganization = strings.TrimSpace(asn.AutonomousSystemOrganization)
	// 只有国家与 ASN 都拿到时才算完整证据，缺一不可。
	result.EvidenceEligible = result.CountryISO != "" && result.AsnNumber > 0

	r.cacheMu.Lock()
	if len(r.cache) < r.cacheSize {
		r.cache[address] = result
	}
	r.cacheMu.Unlock()
	return result
}

// Normalize 返回规范化后的地址文本与类别。IPv4-mapped IPv6 会还原成 IPv4，
// 避免同一地址以两种文本形式各存一行。
func Normalize(rawIP string) (string, string) {
	address, err := netip.ParseAddr(strings.TrimSpace(rawIP))
	if err != nil {
		return "unknown", KindMalformed
	}
	address = address.Unmap()
	normalized := address.String()

	switch {
	case address.IsUnspecified():
		return normalized, KindUnspecified
	case address.IsLoopback():
		return normalized, KindLoopback
	case address.IsLinkLocalUnicast():
		return normalized, KindLinkLocal
	case address.IsMulticast():
		return normalized, KindMulticast
	case address.IsPrivate():
		return normalized, KindPrivate
	case isReserved(address) || !address.IsGlobalUnicast():
		return normalized, KindReserved
	default:
		return normalized, KindPublic
	}
}

func isReserved(address netip.Addr) bool {
	for _, prefix := range reservedPrefixes {
		if prefix.Addr().BitLen() == address.BitLen() && prefix.Contains(address) {
			return true
		}
	}
	return false
}

var defaultResolver struct {
	sync.RWMutex
	resolver *Resolver
	status   Status
}

// InitializeDefault 加载全局解析器。两个路径都为空表示未配置：保持未就绪
// 状态但不返回错误，调用方据此跳过地理判定即可。
func InitializeDefault(countryPath string, asnPath string, cacheSize int) error {
	countryPath = strings.TrimSpace(countryPath)
	asnPath = strings.TrimSpace(asnPath)

	if countryPath == "" && asnPath == "" {
		setDefaultResolver(nil, Status{ErrorCode: "not_configured"})
		return nil
	}
	resolver, err := Open(countryPath, asnPath, cacheSize)
	if err != nil {
		setDefaultResolver(nil, Status{ErrorCode: "open_failed"})
		return err
	}
	previous := setDefaultResolver(resolver, resolver.Status())
	if previous != nil {
		_ = previous.Close()
	}
	return nil
}

func setDefaultResolver(resolver *Resolver, status Status) *Resolver {
	defaultResolver.Lock()
	defer defaultResolver.Unlock()
	previous := defaultResolver.resolver
	defaultResolver.resolver = resolver
	defaultResolver.status = status
	return previous
}

func LookupDefault(rawIP string) Result {
	defaultResolver.RLock()
	resolver := defaultResolver.resolver
	defaultResolver.RUnlock()
	if resolver == nil {
		address, kind := Normalize(rawIP)
		return Result{Ip: address, Kind: kind}
	}
	return resolver.Lookup(rawIP)
}

func DefaultStatus() Status {
	defaultResolver.RLock()
	defer defaultResolver.RUnlock()
	if defaultResolver.resolver == nil {
		status := defaultResolver.status
		if status.ErrorCode == "" {
			status.ErrorCode = "not_initialized"
		}
		return status
	}
	return defaultResolver.status
}

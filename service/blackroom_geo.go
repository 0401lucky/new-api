package service

import (
	"os"
	"strings"

	"github.com/QuantumNous/new-api/pkg/ipgeo"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const blackroomGeoCacheEntries = 100_000

// BlackroomGeoResolverPaths 返回 MMDB 路径与缓存容量。环境变量优先于管理
// 界面配置，便于容器化部署通过挂载点注入，而不必把宿主机路径写进数据库。
func BlackroomGeoResolverPaths() (countryPath string, asnPath string, cacheSize int) {
	setting := operation_setting.GetBlackroomSetting()

	countryPath = strings.TrimSpace(os.Getenv("BLACKROOM_COUNTRY_MMDB"))
	if countryPath == "" {
		countryPath = strings.TrimSpace(setting.CountryMMDBPath)
	}
	asnPath = strings.TrimSpace(os.Getenv("BLACKROOM_ASN_MMDB"))
	if asnPath == "" {
		asnPath = strings.TrimSpace(setting.ASNMMDBPath)
	}
	return countryPath, asnPath, blackroomGeoCacheEntries
}

// ReloadBlackroomGeoResolver 在配置变更后重新加载 MMDB。加载失败时解析器
// 退回未就绪状态，地理判定自动降级，不影响基于 IP 数的既有判定。
func ReloadBlackroomGeoResolver() error {
	countryPath, asnPath, cacheSize := BlackroomGeoResolverPaths()
	return ipgeo.InitializeDefault(countryPath, asnPath, cacheSize)
}

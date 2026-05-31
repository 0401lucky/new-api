package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// CheckinSetting 签到功能配置
type CheckinSetting struct {
	Enabled    bool `json:"enabled"`     // 是否启用签到功能
	MinQuota   int  `json:"min_quota"`   // 随机模式最小额度
	MaxQuota   int  `json:"max_quota"`   // 随机模式最大额度
	FixedQuota int  `json:"fixed_quota"` // 固定模式额度
	RandomMode bool `json:"random_mode"` // 是否启用随机额度
}

// 默认配置
var checkinSetting = CheckinSetting{
	Enabled:    false, // 默认关闭
	MinQuota:   1000,  // 默认最小额度 1000 (约 0.002 USD)
	MaxQuota:   10000, // 默认最大额度 10000 (约 0.02 USD)
	FixedQuota: 1000,  // 默认固定额度
	RandomMode: true,  // 保持原有随机额度行为
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("checkin_setting", &checkinSetting)
}

// GetCheckinSetting 获取签到配置
func GetCheckinSetting() *CheckinSetting {
	return &checkinSetting
}

// IsCheckinEnabled 是否启用签到功能
func IsCheckinEnabled() bool {
	return checkinSetting.Enabled
}

// GetCheckinQuotaRange 获取签到额度范围
func GetCheckinQuotaRange() (min, max int) {
	return checkinSetting.MinQuota, checkinSetting.MaxQuota
}

package operation_setting

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// 签到奖励类型常量
const (
	// RewardTypePermanent 永久额度（维持现有行为）
	RewardTypePermanent = "permanent"
	// RewardTypeTemporary 当日限时额度
	RewardTypeTemporary = "temporary"
)

// CheckinSetting 签到功能配置
type CheckinSetting struct {
	Enabled    bool `json:"enabled"`     // 是否启用签到功能
	MinQuota   int  `json:"min_quota"`   // 随机模式最小额度
	MaxQuota   int  `json:"max_quota"`   // 随机模式最大额度
	FixedQuota int  `json:"fixed_quota"` // 固定模式额度
	RandomMode bool `json:"random_mode"` // 是否启用随机额度

	// RewardType 奖励类型: "permanent" 或 "temporary"，默认 "permanent"
	RewardType string `json:"reward_type"`
	// AvailableFromMinutes 每天第几分钟开放签到，范围 0-1439，默认 0（当天 00:00 开放）
	AvailableFromMinutes int `json:"available_from_minutes"`
}

// 默认配置
var checkinSetting = CheckinSetting{
	Enabled:               false,     // 默认关闭
	MinQuota:              1000,      // 默认最小额度 1000 (约 0.002 USD)
	MaxQuota:              10000,     // 默认最大额度 10000 (约 0.02 USD)
	FixedQuota:            1000,      // 默认固定额度
	RandomMode:            true,      // 保持原有随机额度行为
	RewardType:            RewardTypePermanent,
	AvailableFromMinutes:  0,
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

// IsTemporaryReward 当前配置是否发放限时额度奖励
func (s *CheckinSetting) IsTemporaryReward() bool {
	return s.RewardType == RewardTypeTemporary
}

// IsValidRewardType 判断奖励类型是否合法
func IsValidRewardType(rewardType string) bool {
	return rewardType == RewardTypePermanent || rewardType == RewardTypeTemporary
}

// IsValidAvailableFromMinutes 判断开放时间是否合法（0-1439）
func IsValidAvailableFromMinutes(minutes int) bool {
	return minutes >= 0 && minutes <= 1439
}

// ValidateCheckinConfig 整组校验签到配置，避免保存无效中间状态。
// 返回描述性错误信息，nil 表示通过。
func (s *CheckinSetting) ValidateCheckinConfig() error {
	if !IsValidRewardType(s.RewardType) {
		return &ConfigError{Message: "签到奖励类型无效"}
	}
	if !IsValidAvailableFromMinutes(s.AvailableFromMinutes) {
		return &ConfigError{Message: "签到开放时间无效，必须在 00:00-23:59 之间"}
	}
	if s.MinQuota < 0 || s.MaxQuota < 0 || s.FixedQuota < 0 {
		return &ConfigError{Message: "签到额度必须是非负整数"}
	}
	if s.MinQuota > common.MaxQuota || s.MaxQuota > common.MaxQuota || s.FixedQuota > common.MaxQuota {
		return &ConfigError{Message: "签到额度不能超过上限"}
	}
	if s.Enabled {
		// 启用签到时，实际奖励必须大于零
		if !s.RandomMode && s.FixedQuota <= 0 {
			return &ConfigError{Message: "启用签到时固定奖励必须大于零"}
		}
		if s.RandomMode {
			if s.MinQuota <= 0 || s.MaxQuota <= 0 {
				return &ConfigError{Message: "启用签到时随机奖励必须大于零"}
		}
			if s.MaxQuota < s.MinQuota {
				return &ConfigError{Message: "随机模式最大值必须大于或等于最小值"}
			}
		}
	}
	return nil
}

// ConfigError 配置校验错误
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string {
	return e.Message
}

// ReplaceCheckinSetting 原子替换内存中的签到配置（单指针赋值）。
// 调用方须先持久化数据库；此方法避免逐项更新内存导致的混合中间态。
func ReplaceCheckinSetting(setting CheckinSetting) {
	checkinSetting = setting
}

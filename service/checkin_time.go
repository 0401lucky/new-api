package service

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// 签到状态常量（机器可识别，前端据此控制按钮，不依赖错误字符串）
const (
	CheckinStateNotOpen   = "not_open"   // 尚未到开放时间
	CheckinStateAvailable = "available"  // 可签到
	CheckinStateChecked   = "checked"    // 今天已签到
)

// CheckinTimeInfo 签到状态接口返回的时间领域数据。
// 所有展示时间均按服务启动时区由后端格式化（前端不自行换算浏览器时区）。
type CheckinTimeInfo struct {
	RewardType           string `json:"reward_type"`              // 奖励类型: permanent / temporary
	AvailableFromMinutes int    `json:"available_from_minutes"`   // 当天第几分钟开放
	Timezone             string `json:"timezone"`                 // 服务启动时区
	CurrentDate          string `json:"current_date"`             // 系统时区今天的日期 YYYY-MM-DD
	ServerTime           int64  `json:"server_time"`              // 服务端当前时间（Unix 秒）
	State                string `json:"state"`                    // not_open / available / checked
	AvailableFrom        int64  `json:"available_from"`           // 今天开放时间（Unix 秒）
	ExpiresAt            int64  `json:"expires_at"`               // 今天限时额度失效时间（次日 00:00，Unix 秒）
	NextTransitionAt     int64  `json:"next_transition_at"`       // 下一次状态切换时间（Unix 秒），用于前端自动刷新
	TemporaryQuota       int    `json:"temporary_quota"`          // 当前有效限时额度（已签到未使用完时返回）
	AvailableFromDisplay string `json:"available_from_display"`   // 开放时间展示（HH:mm，服务时区）
	ExpiresAtDisplay     string `json:"expires_at_display"`       // 失效时间展示（MM-DD HH:mm，服务时区）
}

// ComputeCheckinTimeInfo 计算签到时间领域数据。
// 所有日期、开放时间、失效时间均使用服务启动时区（common.NowInStartupTimezone）。
// checkedToday 为 true 表示用户今天已签到。
func ComputeCheckinTimeInfo(userId int, checkedToday bool) (*CheckinTimeInfo, error) {
	return ComputeCheckinTimeInfoAt(userId, common.NowInStartupTimezone(), checkedToday)
}

// ComputeCheckinTimeInfoAt 计算签到时间领域数据，接受指定的基准时间（便于测试边界）。
func ComputeCheckinTimeInfoAt(userId int, now time.Time, checkedToday bool) (*CheckinTimeInfo, error) {
	setting := operation_setting.GetCheckinSetting()
	loc := common.StartupLocation()
	now = now.In(loc)

	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	tomorrowStart := todayStart.AddDate(0, 0, 1)

	availableFrom := todayStart.Add(time.Duration(setting.AvailableFromMinutes) * time.Minute)
	expiresAt := tomorrowStart.Unix()
	availableFromUnix := availableFrom.Unix()

	state := CheckinStateAvailable
	nextTransition := expiresAt
	if checkedToday {
		state = CheckinStateChecked
	} else if now.Before(availableFrom) {
		state = CheckinStateNotOpen
		nextTransition = availableFromUnix
	}

	info := &CheckinTimeInfo{
		RewardType:           setting.RewardType,
		AvailableFromMinutes: setting.AvailableFromMinutes,
		Timezone:             common.StartupTimezoneName(),
		CurrentDate:          todayStart.Format("2006-01-02"),
		ServerTime:           now.Unix(),
		State:                state,
		AvailableFrom:        availableFromUnix,
		ExpiresAt:            expiresAt,
		NextTransitionAt:     nextTransition,
		TemporaryQuota:       0,
		AvailableFromDisplay: availableFrom.Format("15:04"),
		ExpiresAtDisplay:     tomorrowStart.Format("01-02 15:04"),
	}

	// 限时模式下返回当前仍有效的限时额度（已过期额度按零返回）
	if setting.IsTemporaryReward() {
		quota, err := model.GetActiveTemporaryQuota(userId)
		if err == nil {
			info.TemporaryQuota = quota
		}
	}
	return info, nil
}

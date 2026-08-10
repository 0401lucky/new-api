package common

import (
	"os"
	"strings"
	"time"
)

const (
	beijingTimezoneName          = "Asia/Shanghai"
	beijingTimezoneOffsetSeconds = 8 * 60 * 60
)

var (
	startupLocation     = time.Local
	startupTimezoneName = time.Local.String()
	// 北京时间全年固定为 UTC+8。使用固定偏移可兼容未安装 tzdata 的精简部署环境。
	beijingLocation = time.FixedZone(beijingTimezoneName, beijingTimezoneOffsetSeconds)
)

func InitStartupTimezone() {
	timezoneName := strings.TrimSpace(os.Getenv("TZ"))
	if timezoneName == "" {
		startupLocation = time.Local
		startupTimezoneName = startupLocation.String()
		return
	}

	location, err := time.LoadLocation(timezoneName)
	if err != nil {
		SysError("failed to load TZ " + timezoneName + ": " + err.Error())
		startupLocation = time.Local
		startupTimezoneName = startupLocation.String()
		return
	}

	time.Local = location
	startupLocation = location
	startupTimezoneName = timezoneName
}

func StartupLocation() *time.Location {
	if startupLocation == nil {
		return time.Local
	}
	return startupLocation
}

func StartupTimezoneName() string {
	if startupTimezoneName == "" {
		return StartupLocation().String()
	}
	return startupTimezoneName
}

func NowInStartupTimezone() time.Time {
	return time.Now().In(StartupLocation())
}

// BeijingLocation 返回固定的北京时间时区。
// 北京时间全年固定为 UTC+8，不受服务器所在地或全局 TZ 配置影响。
func BeijingLocation() *time.Location {
	return beijingLocation
}

func BeijingTimezoneName() string {
	return beijingTimezoneName
}

func NowInBeijingTimezone() time.Time {
	return time.Now().In(BeijingLocation())
}

// FormatInBeijingTimezone 将 Unix 秒按北京时间格式化为字符串。
func FormatInBeijingTimezone(unix int64, layout string) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).In(BeijingLocation()).Format(layout)
}

// CheckinLocation 返回签到使用的北京时间。
func CheckinLocation() *time.Location {
	return BeijingLocation()
}

func CheckinTimezoneName() string {
	return BeijingTimezoneName()
}

func NowInCheckinTimezone() time.Time {
	return NowInBeijingTimezone()
}

// FormatInCheckinTimezone 将 Unix 秒按签到时区格式化为字符串。
func FormatInCheckinTimezone(unix int64, layout string) string {
	return FormatInBeijingTimezone(unix, layout)
}

// FormatInStartupTimezone 将 Unix 秒按服务启动时区格式化为字符串（用于前端展示）。
// 前端不得用浏览器本地时区自行格式化服务端时间。
func FormatInStartupTimezone(unix int64, layout string) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).In(StartupLocation()).Format(layout)
}

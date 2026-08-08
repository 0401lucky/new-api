package common

import (
	"os"
	"strings"
	"time"
)

var (
	startupLocation     = time.Local
	startupTimezoneName = time.Local.String()
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

// FormatInStartupTimezone 将 Unix 秒按服务启动时区格式化为字符串（用于前端展示）。
// 前端不得用浏览器本地时区自行格式化服务端时间。
func FormatInStartupTimezone(unix int64, layout string) string {
	if unix <= 0 {
		return ""
	}
	return time.Unix(unix, 0).In(StartupLocation()).Format(layout)
}

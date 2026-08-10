package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInitStartupTimezoneCapturesTZ(t *testing.T) {
	previousLocation := startupLocation
	previousName := startupTimezoneName
	previousTimeLocal := time.Local
	t.Cleanup(func() {
		startupLocation = previousLocation
		startupTimezoneName = previousName
		time.Local = previousTimeLocal
	})

	t.Setenv("TZ", "Asia/Shanghai")
	InitStartupTimezone()

	if StartupTimezoneName() != "Asia/Shanghai" {
		t.Fatalf("StartupTimezoneName() = %q, want Asia/Shanghai", StartupTimezoneName())
	}
	if StartupLocation().String() != "Asia/Shanghai" {
		t.Fatalf("StartupLocation() = %q, want Asia/Shanghai", StartupLocation().String())
	}
	if NowInStartupTimezone().Location().String() != "Asia/Shanghai" {
		t.Fatalf("NowInStartupTimezone location = %q, want Asia/Shanghai", NowInStartupTimezone().Location().String())
	}
}

func TestCheckinTimezoneIsIndependentFromStartupTimezone(t *testing.T) {
	previousLocation := startupLocation
	previousName := startupTimezoneName
	previousTimeLocal := time.Local
	t.Cleanup(func() {
		startupLocation = previousLocation
		startupTimezoneName = previousName
		time.Local = previousTimeLocal
	})

	t.Setenv("TZ", "America/New_York")
	InitStartupTimezone()

	require.Equal(t, "Asia/Shanghai", BeijingTimezoneName())
	require.Equal(t, "Asia/Shanghai", BeijingLocation().String())
	require.Equal(t, "Asia/Shanghai", NowInBeijingTimezone().Location().String())
	require.Equal(t, "Asia/Shanghai", CheckinTimezoneName())
	require.Equal(t, "Asia/Shanghai", CheckinLocation().String())
	require.Equal(t, "Asia/Shanghai", NowInCheckinTimezone().Location().String())

	unix := time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC).Unix()
	require.Equal(t, "2026-08-08 08:00", FormatInBeijingTimezone(unix, "2006-01-02 15:04"))
	require.Equal(t, "2026-08-08 08:00", FormatInCheckinTimezone(unix, "2006-01-02 15:04"))
}

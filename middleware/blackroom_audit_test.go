package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupBlackroomGuardTest 提供一个内存库与一份可自由覆盖的小黑屋配置，
// 并在用例结束后完整还原全局状态。
func setupBlackroomGuardTest(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	previousMode := gin.Mode()
	previousSetting := *operation_setting.GetBlackroomSetting()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.BlackroomBan{},
		&model.BlackroomBanEvent{},
		&model.BlackroomIPMinute{},
	))

	model.DB = db
	model.LOG_DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled = previousRedis
		*operation_setting.GetBlackroomSetting() = previousSetting
		gin.SetMode(previousMode)
	})
	return db
}

func newBlackroomGuardContext(userId int, username string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.RemoteAddr = "203.0.113.7:12345"
	c.Set("id", userId)
	c.Set("username", username)
	return c, recorder
}

func seedBlackroomGuardObservations(t *testing.T, userId int, ipCount int, observedAt int64) {
	t.Helper()
	rows := make([]model.BlackroomIPMinute, 0, ipCount)
	for i := range ipCount {
		rows = append(rows, model.BlackroomIPMinute{
			UserId:       userId,
			Ip:           fmt.Sprintf("198.51.100.%d", i+1),
			Minute:       observedAt - observedAt%60,
			FirstSeenAt:  observedAt,
			LastSeenAt:   observedAt,
			RequestCount: 1,
		})
	}
	require.NoError(t, model.DB.Create(&rows).Error)
}

// 命中规则时实时封禁并当场拦截，不必等下一轮定时扫描。
func TestBlackroomRelayGuardBlocksNewlyBannedUser(t *testing.T) {
	db := setupBlackroomGuardTest(t)

	setting := operation_setting.GetBlackroomSetting()
	*setting = operation_setting.BlackroomSetting{
		Enabled:         true,
		AutoBanEnabled:  true,
		RealtimeEnabled: true,
		LookbackHours:   24,
		Rules: []operation_setting.BlackroomRule{
			{IPCount: 3, DurationHours: 6},
		},
		EscalationWindowDays:        30,
		EscalationTemporaryBanCount: 3,
	}

	user := model.User{
		Id:       7001,
		Username: "guard-user",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(&user).Error)
	// 先造 2 个历史 IP，加上本次请求自身的 IP 恰好达到阈值 3：这同时验证
	// 观测先于判定写入，本次请求的 IP 会立即参与本次判定。
	seedBlackroomGuardObservations(t, user.Id, 2, common.GetTimestamp()-60)

	c, recorder := newBlackroomGuardContext(user.Id, user.Username)
	BlackroomRelayGuard()(c)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.True(t, c.IsAborted())

	ban, err := model.GetActiveBlackroomBan(user.Id)
	require.NoError(t, err)
	assert.Equal(t, model.BlackroomBanSourceAuto, ban.Source)
	assert.Contains(t, ban.Reason, "3 个不同 IP")
}

// 未达阈值时正常放行，且观测已经落库，供后续判定与审计使用。
func TestBlackroomRelayGuardPassesUserBelowThreshold(t *testing.T) {
	db := setupBlackroomGuardTest(t)

	setting := operation_setting.GetBlackroomSetting()
	*setting = operation_setting.BlackroomSetting{
		Enabled:         true,
		AutoBanEnabled:  true,
		RealtimeEnabled: true,
		LookbackHours:   24,
		Rules: []operation_setting.BlackroomRule{
			{IPCount: 8, DurationHours: 6},
		},
		EscalationWindowDays:        30,
		EscalationTemporaryBanCount: 3,
	}

	user := model.User{
		Id:       7002,
		Username: "guard-pass-user",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
	}
	require.NoError(t, db.Create(&user).Error)

	c, recorder := newBlackroomGuardContext(user.Id, user.Username)
	BlackroomRelayGuard()(c)

	assert.False(t, c.IsAborted())
	assert.Equal(t, http.StatusOK, recorder.Code)

	// 观测必须先于判定写入，否则窗口内会缺少这段数据。
	var observations int64
	require.NoError(t, db.Model(&model.BlackroomIPMinute{}).
		Where("user_id = ?", user.Id).
		Count(&observations).Error)
	assert.Equal(t, int64(1), observations)
}

// 审计写入失败时不阻断请求：封禁拦截已在鉴权阶段做过一次，再让所有模型
// 调用因数据库抖动失败会放大故障面。
func TestBlackroomRelayGuardDoesNotBlockOnAuditFailure(t *testing.T) {
	db := setupBlackroomGuardTest(t)

	setting := operation_setting.GetBlackroomSetting()
	*setting = operation_setting.BlackroomSetting{
		Enabled:         true,
		AutoBanEnabled:  true,
		RealtimeEnabled: true,
		LookbackHours:   24,
		Rules:           []operation_setting.BlackroomRule{{IPCount: 3, DurationHours: 6}},
	}

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	c, recorder := newBlackroomGuardContext(7003, "guard-db-down-user")
	BlackroomRelayGuard()(c)

	assert.False(t, c.IsAborted())
	assert.NotEqual(t, http.StatusForbidden, recorder.Code)
}

// 小黑屋整体关闭时完全跳过：不写观测、不判定。
func TestBlackroomRelayGuardSkipsWhenDisabled(t *testing.T) {
	db := setupBlackroomGuardTest(t)

	setting := operation_setting.GetBlackroomSetting()
	*setting = operation_setting.BlackroomSetting{
		Enabled:         false,
		AutoBanEnabled:  true,
		RealtimeEnabled: true,
		Rules:           []operation_setting.BlackroomRule{{IPCount: 3, DurationHours: 6}},
	}

	c, recorder := newBlackroomGuardContext(7004, "guard-disabled-user")
	BlackroomRelayGuard()(c)

	assert.False(t, c.IsAborted())
	assert.Equal(t, http.StatusOK, recorder.Code)

	var observations int64
	require.NoError(t, db.Model(&model.BlackroomIPMinute{}).Count(&observations).Error)
	assert.Equal(t, int64(0), observations)
}

package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetUserExposesTodayCheckinStatus 校验 GetUser 暴露的当日签到状态字段，
// 供福利站等外部服务判重使用。
func TestGetUserExposesTodayCheckinStatus(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Checkin{}))

	user := model.User{
		Username: "checkin-status-user",
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "checkin-status-aff",
	}
	require.NoError(t, db.Create(&user).Error)

	today := common.NowInCheckinTimezone().Format("2006-01-02")

	resetCheckins := func(t *testing.T) {
		t.Helper()
		require.NoError(t, db.Where("user_id = ?", user.Id).Delete(&model.Checkin{}).Error)
	}

	createCheckin := func(t *testing.T, quotaType string) {
		t.Helper()
		checkin := model.Checkin{
			UserId:       user.Id,
			CheckinDate:  today,
			QuotaAwarded: 1000,
			QuotaType:    quotaType,
		}
		require.NoError(t, db.Create(&checkin).Error)
		if quotaType == "" {
			// gorm 的 default 标签会把空值补成 permanent，这里直接改库模拟旧数据
			require.NoError(t, db.Model(&model.Checkin{}).Where("id = ?", checkin.Id).
				UpdateColumn("quota_type", "").Error)
		}
	}

	fetch := func(t *testing.T) map[string]any {
		t.Helper()
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/"+strconv.Itoa(user.Id), nil)
		ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(user.Id)}}
		ctx.Set("role", common.RoleRootUser)
		GetUser(ctx)

		var payload map[string]any
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		require.Equal(t, true, payload["success"])
		return payload["data"].(map[string]any)
	}

	t.Run("no checkin today", func(t *testing.T) {
		resetCheckins(t)
		data := fetch(t)
		assert.Equal(t, false, data["checked_in_today"])
		assert.Equal(t, "", data["today_checkin_quota_type"])
		assert.Equal(t, float64(0), data["today_checkin_quota_awarded"])
	})

	t.Run("temporary checkin today", func(t *testing.T) {
		resetCheckins(t)
		createCheckin(t, model.CheckinQuotaTypeTemporary)
		data := fetch(t)
		assert.Equal(t, true, data["checked_in_today"])
		assert.Equal(t, "temporary", data["today_checkin_quota_type"])
		assert.Equal(t, float64(1000), data["today_checkin_quota_awarded"])
	})

	t.Run("permanent checkin today", func(t *testing.T) {
		resetCheckins(t)
		createCheckin(t, model.CheckinQuotaTypePermanent)
		data := fetch(t)
		assert.Equal(t, true, data["checked_in_today"])
		assert.Equal(t, "permanent", data["today_checkin_quota_type"])
		assert.Equal(t, float64(1000), data["today_checkin_quota_awarded"])
	})

	t.Run("legacy empty quota type normalized to permanent", func(t *testing.T) {
		resetCheckins(t)
		createCheckin(t, "")
		var stored model.Checkin
		require.NoError(t, db.Where("user_id = ?", user.Id).First(&stored).Error)
		require.Equal(t, "", stored.QuotaType)

		data := fetch(t)
		assert.Equal(t, true, data["checked_in_today"])
		assert.Equal(t, "permanent", data["today_checkin_quota_type"])
		assert.Equal(t, float64(1000), data["today_checkin_quota_awarded"])
	})

	// 保证既有字段未被破坏
	t.Run("existing fields preserved", func(t *testing.T) {
		resetCheckins(t)
		data := fetch(t)
		assert.Equal(t, float64(user.Id), data["id"])
		assert.Equal(t, user.Username, data["username"])
		_, hasTemporaryQuota := data["temporary_quota"]
		assert.True(t, hasTemporaryQuota)
	})
}

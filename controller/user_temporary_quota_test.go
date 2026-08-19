package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrantUserTemporaryQuota(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Checkin{}))

	user := model.User{
		Username: "temp-quota-user",
		Password: "password123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  "temp-quota-aff",
	}
	require.NoError(t, db.Create(&user).Error)

	run := func(body string) *httptest.ResponseRecorder {
		gin.SetMode(gin.TestMode)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/temporary_quota", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		GrantUserTemporaryQuota(ctx)
		return recorder
	}

	decode := func(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var payload map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
		return payload
	}

	t.Run("grant then accumulate", func(t *testing.T) {
		payload := decode(t, run(`{"user_id":`+strconv.Itoa(user.Id)+`,"quota":5000}`))
		require.Equal(t, true, payload["success"])
		data := payload["data"].(map[string]any)
		assert.Equal(t, float64(5000), data["quota_added"])
		assert.Equal(t, float64(5000), data["quota_remaining"])
		assert.Greater(t, data["expires_at"].(float64), float64(0))
		assert.NotEmpty(t, data["checkin_date"])

		payload = decode(t, run(`{"user_id":`+strconv.Itoa(user.Id)+`,"quota":3000}`))
		require.Equal(t, true, payload["success"])
		data = payload["data"].(map[string]any)
		assert.Equal(t, float64(3000), data["quota_added"])
		assert.Equal(t, float64(8000), data["quota_remaining"])
	})

	t.Run("invalid quota rejected", func(t *testing.T) {
		payload := decode(t, run(`{"user_id":`+strconv.Itoa(user.Id)+`,"quota":0}`))
		assert.Equal(t, false, payload["success"])
	})

	t.Run("unknown user rejected", func(t *testing.T) {
		payload := decode(t, run(`{"user_id":999999,"quota":100}`))
		assert.Equal(t, false, payload["success"])
	})
}

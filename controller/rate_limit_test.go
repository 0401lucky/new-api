package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelfRateLimitUsageRequiresIdentity(t *testing.T) {
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.GET("/self/rate_limit", GetSelfRateLimitUsage)
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/self/rate_limit?user_id=42", nil))
	assert.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestSelfRateLimitUsageIgnoresRequestedIdentityAndFiltersGroups(t *testing.T) {
	previousRedis, previousClient := common.RedisEnabled, common.RDB
	previousEnabled, previousSuccess := setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitSuccessCount
	previousGroups := setting.UserUsableGroups2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	previousLimits := setting.ModelRequestRateLimitGroup2JSONString()
	t.Cleanup(func() {
		common.RedisEnabled, common.RDB = previousRedis, previousClient
		setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitSuccessCount = previousEnabled, previousSuccess
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
		require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(previousLimits))
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","removed":"Removed"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"private":1}`))
	require.NoError(t, setting.UpdateModelRequestRateLimitGroupByJSONString(`{}`))
	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitSuccessCount = 10
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	common.RedisEnabled, common.RDB = true, client
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	require.NoError(t, client.LPush(context.Background(), "rateLimit:MRRLS:817241", now).Err())
	require.NoError(t, client.LPush(context.Background(), "rateLimit:MRRLS:817242", now, now).Err())
	router := gin.New()
	router.GET("/self/rate_limit", func(c *gin.Context) {
		c.Set("id", 817241)
		c.Set("group", "default")
	}, GetSelfRateLimitUsage)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/self/rate_limit?user_id=817242&group=private", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                           `json:"success"`
		Data    middleware.ModelRateLimitUsage `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	require.Len(t, response.Data.Groups, 1)
	assert.Equal(t, "default", response.Data.Groups[0].Group)
	require.NotNil(t, response.Data.Groups[0].Success.Used)
	assert.EqualValues(t, 1, *response.Data.Groups[0].Success.Used)

	server.SetError("ERR unavailable")
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/self/rate_limit", nil))
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.JSONEq(t, `{"success":false,"message":"Failed to load rate limit usage"}`, recorder.Body.String())
}

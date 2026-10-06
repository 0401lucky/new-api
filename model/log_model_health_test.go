package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Neither policy rejections nor intermediate channel attempts finalize health.
func TestErrorLogsDoNotFinalizeModelHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, DB.AutoMigrate(&ModelHealthSlice5m{}))
	t.Cleanup(func() {
		DB.Where("1 = 1").Delete(&ModelHealthSlice5m{})
		DB.Exec("DELETE FROM logs")
	})

	newTestCtx := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		return c
	}

	const gatewayModel = "policy-blocked-model"
	const channelModel = "channel-error-model"

	RecordGatewayErrorLog(newTestCtx(), 1, 0, gatewayModel, "tk", "leak protection blocked request", 0, 0, false, "default", nil)
	RecordErrorLog(newTestCtx(), 1, 0, channelModel, "tk", "upstream 500", 0, 0, false, "default", nil)

	require.NoError(t, FlushModelHealthEvents(context.Background()))
	var channelHealthRows int64
	require.NoError(t, DB.Model(&ModelHealthSlice5m{}).Where("model_name = ?", channelModel).Count(&channelHealthRows).Error)
	assert.Zero(t, channelHealthRows, "an attempt is not the final request result")

	var gatewayHealthRows int64
	DB.Model(&ModelHealthSlice5m{}).Where("model_name = ?", gatewayModel).Count(&gatewayHealthRows)
	assert.Equal(t, int64(0), gatewayHealthRows, "gateway policy error must not be counted in model health")

	// 两条错误日志本身都必须正常写入 logs 表。
	var errorLogRows int64
	DB.Model(&Log{}).
		Where("type = ? AND model_name IN (?, ?)", LogTypeError, gatewayModel, channelModel).
		Count(&errorLogRows)
	assert.Equal(t, int64(2), errorLogRows)
}

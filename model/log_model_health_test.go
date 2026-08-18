package model

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 网关策略拦截（泄漏防护、提示词检查）产生的错误日志只做审计记录，
// 不得计入模型健康度统计；渠道/上游错误日志必须计入。
func TestGatewayErrorLogExcludedFromModelHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, DB.AutoMigrate(&ModelHealthSlice5m{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM model_health_slice_5m")
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

	// 渠道错误经异步写入器落入健康度切片表，作为同步屏障等待它完成。
	require.Eventually(t, func() bool {
		var n int64
		DB.Model(&ModelHealthSlice5m{}).Where("model_name = ?", channelModel).Count(&n)
		return n > 0
	}, 5*time.Second, 20*time.Millisecond, "channel error should be counted in model health")

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

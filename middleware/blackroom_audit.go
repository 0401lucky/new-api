package middleware

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

// BlackroomRelayGuard 记录一次 relay 请求的 IP 观测，并（在开启时）执行实时
// 判定。它只属于真正的模型调用路由，必须挂在 TokenAuth 解析出身份之后、
// 限流与分发之前。
//
// 审计失败不阻断请求：封禁拦截已经在鉴权阶段做过一次，这里再因数据库抖动
// 让所有模型调用失败会放大故障面。失败只记录日志，判定的兜底由定时扫描承担。
func BlackroomRelayGuard() func(c *gin.Context) {
	return func(c *gin.Context) {
		userId := c.GetInt("id")
		decision, err := service.EvaluateBlackroomRelayRequest(userId, c.GetString("username"), c.ClientIP())
		if err != nil {
			common.SysError(fmt.Sprintf("blackroom relay audit failed for user %d: %v", userId, err))
			c.Next()
			return
		}
		if decision == nil || decision.Ban == nil {
			c.Next()
			return
		}
		abortWithOpenAiMessage(c, http.StatusForbidden, decision.Ban.BlockMessage(), types.ErrorCodeAccessDenied)
	}
}

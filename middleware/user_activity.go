package middleware

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func recordUserRequestActivity(c *gin.Context, userID int) {
	if c.GetString(RouteTagKey) != "relay" || userID <= 0 {
		return
	}
	if err := model.RecordUserRequestActivity(userID, common.GetTimestamp()); err != nil {
		common.SysError(fmt.Sprintf("record request activity for user %d: %v", userID, err))
	}
}

// UserRequestActivity covers session-authenticated playground requests. API
// token requests are recorded as soon as their owner is identified in TokenAuth,
// including expired/exhausted tokens and failures before channel selection.
func UserRequestActivity() gin.HandlerFunc {
	return func(c *gin.Context) {
		recordUserRequestActivity(c, c.GetInt("id"))
		c.Next()
	}
}

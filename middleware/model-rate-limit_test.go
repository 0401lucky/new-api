package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
)

func TestShouldBypassModelRequestRateLimitForAdminRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", common.RoleAdminUser)

	if !shouldBypassModelRequestRateLimit(c, 123) {
		t.Fatal("admin role should bypass model request rate limit")
	}
}

func TestShouldBypassModelRequestRateLimitForExemptUser(t *testing.T) {
	oldIDs := setting.ModelRequestRateLimitExemptUserIDs
	t.Cleanup(func() {
		setting.ModelRequestRateLimitMutex.Lock()
		defer setting.ModelRequestRateLimitMutex.Unlock()
		setting.ModelRequestRateLimitExemptUserIDs = oldIDs
	})

	if err := setting.UpdateModelRequestRateLimitExemptUserIDs("123"); err != nil {
		t.Fatalf("UpdateModelRequestRateLimitExemptUserIDs error: %v", err)
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if !shouldBypassModelRequestRateLimit(c, 123) {
		t.Fatal("configured exempt user should bypass model request rate limit")
	}
}

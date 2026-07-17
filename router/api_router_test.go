package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSetApiRouterRegisters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	SetApiRouter(engine)

	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	expectedRoutes := []string{
		http.MethodGet + " /api/authz/catalog",
		http.MethodGet + " /api/channel/ops",
		http.MethodPost + " /api/channel/:id/status",
		http.MethodPost + " /api/channel/status/batch",
		http.MethodGet + " /api/channel/:id/codex/usage/reset-credits",
		http.MethodPost + " /api/channel/:id/codex/usage/reset",
		http.MethodPost + " /api/subscription/admin/users/:id/subscriptions/reset",
		http.MethodPost + " /api/subscription/admin/plans/:id/subscriptions/reset",
		http.MethodPost + " /api/subscription/admin/plans/:id/grant-all",
		http.MethodGet + " /api/subscription/admin/user_subscriptions",
		http.MethodGet + " /api/system-info/instances",
		http.MethodGet + " /api/system-task/list",
	}
	for _, route := range expectedRoutes {
		if !routes[route] {
			t.Fatalf("expected route %s to be registered", route)
		}
	}
}

package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestShouldStopRetryForClientDisconnect(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request = req
	if shouldStopRetryForClientDisconnect(c) {
		t.Fatal("expected active request to keep retrying")
	}

	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	c.Request = req.WithContext(ctx)
	if !shouldStopRetryForClientDisconnect(c) {
		t.Fatal("expected canceled request to stop retrying")
	}
}

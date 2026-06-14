package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func TestRecentCallsCacheRecordsRequestAndResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := newRecentCallsCache(RecentCallsCacheConfig{Capacity: 4})
	t.Cleanup(func() {
		if dir := cache.TempSessionDirForTest(); dir != "" {
			_ = os.RemoveAll(dir)
		}
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer sk-test-secret")
	common.SetContextKey(c, constant.ContextKeyUserId, 7)
	common.SetContextKey(c, constant.ContextKeyUserName, "tester")
	common.SetContextKey(c, constant.ContextKeyChannelId, 9)

	id := cache.BeginFromContext(c, &relaycommon.RelayInfo{OriginModelName: "gpt-test"}, []byte(`{"messages":[{"role":"user","content":"hello"}]}`))
	if id == 0 {
		t.Fatal("expected recent call id")
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.WriteHeader(http.StatusOK)
	cache.UpsertResponseByContext(c, []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n"), false)

	record, ok := cache.Get(id)
	if !ok {
		t.Fatal("expected recent call record")
	}
	if record.UserID != 7 || record.ChannelID != 9 || record.ModelName != "gpt-test" {
		t.Fatalf("unexpected metadata: %+v", record)
	}
	if record.Username != "tester" {
		t.Fatalf("unexpected username: %q", record.Username)
	}
	if !strings.Contains(record.Request.Body, `"hello"`) {
		t.Fatalf("request body was not materialized: %q", record.Request.Body)
	}
	if got := record.Request.Header["Authorization"]; got == "" || got == "Bearer sk-test-secret" {
		t.Fatalf("authorization header was not masked: %q", got)
	}
	if record.Response == nil || record.Response.StatusCode != http.StatusOK {
		t.Fatalf("response was not recorded: %+v", record.Response)
	}
	if record.Stream == nil || record.Stream.AggregatedText != "hi" {
		t.Fatalf("stream text was not aggregated: %+v", record.Stream)
	}

	list := cache.List(10, 0)
	if len(list) != 1 {
		t.Fatalf("expected one list item, got %d", len(list))
	}
	if list[0].Request.Body != "" {
		t.Fatalf("list should omit heavy request body, got %q", list[0].Request.Body)
	}
}

func TestRecentCallResponseCaptureWriter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := newRecentCallsCache(RecentCallsCacheConfig{Capacity: 4})
	original := recentCallsSingleton
	recentCallsSingleton = cache
	t.Cleanup(func() {
		recentCallsSingleton = original
		if dir := cache.TempSessionDirForTest(); dir != "" {
			_ = os.RemoveAll(dir)
		}
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	id := cache.BeginFromContext(c, &relaycommon.RelayInfo{OriginModelName: "gpt-test"}, []byte(`{"prompt":"hello"}`))
	if id == 0 {
		t.Fatal("expected recent call id")
	}

	AttachRecentCallResponseCapture(c)
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusCreated)
	if _, err := c.Writer.Write([]byte(`{"answer":"hi"}`)); err != nil {
		t.Fatalf("write response failed: %v", err)
	}
	FinalizeRecentCallResponse(c)

	record, ok := cache.Get(id)
	if !ok {
		t.Fatal("expected recent call record")
	}
	if record.Response == nil {
		t.Fatal("expected captured response")
	}
	if record.Response.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected response status: %d", record.Response.StatusCode)
	}
	if !strings.Contains(record.Response.Body, `"hi"`) {
		t.Fatalf("response body was not captured: %q", record.Response.Body)
	}
}

func TestRecentCallSuccessfulResponseClearsRetryError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := newRecentCallsCache(RecentCallsCacheConfig{Capacity: 4})
	t.Cleanup(func() {
		if dir := cache.TempSessionDirForTest(); dir != "" {
			_ = os.RemoveAll(dir)
		}
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	id := cache.BeginFromContext(c, &relaycommon.RelayInfo{OriginModelName: "gpt-test"}, []byte(`{"prompt":"hello"}`))
	cache.UpsertErrorByContext(c, "temporary upstream error", "upstream", "retry", http.StatusBadGateway)
	c.Writer.Header().Set("Content-Type", "application/json")
	cache.UpsertResponseByContext(c, []byte(`{"answer":"hi"}`), false)

	record, ok := cache.Get(id)
	if !ok {
		t.Fatal("expected recent call record")
	}
	if record.Error != nil {
		t.Fatalf("successful final response should clear retry error: %+v", record.Error)
	}
}

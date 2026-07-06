package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func TestMemoryConcurrencyLimiterAcquireRelease(t *testing.T) {
	limiter := newMemoryConcurrencyLimiter()

	release, allowed := limiter.Acquire("user:1", 1)
	if !allowed {
		t.Fatal("first acquire should be allowed")
	}

	secondRelease, allowed := limiter.Acquire("user:1", 1)
	if allowed {
		if secondRelease != nil {
			secondRelease()
		}
		t.Fatal("second acquire should be rejected while slot is occupied")
	}

	release()

	thirdRelease, allowed := limiter.Acquire("user:1", 1)
	if !allowed {
		t.Fatal("third acquire should be allowed after release")
	}
	thirdRelease()
}

func TestMemoryConcurrencyLimiterUnlimited(t *testing.T) {
	limiter := newMemoryConcurrencyLimiter()

	for i := 0; i < 3; i++ {
		release, allowed := limiter.Acquire("user:1", 0)
		if !allowed {
			t.Fatal("limit 0 should be unlimited")
		}
		release()
	}
}

func TestModelRequestRateLimitRejectsConcurrentRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldRedisEnabled := common.RedisEnabled
	oldRateLimiter := inMemoryRateLimiter
	oldConcurrencyLimiter := inMemoryConcurrencyLimiter
	oldEnabled := setting.ModelRequestRateLimitEnabled
	oldDuration := setting.ModelRequestRateLimitDurationMinutes
	oldTotalCount := setting.ModelRequestRateLimitCount
	oldSuccessCount := setting.ModelRequestRateLimitSuccessCount
	oldConcurrencyCount := setting.ModelRequestRateLimitConcurrencyCount
	oldGroup := setting.ModelRequestRateLimitGroup
	oldExemptIDs := setting.ModelRequestRateLimitExemptUserIDs

	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		inMemoryRateLimiter = oldRateLimiter
		inMemoryConcurrencyLimiter = oldConcurrencyLimiter
		setting.ModelRequestRateLimitMutex.Lock()
		defer setting.ModelRequestRateLimitMutex.Unlock()
		setting.ModelRequestRateLimitEnabled = oldEnabled
		setting.ModelRequestRateLimitDurationMinutes = oldDuration
		setting.ModelRequestRateLimitCount = oldTotalCount
		setting.ModelRequestRateLimitSuccessCount = oldSuccessCount
		setting.ModelRequestRateLimitConcurrencyCount = oldConcurrencyCount
		setting.ModelRequestRateLimitGroup = oldGroup
		setting.ModelRequestRateLimitExemptUserIDs = oldExemptIDs
	})

	common.RedisEnabled = false
	inMemoryRateLimiter = common.InMemoryRateLimiter{}
	inMemoryConcurrencyLimiter = newMemoryConcurrencyLimiter()
	setting.ModelRequestRateLimitMutex.Lock()
	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 1000
	setting.ModelRequestRateLimitConcurrencyCount = 1
	setting.ModelRequestRateLimitGroup = map[string][3]int{}
	setting.ModelRequestRateLimitExemptUserIDs = map[int]struct{}{}
	setting.ModelRequestRateLimitMutex.Unlock()

	entered := make(chan struct{}, 2)
	releaseFirst := make(chan struct{})
	firstDone := make(chan struct{}, 2)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 456)
		c.Set("role", common.RoleCommonUser)
	})
	router.Use(ModelRequestRateLimit())
	router.GET("/", func(c *gin.Context) {
		entered <- struct{}{}
		<-releaseFirst
		c.Status(http.StatusOK)
		firstDone <- struct{}{}
	})

	firstRecorder := httptest.NewRecorder()
	go router.ServeHTTP(firstRecorder, httptest.NewRequest(http.MethodGet, "/", nil))

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not enter handler")
	}

	secondRecorder := httptest.NewRecorder()
	router.ServeHTTP(secondRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if secondRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("expected concurrent request status 429, got %d", secondRecorder.Code)
	}

	close(releaseFirst)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first request did not finish")
	}
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("expected first request status 200, got %d", firstRecorder.Code)
	}

	thirdRecorder := httptest.NewRecorder()
	router.ServeHTTP(thirdRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if thirdRecorder.Code != http.StatusOK {
		t.Fatalf("expected request after release status 200, got %d", thirdRecorder.Code)
	}
}

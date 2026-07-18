package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldBypassModelRequestRateLimitAdminRoleNoLongerBypasses(t *testing.T) {
	// 管理员角色不再自动绕过，仅豁免用户 ID 可绕过
	assert.False(t, shouldBypassModelRequestRateLimit(123))
}

func TestShouldBypassModelRequestRateLimitForExemptUser(t *testing.T) {
	oldIDs := setting.ModelRequestRateLimitExemptUserIDs
	t.Cleanup(func() {
		setting.ModelRequestRateLimitMutex.Lock()
		defer setting.ModelRequestRateLimitMutex.Unlock()
		setting.ModelRequestRateLimitExemptUserIDs = oldIDs
	})

	require.NoError(t, setting.UpdateModelRequestRateLimitExemptUserIDs("123"))
	assert.True(t, shouldBypassModelRequestRateLimit(123))
	assert.False(t, shouldBypassModelRequestRateLimit(456))
}

func TestMemoryConcurrencyLimiterAcquireRelease(t *testing.T) {
	limiter := newMemoryConcurrencyLimiter()

	release, allowed := limiter.Acquire("user:1", 1)
	require.True(t, allowed)

	secondRelease, allowed := limiter.Acquire("user:1", 1)
	if allowed {
		if secondRelease != nil {
			secondRelease()
		}
		t.Fatal("second acquire should be rejected while slot is occupied")
	}

	release()

	thirdRelease, allowed := limiter.Acquire("user:1", 1)
	require.True(t, allowed)
	thirdRelease()
}

func TestMemoryConcurrencyLimiterUnlimited(t *testing.T) {
	limiter := newMemoryConcurrencyLimiter()

	for i := 0; i < 3; i++ {
		release, allowed := limiter.Acquire("user:1", 0)
		require.True(t, allowed)
		release()
	}
}

func restoreModelRateLimitSettings(
	t *testing.T,
	oldRedisEnabled bool,
	oldRateLimiter common.InMemoryRateLimiter,
	oldConcurrencyLimiter *memoryConcurrencyLimiter,
	oldEnabled bool,
	oldDuration int,
	oldTotalCount int,
	oldSuccessCount int,
	oldConcurrencyCount int,
	oldGroup map[string][3]int,
	oldExemptIDs map[int]struct{},
) {
	t.Helper()
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
}

func TestModelRequestRateLimitRejectsConcurrentRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restoreModelRateLimitSettings(
		t,
		common.RedisEnabled,
		inMemoryRateLimiter,
		inMemoryConcurrencyLimiter,
		setting.ModelRequestRateLimitEnabled,
		setting.ModelRequestRateLimitDurationMinutes,
		setting.ModelRequestRateLimitCount,
		setting.ModelRequestRateLimitSuccessCount,
		setting.ModelRequestRateLimitConcurrencyCount,
		setting.ModelRequestRateLimitGroup,
		setting.ModelRequestRateLimitExemptUserIDs,
	)

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
	assert.Equal(t, http.StatusTooManyRequests, secondRecorder.Code)

	close(releaseFirst)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first request did not finish")
	}
	assert.Equal(t, http.StatusOK, firstRecorder.Code)

	thirdRecorder := httptest.NewRecorder()
	router.ServeHTTP(thirdRecorder, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, thirdRecorder.Code)
}

func TestModelRequestRateLimitAdminNoLongerBypasses(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restoreModelRateLimitSettings(
		t,
		common.RedisEnabled,
		inMemoryRateLimiter,
		inMemoryConcurrencyLimiter,
		setting.ModelRequestRateLimitEnabled,
		setting.ModelRequestRateLimitDurationMinutes,
		setting.ModelRequestRateLimitCount,
		setting.ModelRequestRateLimitSuccessCount,
		setting.ModelRequestRateLimitConcurrencyCount,
		setting.ModelRequestRateLimitGroup,
		setting.ModelRequestRateLimitExemptUserIDs,
	)

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

	entered := make(chan struct{}, 1)
	hold := make(chan struct{})
	done := make(chan struct{}, 1)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("role", common.RoleAdminUser)
	})
	router.Use(ModelRequestRateLimit())
	router.GET("/", func(c *gin.Context) {
		entered <- struct{}{}
		<-hold
		c.Status(http.StatusOK)
		done <- struct{}{}
	})

	first := httptest.NewRecorder()
	go router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("admin first request should still be rate limited (enter handler)")
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusTooManyRequests, second.Code, "admin should no longer bypass concurrency limit")

	close(hold)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first request did not finish")
	}
}

func TestTokenRateLimitAppliesWhenGlobalDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restoreModelRateLimitSettings(
		t,
		common.RedisEnabled,
		inMemoryRateLimiter,
		inMemoryConcurrencyLimiter,
		setting.ModelRequestRateLimitEnabled,
		setting.ModelRequestRateLimitDurationMinutes,
		setting.ModelRequestRateLimitCount,
		setting.ModelRequestRateLimitSuccessCount,
		setting.ModelRequestRateLimitConcurrencyCount,
		setting.ModelRequestRateLimitGroup,
		setting.ModelRequestRateLimitExemptUserIDs,
	)

	common.RedisEnabled = false
	inMemoryRateLimiter = common.InMemoryRateLimiter{}
	inMemoryConcurrencyLimiter = newMemoryConcurrencyLimiter()
	setting.ModelRequestRateLimitMutex.Lock()
	setting.ModelRequestRateLimitEnabled = false // 全局关闭
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 1000
	setting.ModelRequestRateLimitConcurrencyCount = 0
	setting.ModelRequestRateLimitGroup = map[string][3]int{}
	setting.ModelRequestRateLimitExemptUserIDs = map[int]struct{}{}
	setting.ModelRequestRateLimitMutex.Unlock()

	entered := make(chan struct{}, 1)
	hold := make(chan struct{})
	done := make(chan struct{}, 1)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 99)
		c.Set("role", common.RoleAdminUser)
		c.Set("token_id", 7)
		c.Set("token_rate_limit_enabled", true)
		c.Set("token_rate_limit_total", 0)
		c.Set("token_rate_limit_success", 100)
		c.Set("token_rate_limit_concurrency", 1)
	})
	router.Use(ModelRequestRateLimit())
	router.GET("/", func(c *gin.Context) {
		entered <- struct{}{}
		<-hold
		c.Status(http.StatusOK)
		done <- struct{}{}
	})

	first := httptest.NewRecorder()
	go router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("token-limited first request did not enter")
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusTooManyRequests, second.Code, "token concurrency should apply when global is off")

	close(hold)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first request did not finish")
	}
	assert.Equal(t, http.StatusOK, first.Code)
}

func TestTokenRateLimitSuccessCount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	restoreModelRateLimitSettings(
		t,
		common.RedisEnabled,
		inMemoryRateLimiter,
		inMemoryConcurrencyLimiter,
		setting.ModelRequestRateLimitEnabled,
		setting.ModelRequestRateLimitDurationMinutes,
		setting.ModelRequestRateLimitCount,
		setting.ModelRequestRateLimitSuccessCount,
		setting.ModelRequestRateLimitConcurrencyCount,
		setting.ModelRequestRateLimitGroup,
		setting.ModelRequestRateLimitExemptUserIDs,
	)

	common.RedisEnabled = false
	inMemoryRateLimiter = common.InMemoryRateLimiter{}
	inMemoryConcurrencyLimiter = newMemoryConcurrencyLimiter()
	setting.ModelRequestRateLimitMutex.Lock()
	setting.ModelRequestRateLimitEnabled = false
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 1000
	setting.ModelRequestRateLimitConcurrencyCount = 0
	setting.ModelRequestRateLimitGroup = map[string][3]int{}
	setting.ModelRequestRateLimitExemptUserIDs = map[int]struct{}{}
	setting.ModelRequestRateLimitMutex.Unlock()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 50)
		c.Set("token_id", 8)
		c.Set("token_rate_limit_enabled", true)
		c.Set("token_rate_limit_total", 0)
		c.Set("token_rate_limit_success", 1)
		c.Set("token_rate_limit_concurrency", 0)
	})
	router.Use(ModelRequestRateLimit())
	router.GET("/", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, first.Code)

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusTooManyRequests, second.Code)
}

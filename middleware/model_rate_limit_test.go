package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelRedisRateLimitUsesUTCRegardlessOfLocalTimezone(t *testing.T) {
	redisServer, redisClient := useRateLimitMiniRedis(t)
	previousLocation := time.Local
	time.Local = time.FixedZone("test-utc-plus-eight", 8*60*60)
	t.Cleanup(func() { time.Local = previousLocation })

	ctx := context.Background()
	recordKey := "rateLimit:model-utc-record"
	recordRedisRequest(ctx, redisClient, recordKey, 2)
	recorded, err := redisClient.LIndex(ctx, recordKey, 0).Result()
	require.NoError(t, err)
	recordedAt, err := time.Parse(modelRateLimitTimeFormat, recorded)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), recordedAt, 2*time.Second)

	checkKey := "rateLimit:model-utc-check"
	withinWindow := time.Now().UTC().Add(-30 * time.Second).Format(modelRateLimitTimeFormat)
	_, err = redisServer.Push(checkKey, withinWindow, withinWindow)
	require.NoError(t, err)
	allowed, err := checkRedisRateLimit(ctx, redisClient, checkKey, 2, 60)
	require.NoError(t, err)
	assert.False(t, allowed, "an existing UTC timestamp inside the window must remain limited on a non-UTC host")
}

var modelRateLimitTestUsers atomic.Int64

func TestModelRateLimitUsage(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			restoreModelRateLimitSettings(t, common.RedisEnabled, inMemoryRateLimiter, inMemoryConcurrencyLimiter,
				setting.ModelRequestRateLimitEnabled, setting.ModelRequestRateLimitDurationMinutes,
				setting.ModelRequestRateLimitCount, setting.ModelRequestRateLimitSuccessCount,
				setting.ModelRequestRateLimitConcurrencyCount, setting.ModelRequestRateLimitGroup,
				setting.ModelRequestRateLimitExemptUserIDs)
			common.RedisEnabled = false
			inMemoryRateLimiter = &common.InMemoryRateLimiter{}
			inMemoryRateLimiter.Init(0)
			inMemoryConcurrencyLimiter = newMemoryConcurrencyLimiter()
			setting.ModelRequestRateLimitEnabled = true
			setting.ModelRequestRateLimitDurationMinutes = 2
			setting.ModelRequestRateLimitCount = 5
			setting.ModelRequestRateLimitSuccessCount = 3
			setting.ModelRequestRateLimitConcurrencyCount = 2
			setting.ModelRequestRateLimitGroup = map[string][3]int{"free": {2, 2, 1}, "unlimited": {0, 0, 0}}
			setting.ModelRequestRateLimitExemptUserIDs = map[int]struct{}{}
			ctx := context.Background()
			layer := rateLimitLayer{identity: "42", successMaxCount: 3}
			var pending *common.RateLimitReservation
			var release concurrencyReleaseFunc
			if backend == "redis" {
				_, client := useRateLimitMiniRedis(t)
				now := time.Now()
				require.NoError(t, client.HSet(ctx, layer.totalRedisKey(), "tokens", 120, "last_time", now.Unix()).Err())
				require.NoError(t, client.LPush(ctx, layer.successRedisKey(),
					now.Add(-121*time.Second).UTC().Format(modelRateLimitTimeFormat),
					now.Add(-10*time.Second).UTC().Format(modelRateLimitTimeFormat)).Err())
				require.NoError(t, client.ZAdd(ctx, "rateLimit:MRRLC:42", &redis.Z{Score: float64(now.Unix() - 120), Member: "expired"}).Err())
				var allowed bool
				var err error
				release, allowed, err = acquireConcurrencySlot("42", 2)
				require.NoError(t, err)
				require.True(t, allowed)
			} else {
				require.True(t, inMemoryRateLimiter.Request(layer.totalMemoryKey(), 5, 120))
				completed := inMemoryRateLimiter.Reserve(layer.successMemoryKey(), 3, 120)
				require.NotNil(t, completed)
				completed.Complete(true)
				pending = inMemoryRateLimiter.Reserve(layer.successMemoryKey(), 3, 120)
				require.NotNil(t, pending)
				var allowed bool
				release, allowed = inMemoryConcurrencyLimiter.Acquire("rateLimit:MRRLC:42", 2)
				require.True(t, allowed)
			}
			t.Cleanup(release)

			usage, err := GetModelRateLimitUsage(ctx, 42, []string{"unlimited", "free", "default", "free"})
			require.NoError(t, err)
			require.Len(t, usage.Groups, 3)
			assert.Equal(t, 2, usage.WindowMinutes)
			assert.Equal(t, "default", usage.Groups[0].Group)
			assert.Equal(t, 5, usage.Groups[0].Total.Limit)
			assert.Equal(t, 2, usage.Groups[1].Total.Limit)
			assert.EqualValues(t, 1, *usage.Groups[1].Total.Used)
			assert.EqualValues(t, 1, *usage.Groups[1].Concurrency.Used)
			assert.Nil(t, usage.Groups[2].Total.Used)
			assert.Nil(t, usage.Groups[2].Success.Used)
			assert.Nil(t, usage.Groups[2].Concurrency.Used)
			if backend == "memory" {
				assert.EqualValues(t, 2, *usage.Groups[1].Success.Used, "pending reservations occupy success admission")
				pending.Complete(false)
			} else {
				assert.Equal(t, "token_bucket", usage.TotalMode)
				assert.EqualValues(t, 4, *usage.Groups[0].Total.Used, "each group projects its own bucket capacity")
				assert.EqualValues(t, 1, *usage.Groups[1].Success.Used, "expired successes are excluded")
			}
			release()
			again, err := GetModelRateLimitUsage(ctx, 42, []string{"free"})
			require.NoError(t, err)
			assert.EqualValues(t, 1, *again.Groups[0].Total.Used, "reading does not consume request capacity")
			assert.EqualValues(t, 1, *again.Groups[0].Success.Used)
			assert.EqualValues(t, 0, *again.Groups[0].Concurrency.Used)

			other, err := GetModelRateLimitUsage(ctx, 43, []string{"free"})
			require.NoError(t, err)
			assert.EqualValues(t, 0, *other.Groups[0].Total.Used)
			assert.EqualValues(t, 0, *other.Groups[0].Success.Used)
			assert.EqualValues(t, 0, *other.Groups[0].Concurrency.Used)

			setting.ModelRequestRateLimitExemptUserIDs[42] = struct{}{}
			exempt, err := GetModelRateLimitUsage(ctx, 42, []string{"free"})
			require.NoError(t, err)
			assert.True(t, exempt.Exempt)
			assert.Empty(t, exempt.Groups)
			setting.ModelRequestRateLimitEnabled = false
			disabled, err := GetModelRateLimitUsage(ctx, 43, []string{"free"})
			require.NoError(t, err)
			assert.False(t, disabled.Enabled)
			assert.Empty(t, disabled.Groups)
		})
	}
}

func TestModelRateLimitUsageRedisFailureIsNotZeroUsage(t *testing.T) {
	server, _ := useRateLimitMiniRedis(t)
	previous := setting.ModelRequestRateLimitEnabled
	setting.ModelRequestRateLimitEnabled = true
	t.Cleanup(func() { setting.ModelRequestRateLimitEnabled = previous })
	server.SetError("ERR unavailable")
	usage, err := GetModelRateLimitUsage(context.Background(), 718822, []string{"default"})
	require.Error(t, err)
	assert.Nil(t, usage)
}

func TestModelRateLimitStreamFailuresDoNotConsumeSuccessLimit(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		for _, totalLimit := range []int{0, 2} {
			t.Run(fmt.Sprintf("%s/total=%d", backend, totalLimit), func(t *testing.T) {
				userID := 7200000 + int(modelRateLimitTestUsers.Add(1))
				layers := []rateLimitLayer{{identity: strconv.Itoa(userID), totalMaxCount: totalLimit, successMaxCount: 1}}
				handler := memoryRateLimitHandler(60, layers)
				if backend == "redis" {
					useRateLimitMiniRedis(t)
					handler = redisRateLimitHandler(60, layers)
				}
				router := gin.New()
				router.GET("/:outcome", func(c *gin.Context) { c.Set("id", userID) }, handler, func(c *gin.Context) {
					status := relaycommon.NewStreamStatus()
					if c.Param("outcome") == "failed" {
						status.MarkFailed("server_error", "", 0)
					} else {
						status.MarkCompleted()
					}
					common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, status)
					c.Status(http.StatusOK)
				})
				assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
				if totalLimit > 0 {
					assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
				} else {
					assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
				}
				assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
			})
		}
	}
}

func TestModelMemoryRateLimitReservesConcurrentSuccessAdmission(t *testing.T) {
	userID := 7200000 + int(modelRateLimitTestUsers.Add(1))
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	layers := []rateLimitLayer{{identity: strconv.Itoa(userID), successMaxCount: 1}}
	router := gin.New()
	router.GET("/:outcome", func(c *gin.Context) { c.Set("id", userID) }, memoryRateLimitHandler(60, layers), func(c *gin.Context) {
		if c.Param("outcome") == "failed" {
			close(entered)
			<-release
			status := relaycommon.NewStreamStatus()
			status.MarkFailed("server_error", "", 0)
			common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, status)
		}
		c.Status(http.StatusOK)
	})
	go func() {
		defer close(finished)
		assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/failed", "127.0.0.1:1000").Code)
	}()
	<-entered
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
	close(release)
	<-finished
	assert.Equal(t, http.StatusOK, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
	assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/completed", "127.0.0.1:1000").Code)
}

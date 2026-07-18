package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
	ModelRequestRateLimitConcurrencyMark  = "MRRLC"
	concurrencySlotTTLSeconds             = 120
	concurrencySlotRefreshSeconds         = 30
)

var (
	redisAcquireConcurrencySlotScript = redis.NewScript(`
local key = KEYS[1]
local slot = ARGV[1]
local now = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local limit = tonumber(ARGV[4])
redis.call('ZREMRANGEBYSCORE', key, '-inf', now - ttl)
if redis.call('ZSCORE', key, slot) then
  redis.call('ZADD', key, now, slot)
  redis.call('EXPIRE', key, ttl)
  return 1
end
if redis.call('ZCARD', key) >= limit then
  redis.call('EXPIRE', key, ttl)
  return 0
end
redis.call('ZADD', key, now, slot)
redis.call('EXPIRE', key, ttl)
return 1
`)
	redisRefreshConcurrencySlotScript = redis.NewScript(`
local key = KEYS[1]
local slot = ARGV[1]
local now = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
if redis.call('ZSCORE', key, slot) then
  redis.call('ZADD', key, now, slot)
  redis.call('EXPIRE', key, ttl)
  return 1
end
return 0
`)
	inMemoryConcurrencyLimiter = newMemoryConcurrencyLimiter()
)

type concurrencyReleaseFunc func()

type memoryConcurrencyLimiter struct {
	mu    sync.Mutex
	slots map[string]map[string]int64
}

func newMemoryConcurrencyLimiter() *memoryConcurrencyLimiter {
	return &memoryConcurrencyLimiter{
		slots: make(map[string]map[string]int64),
	}
}

func (l *memoryConcurrencyLimiter) Acquire(key string, limit int) (concurrencyReleaseFunc, bool) {
	if limit <= 0 {
		return func() {}, true
	}

	slot := common.GetUUID()
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.slots[key]) >= limit {
		return nil, false
	}
	if l.slots[key] == nil {
		l.slots[key] = make(map[string]int64)
	}
	l.slots[key][slot] = time.Now().Unix()

	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			delete(l.slots[key], slot)
			if len(l.slots[key]) == 0 {
				delete(l.slots, key)
			}
		})
	}, true
}

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
	oldTime, err := time.Parse(timeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	nowTimeStr := time.Now().Format(timeFormat)
	nowTime, err := time.Parse(timeFormat, nowTimeStr)
	if err != nil {
		return false, err
	}
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
		return false, nil
	}

	return true, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int) {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return
	}

	now := time.Now().Format(timeFormat)
	rdb.LPush(ctx, key, now)
	rdb.LTrim(ctx, key, 0, int64(maxCount-1))
	rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
}

func acquireRedisConcurrencySlot(ctx context.Context, rdb *redis.Client, key string, maxCount int) (concurrencyReleaseFunc, bool, error) {
	if maxCount <= 0 {
		return func() {}, true, nil
	}

	slot := common.GetUUID()
	now := time.Now().Unix()
	result, err := redisAcquireConcurrencySlotScript.Run(ctx, rdb, []string{key}, slot, now, concurrencySlotTTLSeconds, maxCount).Int()
	if err != nil {
		return nil, false, err
	}
	if result != 1 {
		return nil, false, nil
	}

	done := make(chan struct{})
	var once sync.Once
	release := func() {
		once.Do(func() {
			close(done)
			releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = rdb.ZRem(releaseCtx, key, slot).Err()
		})
	}

	go func() {
		ticker := time.NewTicker(time.Duration(concurrencySlotRefreshSeconds) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				refreshCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = redisRefreshConcurrencySlotScript.Run(refreshCtx, rdb, []string{key}, slot, time.Now().Unix(), concurrencySlotTTLSeconds).Err()
				cancel()
			case <-done:
				return
			}
		}
	}()

	return release, true, nil
}

func acquireConcurrencySlot(identity string, maxCount int) (concurrencyReleaseFunc, bool, error) {
	if maxCount <= 0 {
		return func() {}, true, nil
	}
	key := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitConcurrencyMark, identity)
	if common.RedisEnabled {
		return acquireRedisConcurrencySlot(context.Background(), common.RDB, key, maxCount)
	}
	release, allowed := inMemoryConcurrencyLimiter.Acquire(key, maxCount)
	return release, allowed, nil
}

type rateLimitLayer struct {
	identity            string
	totalMaxCount       int
	successMaxCount     int
	concurrencyMaxCount int
}

func (l rateLimitLayer) successRedisKey() string {
	return fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, l.identity)
}

func (l rateLimitLayer) totalRedisKey() string {
	return fmt.Sprintf("rateLimit:%s", l.identity)
}

func (l rateLimitLayer) successMemoryKey() string {
	return ModelRequestRateLimitSuccessCountMark + l.identity
}

func (l rateLimitLayer) totalMemoryKey() string {
	return ModelRequestRateLimitCountMark + l.identity
}

// Redis限流：对多层 identity 依次检查，全部通过后执行请求并记录成功
func redisRateLimitHandler(duration int64, layers []rateLimitLayer) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()
		rdb := common.RDB
		releases := make([]concurrencyReleaseFunc, 0, len(layers))
		defer func() {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
		}()

		for _, layer := range layers {
			successKey := layer.successRedisKey()
			allowed, err := checkRedisRateLimit(ctx, rdb, successKey, layer.successMaxCount, duration)
			if err != nil {
				fmt.Println("检查成功请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}
			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, layer.successMaxCount))
				return
			}

			if layer.totalMaxCount > 0 {
				totalKey := layer.totalRedisKey()
				tb := limiter.New(ctx, rdb)
				allowed, err = tb.Allow(
					ctx,
					totalKey,
					limiter.WithCapacity(int64(layer.totalMaxCount)*duration),
					limiter.WithRate(int64(layer.totalMaxCount)),
					limiter.WithRequested(duration),
				)
				if err != nil {
					fmt.Println("检查总请求数限制失败:", err.Error())
					abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
					return
				}
				if !allowed {
					abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, layer.totalMaxCount))
					return
				}
			}

			release, allowed, err := acquireConcurrencySlot(layer.identity, layer.concurrencyMaxCount)
			if err != nil {
				fmt.Println("检查并发请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}
			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到并发请求数限制：最多同时处理%d个请求", layer.concurrencyMaxCount))
				return
			}
			releases = append(releases, release)
		}

		c.Next()

		if c.Writer.Status() < 400 {
			for _, layer := range layers {
				recordRedisRequest(ctx, rdb, layer.successRedisKey(), layer.successMaxCount)
			}
		}
	}
}

// 内存限流：对多层 identity 依次检查
func memoryRateLimitHandler(duration int64, layers []rateLimitLayer) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		releases := make([]concurrencyReleaseFunc, 0, len(layers))
		defer func() {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
		}()

		for _, layer := range layers {
			if layer.totalMaxCount > 0 && !inMemoryRateLimiter.Request(layer.totalMemoryKey(), layer.totalMaxCount, duration) {
				c.Status(http.StatusTooManyRequests)
				c.Abort()
				return
			}

			// 使用临时 key 检查成功请求限制，避免实际记录
			checkKey := layer.successMemoryKey() + "_check"
			if layer.successMaxCount > 0 && !inMemoryRateLimiter.Request(checkKey, layer.successMaxCount, duration) {
				c.Status(http.StatusTooManyRequests)
				c.Abort()
				return
			}

			release, allowed, err := acquireConcurrencySlot(layer.identity, layer.concurrencyMaxCount)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}
			if !allowed {
				c.Status(http.StatusTooManyRequests)
				c.Abort()
				return
			}
			releases = append(releases, release)
		}

		c.Next()

		if c.Writer.Status() < 400 {
			for _, layer := range layers {
				if layer.successMaxCount > 0 {
					inMemoryRateLimiter.Request(layer.successMemoryKey(), layer.successMaxCount, duration)
				}
			}
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
// 支持用户级（全局/分组）与 Token 级双层限流，取更严（两层都检查）。
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		tokenRateLimitEnabled := c.GetBool("token_rate_limit_enabled")
		globalEnabled := setting.ModelRequestRateLimitEnabled

		if !globalEnabled && !tokenRateLimitEnabled {
			c.Next()
			return
		}

		userID := c.GetInt("id")
		bypassUser := shouldBypassModelRequestRateLimit(userID)

		// 用户级可绕过，且 Token 未启用自定义限流时，整段放行
		if bypassUser && !tokenRateLimitEnabled {
			c.Header("X-RateLimit-Bypass", "ModelRequestRateLimit")
			c.Next()
			return
		}

		durationMinutes := setting.ModelRequestRateLimitDurationMinutes
		if durationMinutes <= 0 {
			durationMinutes = 1
		}
		duration := int64(durationMinutes * 60)

		// 用户/分组默认参数
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount
		concurrencyMaxCount := setting.ModelRequestRateLimitConcurrencyCount

		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}
		groupTotalCount, groupSuccessCount, groupConcurrencyCount, found := setting.GetGroupRateLimit(group)
		if found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
			concurrencyMaxCount = groupConcurrencyCount
		}

		layers := make([]rateLimitLayer, 0, 2)

		// Token 层：按 token_id 独立计数
		if tokenRateLimitEnabled {
			tokenID := c.GetInt("token_id")
			tokenTotal := c.GetInt("token_rate_limit_total")
			tokenSuccess := c.GetInt("token_rate_limit_success")
			tokenConcurrency := c.GetInt("token_rate_limit_concurrency")
			// success=0 时回退到全局/分组成功请求上限，避免「启用了却无限」
			if tokenSuccess <= 0 {
				tokenSuccess = successMaxCount
				if tokenSuccess <= 0 {
					tokenSuccess = 1000
				}
			}
			layers = append(layers, rateLimitLayer{
				identity:            fmt.Sprintf("t:%d", tokenID),
				totalMaxCount:       tokenTotal,
				successMaxCount:     tokenSuccess,
				concurrencyMaxCount: tokenConcurrency,
			})
		}

		// 用户层：全局限流开启且用户未豁免时生效
		if globalEnabled && !bypassUser {
			layers = append(layers, rateLimitLayer{
				identity:            strconv.Itoa(userID),
				totalMaxCount:       totalMaxCount,
				successMaxCount:     successMaxCount,
				concurrencyMaxCount: concurrencyMaxCount,
			})
		}

		if len(layers) == 0 {
			c.Next()
			return
		}

		if common.RedisEnabled {
			redisRateLimitHandler(duration, layers)(c)
		} else {
			memoryRateLimitHandler(duration, layers)(c)
		}
	}
}

// shouldBypassModelRequestRateLimit 仅按豁免用户 ID 判断；管理员角色不再自动绕过。
// Token 启用自定义限流时，调用方不会使用本函数的绕过结果。
func shouldBypassModelRequestRateLimit(userID int) bool {
	return setting.IsModelRequestRateLimitExemptUser(userID)
}

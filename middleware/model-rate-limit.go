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

func acquireConcurrencySlot(userId string, maxCount int) (concurrencyReleaseFunc, bool, error) {
	if maxCount <= 0 {
		return func() {}, true, nil
	}
	key := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitConcurrencyMark, userId)
	if common.RedisEnabled {
		return acquireRedisConcurrencySlot(context.Background(), common.RDB, key, maxCount)
	}
	release, allowed := inMemoryConcurrencyLimiter.Acquire(key, maxCount)
	return release, allowed, nil
}

// Redis限流处理器
func redisRateLimitHandler(duration int64, totalMaxCount, successMaxCount, concurrencyMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		ctx := context.Background()
		rdb := common.RDB

		// 1. 检查成功请求数限制
		successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, userId)
		allowed, err := checkRedisRateLimit(ctx, rdb, successKey, successMaxCount, duration)
		if err != nil {
			fmt.Println("检查成功请求数限制失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, successMaxCount))
			return
		}

		//2.检查总请求数限制并记录总请求（当totalMaxCount为0时会自动跳过，使用令牌桶限流器
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s", userId)
			// 初始化
			tb := limiter.New(ctx, rdb)
			allowed, err = tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(int64(totalMaxCount)*duration),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(duration),
			)

			if err != nil {
				fmt.Println("检查总请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}

			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, totalMaxCount))
				return
			}
		}

		release, allowed, err := acquireConcurrencySlot(userId, concurrencyMaxCount)
		if err != nil {
			fmt.Println("检查并发请求数限制失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到并发请求数限制：最多同时处理%d个请求", concurrencyMaxCount))
			return
		}
		defer release()

		// 4. 处理请求
		c.Next()

		// 5. 如果请求成功，记录成功请求
		if c.Writer.Status() < 400 {
			recordRedisRequest(ctx, rdb, successKey, successMaxCount)
		}
	}
}

// 内存限流处理器
func memoryRateLimitHandler(duration int64, totalMaxCount, successMaxCount, concurrencyMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)

	return func(c *gin.Context) {
		userId := strconv.Itoa(c.GetInt("id"))
		totalKey := ModelRequestRateLimitCountMark + userId
		successKey := ModelRequestRateLimitSuccessCountMark + userId

		// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 2. 检查成功请求数限制
		// 使用一个临时key来检查限制，这样可以避免实际记录
		checkKey := successKey + "_check"
		if !inMemoryRateLimiter.Request(checkKey, successMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		release, allowed, err := acquireConcurrencySlot(userId, concurrencyMaxCount)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}
		defer release()

		// 3. 处理请求
		c.Next()

		// 4. 如果请求成功，记录到实际的成功请求计数中
		if c.Writer.Status() < 400 {
			inMemoryRateLimiter.Request(successKey, successMaxCount, duration)
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		userID := c.GetInt("id")
		if shouldBypassModelRequestRateLimit(c, userID) {
			c.Header("X-RateLimit-Bypass", "ModelRequestRateLimit")
			c.Next()
			return
		}

		// 计算限流参数
		duration := int64(setting.ModelRequestRateLimitDurationMinutes * 60)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount
		concurrencyMaxCount := setting.ModelRequestRateLimitConcurrencyCount

		// 获取分组
		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}

		//获取分组的限流配置
		groupTotalCount, groupSuccessCount, groupConcurrencyCount, found := setting.GetGroupRateLimit(group)
		if found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
			concurrencyMaxCount = groupConcurrencyCount
		}

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(duration, totalMaxCount, successMaxCount, concurrencyMaxCount)(c)
		} else {
			memoryRateLimitHandler(duration, totalMaxCount, successMaxCount, concurrencyMaxCount)(c)
		}
	}
}

func shouldBypassModelRequestRateLimit(c *gin.Context, userID int) bool {
	if c.GetInt("role") >= common.RoleAdminUser {
		return true
	}
	return setting.IsModelRequestRateLimitExemptUser(userID)
}

package middleware

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

type RateLimitMetric struct {
	Used  *int64 `json:"used"`
	Limit int    `json:"limit"`
}

type GroupRateLimitUsage struct {
	Group       string          `json:"group"`
	Total       RateLimitMetric `json:"total"`
	Success     RateLimitMetric `json:"success"`
	Concurrency RateLimitMetric `json:"concurrency"`
}

type ModelRateLimitUsage struct {
	Enabled       bool                  `json:"enabled"`
	Exempt        bool                  `json:"exempt"`
	WindowMinutes int                   `json:"window_minutes"`
	TotalMode     string                `json:"total_mode"`
	Groups        []GroupRateLimitUsage `json:"groups"`
}

// GetModelRateLimitUsage reads the same user-level buckets used for admission.
// Groups share counters, but each group's limit (and Redis refill rate) differs.
// Disabled metrics have no counter and must not be presented as zero usage.
func GetModelRateLimitUsage(ctx context.Context, userID int, groups []string) (*ModelRateLimitUsage, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("invalid rate limit user")
	}
	result := &ModelRateLimitUsage{
		Enabled:       setting.ModelRequestRateLimitEnabled,
		Exempt:        shouldBypassModelRequestRateLimit(userID),
		WindowMinutes: max(1, setting.ModelRequestRateLimitDurationMinutes),
		TotalMode:     "sliding_window",
		Groups:        make([]GroupRateLimitUsage, 0, len(groups)),
	}
	if common.RedisEnabled {
		result.TotalMode = "token_bucket"
	}
	if !result.Enabled || result.Exempt {
		return result, nil
	}

	groups = slices.Clone(groups)
	slices.Sort(groups)
	for _, group := range slices.Compact(groups) {
		total, success, concurrency, found := setting.GetGroupRateLimit(group)
		if !found {
			total = setting.ModelRequestRateLimitCount
			success = setting.ModelRequestRateLimitSuccessCount
			concurrency = setting.ModelRequestRateLimitConcurrencyCount
		}
		result.Groups = append(result.Groups, GroupRateLimitUsage{
			Group: group,
			Total: RateLimitMetric{Limit: total}, Success: RateLimitMetric{Limit: success},
			Concurrency: RateLimitMetric{Limit: concurrency},
		})
	}
	if len(result.Groups) == 0 {
		return result, nil
	}

	layer := rateLimitLayer{identity: strconv.Itoa(userID)}
	duration := rateLimitDurationSeconds(result.WindowMinutes)
	concurrencyKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitConcurrencyMark, layer.identity)
	var totalUsed, successUsed, concurrencyUsed int64
	var bucketTokens, bucketTime float64
	var hasBucket bool
	now := time.Now()
	if common.RedisEnabled {
		// One pipeline reads shared counters once, regardless of the group count.
		pipe := common.RDB.Pipeline()
		defer pipe.Close()
		clock := pipe.Time(ctx)
		bucket := pipe.HMGet(ctx, layer.totalRedisKey(), "tokens", "last_time")
		successes := pipe.LRange(ctx, layer.successRedisKey(), 0, -1)
		concurrency := pipe.ZCount(ctx, concurrencyKey,
			"("+strconv.FormatInt(now.Unix()-concurrencySlotTTLSeconds, 10), "+inf")
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, err
		}
		values := bucket.Val()
		if values[0] != nil && values[1] != nil {
			var err error
			bucketTokens, err = strconv.ParseFloat(fmt.Sprint(values[0]), 64)
			if err != nil || math.IsNaN(bucketTokens) || math.IsInf(bucketTokens, 0) {
				return nil, fmt.Errorf("invalid rate limit bucket tokens")
			}
			bucketTime, err = strconv.ParseFloat(fmt.Sprint(values[1]), 64)
			if err != nil || math.IsNaN(bucketTime) || math.IsInf(bucketTime, 0) {
				return nil, fmt.Errorf("invalid rate limit bucket time")
			}
			hasBucket = true
		}
		for _, value := range successes.Val() {
			timestamp, err := time.Parse(modelRateLimitTimeFormat, value)
			if err != nil {
				return nil, err
			}
			if int64(now.Sub(timestamp).Seconds()) < duration {
				successUsed++
			}
		}
		concurrencyUsed = concurrency.Val()
		now = clock.Val()
	} else {
		totalUsed = inMemoryRateLimiter.Usage(layer.totalMemoryKey(), duration)
		successUsed = inMemoryRateLimiter.Usage(layer.successMemoryKey(), duration)
		inMemoryConcurrencyLimiter.mu.Lock()
		concurrencyUsed = int64(len(inMemoryConcurrencyLimiter.slots[concurrencyKey]))
		inMemoryConcurrencyLimiter.mu.Unlock()
	}

	for i := range result.Groups {
		group := &result.Groups[i]
		if group.Total.Limit > 0 {
			used := totalUsed
			if common.RedisEnabled && hasBucket {
				capacity := float64(rateLimitCapacity(group.Total.Limit, duration))
				available := min(capacity, bucketTokens+(float64(now.Unix())-bucketTime)*float64(group.Total.Limit))
				// Report occupied request slots, rounding up fractional refill.
				used = int64(math.Ceil(max(0, capacity-available) / float64(duration)))
			}
			group.Total.Used = &used
		}
		if group.Success.Limit > 0 {
			group.Success.Used = &successUsed
		}
		if group.Concurrency.Limit > 0 {
			group.Concurrency.Used = &concurrencyUsed
		}
	}
	return result, nil
}

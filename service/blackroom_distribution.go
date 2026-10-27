package service

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/ipgeo"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	blackroomDistributionPrefix       = "blackroom:distribution:v1:"
	blackroomDistributionWindow       = int64(600)
	blackroomDistributionPromptWindow = int64(60)
	blackroomDistributionMaxTokens    = 2000
)

type BlackroomPromptSummary struct {
	SampleCount   int64   `json:"sample_count"`
	UniqueCount   int     `json:"unique_count"`
	DominantShare float64 `json:"dominant_share"`
}

type BlackroomDistributionToken struct {
	TokenID         int                    `json:"token_id"`
	TokenName       string                 `json:"token_name"`
	RequestCount    int64                  `json:"request_count"`
	DefaultGoCount  int64                  `json:"default_go_count"`
	HostingGoCount  int64                  `json:"hosting_go_count"`
	GeoSampleCount  int64                  `json:"geo_sample_count"`
	SystemPrompt    BlackroomPromptSummary `json:"system_prompt"`
	FirstUserPrompt BlackroomPromptSummary `json:"first_user_prompt"`
	Subnets         []string               `json:"subnets"`
	Countries       []string               `json:"countries"`
	ASNs            []int64                `json:"asns"`
	PromptLimited   bool                   `json:"prompt_limited"`
	NetworkLimited  bool                   `json:"network_limited"`
	Score           int                    `json:"score"`
	Reasons         []string               `json:"reasons"`
	FirstSeenAt     int64                  `json:"first_seen_at"`
	LastSeenAt      int64                  `json:"last_seen_at"`
}

type BlackroomDistributionItem struct {
	UserID       int                          `json:"user_id"`
	Username     string                       `json:"username"`
	Score        int                          `json:"score"`
	RequestCount int64                        `json:"request_count"`
	LastSeenAt   int64                        `json:"last_seen_at"`
	Reasons      []string                     `json:"reasons"`
	Tokens       []BlackroomDistributionToken `json:"tokens"`
}

type BlackroomDistributionResult struct {
	Available           bool                        `json:"available"`
	UnavailableReason   string                      `json:"unavailable_reason,omitempty"`
	WindowSeconds       int64                       `json:"window_seconds"`
	PromptWindowSeconds int64                       `json:"prompt_window_seconds"`
	ObservedAt          int64                       `json:"observed_at"`
	ScopeLimited        bool                        `json:"scope_limited"`
	Total               int                         `json:"total"`
	Page                int                         `json:"page"`
	PageSize            int                         `json:"page_size"`
	Items               []BlackroomDistributionItem `json:"items"`
}

type blackroomDistributionObservation struct {
	UserID, TokenID           int
	Username, TokenName       string
	SystemHash, FirstUserHash string
	Subnet, Country, ASN      string
	DefaultGo, HostingGo, Geo bool
	ObservedAt                int64
}

var blackroomDistributionLastError atomic.Int64

// Counts use one bucket per second, including repeated occurrences of the same
// prompt. Distinct hashes alone cannot establish the dominant prompt share.
// All trimming and increments are atomic. Limits bound attacker-controlled
// cardinality; incomplete prompt/network evidence never contributes to scores.
var blackroomDistributionRecord = redis.NewScript(`
local now = tonumber(ARGV[1])
local window = tonumber(ARGV[13])
local promptWindow = tonumber(ARGV[14])
local ttl = window + 60
local second = tostring(now)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
redis.call('ZADD', KEYS[1], now, ARGV[2])
redis.call('HSET', KEYS[2], 'username', ARGV[3], 'token_name', ARGV[4])

local expired = redis.call('ZRANGEBYSCORE', KEYS[3], '-inf', now - window)
for _, bucket in ipairs(expired) do
  redis.call('HDEL', KEYS[4], bucket .. ':r', bucket .. ':d', bucket .. ':h', bucket .. ':g')
end
redis.call('ZREMRANGEBYSCORE', KEYS[3], '-inf', now - window)
redis.call('ZADD', KEYS[3], now, second)
redis.call('HINCRBY', KEYS[4], second .. ':r', 1)
for i, kind in ipairs({'d', 'h', 'g'}) do
  if ARGV[9 + i] == 'true' then
    redis.call('HINCRBY', KEYS[4], second .. ':' .. kind, 1)
  end
end

expired = redis.call('ZRANGEBYSCORE', KEYS[5], '-inf', now - promptWindow)
for _, field in ipairs(expired) do redis.call('HDEL', KEYS[6], field) end
redis.call('ZREMRANGEBYSCORE', KEYS[5], '-inf', now - promptWindow)
for i, kind in ipairs({'s', 'u'}) do
  local fingerprint = ARGV[4 + i]
  if fingerprint ~= '' then
    local field = second .. ':' .. kind .. ':' .. fingerprint
    redis.call('ZADD', KEYS[5], now, field)
    redis.call('HINCRBY', KEYS[6], field, 1)
  end
end
local overflow = redis.call('ZCARD', KEYS[5]) - 2048
if overflow > 0 then
  local removed = redis.call('ZRANGE', KEYS[5], 0, overflow - 1)
  for _, field in ipairs(removed) do redis.call('HDEL', KEYS[6], field) end
  redis.call('ZREMRANGEBYRANK', KEYS[5], 0, overflow - 1)
  redis.call('HSET', KEYS[2], 'prompt_limited_until', now + promptWindow)
end

redis.call('ZREMRANGEBYSCORE', KEYS[7], '-inf', now - window)
for i, kind in ipairs({'n', 'c', 'a'}) do
  if ARGV[6 + i] ~= '' then
    redis.call('ZADD', KEYS[7], now, kind .. ':' .. ARGV[6 + i])
  end
end
overflow = redis.call('ZCARD', KEYS[7]) - 256
if overflow > 0 then
  redis.call('ZREMRANGEBYRANK', KEYS[7], 0, overflow - 1)
  redis.call('HSET', KEYS[2], 'network_limited_until', now + window)
end
for i, key in ipairs(KEYS) do
  local expiry = ttl
  if i == 5 or i == 6 then expiry = promptWindow + 10 end
  redis.call('EXPIRE', key, expiry)
end
return 1
`)

// ObserveBlackroomDistribution observes the original, validated client request
// once, before provider conversions/retries. It has no enforcement side effects.
// Never persist prompt text, authentication headers, or usable API credentials.
func ObserveBlackroomDistribution(c *gin.Context, request dto.Request) {
	userID := c.GetInt("id")
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	if userID <= 0 || tokenID <= 0 || !operation_setting.GetBlackroomSetting().Enabled || !common.RedisEnabled || common.RDB == nil {
		return
	}
	if observed, _ := c.Get("blackroom_distribution_observed"); observed == true {
		return
	}
	c.Set("blackroom_distribution_observed", true)
	system, firstUser := blackroomDistributionPrompts(request)
	lookup := ipgeo.LookupDefault(c.ClientIP())
	observation := blackroomDistributionObservation{
		UserID:        userID,
		TokenID:       tokenID,
		Username:      c.GetString("username"),
		TokenName:     c.GetString("token_name"),
		SystemHash:    blackroomPromptFingerprint(userID, tokenID, "system", system),
		FirstUserHash: blackroomPromptFingerprint(userID, tokenID, "first_user", firstUser),
		DefaultGo:     strings.EqualFold(strings.TrimSpace(c.Request.UserAgent()), "Go-http-client/1.1"),
		ObservedAt:    time.Now().Unix(),
	}
	if lookup.Kind == ipgeo.KindPublic {
		address, err := netip.ParseAddr(lookup.Ip)
		if err == nil {
			bits := 64
			if address.Is4() {
				bits = 24
			}
			observation.Subnet = netip.PrefixFrom(address, bits).Masked().String()
		}
		if lookup.EvidenceEligible {
			observation.Country = lookup.CountryISO
			observation.ASN = strconv.FormatInt(lookup.AsnNumber, 10)
			observation.Geo = true
			observation.HostingGo = observation.DefaultGo && blackroomHostingASNHint(lookup.AsnOrganization)
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 200*time.Millisecond)
	defer cancel()
	if err := recordBlackroomDistribution(ctx, observation); err != nil && c.Request.Context().Err() == nil {
		last := blackroomDistributionLastError.Load()
		if observation.ObservedAt-last >= 60 && blackroomDistributionLastError.CompareAndSwap(last, observation.ObservedAt) {
			common.SysError("blackroom distribution observation unavailable; relay continues")
		}
	}
}

func recordBlackroomDistribution(ctx context.Context, observation blackroomDistributionObservation) error {
	member := strconv.Itoa(observation.UserID) + ":" + strconv.Itoa(observation.TokenID)
	prefix := blackroomDistributionPrefix + member + ":"
	_, err := blackroomDistributionRecord.Run(ctx, common.RDB, []string{
		blackroomDistributionPrefix + "active",
		prefix + "meta", prefix + "buckets", prefix + "counts",
		prefix + "prompt_buckets", prefix + "prompts", prefix + "networks",
	}, observation.ObservedAt, member, observation.Username, observation.TokenName,
		observation.SystemHash, observation.FirstUserHash, observation.Subnet, observation.Country, observation.ASN,
		strconv.FormatBool(observation.DefaultGo), strconv.FormatBool(observation.HostingGo), strconv.FormatBool(observation.Geo),
		blackroomDistributionWindow, blackroomDistributionPromptWindow).Result()
	return err
}

// This is an ASN organization-name hint, not a verified hosting classification.
// It is deliberately a low-weight signal, and is only paired with default Go UA.
func blackroomHostingASNHint(organization string) bool {
	organization = strings.ToLower(organization)
	for _, name := range []string{"amazon", "microsoft", "digitalocean", "hetzner", "ovh", "cloudflare", "alibaba", "google cloud", "tencent cloud", "oracle", "vultr", "linode", "choopa", "contabo", "leaseweb", "hosting"} {
		if strings.Contains(organization, name) {
			return true
		}
	}
	return false
}

// ListBlackroomDistribution is a Redis-only observation view. No schema, log
// database query, quota accounting, or automatic ban is involved.
func ListBlackroomDistribution(ctx context.Context, filter string, page, pageSize int, now int64) (*BlackroomDistributionResult, error) {
	result := &BlackroomDistributionResult{
		WindowSeconds: blackroomDistributionWindow, PromptWindowSeconds: blackroomDistributionPromptWindow,
		ObservedAt: now, Page: max(1, page), PageSize: min(100, max(1, pageSize)),
		Items: []BlackroomDistributionItem{},
	}
	if !operation_setting.GetBlackroomSetting().Enabled {
		result.UnavailableReason = "blackroom_disabled"
		return result, nil
	}
	if !common.RedisEnabled || common.RDB == nil {
		result.UnavailableReason = "redis_disabled"
		return result, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	active, err := common.RDB.ZRevRangeByScore(ctx, blackroomDistributionPrefix+"active", &redis.ZRangeBy{
		Min: "(" + strconv.FormatInt(now-blackroomDistributionWindow, 10), Max: strconv.FormatInt(now, 10),
		Count: blackroomDistributionMaxTokens + 1,
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("distribution observations unavailable: %w", err)
	}
	result.Available = true
	if len(active) > blackroomDistributionMaxTokens {
		result.ScopeLimited = true
		active = active[:blackroomDistributionMaxTokens]
	}
	users := make(map[int]*BlackroomDistributionItem)
	// Bound each pipeline response rather than requesting every active hash at once.
	for offset := 0; offset < len(active); offset += 100 {
		members := active[offset:min(offset+100, len(active))]
		pipeline := common.RDB.Pipeline()
		metas := make([]*redis.StringStringMapCmd, len(members))
		counts := make([]*redis.StringStringMapCmd, len(members))
		prompts := make([]*redis.StringStringMapCmd, len(members))
		networks := make([]*redis.StringSliceCmd, len(members))
		for i, member := range members {
			prefix := blackroomDistributionPrefix + member + ":"
			metas[i] = pipeline.HGetAll(ctx, prefix+"meta")
			counts[i] = pipeline.HGetAll(ctx, prefix+"counts")
			prompts[i] = pipeline.HGetAll(ctx, prefix+"prompts")
			networks[i] = pipeline.ZRangeByScore(ctx, prefix+"networks", &redis.ZRangeBy{
				Min: "(" + strconv.FormatInt(now-blackroomDistributionWindow, 10), Max: strconv.FormatInt(now, 10),
			})
		}
		if _, err := pipeline.Exec(ctx); err != nil {
			return nil, fmt.Errorf("distribution observations unavailable: %w", err)
		}
		for i, member := range members {
			userPart, tokenPart, ok := strings.Cut(member, ":")
			userID, _ := strconv.Atoi(userPart)
			tokenID, _ := strconv.Atoi(tokenPart)
			if !ok || userID <= 0 || tokenID <= 0 {
				continue
			}
			token := summarizeBlackroomDistributionToken(tokenID, metas[i].Val(), counts[i].Val(), prompts[i].Val(), networks[i].Val(), now)
			if token.RequestCount == 0 {
				continue
			}
			user := users[userID]
			if user == nil {
				user = &BlackroomDistributionItem{UserID: userID, Reasons: []string{}, Tokens: []BlackroomDistributionToken{}}
				users[userID] = user
			}
			if token.LastSeenAt >= user.LastSeenAt {
				user.Username = metas[i].Val()["username"]
			}
			user.LastSeenAt = max(user.LastSeenAt, token.LastSeenAt)
			user.Score = max(user.Score, token.Score)
			user.RequestCount += token.RequestCount
			user.Tokens = append(user.Tokens, token)
			for _, reason := range token.Reasons {
				if !slices.Contains(user.Reasons, reason) {
					user.Reasons = append(user.Reasons, reason)
				}
			}
		}
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	filterID, _ := strconv.Atoi(filter)
	for _, user := range users {
		if filter != "" && user.UserID != filterID && !strings.Contains(strings.ToLower(user.Username), filter) {
			continue
		}
		slices.SortFunc(user.Tokens, func(a, b BlackroomDistributionToken) int {
			if a.Score != b.Score {
				return b.Score - a.Score
			}
			return a.TokenID - b.TokenID
		})
		slices.Sort(user.Reasons)
		result.Items = append(result.Items, *user)
	}
	slices.SortFunc(result.Items, func(a, b BlackroomDistributionItem) int {
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		if a.RequestCount > b.RequestCount {
			return -1
		}
		if a.RequestCount < b.RequestCount {
			return 1
		}
		return a.UserID - b.UserID
	})
	result.Total = len(result.Items)
	pageCount := max(1, (result.Total+result.PageSize-1)/result.PageSize)
	result.Page = min(result.Page, pageCount)
	start := (result.Page - 1) * result.PageSize
	result.Items = result.Items[start:min(start+result.PageSize, result.Total)]
	return result, nil
}

func summarizeBlackroomDistributionToken(tokenID int, meta, counts, prompts map[string]string, networks []string, now int64) BlackroomDistributionToken {
	token := BlackroomDistributionToken{
		TokenID: tokenID, TokenName: meta["token_name"],
		Reasons: []string{}, Subnets: []string{}, Countries: []string{}, ASNs: []int64{},
	}
	for field, value := range counts {
		second, kind, ok := strings.Cut(field, ":")
		observedAt, _ := strconv.ParseInt(second, 10, 64)
		count, _ := strconv.ParseInt(value, 10, 64)
		if !ok || observedAt <= now-blackroomDistributionWindow || observedAt > now || count <= 0 {
			continue
		}
		switch kind {
		case "r":
			token.RequestCount += count
			token.LastSeenAt = max(token.LastSeenAt, observedAt)
			if token.FirstSeenAt == 0 || observedAt < token.FirstSeenAt {
				token.FirstSeenAt = observedAt
			}
		case "d":
			token.DefaultGoCount += count
		case "h":
			token.HostingGoCount += count
		case "g":
			token.GeoSampleCount += count
		}
	}
	systemCounts := make(map[string]int64)
	userCounts := make(map[string]int64)
	for field, value := range prompts {
		second, fingerprint, ok := strings.Cut(field, ":")
		observedAt, _ := strconv.ParseInt(second, 10, 64)
		count, _ := strconv.ParseInt(value, 10, 64)
		if !ok || observedAt <= now-blackroomDistributionPromptWindow || observedAt > now || count <= 0 {
			continue
		}
		kind, hash, ok := strings.Cut(fingerprint, ":")
		if !ok || hash == "" {
			continue
		}
		if kind == "s" {
			systemCounts[hash] += count
		} else if kind == "u" {
			userCounts[hash] += count
		}
	}
	token.SystemPrompt = summarizeBlackroomPrompt(systemCounts)
	token.FirstUserPrompt = summarizeBlackroomPrompt(userCounts)
	for _, network := range networks {
		kind, value, ok := strings.Cut(network, ":")
		if !ok {
			continue
		}
		switch kind {
		case "n":
			token.Subnets = append(token.Subnets, value)
		case "c":
			token.Countries = append(token.Countries, value)
		case "a":
			asn, _ := strconv.ParseInt(value, 10, 64)
			if asn > 0 {
				token.ASNs = append(token.ASNs, asn)
			}
		}
	}
	slices.Sort(token.Subnets)
	slices.Sort(token.Countries)
	slices.Sort(token.ASNs)
	promptLimitedUntil, _ := strconv.ParseInt(meta["prompt_limited_until"], 10, 64)
	networkLimitedUntil, _ := strconv.ParseInt(meta["network_limited_until"], 10, 64)
	token.PromptLimited = promptLimitedUntil > now
	token.NetworkLimited = networkLimitedUntil > now
	scoreBlackroomDistributionToken(&token)
	return token
}

func summarizeBlackroomPrompt(counts map[string]int64) BlackroomPromptSummary {
	summary := BlackroomPromptSummary{UniqueCount: len(counts)}
	var dominant int64
	for _, count := range counts {
		summary.SampleCount += count
		dominant = max(dominant, count)
	}
	if summary.SampleCount > 0 {
		summary.DominantShare = float64(dominant) / float64(summary.SampleCount)
	}
	return summary
}

func scoreBlackroomDistributionToken(token *BlackroomDistributionToken) {
	if !token.PromptLimited {
		if token.FirstUserPrompt.SampleCount >= 8 && token.FirstUserPrompt.UniqueCount >= 5 && token.FirstUserPrompt.DominantShare < 0.6 {
			token.Score += 20
			token.Reasons = append(token.Reasons, "first_prompt_diversity")
		}
		if token.SystemPrompt.SampleCount >= 8 && token.SystemPrompt.UniqueCount >= 4 && token.SystemPrompt.DominantShare < 0.6 {
			token.Score += 25
			token.Reasons = append(token.Reasons, "system_prompt_diversity")
		}
	}
	if !token.NetworkLimited {
		if len(token.Subnets) >= 4 {
			token.Score += 15
			token.Reasons = append(token.Reasons, "subnet_diversity")
		}
		if len(token.Countries) >= 3 && len(token.ASNs) >= 3 {
			token.Score += 20
			token.Reasons = append(token.Reasons, "geo_asn_diversity")
		}
	}
	if token.RequestCount >= 5 && float64(token.DefaultGoCount)/float64(token.RequestCount) >= 0.8 {
		token.Score += 5
		token.Reasons = append(token.Reasons, "default_go_client")
	}
	if token.RequestCount >= 5 && token.HostingGoCount >= 5 && float64(token.HostingGoCount)/float64(token.RequestCount) >= 0.5 {
		token.Score += 15
		token.Reasons = append(token.Reasons, "hosting_go_client")
	}
	token.Score = min(100, token.Score)
}

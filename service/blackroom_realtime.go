package service

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/ipgeo"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"gorm.io/gorm"
)

// blackroomRealtimeCacheTTL 是「未命中」结论的缓存时长。它只用来避免同一
// 用户的连续 relay 请求反复查库；一旦命中封禁，后续请求由鉴权阶段的封禁
// 缓存直接拦截，不会再走到这里。
const blackroomRealtimeCacheTTL = 10 * time.Second

type blackroomRealtimeCacheEntry struct {
	ExpiresAt int64
}

var blackroomRealtimeCache sync.Map

// EvaluateBlackroomRelayRequest 在 relay 请求分发前记录一次 IP 观测，并在
// 开启时执行实时判定。
//
// 观测记录先于判定开关执行：即使关闭了实时判定或处于影子模式，审计数据也
// 必须持续累积，否则管理界面和后续判定都会缺少这段窗口的数据。
//
// 代价是每个 relay 请求多一次 upsert（同一分钟内同一用户的重复请求命中
// 冲突分支，只做计数自增）。不需要该开销时把 blackroom_setting.enabled 置为
// false 即可完全跳过；只保留审计不做自动封禁则关闭 auto_ban_enabled。
func EvaluateBlackroomRelayRequest(userId int, username string, clientIP string) (*BlackroomDecision, error) {
	setting := operation_setting.GetBlackroomSetting()
	if userId <= 0 || !setting.Enabled || model.DB == nil {
		return nil, nil
	}

	lookup := ipgeo.LookupDefault(clientIP)
	if err := model.RecordBlackroomIPObservation(model.BlackroomIPObservation{
		UserId:           userId,
		Username:         username,
		Ip:               lookup.Ip,
		CountryISO:       lookup.CountryISO,
		AsnNumber:        lookup.AsnNumber,
		AsnOrganization:  lookup.AsnOrganization,
		ResolverVersion:  lookup.ResolverVersion,
		IpKind:           lookup.Kind,
		EvidenceEligible: lookup.EvidenceEligible,
		ObservedAt:       time.Now(),
	}); err != nil {
		return nil, fmt.Errorf("记录 IP 观测失败: %w", err)
	}

	if !setting.RealtimeEnabled || !setting.AutoBanEnabled {
		return nil, nil
	}

	now := common.GetTimestamp()
	if blackroomRealtimeCacheFresh(userId, now) {
		return nil, nil
	}

	existing, err := model.GetActiveBlackroomBan(userId)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if existing != nil {
		return &BlackroomDecision{Ban: existing}, nil
	}

	decision, err := evaluateBlackroomUser(setting, userId, username, now, true)
	if err != nil {
		return nil, err
	}
	if decision == nil || decision.Skipped {
		// 未命中时记一次缓存，避免同一用户的连续请求反复查库。
		blackroomRealtimeCache.Store(userId, blackroomRealtimeCacheEntry{
			ExpiresAt: now + int64(blackroomRealtimeCacheTTL/time.Second),
		})
		return nil, nil
	}
	return decision, nil
}

func blackroomRealtimeCacheFresh(userId int, now int64) bool {
	entry, ok := blackroomRealtimeCache.Load(userId)
	if !ok {
		return false
	}
	cacheEntry, ok := entry.(blackroomRealtimeCacheEntry)
	return ok && cacheEntry.ExpiresAt > now
}

// InvalidateBlackroomRealtimeCache 在封禁状态变化后清除判定缓存，让下一次
// relay 请求立即重新判定，而不是沿用旧的「未命中」结论。
func InvalidateBlackroomRealtimeCache(userId int) {
	if userId <= 0 {
		return
	}
	blackroomRealtimeCache.Delete(userId)
}

package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// 小黑屋事件类型。一次封禁的每次变更都会留下独立事件，因此
// BlackroomBan 上被覆盖掉的中间状态仍然可以追溯。
const (
	BlackroomBanEventApply       = "apply"
	BlackroomBanEventReapply     = "reapply"
	BlackroomBanEventExtend      = "extend"
	BlackroomBanEventRelease     = "release"
	BlackroomBanEventExpire      = "expire"
	BlackroomBanEventShadowMatch = "shadow_match"
)

var ErrBlackroomBanEventImmutable = errors.New("小黑屋事件不可修改")

// BlackroomBanEvent 是一条只追加的封禁事件。
//
// 状态表 BlackroomBan 上，同一用户的再次命中会覆盖 reason / evidence /
// ip_list，只看状态表无法回答「第一次为什么封、第二次为什么延长」。
// 事件表为每次变更留下不可变快照，BanId 为 0 表示该事件没有对应的封禁
// （目前只有影子模式命中）。
type BlackroomBanEvent struct {
	Id                 int64  `json:"id" gorm:"primaryKey"`
	BanId              int    `json:"ban_id" gorm:"index"`
	UserId             int    `json:"user_id" gorm:"index;index:idx_blackroom_event_user_time,priority:1"`
	EventType          string `json:"event_type" gorm:"type:varchar(24);index"`
	Source             string `json:"source" gorm:"type:varchar(16);index;default:''"`
	Reason             string `json:"reason" gorm:"type:varchar(255);default:''"`
	Evidence           string `json:"evidence" gorm:"type:text"`
	IpCount            int    `json:"ip_count"`
	IpList             string `json:"ip_list" gorm:"type:text"`
	WindowStart        int64  `json:"window_start" gorm:"bigint"`
	WindowEnd          int64  `json:"window_end" gorm:"bigint"`
	BanDurationSeconds int64  `json:"ban_duration_seconds" gorm:"bigint"`
	BannedUntil        int64  `json:"banned_until" gorm:"bigint"`
	ActorUserId        int    `json:"actor_user_id" gorm:"index;default:0"`
	CreatedAt          int64  `json:"created_at" gorm:"bigint;autoCreateTime;index:idx_blackroom_event_user_time,priority:2"`
}

func (BlackroomBanEvent) TableName() string {
	return "blackroom_ban_events"
}

// 事件是审计凭据，只允许追加。清理需要绕过 GORM 钩子，以避免误删。
func (*BlackroomBanEvent) BeforeUpdate(*gorm.DB) error {
	return ErrBlackroomBanEventImmutable
}

func (*BlackroomBanEvent) BeforeDelete(*gorm.DB) error {
	return ErrBlackroomBanEventImmutable
}

// appendBlackroomBanEvent 在事务内写入一条事件快照。
func appendBlackroomBanEvent(tx *gorm.DB, ban *BlackroomBan, eventType string, actorUserId int) error {
	return tx.Create(&BlackroomBanEvent{
		BanId:              ban.Id,
		UserId:             ban.UserId,
		EventType:          eventType,
		Source:             ban.Source,
		Reason:             ban.Reason,
		Evidence:           ban.Evidence,
		IpCount:            ban.IpCount,
		IpList:             ban.IpList,
		WindowStart:        ban.WindowStart,
		WindowEnd:          ban.WindowEnd,
		BanDurationSeconds: ban.BanDurationSeconds,
		BannedUntil:        ban.BannedUntil,
		ActorUserId:        actorUserId,
		CreatedAt:          common.GetTimestamp(),
	}).Error
}

// RecordBlackroomShadowMatch 记录一次影子模式命中。同一个用户在一分钟内
// 只记一条，避免高频请求把事件表刷满。
func RecordBlackroomShadowMatch(userId int, reason string, evidence string) error {
	if userId <= 0 {
		return nil
	}
	now := common.GetTimestamp()
	var recent int64
	if err := DB.Model(&BlackroomBanEvent{}).
		Where("user_id = ? AND event_type = ? AND created_at > ?", userId, BlackroomBanEventShadowMatch, now-60).
		Count(&recent).Error; err != nil {
		return err
	}
	if recent > 0 {
		return nil
	}
	return DB.Create(&BlackroomBanEvent{
		UserId:    userId,
		EventType: BlackroomBanEventShadowMatch,
		Source:    BlackroomBanSourceAuto,
		Reason:    reason,
		Evidence:  evidence,
		CreatedAt: now,
	}).Error
}

func ListBlackroomBanEvents(userId int, limit int) ([]BlackroomBanEvent, error) {
	if userId <= 0 {
		return []BlackroomBanEvent{}, nil
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	events := make([]BlackroomBanEvent, 0)
	err := DB.Where("user_id = ?", userId).
		Order("created_at desc, id desc").
		Limit(limit).
		Find(&events).Error
	return events, err
}

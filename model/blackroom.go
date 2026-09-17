package model

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	BlackroomBanStatusActive   = "active"
	BlackroomBanStatusReleased = "released"
	BlackroomBanStatusExpired  = "expired"

	BlackroomBanSourceAuto     = "auto"
	BlackroomBanSourceManual   = "manual"
	BlackroomBanSourceExternal = "external"

	blackroomCacheNone = "-"
	blackroomCacheTTL  = 10 * time.Second
)

type BlackroomBan struct {
	Id                 int     `json:"id"`
	UserId             int     `json:"user_id" gorm:"index;index:idx_blackroom_user_status,priority:1"`
	ActiveKey          *string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	Username           string  `json:"username" gorm:"type:varchar(64);index;default:''"`
	Status             string  `json:"status" gorm:"type:varchar(16);index;index:idx_blackroom_user_status,priority:2;default:'active'"`
	Source             string  `json:"source" gorm:"type:varchar(16);index;default:'auto'"`
	Reason             string  `json:"reason" gorm:"type:varchar(255);default:''"`
	Evidence           string  `json:"evidence" gorm:"type:text"`
	IpCount            int     `json:"ip_count" gorm:"index;default:0"`
	IpList             string  `json:"ip_list" gorm:"type:text"`
	WindowStart        int64   `json:"window_start" gorm:"bigint;index;default:0"`
	WindowEnd          int64   `json:"window_end" gorm:"bigint;default:0"`
	BanDurationSeconds int64   `json:"ban_duration_seconds" gorm:"bigint;default:0"`
	BannedUntil        int64   `json:"banned_until" gorm:"bigint;index;default:0"`
	CreatedAt          int64   `json:"created_at" gorm:"bigint;autoCreateTime;index"`
	UpdatedAt          int64   `json:"updated_at" gorm:"bigint;autoUpdateTime"`
	ReleasedAt         int64   `json:"released_at" gorm:"bigint;default:0"`
	ReleasedBy         int     `json:"released_by" gorm:"default:0"`
	ReleaseReason      string  `json:"release_reason" gorm:"type:varchar(255);default:''"`
}

type BlackroomBanInput struct {
	UserId             int
	Username           string
	Source             string
	Reason             string
	Evidence           string
	IpCount            int
	IpList             string
	WindowStart        int64
	WindowEnd          int64
	BanDurationSeconds int64
	BannedUntil        int64
}

// BlackroomIPCandidate 是扫描阶段筛出的候选用户。判定所需的其他数据由
// 共享判定逻辑按用户 ID 重新读取，避免在筛选阶段使用可能过期的快照。
type BlackroomIPCandidate struct {
	UserId int `gorm:"column:user_id" json:"user_id"`
}

type blackroomCacheEntry struct {
	Ban       *BlackroomBan
	ExpiresAt int64
}

var blackroomCache sync.Map

func blackroomBanCacheKey(userID int) string {
	return fmt.Sprintf("blackroom:active:%d", userID)
}

func blackroomActiveKey(userID int) *string {
	if userID <= 0 {
		return nil
	}
	key := strconv.Itoa(userID)
	return &key
}

func (ban *BlackroomBan) IsPermanent() bool {
	return ban != nil && ban.BannedUntil == 0
}

func (ban *BlackroomBan) BlockMessage() string {
	if ban == nil {
		return "账号已进入小黑屋"
	}
	reason := ban.Reason
	if reason == "" {
		reason = "触发风控规则"
	}
	if ban.BannedUntil == 0 {
		return fmt.Sprintf("账号已进入小黑屋：%s，封禁类型：永久", reason)
	}
	return fmt.Sprintf(
		"账号已进入小黑屋：%s，解封时间：%s（北京时间）",
		reason,
		common.FormatInBeijingTimezone(ban.BannedUntil, "2006-01-02 15:04:05"),
	)
}

func ListBlackroomBans(keyword string, status string, source string, userID int, startIdx int, num int) ([]*BlackroomBan, int64, error) {
	tx := DB.Model(&BlackroomBan{})
	if keyword != "" {
		like := "%" + keyword + "%"
		if keywordUserID, err := strconv.Atoi(keyword); err == nil && keywordUserID > 0 {
			tx = tx.Where("username LIKE ? OR reason LIKE ? OR user_id = ?", like, like, keywordUserID)
		} else {
			tx = tx.Where("username LIKE ? OR reason LIKE ?", like, like)
		}
	}
	if status != "" {
		tx = tx.Where("status = ?", status)
	}
	if source != "" {
		tx = tx.Where("source = ?", source)
	}
	if userID > 0 {
		tx = tx.Where("user_id = ?", userID)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var bans []*BlackroomBan
	err := tx.Order("id desc").Limit(num).Offset(startIdx).Find(&bans).Error
	return bans, total, err
}

func GetBlackroomBanByID(id int) (*BlackroomBan, error) {
	if id <= 0 {
		return nil, errors.New("无效的小黑屋记录 ID")
	}
	var ban BlackroomBan
	if err := DB.First(&ban, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &ban, nil
}

func GetActiveBlackroomBan(userID int) (*BlackroomBan, error) {
	if userID <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	now := common.GetTimestamp()
	var ban BlackroomBan
	err := DB.Where("user_id = ? AND status = ? AND (banned_until = 0 OR banned_until > ?)", userID, BlackroomBanStatusActive, now).
		Order("id desc").
		First(&ban).Error
	if err != nil {
		return nil, err
	}
	return &ban, nil
}

func GetActiveBlackroomBanCached(userID int) (*BlackroomBan, error) {
	if userID <= 0 {
		return nil, gorm.ErrRecordNotFound
	}

	now := common.GetTimestamp()
	if common.RedisEnabled && common.RDB != nil {
		cacheValue, err := common.RedisGet(blackroomBanCacheKey(userID))
		if err == nil {
			if cacheValue == blackroomCacheNone {
				return nil, gorm.ErrRecordNotFound
			}
			var ban BlackroomBan
			if err := common.Unmarshal([]byte(cacheValue), &ban); err == nil {
				if ban.BannedUntil == 0 || ban.BannedUntil > now {
					return &ban, nil
				}
			}
		}
	} else if value, ok := blackroomCache.Load(userID); ok {
		if entry, ok := value.(blackroomCacheEntry); ok && entry.ExpiresAt > now {
			if entry.Ban == nil {
				return nil, gorm.ErrRecordNotFound
			}
			return entry.Ban, nil
		}
	}

	ban, err := GetActiveBlackroomBan(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			cacheBlackroomBan(userID, nil)
		}
		return nil, err
	}
	cacheBlackroomBan(userID, ban)
	return ban, nil
}

func cacheBlackroomBan(userID int, ban *BlackroomBan) {
	if userID <= 0 {
		return
	}
	now := common.GetTimestamp()
	expiresAt := now + int64(blackroomCacheTTL/time.Second)
	if common.RedisEnabled && common.RDB != nil {
		value := blackroomCacheNone
		if ban != nil {
			bytes, err := common.Marshal(ban)
			if err != nil {
				return
			}
			value = string(bytes)
		}
		_ = common.RedisSet(blackroomBanCacheKey(userID), value, blackroomCacheTTL)
		return
	}
	blackroomCache.Store(userID, blackroomCacheEntry{Ban: ban, ExpiresAt: expiresAt})
}

func InvalidateBlackroomBanCache(userID int) {
	if userID <= 0 {
		return
	}
	blackroomCache.Delete(userID)
	if common.RedisEnabled && common.RDB != nil {
		_ = common.RedisDelKey(blackroomBanCacheKey(userID))
	}
}

func InvalidateBlackroomUserAuthCache(userID int) {
	InvalidateBlackroomBanCache(userID)
	_ = InvalidateUserCache(userID)
	_ = InvalidateUserTokensCache(userID)
}

func UpsertActiveBlackroomBan(input BlackroomBanInput) (*BlackroomBan, bool, error) {
	if input.UserId <= 0 {
		return nil, false, errors.New("无效的用户 ID")
	}
	if input.Source == "" {
		input.Source = BlackroomBanSourceAuto
	}
	if input.Reason == "" {
		input.Reason = "触发小黑屋规则"
	}

	now := common.GetTimestamp()
	var ban BlackroomBan
	created := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("user_id = ? AND status = ? AND (banned_until = 0 OR banned_until > ?)", input.UserId, BlackroomBanStatusActive, now).
			Order("id desc").
			First(&ban).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err == nil {
			return updateActiveBlackroomBan(tx, &ban, input, now)
		}

		ban = BlackroomBan{
			UserId:             input.UserId,
			ActiveKey:          blackroomActiveKey(input.UserId),
			Username:           input.Username,
			Status:             BlackroomBanStatusActive,
			Source:             input.Source,
			Reason:             input.Reason,
			Evidence:           input.Evidence,
			IpCount:            input.IpCount,
			IpList:             input.IpList,
			WindowStart:        input.WindowStart,
			WindowEnd:          input.WindowEnd,
			BanDurationSeconds: input.BanDurationSeconds,
			BannedUntil:        input.BannedUntil,
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ban)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if err := tx.Where("user_id = ? AND status = ? AND (banned_until = 0 OR banned_until > ?)", input.UserId, BlackroomBanStatusActive, now).
				Order("id desc").
				First(&ban).Error; err != nil {
				return err
			}
			return updateActiveBlackroomBan(tx, &ban, input, now)
		}
		created = true
		return appendBlackroomBanEvent(tx, &ban, BlackroomBanEventApply, 0)
	})
	if err != nil {
		return nil, false, err
	}
	InvalidateBlackroomUserAuthCache(input.UserId)
	return &ban, created, nil
}

func updateActiveBlackroomBan(tx *gorm.DB, ban *BlackroomBan, input BlackroomBanInput, now int64) error {
	if ban.Source == BlackroomBanSourceManual && input.Source == BlackroomBanSourceAuto {
		return nil
	}

	updates := map[string]any{
		"active_key":   blackroomActiveKey(input.UserId),
		"username":     input.Username,
		"source":       input.Source,
		"reason":       input.Reason,
		"evidence":     input.Evidence,
		"ip_count":     input.IpCount,
		"ip_list":      input.IpList,
		"window_start": input.WindowStart,
		"window_end":   input.WindowEnd,
		"updated_at":   now,
	}
	if ban.BannedUntil != 0 {
		if input.BannedUntil == 0 || input.BannedUntil > ban.BannedUntil {
			updates["banned_until"] = input.BannedUntil
			updates["ban_duration_seconds"] = input.BanDurationSeconds
		}
	}
	if input.Source == BlackroomBanSourceManual {
		updates["banned_until"] = input.BannedUntil
		updates["ban_duration_seconds"] = input.BanDurationSeconds
	}
	if err := tx.Model(&BlackroomBan{}).Where("id = ?", ban.Id).Updates(updates).Error; err != nil {
		return err
	}
	if err := tx.First(ban, "id = ?", ban.Id).Error; err != nil {
		return err
	}
	// 覆盖前的取值已随上一条事件留存，这里记录本次变更后的快照。
	return appendBlackroomBanEvent(tx, ban, BlackroomBanEventReapply, 0)
}

// SetBlackroomUserStatus 仅更新用户的 status 列，供小黑屋 external 来源联动
// users.status 使用（封禁置 2、解封置 1）。
func SetBlackroomUserStatus(userID int, status int) error {
	return setBlackroomUserStatusTx(DB, userID, status)
}

func setBlackroomUserStatusTx(tx *gorm.DB, userID int, status int) error {
	if userID <= 0 {
		return errors.New("无效的用户 ID")
	}
	return tx.Model(&User{}).Where("id = ?", userID).Update("status", status).Error
}

func ReleaseBlackroomBan(id int, releasedBy int, reason string) (*BlackroomBan, error) {
	ban, err := GetBlackroomBanByID(id)
	if err != nil {
		return nil, err
	}
	if ban.Status != BlackroomBanStatusActive {
		return nil, errors.New("该小黑屋记录不是生效状态")
	}
	now := common.GetTimestamp()
	updates := map[string]any{
		"status":         BlackroomBanStatusReleased,
		"active_key":     nil,
		"released_at":    now,
		"released_by":    releasedBy,
		"release_reason": reason,
		"updated_at":     now,
	}

	var released BlackroomBan
	err = DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&BlackroomBan{}).Where("id = ? AND status = ?", id, BlackroomBanStatusActive).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("该小黑屋记录不是生效状态")
		}
		if ban.Source == BlackroomBanSourceExternal {
			if err := setBlackroomUserStatusTx(tx, ban.UserId, common.UserStatusEnabled); err != nil {
				return err
			}
		}
		if err := tx.First(&released, "id = ?", id).Error; err != nil {
			return err
		}
		return appendBlackroomBanEvent(tx, &released, BlackroomBanEventRelease, releasedBy)
	})
	if err != nil {
		return nil, err
	}
	InvalidateBlackroomUserAuthCache(ban.UserId)
	return &released, nil
}

func ExpireDueBlackroomBans() (int64, error) {
	now := common.GetTimestamp()
	// 取完整记录：过期事件需要写入当时的完整快照。
	var bans []BlackroomBan
	if err := DB.Where("status = ? AND banned_until > 0 AND banned_until <= ?", BlackroomBanStatusActive, now).
		Find(&bans).Error; err != nil {
		return 0, err
	}
	if len(bans) == 0 {
		return 0, nil
	}
	// external 来源封禁时联动了 users.status，到期需先恢复账号再标记过期；
	// 恢复失败的记录留给下一轮重试，避免记录过期后账号永远停在禁用状态。
	ids := make([]int, 0, len(bans))
	userIDs := make([]int, 0, len(bans))
	expiring := make([]BlackroomBan, 0, len(bans))
	for _, ban := range bans {
		if ban.Source == BlackroomBanSourceExternal {
			if err := SetBlackroomUserStatus(ban.UserId, common.UserStatusEnabled); err != nil {
				common.SysError(fmt.Sprintf("blackroom expire restore user status failed: user_id=%d error=%v", ban.UserId, err))
				continue
			}
		}
		ids = append(ids, ban.Id)
		userIDs = append(userIDs, ban.UserId)
		expiring = append(expiring, ban)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := DB.Model(&BlackroomBan{}).
		Where("id IN ? AND status = ? AND banned_until > 0 AND banned_until <= ?", ids, BlackroomBanStatusActive, now).
		Updates(map[string]any{
			"status":     BlackroomBanStatusExpired,
			"active_key": nil,
			"updated_at": now,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	// 过期是批量自动行为，事件写入失败只记录日志，不回滚已生效的过期状态。
	for i := range expiring {
		if err := appendBlackroomBanEvent(DB, &expiring[i], BlackroomBanEventExpire, 0); err != nil {
			common.SysError(fmt.Sprintf("blackroom expire event failed: ban_id=%d error=%v", expiring[i].Id, err))
		}
	}
	for _, userID := range userIDs {
		InvalidateBlackroomUserAuthCache(userID)
	}
	return result.RowsAffected, nil
}

// GetLatestBlackroomBanWindowEnd 返回用户最近一条小黑屋记录的 window_end（无记录时为 0）。
// 扫描定罪时以它为界排除上次封禁已覆盖的日志，避免同一批日志被重复处罚。
func GetLatestBlackroomBanWindowEnd(userID int) (int64, error) {
	if userID <= 0 {
		return 0, nil
	}
	var windowEnds []int64
	err := DB.Model(&BlackroomBan{}).
		Where("user_id = ?", userID).
		Order("id desc").
		Limit(1).
		Pluck("window_end", &windowEnds).Error
	if err != nil {
		return 0, err
	}
	if len(windowEnds) == 0 {
		return 0, nil
	}
	return windowEnds[0], nil
}

func CountRecentTemporaryBlackroomBans(userID int, since int64) (int64, error) {
	if userID <= 0 {
		return 0, nil
	}
	var count int64
	err := DB.Model(&BlackroomBan{}).
		Where("user_id = ? AND ban_duration_seconds > 0 AND created_at >= ?", userID, since).
		Count(&count).Error
	return count, err
}

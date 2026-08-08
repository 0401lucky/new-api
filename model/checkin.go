package model

import (
	"errors"
	"math/rand"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

// Checkin 签到记录
type Checkin struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId       int    `json:"user_id" gorm:"not null;uniqueIndex:idx_user_checkin_date"`
	CheckinDate  string `json:"checkin_date" gorm:"type:varchar(10);not null;uniqueIndex:idx_user_checkin_date"` // 格式: YYYY-MM-DD
	QuotaAwarded int    `json:"quota_awarded" gorm:"not null"`

	// QuotaType 奖励类型: "permanent" 或 "temporary"。旧记录空值按 permanent 处理。
	QuotaType string `json:"quota_type" gorm:"type:varchar(16);index;default:permanent"`
	// QuotaRemaining 限时额度剩余量（仅 temporary 记录使用）
	QuotaRemaining int `json:"quota_remaining" gorm:"not null;default:0"`
	// QuotaExpiresAt 限时额度失效时间（Unix 秒，系统时区次日 00:00），0 表示不失效
	QuotaExpiresAt int64 `json:"quota_expires_at" gorm:"bigint;default:0"`

	CreatedAt int64 `json:"created_at" gorm:"bigint"`
}

// IsTemporary 判断签到记录是否为限时额度奖励
func (c *Checkin) IsTemporary() bool {
	return c.QuotaType == "temporary"
}

// CheckinRecord 用于API返回的签到记录（不包含敏感字段）
type CheckinRecord struct {
	CheckinDate  string `json:"checkin_date"`
	QuotaAwarded int    `json:"quota_awarded"`
}

func (Checkin) TableName() string {
	return "checkins"
}

// GetUserCheckinRecords 获取用户在指定日期范围内的签到记录
func GetUserCheckinRecords(userId int, startDate, endDate string) ([]Checkin, error) {
	var records []Checkin
	err := DB.Where("user_id = ? AND checkin_date >= ? AND checkin_date <= ?",
		userId, startDate, endDate).
		Order("checkin_date DESC").
		Find(&records).Error
	return records, err
}

// HasCheckedInToday 检查用户今天是否已签到（按系统时区判断）
func HasCheckedInToday(userId int) (bool, error) {
	today := common.NowInStartupTimezone().Format("2006-01-02")
	var count int64
	err := DB.Model(&Checkin{}).
		Where("user_id = ? AND checkin_date = ?", userId, today).
		Count(&count).Error
	return count > 0, err
}

// GetTodayCheckin 获取用户今天的签到记录（按系统时区），无则返回 nil。
func GetTodayCheckin(userId int) (*Checkin, error) {
	today := common.NowInStartupTimezone().Format("2006-01-02")
	var checkin Checkin
	err := DB.Where("user_id = ? AND checkin_date = ?", userId, today).
		Order("id DESC").
		First(&checkin).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &checkin, nil
}

// UserCheckin 执行用户签到
// 永久模式增加永久额度；限时模式写入签到额度桶，不修改永久额度。
// MySQL 和 PostgreSQL 使用事务保证原子性
// SQLite 不支持嵌套事务，使用顺序操作 + 手动回滚
func UserCheckin(userId int) (*Checkin, error) {
	setting := operation_setting.GetCheckinSetting()
	if !setting.Enabled {
		return nil, errors.New("签到功能未启用")
	}

	// 运行时整组校验（防止配置在保存时被绕过校验直接写入非法值）
	if err := setting.ValidateCheckinConfig(); err != nil {
		return nil, errors.New("签到奖励配置无效: " + err.Error())
	}

	// 检查是否已到开放时间（按系统时区）
	now := common.NowInStartupTimezone()
	if err := checkinAvailableCheck(setting, now); err != nil {
		return nil, err
	}

	// 检查今天是否已签到
	hasChecked, err := HasCheckedInToday(userId)
	if err != nil {
		return nil, err
	}
	if hasChecked {
		return nil, errors.New("今日已签到")
	}

	quotaAwarded := setting.FixedQuota
	if setting.RandomMode {
		quotaAwarded = setting.MinQuota
	}
	if setting.RandomMode && setting.MaxQuota > setting.MinQuota {
		quotaAwarded = setting.MinQuota + rand.Intn(setting.MaxQuota-setting.MinQuota+1)
	}

	today := now.Format("2006-01-02")
	checkin := &Checkin{
		UserId:       userId,
		CheckinDate:  today,
		QuotaAwarded: quotaAwarded,
		QuotaType:    CheckinQuotaTypePermanent,
		CreatedAt:    common.GetTimestamp(),
	}

	temporary := setting.IsTemporaryReward()
	if temporary {
		// 限时模式：写入签到额度桶，失效时间为系统时区次日 00:00
		checkin.QuotaType = CheckinQuotaTypeTemporary
		checkin.QuotaRemaining = quotaAwarded
		checkin.QuotaExpiresAt = nextDayMidnightUnix(now)
	}

	// 根据数据库类型选择不同的策略
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		// SQLite 不支持嵌套事务，使用顺序操作 + 手动回滚
		return userCheckinWithoutTransaction(checkin, userId, quotaAwarded, temporary)
	}

	// MySQL 和 PostgreSQL 支持事务，使用事务保证原子性
	return userCheckinWithTransaction(checkin, userId, quotaAwarded, temporary)
}

// checkinAvailableCheck 校验当前时间是否已到签到开放时间。
func checkinAvailableCheck(setting *operation_setting.CheckinSetting, now time.Time) error {
	loc := common.StartupLocation()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	availableFrom := todayStart.Add(time.Duration(setting.AvailableFromMinutes) * time.Minute)
	if now.Before(availableFrom) {
		return errors.New("签到尚未开始")
	}
	return nil
}

// nextDayMidnightUnix 返回系统时区次日 00:00 的 Unix 秒。
func nextDayMidnightUnix(now time.Time) int64 {
	loc := common.StartupLocation()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	return todayStart.AddDate(0, 0, 1).Unix()
}

// userCheckinWithTransaction 使用事务执行签到（适用于 MySQL 和 PostgreSQL）
func userCheckinWithTransaction(checkin *Checkin, userId int, quotaAwarded int, temporary bool) (*Checkin, error) {
	err := DB.Transaction(func(tx *gorm.DB) error {
		// 步骤1: 创建签到记录
		// 数据库有唯一约束 (user_id, checkin_date)，可以防止并发重复签到
		if err := tx.Create(checkin).Error; err != nil {
			return errors.New("签到失败，请稍后重试")
		}

		// 步骤2: 发放奖励
		if temporary {
			// 限时额度已经随签到记录写入额度桶，无需修改永久额度
			return nil
		}
		// 永久额度：在事务中增加用户额度
		result := tx.Model(&User{}).Where("id = ?", userId).
			Update("quota", gorm.Expr("quota + ?", quotaAwarded))
		if result.Error != nil {
			return errors.New("签到失败：更新额度出错")
		}
		if result.RowsAffected == 0 {
			return errors.New("签到失败：用户不存在")
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// 事务成功后，异步更新缓存（仅永久模式需要）
	if !temporary {
		go func() {
			_ = cacheIncrUserQuota(userId, int64(quotaAwarded))
		}()
	}

	return checkin, nil
}

// userCheckinWithoutTransaction 不使用事务执行签到（适用于 SQLite）
func userCheckinWithoutTransaction(checkin *Checkin, userId int, quotaAwarded int, temporary bool) (*Checkin, error) {
	// 步骤1: 创建签到记录
	// 数据库有唯一约束 (user_id, checkin_date)，可以防止并发重复签到
	if err := DB.Create(checkin).Error; err != nil {
		return nil, errors.New("签到失败，请稍后重试")
	}

	if temporary {
		// 限时额度已经随签到记录写入额度桶，无需修改永久额度
		return checkin, nil
	}

	// 步骤2: 增加用户额度
	// 使用 db=true 强制直接写入数据库，不使用批量更新
	if err := IncreaseUserQuota(userId, quotaAwarded, true); err != nil {
		// 如果增加额度失败，需要回滚签到记录
		DB.Delete(checkin)
		return nil, errors.New("签到失败：更新额度出错")
	}

	return checkin, nil
}

// GetUserCheckinStats 获取用户签到统计信息
func GetUserCheckinStats(userId int, month string) (map[string]interface{}, error) {
	// 获取指定月份的所有签到记录
	startDate := month + "-01"
	endDate := month + "-31"

	records, err := GetUserCheckinRecords(userId, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// 转换为不包含敏感字段的记录
	checkinRecords := make([]CheckinRecord, len(records))
	for i, r := range records {
		checkinRecords[i] = CheckinRecord{
			CheckinDate:  r.CheckinDate,
			QuotaAwarded: r.QuotaAwarded,
		}
	}

	// 检查今天是否已签到
	hasCheckedToday, _ := HasCheckedInToday(userId)

	// 获取用户所有时间的签到统计
	var totalCheckins int64
	var totalQuota int64
	DB.Model(&Checkin{}).Where("user_id = ?", userId).Count(&totalCheckins)
	DB.Model(&Checkin{}).Where("user_id = ?", userId).Select("COALESCE(SUM(quota_awarded), 0)").Scan(&totalQuota)

	return map[string]interface{}{
		"total_quota":      totalQuota,      // 所有时间累计获得的额度
		"total_checkins":   totalCheckins,   // 所有时间累计签到次数
		"checkin_count":    len(records),    // 本月签到次数
		"checked_in_today": hasCheckedToday, // 今天是否已签到
		"records":          checkinRecords,  // 本月签到记录详情（不含id和user_id）
	}, nil
}

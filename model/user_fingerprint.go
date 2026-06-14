package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UserFingerprint 记录用户设备指纹与访问来源，用于管理员排查关联账号。
type UserFingerprint struct {
	Id int `json:"id" gorm:"primaryKey;autoIncrement"`

	UserId    int    `json:"user_id" gorm:"not null;index;uniqueIndex:ux_user_fingerprints_user_visitor_ip"`
	VisitorId string `json:"visitor_id" gorm:"type:varchar(64);not null;index;uniqueIndex:ux_user_fingerprints_user_visitor_ip"`
	IP        string `json:"ip" gorm:"type:varchar(64);index;uniqueIndex:ux_user_fingerprints_user_visitor_ip"`

	UserAgent string    `json:"user_agent" gorm:"type:varchar(512)"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (UserFingerprint) TableName() string {
	return "user_fingerprints"
}

// RecordFingerprint 按 (user_id, visitor_id, ip) 去重，并只保留用户最近 5 组记录。
func RecordFingerprint(userId int, visitorId string, userAgent string, ip string) error {
	now := time.Now()
	fingerprint := UserFingerprint{
		UserId:    userId,
		VisitorId: visitorId,
		UserAgent: userAgent,
		IP:        ip,
		UpdatedAt: now,
	}

	if err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "visitor_id"},
			{Name: "ip"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"user_agent", "updated_at"}),
	}).Create(&fingerprint).Error; err != nil {
		return err
	}

	var count int64
	if err := DB.Model(&UserFingerprint{}).Where("user_id = ?", userId).Count(&count).Error; err != nil {
		return err
	}

	if count <= 5 {
		return nil
	}

	var fifth UserFingerprint
	err := DB.
		Select("id, updated_at").
		Where("user_id = ?", userId).
		Order("updated_at desc, id desc").
		Offset(4).
		Limit(1).
		Take(&fifth).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	return DB.
		Where("user_id = ?", userId).
		Where("(updated_at < ?) OR (updated_at = ? AND id < ?)", fifth.UpdatedAt, fifth.UpdatedAt, fifth.Id).
		Delete(&UserFingerprint{}).Error
}

func GetUserFingerprints(userId int) ([]UserFingerprint, error) {
	var fingerprints []UserFingerprint
	err := DB.Where("user_id = ?", userId).
		Order("updated_at desc, id desc").
		Limit(5).
		Find(&fingerprints).Error
	return fingerprints, err
}

type UserWithFingerprint struct {
	Id           int    `json:"id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Status       int    `json:"status"`
	Role         int    `json:"role"`
	Quota        int    `json:"quota"`
	UsedQuota    int    `json:"used_quota"`
	RequestCount int    `json:"request_count"`
	VisitorId    string `json:"visitor_id"`
	RecordTime   string `json:"record_time"`
	IP           string `json:"ip"`
}

func FindUsersByVisitorId(visitorId string, ip string, pageInfo *common.PageInfo) ([]UserWithFingerprint, int64, error) {
	var results []UserWithFingerprint
	var total int64

	baseWhere := "f.visitor_id = ?"
	args := []interface{}{visitorId}
	if ip != "" {
		baseWhere += " AND f.ip = ?"
		args = append(args, ip)
	}

	countQuery := `SELECT COUNT(DISTINCT f.user_id) FROM user_fingerprints f WHERE ` + baseWhere
	if err := DB.Raw(countQuery, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	query := `
		SELECT u.id, u.username, u.display_name, u.email, u.status, u.role,
			   u.quota, u.used_quota, u.request_count,
			   f.visitor_id, f.created_at as record_time, f.ip
		FROM user_fingerprints f
		JOIN users u ON f.user_id = u.id
		WHERE ` + baseWhere + `
		AND f.id IN (
			SELECT MAX(f2.id) FROM user_fingerprints f2
			WHERE f2.visitor_id = ?` + func() string {
		if ip != "" {
			return " AND f2.ip = ?"
		}
		return ""
	}() + ` GROUP BY f2.user_id
		)
		ORDER BY f.created_at DESC
		LIMIT ? OFFSET ?
	`

	queryArgs := make([]interface{}, 0, len(args)*2+2)
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, visitorId)
	if ip != "" {
		queryArgs = append(queryArgs, ip)
	}
	queryArgs = append(queryArgs, pageInfo.GetPageSize(), pageInfo.GetStartIdx())

	err := DB.Raw(query, queryArgs...).Scan(&results).Error
	return results, total, err
}

func FindUsersByIP(ip string, pageInfo *common.PageInfo) ([]UserWithFingerprint, int64, error) {
	var results []UserWithFingerprint
	var total int64

	countQuery := `SELECT COUNT(DISTINCT f.user_id) FROM user_fingerprints f WHERE f.ip = ?`
	if err := DB.Raw(countQuery, ip).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	query := `
		SELECT u.id, u.username, u.display_name, u.email, u.status, u.role,
			   u.quota, u.used_quota, u.request_count,
			   f.visitor_id, f.created_at as record_time, f.ip
		FROM user_fingerprints f
		JOIN users u ON f.user_id = u.id
		WHERE f.ip = ?
		AND f.id IN (
			SELECT MAX(f2.id) FROM user_fingerprints f2
			WHERE f2.ip = ?
			GROUP BY f2.user_id
		)
		ORDER BY f.created_at DESC
		LIMIT ? OFFSET ?
	`
	err := DB.Raw(query, ip, ip, pageInfo.GetPageSize(), pageInfo.GetStartIdx()).Scan(&results).Error
	return results, total, err
}

func GetAllFingerprints(pageInfo *common.PageInfo) ([]UserWithFingerprint, int64, error) {
	var results []UserWithFingerprint
	var total int64

	if err := DB.Model(&UserFingerprint{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query := `
		SELECT u.id, u.username, u.display_name, u.email, u.status, u.role,
			   f.visitor_id, f.created_at as record_time, f.ip
		FROM user_fingerprints f
		JOIN users u ON f.user_id = u.id
		ORDER BY f.created_at DESC
		LIMIT ? OFFSET ?
	`
	err := DB.Raw(query, pageInfo.GetPageSize(), pageInfo.GetStartIdx()).Scan(&results).Error
	return results, total, err
}

func SearchFingerprints(keyword string, pageInfo *common.PageInfo) ([]UserWithFingerprint, int64, error) {
	var results []UserWithFingerprint
	var total int64

	searchWhere := fingerprintSearchWhereClause()
	countQuery := `
		SELECT COUNT(*) FROM user_fingerprints f
		JOIN users u ON f.user_id = u.id
		WHERE ` + searchWhere + `
	`
	likeKeyword := "%" + keyword + "%"
	if err := DB.Raw(countQuery, likeKeyword, likeKeyword, likeKeyword).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	query := `
		SELECT u.id, u.username, u.display_name, u.email, u.status, u.role,
			   f.visitor_id, f.created_at as record_time, f.ip
		FROM user_fingerprints f
		JOIN users u ON f.user_id = u.id
		WHERE ` + searchWhere + `
		ORDER BY f.created_at DESC
		LIMIT ? OFFSET ?
	`
	err := DB.Raw(query, likeKeyword, likeKeyword, likeKeyword, pageInfo.GetPageSize(), pageInfo.GetStartIdx()).Scan(&results).Error
	return results, total, err
}

func fingerprintSearchWhereClause() string {
	return "LOWER(f.visitor_id) LIKE LOWER(?) OR LOWER(u.username) LIKE LOWER(?) OR LOWER(u.email) LIKE LOWER(?)"
}

func GetDuplicateVisitorIds(pageInfo *common.PageInfo) ([]map[string]interface{}, int64, error) {
	var total int64

	countQuery := `
		SELECT COUNT(*) FROM (
			SELECT visitor_id, ip FROM user_fingerprints
			GROUP BY visitor_id, ip
			HAVING COUNT(DISTINCT user_id) > 1
		) AS duplicates
	`
	if err := DB.Raw(countQuery).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	query := `
		SELECT visitor_id, ip, COUNT(DISTINCT user_id) as user_count, MAX(created_at) as last_seen
		FROM user_fingerprints
		GROUP BY visitor_id, ip
		HAVING COUNT(DISTINCT user_id) > 1
		ORDER BY user_count DESC, last_seen DESC
		LIMIT ? OFFSET ?
	`

	var rows []struct {
		VisitorId string    `json:"visitor_id"`
		IP        string    `json:"ip"`
		UserCount int       `json:"user_count"`
		LastSeen  time.Time `json:"last_seen"`
	}
	if err := DB.Raw(query, pageInfo.GetPageSize(), pageInfo.GetStartIdx()).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	results := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		results = append(results, map[string]interface{}{
			"visitor_id": row.VisitorId,
			"ip":         row.IP,
			"user_count": row.UserCount,
			"last_seen":  row.LastSeen,
		})
	}

	return results, total, nil
}

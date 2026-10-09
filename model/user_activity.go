package model

import (
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const UserActivityBatchLimit = 100

const userActivityDay = int64(24 * 60 * 60)

var ErrInactiveUserSelection = errors.New("Some selected users are no longer eligible for cleanup. Refresh the list and select again.")

// Request activity is independent of billing and log retention. Failed requests
// with an identifiable account update the same timestamp as successful ones.
type UserRequestActivity struct {
	UserID        int   `gorm:"primaryKey;autoIncrement:false"`
	LastRequestAt int64 `gorm:"not null;index"`
}

// A separate checkpoint keeps an administrator's log/settings cleanup from
// turning an account with missing history into a supposedly never-used account.
type UserActivityState struct {
	ID                  int   `gorm:"primaryKey;autoIncrement:false"`
	TrackingStartedAt   int64 `gorm:"not null"`
	BackfillCompletedAt int64 `gorm:"not null"`
}

func RecordUserRequestActivity(userID int, requestedAt int64) error {
	if userID <= 0 || requestedAt <= 0 {
		return nil
	}
	column := clause.Column{Table: clause.CurrentTable, Name: "last_request_at"}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_request_at": gorm.Expr("CASE WHEN ? < ? THEN ? ELSE ? END", column, requestedAt, requestedAt, column),
		}),
	}).Create(&UserRequestActivity{UserID: userID, LastRequestAt: requestedAt}).Error
}

// InitUserActivity runs after both databases are ready, before serving traffic.
// Retained consume/error logs seed the timestamp once, including separate log
// databases. A restart can resume the backfill without moving timestamps back.
func InitUserActivity() error {
	state := UserActivityState{ID: 1, TrackingStartedAt: common.GetTimestamp()}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return err
	}
	if err := DB.First(&state, 1).Error; err != nil {
		return err
	}
	if state.BackfillCompletedAt > 0 {
		return nil
	}
	lastID := 0
	for {
		var users []User
		if err := DB.Select("id", "created_at").Where("id > ?", lastID).Order("id").Limit(500).Find(&users).Error; err != nil {
			return err
		}
		if len(users) == 0 {
			break
		}
		ids := make([]int, len(users))
		createdAt := make(map[int]int64, len(users))
		for i, user := range users {
			ids[i] = user.Id
			createdAt[user.Id] = user.CreatedAt
		}
		var activity []UserRequestActivity
		if err := LOG_DB.Model(&Log{}).
			Select("user_id, MAX(created_at) AS last_request_at").
			Where("user_id IN ? AND type IN ? AND created_at <= ?", ids, []int{LogTypeConsume, LogTypeError}, common.GetTimestamp()).
			Group("user_id").Scan(&activity).Error; err != nil {
			return err
		}
		for _, request := range activity {
			// Do not inherit logs belonging to a deleted account whose ID was reused.
			if request.LastRequestAt < createdAt[request.UserID] {
				continue
			}
			if err := RecordUserRequestActivity(request.UserID, request.LastRequestAt); err != nil {
				return err
			}
		}
		lastID = users[len(users)-1].Id
	}
	return DB.Model(&state).Update("backfill_completed_at", common.GetTimestamp()).Error
}

type UserActivityFilter struct {
	Keyword  string
	Group    string
	Role     *int
	Status   *int
	Activity string
	SortBy   string
	SortDesc bool
}

type UserActivityItem struct {
	User
	LastRequestAt   int64  `json:"last_request_at"`
	Activity        string `json:"activity"`
	NoRequestDays   int64  `json:"no_request_days"`
	CleanupEligible bool   `json:"cleanup_eligible"`
}

type UserActivitySummary struct {
	Total             int64 `json:"total"`
	Active            int64 `json:"active"`
	Inactive          int64 `json:"inactive"`
	VeryInactive      int64 `json:"very_inactive"`
	NeverRequested    int64 `json:"never_requested"`
	Unknown           int64 `json:"unknown"`
	CleanupCandidates int64 `json:"cleanup_candidates"`
}

type UserActivityPage struct {
	Items             []UserActivityItem  `json:"items"`
	Total             int64               `json:"total"`
	Page              int                 `json:"page"`
	PageSize          int                 `json:"page_size"`
	Summary           UserActivitySummary `json:"summary"`
	TrackingStartedAt int64               `json:"tracking_started_at"`
	AsOf              int64               `json:"as_of"`
}

func GetUserActivity(filter UserActivityFilter, page *common.PageInfo, now int64) (*UserActivityPage, error) {
	var state UserActivityState
	if err := DB.First(&state, 1).Error; err != nil {
		return nil, err
	}
	if state.BackfillCompletedAt == 0 {
		return nil, errors.New("User activity history is still being initialized.")
	}
	cutoff := now - 30*userActivityDay
	activity := gorm.Expr(`CASE
		WHEN a.last_request_at >= ? THEN 'active'
		WHEN a.last_request_at > ? THEN 'inactive'
		WHEN a.last_request_at > 0 THEN 'very_inactive'
		WHEN u.created_at > ? THEN 'never_requested'
		ELSE 'unknown' END`, now-7*userActivityDay, cutoff, state.TrackingStartedAt)
	eligible := gorm.Expr(`CASE WHEN u.role = ? AND u.created_at > 0 AND u.created_at <= ?
		AND COALESCE(NULLIF(a.last_request_at, 0), ?) <= ? THEN 1 ELSE 0 END`,
		common.RoleCommonUser, cutoff, state.TrackingStartedAt, cutoff)

	// Explicit projection prevents this read-only endpoint from returning
	// credentials, account bindings, or private settings.
	query := DB.Table("? AS u", clause.Table{Name: DB.NamingStrategy.TableName("User")}).
		Joins("LEFT JOIN ? AS a ON a.user_id = u.id AND a.last_request_at >= u.created_at", clause.Table{Name: DB.NamingStrategy.TableName("UserRequestActivity")}).
		Where("u.deleted_at IS NULL").
		Select(`u.id, u.username, u.display_name, u.email, u.role, u.status, ?, u.remark,
			u.created_at, u.last_login_at, COALESCE(a.last_request_at, 0) AS last_request_at,
			? AS activity, ? AS cleanup_eligible`, clause.Column{Table: "u", Name: "group"}, activity, eligible)
	keyword := strings.TrimSpace(filter.Keyword)
	if exactID, ok := strings.CutPrefix(keyword, "#"); ok {
		id, err := strconv.Atoi(strings.TrimSpace(exactID))
		if err != nil || id <= 0 {
			query = query.Where("1 = 0")
		} else {
			query = query.Where("u.id = ?", id)
		}
	} else if keyword != "" {
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(keyword) + "%"
		query = query.Where("(u.username LIKE ? ESCAPE '!' OR u.display_name LIKE ? ESCAPE '!' OR u.email LIKE ? ESCAPE '!')", pattern, pattern, pattern)
	}
	if filter.Group != "" {
		query = query.Where(clause.Eq{Column: clause.Column{Table: "u", Name: "group"}, Value: filter.Group})
	}
	if filter.Role != nil {
		query = query.Where("u.role = ?", *filter.Role)
	}
	if filter.Status != nil {
		query = query.Where("u.status = ?", *filter.Status)
	}
	result := &UserActivityPage{
		Items: []UserActivityItem{}, Page: page.GetPage(), PageSize: page.GetPageSize(),
		TrackingStartedAt: state.TrackingStartedAt, AsOf: now,
	}
	var counts []struct {
		Activity string
		Total    int64
		Eligible int64
	}
	if err := DB.Table("(?) AS activity_users", query).
		Select("activity, COUNT(*) AS total, SUM(cleanup_eligible) AS eligible").
		Group("activity").Scan(&counts).Error; err != nil {
		return nil, err
	}
	for _, count := range counts {
		result.Summary.Total += count.Total
		result.Summary.CleanupCandidates += count.Eligible
		switch count.Activity {
		case "active":
			result.Summary.Active = count.Total
		case "inactive":
			result.Summary.Inactive = count.Total
		case "very_inactive":
			result.Summary.VeryInactive = count.Total
		case "never_requested":
			result.Summary.NeverRequested = count.Total
		case "unknown":
			result.Summary.Unknown = count.Total
		}
	}
	list := DB.Table("(?) AS activity_users", query)
	if filter.Activity == "cleanup" {
		list = list.Where("cleanup_eligible = ?", 1)
	} else if filter.Activity != "" {
		list = list.Where("activity = ?", filter.Activity)
	}
	if err := list.Count(&result.Total).Error; err != nil {
		return nil, err
	}
	sortBy := filter.SortBy
	if !slices.Contains([]string{"id", "username", "created_at", "last_request_at"}, sortBy) {
		sortBy = "last_request_at"
	}
	if err := list.Order(clause.OrderByColumn{Column: clause.Column{Name: sortBy}, Desc: filter.SortDesc}).
		Order("id asc").Offset(page.GetStartIdx()).Limit(page.GetPageSize()).Scan(&result.Items).Error; err != nil {
		return nil, err
	}
	for i := range result.Items {
		user := &result.Items[i]
		since := user.LastRequestAt
		if since == 0 {
			since = max(user.CreatedAt, state.TrackingStartedAt)
		}
		user.NoRequestDays = max(0, (now-since)/userActivityDay)
	}
	return result, nil
}

type InactiveUserDeletion struct {
	UserID              int
	Username            string
	RevokedAccessTokens int64
}

// DeleteInactiveUsers rechecks the whole selection under locks before removing
// anything. Request tracking uses the same activity rows, so a request recorded
// after the list was displayed prevents deletion.
func DeleteInactiveUsers(ids []int, clearAuthorization func(*gorm.DB, int) error) ([]InactiveUserDeletion, error) {
	if len(ids) == 0 || len(ids) > UserActivityBatchLimit {
		return nil, ErrInactiveUserSelection
	}
	ids = slices.Clone(ids)
	slices.Sort(ids)
	if ids[0] <= 0 || len(slices.Compact(slices.Clone(ids))) != len(ids) {
		return nil, ErrInactiveUserSelection
	}
	deleted := make([]InactiveUserDeletion, 0, len(ids))
	err := DB.Transaction(func(tx *gorm.DB) error {
		var state UserActivityState
		if err := tx.First(&state, 1).Error; err != nil {
			return err
		}
		if state.BackfillCompletedAt == 0 {
			return ErrInactiveUserSelection
		}
		var users []User
		if err := lockForUpdate(tx).Where("id IN ?", ids).Order("id").Find(&users).Error; err != nil {
			return err
		}
		if len(users) != len(ids) {
			return ErrInactiveUserSelection
		}
		for _, id := range ids {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&UserRequestActivity{UserID: id}).Error; err != nil {
				return err
			}
		}
		var activity []UserRequestActivity
		if err := lockForUpdate(tx).Where("user_id IN ?", ids).Order("user_id").Find(&activity).Error; err != nil {
			return err
		}
		cutoff := common.GetTimestamp() - 30*userActivityDay
		for i, user := range users {
			lastRequestAt := activity[i].LastRequestAt
			if lastRequestAt < user.CreatedAt {
				lastRequestAt = 0
			}
			if lastRequestAt == 0 {
				lastRequestAt = max(user.CreatedAt, state.TrackingStartedAt)
			}
			if user.Role != common.RoleCommonUser || user.CreatedAt <= 0 || user.CreatedAt > cutoff || lastRequestAt > cutoff {
				return ErrInactiveUserSelection
			}
		}
		for _, user := range users {
			if err := clearAuthorization(tx, user.Id); err != nil {
				return err
			}
			revoked, err := HardDeleteUserByIdWithTx(tx, user.Id)
			if err != nil {
				return err
			}
			deleted = append(deleted, InactiveUserDeletion{UserID: user.Id, Username: user.Username, RevokedAccessTokens: revoked})
		}
		return tx.Delete(&UserRequestActivity{}, "user_id IN ?", ids).Error
	})
	if err != nil {
		return nil, err
	}
	return deleted, nil
}

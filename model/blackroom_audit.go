package model

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// BlackroomIPAuditRetentionDays 观测记录保留天数。
	BlackroomIPAuditRetentionDays = 30
	// BlackroomIPAuditMaxEvaluationRows 单次证据计算最多读取的观测行数。
	BlackroomIPAuditMaxEvaluationRows = 2000
)

// BlackroomIPMinute 是按 (用户, IP, 分钟) 聚合的请求观测。
//
// 它刻意独立于业务日志表：日志受 LogConsumeEnabled 开关、日志清理任务和
// LOG_DB 配置影响，任何一项变动都会让依赖日志的判定失效或出现空洞。
// 本表是小黑屋判定的唯一数据源。
type BlackroomIPMinute struct {
	UserId           int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	Ip               string `json:"ip" gorm:"primaryKey;size:64"`
	Minute           int64  `json:"minute" gorm:"primaryKey;autoIncrement:false;index"`
	Username         string `json:"username" gorm:"size:64;default:''"`
	RequestCount     int64  `json:"request_count"`
	FirstSeenAt      int64  `json:"first_seen_at" gorm:"bigint;index"`
	LastSeenAt       int64  `json:"last_seen_at" gorm:"bigint;index"`
	CountryISO       string `json:"country_iso" gorm:"size:2;default:'';index"`
	AsnNumber        int64  `json:"asn_number" gorm:"bigint;index"`
	AsnOrganization  string `json:"asn_organization" gorm:"size:255;default:''"`
	ResolverVersion  string `json:"resolver_version" gorm:"size:96;default:''"`
	IpKind           string `json:"ip_kind" gorm:"size:24;default:'';index"`
	EvidenceEligible bool   `json:"evidence_eligible"`
}

func (BlackroomIPMinute) TableName() string {
	return "blackroom_ip_minutes"
}

// BlackroomIPObservation 是一次请求的观测输入。Geo 字段为空时表示本次
// 没有可用的解析结果，写入时会保留行上已有的值。
type BlackroomIPObservation struct {
	UserId           int
	Username         string
	Ip               string
	CountryISO       string
	AsnNumber        int64
	AsnOrganization  string
	ResolverVersion  string
	IpKind           string
	EvidenceEligible bool
	ObservedAt       time.Time
}

var (
	blackroomAuditCleanupDay atomic.Int64
	blackroomAuditCleanupMu  sync.Mutex
)

// RecordBlackroomIPObservation 累加一条观测。同一 (用户, IP, 分钟) 重复写入
// 只增加 request_count，不会产生新行。
func RecordBlackroomIPObservation(observation BlackroomIPObservation) error {
	if observation.UserId <= 0 || DB == nil {
		return nil
	}
	ip := strings.TrimSpace(observation.Ip)
	if ip == "" {
		return nil
	}

	observedAt := observation.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	observedAt = observedAt.UTC()

	row := &BlackroomIPMinute{
		UserId:           observation.UserId,
		Ip:               ip,
		Minute:           observedAt.Truncate(time.Minute).Unix(),
		Username:         strings.TrimSpace(observation.Username),
		RequestCount:     1,
		FirstSeenAt:      observedAt.Unix(),
		LastSeenAt:       observedAt.Unix(),
		CountryISO:       strings.ToUpper(strings.TrimSpace(observation.CountryISO)),
		AsnNumber:        observation.AsnNumber,
		AsnOrganization:  strings.TrimSpace(observation.AsnOrganization),
		ResolverVersion:  strings.TrimSpace(observation.ResolverVersion),
		IpKind:           strings.TrimSpace(observation.IpKind),
		EvidenceEligible: observation.EvidenceEligible,
	}

	// ON CONFLICT DO UPDATE 的表达式里必须用限定列名：PostgreSQL 会把未限定
	// 的列名与 EXCLUDED 一起判为歧义引用（SQLSTATE 42702）。
	requestCountColumn := clause.Column{Table: clause.CurrentTable, Name: "request_count"}
	firstSeenAtColumn := clause.Column{Table: clause.CurrentTable, Name: "first_seen_at"}
	lastSeenAtColumn := clause.Column{Table: clause.CurrentTable, Name: "last_seen_at"}
	countryISOColumn := clause.Column{Table: clause.CurrentTable, Name: "country_iso"}
	asnNumberColumn := clause.Column{Table: clause.CurrentTable, Name: "asn_number"}
	asnOrganizationColumn := clause.Column{Table: clause.CurrentTable, Name: "asn_organization"}
	resolverVersionColumn := clause.Column{Table: clause.CurrentTable, Name: "resolver_version"}
	ipKindColumn := clause.Column{Table: clause.CurrentTable, Name: "ip_kind"}
	evidenceEligibleColumn := clause.Column{Table: clause.CurrentTable, Name: "evidence_eligible"}

	err := DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "ip"},
			{Name: "minute"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"username":      row.Username,
			"request_count": gorm.Expr("? + 1", requestCountColumn),
			// 取窗口内的最早/最晚时刻，不依赖写入顺序。
			"first_seen_at": gorm.Expr(
				"CASE WHEN ? IS NULL OR ? = 0 OR ? > ? THEN ? ELSE ? END",
				firstSeenAtColumn, firstSeenAtColumn, firstSeenAtColumn, row.FirstSeenAt, row.FirstSeenAt, firstSeenAtColumn,
			),
			"last_seen_at": gorm.Expr(
				"CASE WHEN ? < ? THEN ? ELSE ? END",
				lastSeenAtColumn, row.LastSeenAt, row.LastSeenAt, lastSeenAtColumn,
			),
			// 只有本次解析完整时才覆盖地理字段，避免不完整的解析结果
			// 冲掉同一分钟内已写入的有效值。
			"country_iso": gorm.Expr(
				"CASE WHEN ? THEN ? ELSE ? END", row.EvidenceEligible, row.CountryISO, countryISOColumn,
			),
			"asn_number": gorm.Expr(
				"CASE WHEN ? THEN ? ELSE ? END", row.EvidenceEligible, row.AsnNumber, asnNumberColumn,
			),
			"asn_organization": gorm.Expr(
				"CASE WHEN ? THEN ? ELSE ? END", row.EvidenceEligible, row.AsnOrganization, asnOrganizationColumn,
			),
			"resolver_version": gorm.Expr(
				"CASE WHEN ? THEN ? ELSE ? END", row.EvidenceEligible, row.ResolverVersion, resolverVersionColumn,
			),
			"ip_kind": gorm.Expr(
				"CASE WHEN ? THEN ? ELSE ? END", row.EvidenceEligible, row.IpKind, ipKindColumn,
			),
			// 一旦某次观测拿到过完整解析，该行就一直是合格证据。
			"evidence_eligible": gorm.Expr("? OR ?", evidenceEligibleColumn, row.EvidenceEligible),
		}),
	}).Create(row).Error
	if err != nil {
		return err
	}

	if err := cleanupBlackroomIPAuditIfNeeded(observedAt); err != nil {
		common.SysError(err.Error())
	}
	return nil
}

// cleanupBlackroomIPAuditIfNeeded 每天最多执行一次过期清理。
func cleanupBlackroomIPAuditIfNeeded(now time.Time) error {
	day := now.UTC().Unix() / int64((24 * time.Hour).Seconds())
	if blackroomAuditCleanupDay.Load() == day {
		return nil
	}
	blackroomAuditCleanupMu.Lock()
	defer blackroomAuditCleanupMu.Unlock()
	if blackroomAuditCleanupDay.Load() == day {
		return nil
	}
	cutoff := now.UTC().AddDate(0, 0, -BlackroomIPAuditRetentionDays).Truncate(time.Minute).Unix()
	if err := DB.Where("minute < ?", cutoff).Delete(&BlackroomIPMinute{}).Error; err != nil {
		return fmt.Errorf("清理过期的 IP 观测记录失败: %w", err)
	}
	blackroomAuditCleanupDay.Store(day)
	return nil
}

// BlackroomIPAuditSummary 是某个用户在给定窗口内的观测聚合，供规则匹配使用。
type BlackroomIPAuditSummary struct {
	IPCount         int
	CountryCount    int
	ASNCount        int
	RequestCount    int64
	FirstSeenAt     int64
	LastSeenAt      int64
	EligibleIPCount int
}

// SummarizeBlackroomIPAudit 用一次聚合查询拿到判定所需的全部计数。
// 窗口语义为半开区间 (startExclusive, endInclusive]：任何与该窗口有交集的
// 观测都计入。
func SummarizeBlackroomIPAudit(userID int, startExclusive int64, endInclusive int64) (*BlackroomIPAuditSummary, error) {
	if userID <= 0 {
		return &BlackroomIPAuditSummary{}, nil
	}
	var row struct {
		IPCount         int
		CountryCount    int
		ASNCount        int
		RequestCount    int64
		FirstSeenAt     int64
		LastSeenAt      int64
		EligibleIPCount int
	}
	err := DB.Model(&BlackroomIPMinute{}).
		Select(
			"COUNT(DISTINCT ip) AS ip_count, "+
				"COUNT(DISTINCT CASE WHEN evidence_eligible AND country_iso <> '' THEN country_iso END) AS country_count, "+
				"COUNT(DISTINCT CASE WHEN evidence_eligible AND asn_number > 0 THEN asn_number END) AS asn_count, "+
				"COUNT(DISTINCT CASE WHEN evidence_eligible THEN ip END) AS eligible_ip_count, "+
				"COALESCE(SUM(request_count), 0) AS request_count, "+
				"COALESCE(MIN(first_seen_at), 0) AS first_seen_at, "+
				"COALESCE(MAX(last_seen_at), 0) AS last_seen_at",
		).
		Where("user_id = ? AND last_seen_at > ? AND first_seen_at <= ?", userID, startExclusive, endInclusive).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	return &BlackroomIPAuditSummary{
		IPCount:         row.IPCount,
		CountryCount:    row.CountryCount,
		ASNCount:        row.ASNCount,
		RequestCount:    row.RequestCount,
		FirstSeenAt:     row.FirstSeenAt,
		LastSeenAt:      row.LastSeenAt,
		EligibleIPCount: row.EligibleIPCount,
	}, nil
}

// GetBlackroomIPRowsForEvaluation 拉取窗口内的观测行，用于计算需要逐行比较的
// 证据（例如最快 IP 切换间隔）。最多返回 limit+1 行，超出时由调用方标记截断。
func GetBlackroomIPRowsForEvaluation(userID int, startExclusive int64, endInclusive int64, limit int) ([]BlackroomIPMinute, error) {
	if userID <= 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = BlackroomIPAuditMaxEvaluationRows
	}
	rows := make([]BlackroomIPMinute, 0, limit+1)
	err := DB.
		Where("user_id = ? AND last_seen_at > ? AND first_seen_at <= ?", userID, startExclusive, endInclusive).
		Order("last_seen_at desc, ip asc").
		Limit(limit + 1).
		Find(&rows).Error
	return rows, err
}

// FindBlackroomAuditCandidates 从审计表找出窗口内 IP 数达到阈值的用户。
func FindBlackroomAuditCandidates(windowStart int64, windowEnd int64, minIPCount int, minRequests int, limit int) ([]BlackroomIPCandidate, error) {
	if minIPCount <= 0 {
		return []BlackroomIPCandidate{}, nil
	}
	if limit <= 0 {
		limit = 1000
	}

	tx := DB.Model(&BlackroomIPMinute{}).
		Select("user_id").
		Where("last_seen_at > ? AND first_seen_at <= ?", windowStart, windowEnd).
		Group("user_id").
		Having("COUNT(DISTINCT ip) >= ?", minIPCount)
	if minRequests > 0 {
		// 一行是一个 (用户, IP, 分钟) 组合而非一次请求，因此按请求数求和比较。
		tx = tx.Having("COALESCE(SUM(request_count), 0) >= ?", minRequests)
	}

	var candidates []BlackroomIPCandidate
	err := tx.Order("COUNT(DISTINCT ip) DESC").Limit(limit).Scan(&candidates).Error
	return candidates, err
}

// ListBlackroomIPsForUser 返回窗口内该用户观测到的去重 IP 列表。
func ListBlackroomIPsForUser(userID int, startExclusive int64, endInclusive int64, limit int) ([]string, error) {
	if userID <= 0 {
		return []string{}, nil
	}
	if limit <= 0 {
		limit = 200
	}
	ips := make([]string, 0, limit)
	err := DB.Model(&BlackroomIPMinute{}).
		Distinct("ip").
		Where("user_id = ? AND last_seen_at > ? AND first_seen_at <= ?", userID, startExclusive, endInclusive).
		Order("ip asc").
		Limit(limit).
		Pluck("ip", &ips).Error
	return ips, err
}

// BlackroomIPEvidence 是写入封禁记录的判定证据。
type BlackroomIPEvidence struct {
	WindowStartExclusive int64               `json:"window_start_exclusive"`
	WindowEndInclusive   int64               `json:"window_end_inclusive"`
	DistinctIPs          int                 `json:"distinct_ips"`
	EligibleIPs          int                 `json:"eligible_ips"`
	CountryCount         int                 `json:"country_count"`
	ASNCount             int                 `json:"asn_count"`
	MinimumGapSeconds    int64               `json:"minimum_gap_seconds"`
	RequestCount         int64               `json:"request_count"`
	InputTruncated       bool                `json:"input_truncated"`
	MMDBVersion          string              `json:"mmdb_version,omitempty"`
	Observations         []BlackroomIPMinute `json:"observations,omitempty"`
}

// BuildBlackroomIPEvidence 汇总判定证据。minimumGapSeconds 为 -1 表示
// 窗口内没有两个不同 IP 的合格观测，无法计算间隔。
func BuildBlackroomIPEvidence(rows []BlackroomIPMinute, summary *BlackroomIPAuditSummary, startExclusive int64, endInclusive int64, mmdbVersion string) BlackroomIPEvidence {
	inputTruncated := len(rows) > BlackroomIPAuditMaxEvaluationRows
	if inputTruncated {
		rows = rows[:BlackroomIPAuditMaxEvaluationRows]
	}

	evidence := BlackroomIPEvidence{
		WindowStartExclusive: startExclusive,
		WindowEndInclusive:   endInclusive,
		MinimumGapSeconds:    -1,
		InputTruncated:       inputTruncated,
		MMDBVersion:          mmdbVersion,
		Observations:         make([]BlackroomIPMinute, 0, len(rows)),
	}
	if summary != nil {
		evidence.DistinctIPs = summary.IPCount
		evidence.EligibleIPs = summary.EligibleIPCount
		evidence.CountryCount = summary.CountryCount
		evidence.ASNCount = summary.ASNCount
		evidence.RequestCount = summary.RequestCount
	}

	type observationTime struct {
		Ip        string
		Timestamp int64
	}
	// times 只收合格观测：不合格的行若留在时间轴上，会把真正相邻的两个
	// 合格 IP 隔开，使切换间隔被算成「跨过一次不合格观测」的距离。
	times := make([]observationTime, 0, len(rows)*2)
	eligibleIPs := make(map[string]struct{})
	countries := make(map[string]struct{})
	asns := make(map[int64]struct{})
	allIPs := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		allIPs[row.Ip] = struct{}{}
		if !row.EvidenceEligible || row.IpKind != "public" {
			continue
		}
		eligibleIPs[row.Ip] = struct{}{}
		if row.CountryISO != "" {
			countries[row.CountryISO] = struct{}{}
		}
		if row.AsnNumber > 0 {
			asns[row.AsnNumber] = struct{}{}
		}
		// 同一分钟只观测到一次时两个时刻相同，重复计入会产生虚假的零间隔。
		if row.FirstSeenAt > startExclusive && row.FirstSeenAt <= endInclusive {
			times = append(times, observationTime{Ip: row.Ip, Timestamp: row.FirstSeenAt})
		}
		if row.LastSeenAt != row.FirstSeenAt && row.LastSeenAt > startExclusive && row.LastSeenAt <= endInclusive {
			times = append(times, observationTime{Ip: row.Ip, Timestamp: row.LastSeenAt})
		}
		evidence.Observations = append(evidence.Observations, row)
	}
	if summary == nil {
		evidence.DistinctIPs = len(allIPs)
		evidence.EligibleIPs = len(eligibleIPs)
		evidence.CountryCount = len(countries)
		evidence.ASNCount = len(asns)
	}

	slices.SortFunc(times, func(left, right observationTime) int {
		if left.Timestamp != right.Timestamp {
			return cmp.Compare(left.Timestamp, right.Timestamp)
		}
		return strings.Compare(left.Ip, right.Ip)
	})
	for index := 1; index < len(times); index++ {
		previous := times[index-1]
		current := times[index]
		if previous.Ip == current.Ip {
			continue
		}
		gap := current.Timestamp - previous.Timestamp
		if evidence.MinimumGapSeconds < 0 || gap < evidence.MinimumGapSeconds {
			evidence.MinimumGapSeconds = gap
		}
	}
	slices.SortFunc(evidence.Observations, func(left, right BlackroomIPMinute) int {
		if left.LastSeenAt != right.LastSeenAt {
			return cmp.Compare(right.LastSeenAt, left.LastSeenAt)
		}
		return strings.Compare(left.Ip, right.Ip)
	})
	return evidence
}

// BlackroomIPAuditUser 是 IP 视角下列出的关联用户。
type BlackroomIPAuditUser struct {
	UserId       int    `json:"user_id"`
	Username     string `json:"username"`
	RequestCount int64  `json:"request_count"`
}

// BlackroomIPAuditItem 是 IP 维度的一行聚合结果。
// Users 由查询后另行装载，标记 gorm:"-" 以免被当成关联关系解析。
type BlackroomIPAuditItem struct {
	Ip             string                 `json:"ip"`
	RequestCount   int64                  `json:"request_count"`
	UserCount      int64                  `json:"user_count"`
	FirstSeenAt    int64                  `json:"first_seen_at"`
	LastSeenAt     int64                  `json:"last_seen_at"`
	Users          []BlackroomIPAuditUser `json:"users" gorm:"-"`
	UsersTruncated bool                   `json:"users_truncated" gorm:"-"`
}

// BlackroomIPAuditQuery 是 IP 维度审计的查询条件。
type BlackroomIPAuditQuery struct {
	StartAt  int64
	EndAt    int64
	Keyword  string
	Page     int
	PageSize int
}

// BlackroomIPAuditResult 是 IP 维度审计的分页结果。
type BlackroomIPAuditResult struct {
	StartAt  int64                  `json:"start_at"`
	EndAt    int64                  `json:"end_at"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
	Items    []BlackroomIPAuditItem `json:"items"`
}

// ListBlackroomIPAudit 按 IP 聚合观测，用于发现「一个 IP 被大量账号共用」
// 这类用户维度看不到的信号。
func ListBlackroomIPAudit(query BlackroomIPAuditQuery) (*BlackroomIPAuditResult, error) {
	if query.EndAt <= 0 {
		query.EndAt = common.GetTimestamp()
	}
	if query.StartAt <= 0 || query.StartAt >= query.EndAt {
		query.StartAt = query.EndAt - 24*3600
	}
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = 20
	}
	if query.PageSize > 100 {
		query.PageSize = 100
	}

	result := &BlackroomIPAuditResult{
		StartAt:  query.StartAt,
		EndAt:    query.EndAt,
		Page:     query.Page,
		PageSize: query.PageSize,
		Items:    make([]BlackroomIPAuditItem, 0, query.PageSize),
	}

	tx := DB.Model(&BlackroomIPMinute{}).
		Where("last_seen_at > ? AND first_seen_at <= ?", query.StartAt, query.EndAt)
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		tx = tx.Where("ip LIKE ?", "%"+keyword+"%")
	}
	if err := tx.Distinct("ip").Count(&result.Total).Error; err != nil {
		return nil, err
	}

	items := make([]BlackroomIPAuditItem, 0, query.PageSize)
	err := tx.
		Select(
			"ip, COUNT(DISTINCT user_id) AS user_count, " +
				"COALESCE(SUM(request_count), 0) AS request_count, " +
				"COALESCE(MIN(first_seen_at), 0) AS first_seen_at, " +
				"COALESCE(MAX(last_seen_at), 0) AS last_seen_at",
		).
		Group("ip").
		Order("user_count desc, request_count desc, ip asc").
		Limit(query.PageSize).
		Offset((query.Page - 1) * query.PageSize).
		Scan(&items).Error
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return result, nil
	}

	ips := make([]string, 0, len(items))
	for _, item := range items {
		ips = append(ips, item.Ip)
	}
	userRows := make([]struct {
		Ip           string
		UserId       int
		Username     string
		RequestCount int64
	}, 0)
	err = DB.Model(&BlackroomIPMinute{}).
		Select("ip, user_id, MAX(username) AS username, COALESCE(SUM(request_count), 0) AS request_count").
		Where("last_seen_at > ? AND first_seen_at <= ? AND ip IN ?", query.StartAt, query.EndAt, ips).
		Group("ip, user_id").
		Scan(&userRows).Error
	if err != nil {
		return nil, err
	}

	usersByIP := make(map[string][]BlackroomIPAuditUser, len(items))
	for _, row := range userRows {
		usersByIP[row.Ip] = append(usersByIP[row.Ip], BlackroomIPAuditUser{
			UserId:       row.UserId,
			Username:     row.Username,
			RequestCount: row.RequestCount,
		})
	}
	for i := range items {
		users := usersByIP[items[i].Ip]
		slices.SortFunc(users, func(left, right BlackroomIPAuditUser) int {
			if left.RequestCount != right.RequestCount {
				return cmp.Compare(right.RequestCount, left.RequestCount)
			}
			return cmp.Compare(left.UserId, right.UserId)
		})
		if len(users) > 5 {
			items[i].Users = users[:5]
			items[i].UsersTruncated = true
		} else {
			items[i].Users = users
		}
	}
	result.Items = items
	return result, nil
}

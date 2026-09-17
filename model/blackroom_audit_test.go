package model

import (
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

func observationAt(userID int, ip string, minute time.Time, geo bool) BlackroomIPObservation {
	observation := BlackroomIPObservation{
		UserId:     userID,
		Username:   "audit-user",
		Ip:         ip,
		ObservedAt: minute,
		IpKind:     "public",
	}
	if geo {
		observation.CountryISO = "SG"
		observation.AsnNumber = 4657
		observation.AsnOrganization = "Example ISP"
		observation.ResolverVersion = "country-1/asn-2"
		observation.EvidenceEligible = true
	}
	return observation
}

func TestBlackroomIPObservationAccumulatesWithinMinute(t *testing.T) {
	truncateTables(t)

	minute := time.Date(2026, time.September, 17, 10, 30, 0, 0, time.UTC)
	// 同一 (用户, IP, 分钟) 的三次观测，中间夹着一次更早的时刻，用来验证
	// first_seen_at 取最早、last_seen_at 取最晚，而不是被最后一次写入覆盖。
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute.Add(20*time.Second), true)))
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute.Add(50*time.Second), true)))
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute.Add(5*time.Second), true)))

	var rows []BlackroomIPMinute
	require.NoError(t, DB.Where("user_id = ?", 7).Find(&rows).Error)
	require.Len(t, rows, 1)

	row := rows[0]
	assert.Equal(t, int64(3), row.RequestCount)
	assert.Equal(t, minute.Unix(), row.Minute)
	assert.Equal(t, minute.Add(5*time.Second).Unix(), row.FirstSeenAt)
	assert.Equal(t, minute.Add(50*time.Second).Unix(), row.LastSeenAt)
	assert.Equal(t, "SG", row.CountryISO)
	assert.Equal(t, int64(4657), row.AsnNumber)
	assert.True(t, row.EvidenceEligible)
}

func TestBlackroomIPObservationSeparatesRowsByMinuteAndIP(t *testing.T) {
	truncateTables(t)

	minute := time.Date(2026, time.September, 17, 10, 30, 0, 0, time.UTC)
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute, true)))
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.11", minute, true)))
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute.Add(time.Minute), true)))
	require.NoError(t, RecordBlackroomIPObservation(observationAt(8, "203.0.113.10", minute, true)))

	var count int64
	require.NoError(t, DB.Model(&BlackroomIPMinute{}).Count(&count).Error)
	assert.Equal(t, int64(4), count)
}

func TestBlackroomIPObservationKeepsGeoFromCompleteObservation(t *testing.T) {
	truncateTables(t)

	minute := time.Date(2026, time.September, 17, 10, 30, 0, 0, time.UTC)
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute, true)))
	// 解析器不可用时观测仍然要计数，但不能用空地理信息冲掉已有值，
	// 也不能把该行降级为非证据。
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute.Add(10*time.Second), false)))

	var row BlackroomIPMinute
	require.NoError(t, DB.Where("user_id = ?", 7).First(&row).Error)
	assert.Equal(t, int64(2), row.RequestCount)
	assert.Equal(t, "SG", row.CountryISO)
	assert.Equal(t, int64(4657), row.AsnNumber)
	assert.True(t, row.EvidenceEligible)
}

func TestBlackroomIPObservationEvidenceEligibleIsSticky(t *testing.T) {
	truncateTables(t)

	minute := time.Date(2026, time.September, 17, 10, 30, 0, 0, time.UTC)
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute, false)))
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute.Add(10*time.Second), true)))
	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute.Add(20*time.Second), false)))

	var row BlackroomIPMinute
	require.NoError(t, DB.Where("user_id = ?", 7).First(&row).Error)
	assert.True(t, row.EvidenceEligible)
	assert.Equal(t, "SG", row.CountryISO)
}

func TestSummarizeBlackroomIPAuditCountsDimensionsInWindow(t *testing.T) {
	truncateTables(t)

	minute := time.Date(2026, time.September, 17, 10, 30, 0, 0, time.UTC).Unix()
	seeded := []BlackroomIPMinute{
		{UserId: 7, Ip: "203.0.113.10", Minute: minute, FirstSeenAt: minute, LastSeenAt: minute + 10, CountryISO: "SG", AsnNumber: 4657, IpKind: "public", EvidenceEligible: true, RequestCount: 3},
		{UserId: 7, Ip: "203.0.113.11", Minute: minute, FirstSeenAt: minute, LastSeenAt: minute + 20, CountryISO: "JP", AsnNumber: 2497, IpKind: "public", EvidenceEligible: true, RequestCount: 2},
		{UserId: 7, Ip: "203.0.113.12", Minute: minute, FirstSeenAt: minute, LastSeenAt: minute + 30, CountryISO: "SG", AsnNumber: 4657, IpKind: "public", EvidenceEligible: true, RequestCount: 1},
		// 没有解析结果的行只计入 IP 数与请求数，不计入国家/ASN。
		{UserId: 7, Ip: "203.0.113.13", Minute: minute, FirstSeenAt: minute, LastSeenAt: minute + 40, IpKind: "unknown", RequestCount: 5},
		// 窗口之外的观测不应被计入。
		{UserId: 7, Ip: "198.51.100.1", Minute: minute - 7200, FirstSeenAt: minute - 7200, LastSeenAt: minute - 7200, CountryISO: "US", AsnNumber: 1, IpKind: "public", EvidenceEligible: true, RequestCount: 9},
	}
	require.NoError(t, DB.Create(&seeded).Error)

	summary, err := SummarizeBlackroomIPAudit(7, minute-3600, minute+60)
	require.NoError(t, err)
	assert.Equal(t, 4, summary.IPCount)
	assert.Equal(t, 2, summary.CountryCount)
	assert.Equal(t, 2, summary.ASNCount)
	assert.Equal(t, 3, summary.EligibleIPCount)
	assert.Equal(t, int64(11), summary.RequestCount)
}

func TestBuildBlackroomIPEvidenceComputesMinimumGap(t *testing.T) {
	base := int64(1_800_000_000)
	rows := []BlackroomIPMinute{
		{UserId: 7, Ip: "203.0.113.10", FirstSeenAt: base, LastSeenAt: base, CountryISO: "SG", AsnNumber: 4657, IpKind: "public", EvidenceEligible: true},
		{UserId: 7, Ip: "203.0.113.11", FirstSeenAt: base + 90, LastSeenAt: base + 90, CountryISO: "JP", AsnNumber: 2497, IpKind: "public", EvidenceEligible: true},
		{UserId: 7, Ip: "203.0.113.12", FirstSeenAt: base + 300, LastSeenAt: base + 300, CountryISO: "US", AsnNumber: 15169, IpKind: "public", EvidenceEligible: true},
	}

	evidence := BuildBlackroomIPEvidence(rows, nil, base-60, base+600, "country-1/asn-2")
	// 相邻间隔为 90 和 210，证据取最小值 90。
	assert.Equal(t, int64(90), evidence.MinimumGapSeconds)
	assert.Equal(t, 3, evidence.EligibleIPs)
	assert.Equal(t, 3, evidence.CountryCount)
	assert.Equal(t, 3, evidence.ASNCount)
	assert.False(t, evidence.InputTruncated)

	// 只有一个 IP 时无法计算切换间隔，用 -1 表示。
	single := BuildBlackroomIPEvidence(rows[:1], nil, base-60, base+600, "")
	assert.Equal(t, int64(-1), single.MinimumGapSeconds)
}

func TestBuildBlackroomIPEvidenceIgnoresIneligibleIPsForGap(t *testing.T) {
	base := int64(1_800_000_000)
	rows := []BlackroomIPMinute{
		{UserId: 7, Ip: "203.0.113.10", FirstSeenAt: base, LastSeenAt: base, CountryISO: "SG", AsnNumber: 4657, IpKind: "public", EvidenceEligible: true},
		// 私有地址与解析失败的行不能参与快速切换判定。
		{UserId: 7, Ip: "10.0.0.5", FirstSeenAt: base + 10, LastSeenAt: base + 10, IpKind: "private"},
		{UserId: 7, Ip: "203.0.113.11", FirstSeenAt: base + 200, LastSeenAt: base + 200, CountryISO: "JP", AsnNumber: 2497, IpKind: "public", EvidenceEligible: true},
	}

	evidence := BuildBlackroomIPEvidence(rows, nil, base-60, base+600, "")
	assert.Equal(t, int64(200), evidence.MinimumGapSeconds)
	assert.Equal(t, 2, evidence.EligibleIPs)
}

func TestBuildBlackroomIPEvidenceMarksTruncatedInput(t *testing.T) {
	rows := make([]BlackroomIPMinute, 0, BlackroomIPAuditMaxEvaluationRows+1)
	for i := range BlackroomIPAuditMaxEvaluationRows + 1 {
		rows = append(rows, BlackroomIPMinute{
			UserId: 7, Ip: "203.0.113." + string(rune('a'+i%26)), IpKind: "public",
		})
	}

	evidence := BuildBlackroomIPEvidence(rows, nil, 0, 1_800_000_000, "")
	assert.True(t, evidence.InputTruncated)
}

func TestCleanupRemovesExpiredBlackroomIPObservations(t *testing.T) {
	truncateTables(t)
	previousDay := blackroomAuditCleanupDay.Load()
	blackroomAuditCleanupDay.Store(0)
	t.Cleanup(func() { blackroomAuditCleanupDay.Store(previousDay) })

	now := time.Date(2026, time.September, 17, 10, 0, 0, 0, time.UTC)
	expired := now.AddDate(0, 0, -(BlackroomIPAuditRetentionDays + 1))
	fresh := now.Add(-time.Hour)
	require.NoError(t, DB.Create(&BlackroomIPMinute{
		UserId: 7, Ip: "198.51.100.1", Minute: expired.Unix(), FirstSeenAt: expired.Unix(), LastSeenAt: expired.Unix(),
	}).Error)
	require.NoError(t, DB.Create(&BlackroomIPMinute{
		UserId: 7, Ip: "203.0.113.10", Minute: fresh.Unix(), FirstSeenAt: fresh.Unix(), LastSeenAt: fresh.Unix(),
	}).Error)

	require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.99", now, true)))

	var ips []string
	require.NoError(t, DB.Model(&BlackroomIPMinute{}).Order("ip asc").Pluck("ip", &ips).Error)
	assert.Equal(t, []string{"203.0.113.10", "203.0.113.99"}, ips)
}

func TestListBlackroomIPAuditGroupsUsersByIP(t *testing.T) {
	truncateTables(t)

	minute := time.Date(2026, time.September, 17, 10, 30, 0, 0, time.UTC).Unix()
	seeded := []BlackroomIPMinute{
		{UserId: 7, Ip: "203.0.113.10", Username: "alice", Minute: minute, FirstSeenAt: minute, LastSeenAt: minute, RequestCount: 5},
		{UserId: 8, Ip: "203.0.113.10", Username: "bob", Minute: minute, FirstSeenAt: minute, LastSeenAt: minute, RequestCount: 3},
		{UserId: 9, Ip: "203.0.113.11", Username: "carol", Minute: minute, FirstSeenAt: minute, LastSeenAt: minute, RequestCount: 1},
	}
	require.NoError(t, DB.Create(&seeded).Error)

	result, err := ListBlackroomIPAudit(BlackroomIPAuditQuery{StartAt: minute - 600, EndAt: minute + 600})
	require.NoError(t, err)
	require.Len(t, result.Items, 2)
	assert.Equal(t, int64(2), result.Total)

	// 默认按关联用户数倒序，共用 IP 排在前面。
	assert.Equal(t, "203.0.113.10", result.Items[0].Ip)
	assert.Equal(t, int64(2), result.Items[0].UserCount)
	assert.Equal(t, int64(8), result.Items[0].RequestCount)
	require.Len(t, result.Items[0].Users, 2)
	assert.Equal(t, 7, result.Items[0].Users[0].UserId)
	assert.Equal(t, int64(5), result.Items[0].Users[0].RequestCount)
}

// openBlackroomAuditMatrixDB 为指定方言打开一个隔离连接：SQLite 用独立内存库，
// MySQL 用表前缀，PostgreSQL 用 schema（其索引名是 schema 级的，前缀无法隔离）。
func openBlackroomAuditMatrixDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()

	if dialect == "sqlite" {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = sqlDB.Close() })
		return db
	}

	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dialect == "postgres" {
		dsn = os.Getenv("TEST_POSTGRES_DSN")
	}
	if dsn == "" {
		t.Skipf("%s is not configured", dialect)
	}

	t.Setenv("BLACKROOM_AUDIT_MATRIX_DSN", dsn)
	prefix := "bra_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "") + "_"
	db, _, err := chooseDB("BLACKROOM_AUDIT_MATRIX_DSN", false)
	require.NoError(t, err)

	if dialect == "postgres" {
		require.NoError(t, db.Exec("CREATE SCHEMA ?", clause.Table{Name: prefix}).Error)
		parsed, err := url.Parse(dsn)
		require.NoError(t, err)
		query := parsed.Query()
		query.Set("search_path", prefix)
		parsed.RawQuery = query.Encode()
		t.Setenv("BLACKROOM_AUDIT_MATRIX_DSN", parsed.String())
		db, _, err = chooseDB("BLACKROOM_AUDIT_MATRIX_DSN", false)
		require.NoError(t, err)
	}

	db.Config.NamingStrategy = schema.NamingStrategy{TablePrefix: prefix}
	db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
	t.Cleanup(func() {
		assert.NoError(t, db.Migrator().DropTable(&BlackroomIPMinute{}))
		if dialect == "postgres" {
			assert.NoError(t, db.Exec("DROP SCHEMA ? CASCADE", clause.Table{Name: prefix}).Error)
		}
		sqlDB, err := db.DB()
		require.NoError(t, err)
		assert.NoError(t, sqlDB.Close())
	})
	return db
}

// TestBlackroomIPAuditDatabaseMatrix 在真实 SQLite / MySQL / PostgreSQL 上验证
// 迁移与核心 SQL 语义：upsert 累加、最早/最晚时刻、地理字段的条件覆盖，
// 以及 COUNT(DISTINCT ... CASE WHEN ...) 聚合。
func TestBlackroomIPAuditDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openBlackroomAuditMatrixDB(t, dialect)

			previousDB, previousLogDB := DB, LOG_DB
			previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(dbTypeOf(dialect), dbTypeOf(dialect))
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = previousDB, previousLogDB
				common.SetDatabaseTypes(previousMainType, previousLogType)
				initCol()
			})

			// 迁移跑两次，第二次必须不产生任何变更（幂等）。
			require.NoError(t, db.AutoMigrate(&BlackroomIPMinute{}))
			recorder := &migrationSQLRecorder{}
			require.NoError(t, db.Session(&gorm.Session{Logger: recorder}).AutoMigrate(&BlackroomIPMinute{}))
			assert.Empty(t, recorder.schemaMutations(), "重启不应重写表结构")

			var version string
			versionSQL := "SELECT VERSION()"
			if dialect == "sqlite" {
				versionSQL = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(versionSQL).Scan(&version).Error)
			t.Logf("真实数据库 %s 版本=%s", dialect, version)

			minute := time.Date(2026, time.September, 17, 10, 30, 0, 0, time.UTC)
			require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute.Add(30*time.Second), true)))
			require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.10", minute.Add(10*time.Second), false)))
			require.NoError(t, RecordBlackroomIPObservation(observationAt(7, "203.0.113.11", minute, true)))

			var rows []BlackroomIPMinute
			require.NoError(t, db.Where("user_id = ?", 7).Order("ip asc").Find(&rows).Error)
			require.Len(t, rows, 2)
			assert.Equal(t, int64(2), rows[0].RequestCount)
			assert.Equal(t, minute.Add(10*time.Second).Unix(), rows[0].FirstSeenAt)
			assert.Equal(t, minute.Add(30*time.Second).Unix(), rows[0].LastSeenAt)
			// 后写入的不完整观测不能让该行丢掉地理信息或证据资格。
			assert.Equal(t, "SG", rows[0].CountryISO)
			assert.True(t, rows[0].EvidenceEligible)

			summary, err := SummarizeBlackroomIPAudit(7, minute.Unix()-600, minute.Unix()+600)
			require.NoError(t, err)
			assert.Equal(t, 2, summary.IPCount)
			assert.Equal(t, 1, summary.CountryCount)
			assert.Equal(t, 1, summary.ASNCount)
			assert.Equal(t, int64(3), summary.RequestCount)

			// 候选筛选在 HAVING/ORDER BY 里使用聚合表达式，需在各方言上验证。
			// 先为第二个用户造出更多 IP，确认排序与阈值都生效。
			require.NoError(t, RecordBlackroomIPObservation(observationAt(8, "198.51.100.1", minute, true)))
			require.NoError(t, RecordBlackroomIPObservation(observationAt(8, "198.51.100.2", minute, true)))
			require.NoError(t, RecordBlackroomIPObservation(observationAt(8, "198.51.100.3", minute, true)))

			candidates, err := FindBlackroomAuditCandidates(minute.Unix()-600, minute.Unix()+600, 2, 0, 100)
			require.NoError(t, err)
			require.Len(t, candidates, 2)
			// 用户 8 有 3 个 IP，排在只有 2 个 IP 的用户 7 前面。
			assert.Equal(t, 8, candidates[0].UserId)
			assert.Equal(t, 7, candidates[1].UserId)

			// minRequests 按请求数（而非行数）过滤。
			candidates, err = FindBlackroomAuditCandidates(minute.Unix()-600, minute.Unix()+600, 2, 3, 100)
			require.NoError(t, err)
			require.Len(t, candidates, 2)

			candidates, err = FindBlackroomAuditCandidates(minute.Unix()-600, minute.Unix()+600, 2, 4, 100)
			require.NoError(t, err)
			assert.Empty(t, candidates)
		})
	}
}

// dbTypeOf 把测试方言名映射到 common 的数据库类型常量。
func dbTypeOf(dialect string) common.DatabaseType {
	switch dialect {
	case "mysql":
		return common.DatabaseTypeMySQL
	case "postgres":
		return common.DatabaseTypePostgreSQL
	default:
		return common.DatabaseTypeSQLite
	}
}

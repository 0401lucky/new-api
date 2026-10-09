package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const activityTestDay = int64(86400)

func setupUserActivityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupManageUserTestDB(t)
	// Initialize the dialect-specific reserved-column names used by TokenAuth,
	// while retaining the fixture's separately configured log connection.
	logDB, master := model.LOG_DB, common.IsMasterNode
	t.Setenv("LOG_SQL_DSN", "")
	common.IsMasterNode = false
	err := model.InitLogDB()
	model.LOG_DB, common.IsMasterNode = logDB, master
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.UserRequestActivity{}, &model.UserActivityState{}, &model.Token{}, &model.BlackroomBan{},
		&model.TwoFABackupCode{}, &model.ExternalIdentityClaim{}, &model.UserOAuthBinding{}, &model.Option{},
	))
	return db
}

func createActivityTestUser(t *testing.T, db *gorm.DB, name string, createdAt, lastRequestAt int64) model.User {
	t.Helper()
	user := model.User{
		Username: name, DisplayName: name, Email: name + "@example.com", Password: "private-password-hash",
		AffCode: name, Role: common.RoleCommonUser, Status: common.UserStatusEnabled,
		Group: "default", CreatedAt: createdAt, AuthVersion: 1, Setting: `{"private":"user-settings"}`,
	}
	require.NoError(t, db.Create(&user).Error)
	if lastRequestAt > 0 {
		require.NoError(t, model.RecordUserRequestActivity(user.Id, lastRequestAt))
	}
	return user
}

func TestUserActivityBackfillAndRestartPreserveHistory(t *testing.T) {
	db := setupUserActivityTestDB(t)
	now := common.GetTimestamp()
	failed := createActivityTestUser(t, db, "failed-only", now-60*activityTestDay, 0)
	old := createActivityTestUser(t, db, "missing-history", now-60*activityTestDay, 0)
	for _, log := range []model.Log{
		{UserId: failed.Id, Type: model.LogTypeConsume, CreatedAt: now - 20*activityTestDay},
		{UserId: failed.Id, Type: model.LogTypeError, CreatedAt: now - activityTestDay},
		{UserId: failed.Id, Type: model.LogTypeLogin, CreatedAt: now},
		{UserId: old.Id, Type: model.LogTypeTopup, CreatedAt: now},
		{UserId: old.Id, Type: model.LogTypeRefund, CreatedAt: now},
	} {
		require.NoError(t, model.LOG_DB.Create(&log).Error)
	}
	// The pre-existing users/logs schema is unchanged; only the independent
	// activity tables are migrated, twice, as on first upgrade and restart.
	for range 2 {
		require.NoError(t, db.AutoMigrate(&model.UserRequestActivity{}, &model.UserActivityState{}))
		require.NoError(t, model.InitUserActivity())
	}
	var checkpoint model.UserActivityState
	require.NoError(t, db.First(&checkpoint, 1).Error)
	assert.Positive(t, checkpoint.TrackingStartedAt)
	assert.Positive(t, checkpoint.BackfillCompletedAt)

	page, err := model.GetUserActivity(model.UserActivityFilter{}, &common.PageInfo{Page: 1, PageSize: 20}, now)
	require.NoError(t, err)
	assert.EqualValues(t, 1, page.Summary.Active)
	assert.EqualValues(t, 1, page.Summary.Unknown)
	assert.Zero(t, page.Summary.NeverRequested)
	assert.Zero(t, page.Summary.CleanupCandidates)
	var request model.UserRequestActivity
	require.NoError(t, db.First(&request, "user_id = ?", failed.Id).Error)
	assert.Equal(t, now-activityTestDay, request.LastRequestAt)

	// Log cleanup and an out-of-order write must not erase a request or move
	// the last-request timestamp backwards.
	require.NoError(t, model.LOG_DB.Where("user_id IN ?", []int{failed.Id, old.Id}).Delete(&model.Log{}).Error)
	require.NoError(t, model.RecordUserRequestActivity(failed.Id, now))
	require.NoError(t, model.RecordUserRequestActivity(failed.Id, now-2*activityTestDay))
	require.NoError(t, model.InitUserActivity())
	require.NoError(t, db.First(&request, "user_id = ?", failed.Id).Error)
	assert.Equal(t, now, request.LastRequestAt)
	var preserved model.User
	require.NoError(t, db.First(&preserved, old.Id).Error)
	assert.Equal(t, old.Password, preserved.Password)
	assert.Equal(t, old.CreatedAt, preserved.CreatedAt)
	duplicate := old
	duplicate.Id = 0
	assert.Error(t, db.Create(&duplicate).Error, "existing username/affiliate uniqueness survives the migration")
}

func TestUserActivityFreshDatabaseAndClassification(t *testing.T) {
	db := setupUserActivityTestDB(t)
	now := common.GetTimestamp()
	start := now - 90*activityTestDay
	require.NoError(t, db.Create(&model.UserActivityState{ID: 1, TrackingStartedAt: start, BackfillCompletedAt: start}).Error)
	for range 2 {
		require.NoError(t, db.AutoMigrate(&model.UserRequestActivity{}, &model.UserActivityState{}))
		require.NoError(t, model.InitUserActivity())
	}
	empty, err := model.GetUserActivity(model.UserActivityFilter{}, &common.PageInfo{Page: 1, PageSize: 20}, now)
	require.NoError(t, err)
	assert.Empty(t, empty.Items)
	assert.Zero(t, empty.Total)
	fixtures := []struct {
		name     string
		created  int64
		request  int64
		activity string
		eligible bool
	}{
		{"active-boundary", now - 60*activityTestDay, now - 7*activityTestDay, "active", false},
		{"inactive-boundary", now - 60*activityTestDay, now - 7*activityTestDay - 1, "inactive", false},
		{"inactive-recent", now - 60*activityTestDay, now - 30*activityTestDay + 1, "inactive", false},
		{"old-boundary", now - 60*activityTestDay, now - 30*activityTestDay, "very_inactive", true},
		{"old-request", now - 60*activityTestDay, now - 31*activityTestDay, "very_inactive", true},
		{"never-old", now - 30*activityTestDay, 0, "never_requested", true},
		{"never-new", now - activityTestDay, 0, "never_requested", false},
		{"unknown-old", now - 120*activityTestDay, 0, "unknown", true},
	}
	ids := make(map[string]int, len(fixtures))
	for _, fixture := range fixtures {
		user := createActivityTestUser(t, db, fixture.name, fixture.created, fixture.request)
		ids[fixture.name] = user.Id
	}
	deleted := createActivityTestUser(t, db, "already-deleted", now-60*activityTestDay, now)
	require.NoError(t, db.Delete(&deleted).Error)
	admin := createActivityTestUser(t, db, "protected-admin", now-60*activityTestDay, now-40*activityTestDay)
	require.NoError(t, db.Model(&admin).Update("role", common.RoleAdminUser).Error)
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", ids["old-request"]).Updates(map[string]any{"group": "premium", "status": common.UserStatusDisabled}).Error)

	page, err := model.GetUserActivity(model.UserActivityFilter{}, &common.PageInfo{Page: 1, PageSize: 20}, now)
	require.NoError(t, err)
	assert.EqualValues(t, 9, page.Total)
	assert.Equal(t, model.UserActivitySummary{Total: 9, Active: 1, Inactive: 2, VeryInactive: 3, NeverRequested: 2, Unknown: 1, CleanupCandidates: 4}, page.Summary)
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			filtered, err := model.GetUserActivity(model.UserActivityFilter{Keyword: fmt.Sprintf("#%d", ids[fixture.name])}, &common.PageInfo{Page: 1, PageSize: 20}, now)
			require.NoError(t, err)
			require.Len(t, filtered.Items, 1)
			item := filtered.Items[0]
			assert.Equal(t, fixture.activity, item.Activity)
			assert.Equal(t, fixture.eligible, item.CleanupEligible)
			assert.Equal(t, fixture.request, item.LastRequestAt)
			assert.Empty(t, item.Password)
			assert.Empty(t, item.Setting)
			assert.Nil(t, item.AccessToken)
		})
	}
	status, role := common.UserStatusDisabled, common.RoleCommonUser
	filtered, err := model.GetUserActivity(model.UserActivityFilter{Keyword: "request", Group: "premium", Status: &status, Role: &role, Activity: "cleanup"}, &common.PageInfo{Page: 1, PageSize: 20}, now)
	require.NoError(t, err)
	require.Len(t, filtered.Items, 1)
	assert.Equal(t, ids["old-request"], filtered.Items[0].Id)
	assert.EqualValues(t, 1, filtered.Summary.Total)

	first, err := model.GetUserActivity(model.UserActivityFilter{Activity: "cleanup", SortBy: "id"}, &common.PageInfo{Page: 1, PageSize: 2}, now)
	require.NoError(t, err)
	second, err := model.GetUserActivity(model.UserActivityFilter{Activity: "cleanup", SortBy: "id"}, &common.PageInfo{Page: 2, PageSize: 2}, now)
	require.NoError(t, err)
	require.Len(t, first.Items, 2)
	require.Len(t, second.Items, 2)
	assert.EqualValues(t, 4, first.Total)
	assert.Equal(t, []int{ids["old-boundary"], ids["old-request"], ids["never-old"], ids["unknown-old"]}, []int{first.Items[0].Id, first.Items[1].Id, second.Items[0].Id, second.Items[1].Id})
	assert.Equal(t, page.Summary, first.Summary, "category selection does not change the summary for the current search filters")
}

func TestUserActivityPreservesRequestsWithMissingRegistrationTime(t *testing.T) {
	db := setupUserActivityTestDB(t)
	now := common.GetTimestamp()
	start := now - 90*activityTestDay
	require.NoError(t, db.Create(&model.UserActivityState{ID: 1, TrackingStartedAt: start, BackfillCompletedAt: start}).Error)
	fixtures := []struct {
		name        string
		createdAt   any
		requestAt   int64
		wantRequest int64
		activity    string
	}{
		{"legacy-active", nil, now - activityTestDay, now - activityTestDay, "active"},
		{"legacy-inactive", nil, now - 14*activityTestDay, now - 14*activityTestDay, "inactive"},
		{"legacy-old", nil, now - 40*activityTestDay, now - 40*activityTestDay, "very_inactive"},
		{"legacy-unknown", nil, 0, 0, "unknown"},
		{"reused-account", now - activityTestDay, now - 40*activityTestDay, 0, "never_requested"},
	}
	ids := make([]int, len(fixtures))
	for i, fixture := range fixtures {
		user := createActivityTestUser(t, db, fixture.name, now-120*activityTestDay, fixture.requestAt)
		require.NoError(t, db.Model(&user).Update("created_at", fixture.createdAt).Error)
		ids[i] = user.Id
	}

	page, err := model.GetUserActivity(model.UserActivityFilter{SortBy: "id"}, &common.PageInfo{Page: 1, PageSize: 20}, now)
	require.NoError(t, err)
	require.Len(t, page.Items, len(fixtures))
	assert.Equal(t, model.UserActivitySummary{Total: 5, Active: 1, Inactive: 1, VeryInactive: 1, NeverRequested: 1, Unknown: 1}, page.Summary)
	for i, fixture := range fixtures {
		item := page.Items[i]
		assert.Equal(t, ids[i], item.Id)
		assert.Equal(t, fixture.activity, item.Activity, fixture.name)
		assert.Equal(t, fixture.wantRequest, item.LastRequestAt, fixture.name)
		assert.False(t, item.CleanupEligible, fixture.name)
	}
	assert.EqualValues(t, 40, page.Items[2].NoRequestDays)
	assert.EqualValues(t, 90, page.Items[3].NoRequestDays)

	// Missing registration dates must not make legacy accounts eligible for
	// permanent cleanup, even after a full observation window without requests.
	deleted, err := model.DeleteInactiveUsers([]int{ids[2], ids[3]}, authz.ClearUserAuthorizationInTx)
	assert.ErrorIs(t, err, model.ErrInactiveUserSelection)
	assert.Empty(t, deleted)
	var count int64
	require.NoError(t, db.Model(&model.User{}).Where("id IN ?", ids).Count(&count).Error)
	assert.EqualValues(t, len(fixtures), count)
}

func TestUserActivityCountsSuccessfulAndFailedRequestsWithoutConsumeLogs(t *testing.T) {
	db := setupUserActivityTestDB(t)
	now := common.GetTimestamp()
	user := createActivityTestUser(t, db, "request-owner", now-activityTestDay, 0)
	previousLogConsume := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = previousLogConsume })
	key := "activityrequestkey0123456789"
	token := model.Token{UserId: user.Id, Key: key, Status: common.TokenStatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	for _, tc := range []struct {
		name     string
		tag      string
		key      string
		status   int
		response int
		want     bool
	}{
		{"success", "relay", key, common.TokenStatusEnabled, http.StatusOK, true},
		{"upstream failure", "relay", key, common.TokenStatusEnabled, http.StatusServiceUnavailable, true},
		{"expired token", "relay", key, common.TokenStatusExpired, http.StatusUnauthorized, true},
		{"unknown token", "relay", "unknownkey", common.TokenStatusEnabled, http.StatusUnauthorized, false},
		{"dashboard activity", "api", key, common.TokenStatusEnabled, http.StatusOK, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, db.Where("user_id = ?", user.Id).Delete(&model.UserRequestActivity{}).Error)
			require.NoError(t, db.Model(&token).Update("status", tc.status).Error)
			engine := gin.New()
			engine.POST("/v1/chat/completions", middleware.RouteTag(tc.tag), middleware.TokenAuth(), func(c *gin.Context) {
				c.AbortWithStatus(tc.response)
			})
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			request.Header.Set("Authorization", "Bearer "+tc.key)
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			var activity []model.UserRequestActivity
			require.NoError(t, db.Where("user_id = ?", user.Id).Find(&activity).Error)
			if tc.want {
				require.Len(t, activity, 1)
				assert.GreaterOrEqual(t, activity[0].LastRequestAt, now)
			} else {
				assert.Empty(t, activity)
			}
			assert.Equal(t, tc.response, recorder.Code)
		})
	}
}

func activityCleanupRequest(t *testing.T, engine *gin.Engine, accessToken, proof string, ids []int) *httptest.ResponseRecorder {
	t.Helper()
	body, err := common.Marshal(service.AdminUserBatchDeleteContext{UserIDs: ids})
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/user/activity/batch-delete", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	if proof != "" {
		request.Header.Set("X-Security-Proof", proof)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder
}

func activityCleanupProof(t *testing.T, identity service.AuthIdentity, ids []int) string {
	t.Helper()
	context, err := common.Marshal(service.AdminUserBatchDeleteContext{UserIDs: ids})
	require.NoError(t, err)
	binding, err := service.BindVerificationOperation(service.VerificationOperation{Scope: service.VerificationScopeAdminUserBatchDelete, Context: context})
	require.NoError(t, err)
	proof, _, err := service.IssueSecurityProof(identity, service.VerificationMethodPassword, binding)
	require.NoError(t, err)
	return proof
}

func TestInactiveUserCleanupRequiresBoundSingleUseProofAndRevokesAccess(t *testing.T) {
	db := setupUserActivityTestDB(t)
	redisServer := miniredis.RunT(t)
	previousRDB := common.RDB
	common.RDB = redis.NewClient(&redis.Options{Addr: redisServer.Addr(), MaxRetries: -1})
	common.RedisEnabled = true
	t.Cleanup(func() { _ = common.RDB.Close(); common.RDB = previousRDB })
	now := common.GetTimestamp()
	start := now - 90*activityTestDay
	require.NoError(t, db.Create(&model.UserActivityState{ID: 1, TrackingStartedAt: start, BackfillCompletedAt: start}).Error)
	first := createActivityTestUser(t, db, "cleanup-first", now-60*activityTestDay, now-40*activityTestDay)
	second := createActivityTestUser(t, db, "cleanup-second", now-40*activityTestDay, 0)
	op := createQuotaTestOperator(t, db, common.RoleAdminUser)
	require.NoError(t, authz.Init(db))
	login, err := service.CreateLoginSession(op.Id, "password", "127.0.0.1", "activity-cleanup")
	require.NoError(t, err)
	identity, err := service.ParseAccessToken(login.AccessToken)
	require.NoError(t, err)
	victimSession, err := service.CreateLoginSession(first.Id, "password", "127.0.0.1", "victim")
	require.NoError(t, err)
	key := "cleanupvictimkey0123456789"
	require.NoError(t, db.Create(&model.Token{UserId: first.Id, Key: key, UnlimitedQuota: true, ExpiredTime: -1, Status: common.TokenStatusEnabled}).Error)
	_, err = model.GetTokenByKey(key, false)
	require.NoError(t, err)
	engine := gin.New()
	engine.POST("/api/user/activity/batch-delete", middleware.AdminAuth(), BatchDeleteInactiveUsers)
	engine.GET("/api/user/activity", middleware.AdminAuth(), GetUserActivity)
	engine.GET("/api/user/self", middleware.UserAuth(), func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.POST("/v1/chat/completions", middleware.RouteTag("relay"), middleware.TokenAuth(), func(c *gin.Context) { c.Status(http.StatusOK) })
	ids := []int{first.Id, second.Id}
	profileRequest := httptest.NewRequest(http.MethodGet, "/api/user/self", nil)
	profileRequest.Header.Set("Authorization", "Bearer "+victimSession.AccessToken)
	profileResponse := httptest.NewRecorder()
	engine.ServeHTTP(profileResponse, profileRequest)
	require.Equal(t, http.StatusOK, profileResponse.Code)

	assert.Equal(t, http.StatusForbidden, activityCleanupRequest(t, engine, login.AccessToken, "", ids).Code)
	wrong := activityCleanupProof(t, identity, []int{first.Id})
	assert.Equal(t, http.StatusForbidden, activityCleanupRequest(t, engine, login.AccessToken, wrong, ids).Code)
	expired := activityCleanupProof(t, identity, ids)
	require.NoError(t, db.Model(&model.AuthFlow{}).Where("user_id = ? AND purpose = ?", op.Id, model.AuthFlowPurposeSecurityProof).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	assert.Equal(t, http.StatusForbidden, activityCleanupRequest(t, engine, login.AccessToken, expired, ids).Code)
	var count int64
	require.NoError(t, db.Model(&model.User{}).Where("id IN ?", ids).Count(&count).Error)
	assert.EqualValues(t, 2, count)

	// A non-administrator cannot list user activity or invoke cleanup even if
	// they possess someone else's proof.
	proof := activityCleanupProof(t, identity, ids)
	assert.Equal(t, http.StatusForbidden, activityCleanupRequest(t, engine, victimSession.AccessToken, proof, ids).Code)
	readRequest := httptest.NewRequest(http.MethodGet, "/api/user/activity", nil)
	readRequest.Header.Set("Authorization", "Bearer "+victimSession.AccessToken)
	readResponse := httptest.NewRecorder()
	engine.ServeHTTP(readResponse, readRequest)
	assert.Equal(t, http.StatusForbidden, readResponse.Code)

	// An administrator with MFA enabled cannot use a password-only proof for
	// the new batch scope as an alternative path around that assurance level.
	mfa := model.TwoFA{UserId: op.Id, Secret: "test-secret", IsEnabled: true}
	require.NoError(t, db.Create(&mfa).Error)
	assert.Equal(t, http.StatusForbidden, activityCleanupRequest(t, engine, login.AccessToken, proof, ids).Code)
	require.NoError(t, db.Unscoped().Delete(&mfa).Error)

	changed := createActivityTestUser(t, db, "resumed-account", now-60*activityTestDay, now-40*activityTestDay)
	staleIDs := []int{first.Id, changed.Id}
	staleProof := activityCleanupProof(t, identity, staleIDs)
	require.NoError(t, model.RecordUserRequestActivity(changed.Id, now))
	assert.Equal(t, http.StatusConflict, activityCleanupRequest(t, engine, login.AccessToken, staleProof, staleIDs).Code)
	assert.Equal(t, http.StatusForbidden, activityCleanupRequest(t, engine, login.AccessToken, staleProof, staleIDs).Code, "a rejected cleanup must not restore its consumed proof")

	response := activityCleanupRequest(t, engine, login.AccessToken, proof, []int{second.Id, first.Id})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `"deleted":2`)
	require.NoError(t, db.Unscoped().Model(&model.User{}).Where("id IN ?", ids).Count(&count).Error)
	assert.Zero(t, count)
	for _, data := range []any{&model.Token{}, &model.UserSession{}, &model.UserAccessToken{}, &model.UserRequestActivity{}} {
		require.NoError(t, db.Unscoped().Model(data).Where("user_id IN ?", ids).Count(&count).Error)
		assert.Zero(t, count)
	}
	assert.Equal(t, http.StatusForbidden, activityCleanupRequest(t, engine, login.AccessToken, proof, ids).Code)
	_, err = service.ParseAccessToken(victimSession.AccessToken)
	// Parsing a signed access JWT is not session authorization. The request
	// middleware must reject it after the account/session records are removed.
	require.NoError(t, err)
	profileResponse = httptest.NewRecorder()
	engine.ServeHTTP(profileResponse, profileRequest)
	assert.Equal(t, http.StatusUnauthorized, profileResponse.Code)
	relayRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	relayRequest.Header.Set("Authorization", "Bearer "+key)
	relayResponse := httptest.NewRecorder()
	engine.ServeHTTP(relayResponse, relayRequest)
	assert.Equal(t, http.StatusUnauthorized, relayResponse.Code, "cached relay tokens must not authorize a deleted account")
	var audits []model.AuditLog
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND action = ?", op.Id, "user.delete").Find(&audits).Error)
	require.Len(t, audits, 2)
	serialized, err := common.Marshal(audits)
	require.NoError(t, err)
	assert.NotContains(t, string(serialized), proof)
	assert.NotContains(t, string(serialized), key)
	assert.NotContains(t, string(serialized), victimSession.AccessToken)
}

func TestInactiveUserCleanupRejectsInvalidSelections(t *testing.T) {
	for _, context := range []string{
		`{"user_ids":[]}`, `{"user_ids":[1,1]}`, `{"user_ids":[0]}`, `{"user_ids":[-1]}`,
		`{"user_ids":[1],"all":true}`, `{"user_ids":["1"]}`,
	} {
		t.Run(context, func(t *testing.T) {
			_, err := service.BindVerificationOperation(service.VerificationOperation{Scope: service.VerificationScopeAdminUserBatchDelete, Context: []byte(context)})
			assert.ErrorIs(t, err, service.ErrVerificationContextInvalid)
		})
	}
	tooMany := make([]int, model.UserActivityBatchLimit+1)
	for i := range tooMany {
		tooMany[i] = i + 1
	}
	context, err := common.Marshal(service.AdminUserBatchDeleteContext{UserIDs: tooMany})
	require.NoError(t, err)
	_, err = service.BindVerificationOperation(service.VerificationOperation{Scope: service.VerificationScopeAdminUserBatchDelete, Context: context})
	assert.ErrorIs(t, err, service.ErrVerificationContextInvalid)
}

// Run against an isolated database first initialized by the released binary.
// Only the two new activity tables are migrated; existing users, logs and their
// constraints must survive both the initial upgrade and the next startup.
func TestUserActivityReleasedSchemaUpgrade(t *testing.T) {
	dialect := os.Getenv("TEST_USER_ACTIVITY_RELEASE_DIALECT")
	if dialect == "" {
		t.Skip("TEST_USER_ACTIVITY_RELEASE_DIALECT is not configured")
	}
	dsn := os.Getenv("TEST_USER_ACTIVITY_RELEASE_DSN")
	logDSN := os.Getenv("TEST_USER_ACTIVITY_RELEASE_LOG_DSN")
	var driver, logDriver gorm.Dialector
	var databaseType common.DatabaseType
	switch dialect {
	case "sqlite":
		require.Contains(t, strings.ReplaceAll(dsn, `\`, "/"), "/user-activity-release/")
		driver = sqlite.Open(dsn)
		databaseType = common.DatabaseTypeSQLite
	case "mysql":
		require.Contains(t, dsn, "127.0.0.1:")
		require.Contains(t, dsn, "/activity_release_mysql")
		require.Contains(t, logDSN, "/activity_release_mysql_log")
		driver, logDriver = mysql.Open(dsn), mysql.Open(logDSN)
		databaseType = common.DatabaseTypeMySQL
	case "postgres":
		require.Contains(t, dsn, "127.0.0.1:")
		require.Contains(t, dsn, "/activity_release_pg")
		require.Contains(t, logDSN, "/activity_release_pg_log")
		driver, logDriver = postgres.Open(dsn), postgres.Open(logDSN)
		databaseType = common.DatabaseTypePostgreSQL
	default:
		t.Fatalf("unsupported release fixture dialect: %s", dialect)
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	logDB := db
	if logDriver != nil {
		logDB, err = gorm.Open(logDriver, &gorm.Config{})
		require.NoError(t, err)
	}
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, logDB
	common.SetDatabaseTypes(databaseType, databaseType)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		connection, err := db.DB()
		if err == nil {
			_ = connection.Close()
		}
		if logDB != db {
			connection, err := logDB.DB()
			if err == nil {
				_ = connection.Close()
			}
		}
	})
	require.True(t, db.Migrator().HasTable(&model.User{}), "the released application must initialize the database first")
	require.True(t, logDB.Migrator().HasTable(&model.Log{}))
	require.False(t, db.Migrator().HasTable(&model.UserRequestActivity{}))
	beforeIndexes, err := db.Migrator().GetIndexes(&model.User{})
	require.NoError(t, err)
	now := common.GetTimestamp()
	for _, user := range []map[string]any{
		{"id": 77001, "username": "activity-release-used", "password": "release-fixture-hash", "role": 1, "status": 1, "group": "default", "aff_code": "release-used", "created_at": now - 60*activityTestDay},
		{"id": 77002, "username": "activity-release-idle", "password": "release-fixture-hash", "role": 1, "status": 1, "group": "default", "aff_code": "release-idle", "created_at": now - 60*activityTestDay},
	} {
		require.NoError(t, db.Table("users").Create(user).Error)
	}
	require.NoError(t, logDB.Table("logs").Create(map[string]any{"user_id": 77001, "type": model.LogTypeError, "created_at": now - 2*activityTestDay}).Error)
	var before, after []model.User
	require.NoError(t, db.Where("id IN ?", []int{77001, 77002}).Order("id").Find(&before).Error)
	for range 2 {
		require.NoError(t, db.AutoMigrate(&model.UserRequestActivity{}, &model.UserActivityState{}))
		require.NoError(t, model.InitUserActivity())
	}
	require.NoError(t, db.Where("id IN ?", []int{77001, 77002}).Order("id").Find(&after).Error)
	assert.Equal(t, before, after)
	afterIndexes, err := db.Migrator().GetIndexes(&model.User{})
	require.NoError(t, err)
	require.Len(t, afterIndexes, len(beforeIndexes))
	for _, index := range beforeIndexes {
		assert.True(t, db.Migrator().HasIndex(&model.User{}, index.Name()))
	}
	result, err := model.GetUserActivity(model.UserActivityFilter{Keyword: "activity-release-"}, &common.PageInfo{Page: 1, PageSize: 20}, now)
	require.NoError(t, err)
	assert.EqualValues(t, 2, result.Total)
	assert.EqualValues(t, 1, result.Summary.Active)
	assert.EqualValues(t, 1, result.Summary.Unknown)
	assert.Zero(t, result.Summary.NeverRequested)
	assert.Error(t, db.Table("users").Create(map[string]any{"id": 77003, "username": "activity-release-used", "password": "test-hash", "aff_code": "release-duplicate"}).Error)
	versionQuery := "SELECT version()"
	if dialect == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	var version string
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("released schema upgrade: %s %s, separate log database: %v", dialect, version, logDB != db)
}

func TestInactiveUserCleanupRechecksTheEntireSelection(t *testing.T) {
	for _, scenario := range []string{"new request", "administrator", "new registration", "incomplete observation", "missing user"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupUserActivityTestDB(t)
			now := common.GetTimestamp()
			start := now - 90*activityTestDay
			require.NoError(t, db.Create(&model.UserActivityState{ID: 1, TrackingStartedAt: start, BackfillCompletedAt: start}).Error)
			first := createActivityTestUser(t, db, "still-inactive", now-60*activityTestDay, now-40*activityTestDay)
			changed := createActivityTestUser(t, db, "changed-user", now-40*activityTestDay, 0)
			ids := []int{first.Id, changed.Id}
			switch scenario {
			case "new request":
				require.NoError(t, model.RecordUserRequestActivity(changed.Id, now))
			case "administrator":
				require.NoError(t, db.Model(&changed).Update("role", common.RoleAdminUser).Error)
			case "new registration":
				require.NoError(t, db.Model(&changed).Update("created_at", now-activityTestDay).Error)
			case "incomplete observation":
				require.NoError(t, db.Model(&model.UserActivityState{}).Where("id = ?", 1).Update("tracking_started_at", now-activityTestDay).Error)
			case "missing user":
				ids[1] = changed.Id + 1000
			}
			deleted, err := model.DeleteInactiveUsers(ids, authz.ClearUserAuthorizationInTx)
			assert.ErrorIs(t, err, model.ErrInactiveUserSelection)
			assert.Empty(t, deleted)
			var preserved model.User
			require.NoError(t, db.First(&preserved, first.Id).Error, "no partial deletion")
			assert.EqualValues(t, 1, preserved.AuthVersion)
		})
	}
}

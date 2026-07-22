package model

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	DB = db
	LOG_DB = db

	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false
	common.LogConsumeEnabled = true
	initCol()

	sqlDB, err := db.DB()
	if err != nil {
		panic("failed to get sql.DB: " + err.Error())
	}
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(
		&Task{},
		&User{},
		&Token{},
		&PasskeyCredential{},
		&TwoFA{},
		&TwoFABackupCode{},
		&Log{},
		&QuotaData{},
		&Channel{},
		&Ability{},
		&TopUp{},
		&SubscriptionPlan{},
		&SubscriptionOrder{},
		&UserSubscription{},
		&PerfMetric{},
		&BlackroomBan{},
		&Redemption{},
		&InvitationCode{},
		&Model{},
		&QuotaData{},
		&UserOAuthBinding{},
		&SystemInstance{},
		&SystemTask{},
		&SystemTaskLock{},
	); err != nil {
		panic("failed to migrate: " + err.Error())
	}

	os.Exit(m.Run())
}

func truncateTables(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		DB.Exec("DELETE FROM tasks")
		DB.Exec("DELETE FROM passkey_credentials")
		DB.Exec("DELETE FROM two_fa_backup_codes")
		DB.Exec("DELETE FROM two_fas")
		DB.Exec("DELETE FROM tokens")
		DB.Exec("DELETE FROM user_oauth_bindings")
		DB.Exec("DELETE FROM users")
		DB.Exec("DELETE FROM logs")
		DB.Exec("DELETE FROM quota_data")
		DB.Exec("DELETE FROM channels")
		DB.Exec("DELETE FROM abilities")
		DB.Exec("DELETE FROM top_ups")
		DB.Exec("DELETE FROM subscription_orders")
		DB.Exec("DELETE FROM subscription_plans")
		DB.Exec("DELETE FROM user_subscriptions")
		DB.Exec("DELETE FROM perf_metrics")
		DB.Exec("DELETE FROM blackroom_bans")
		DB.Exec("DELETE FROM redemptions")
		DB.Exec("DELETE FROM invitation_codes")
		DB.Exec("DELETE FROM models")
		DB.Exec("DELETE FROM quota_data")
		DB.Exec("DELETE FROM user_oauth_bindings")
		DB.Exec("DELETE FROM system_instances")
		DB.Exec("DELETE FROM system_task_locks")
		DB.Exec("DELETE FROM system_tasks")
	})
}

func insertTask(t *testing.T, task *Task) {
	t.Helper()
	task.CreatedAt = time.Now().Unix()
	task.UpdatedAt = time.Now().Unix()
	require.NoError(t, DB.Create(task).Error)
}

// ---------------------------------------------------------------------------
// Snapshot / Equal — pure logic tests (no DB)
// ---------------------------------------------------------------------------

func TestSnapshotEqual_Same(t *testing.T) {
	s := taskSnapshot{
		Status:     TaskStatusInProgress,
		Progress:   "50%",
		StartTime:  1000,
		FinishTime: 0,
		FailReason: "",
		ResultURL:  "",
		Data:       json.RawMessage(`{"key":"value"}`),
	}
	assert.True(t, s.Equal(s))
}

func TestSnapshotEqual_DifferentStatus(t *testing.T) {
	a := taskSnapshot{Status: TaskStatusInProgress, Data: json.RawMessage(`{}`)}
	b := taskSnapshot{Status: TaskStatusSuccess, Data: json.RawMessage(`{}`)}
	assert.False(t, a.Equal(b))
}

func TestSnapshotEqual_DifferentProgress(t *testing.T) {
	a := taskSnapshot{Status: TaskStatusInProgress, Progress: "30%", Data: json.RawMessage(`{}`)}
	b := taskSnapshot{Status: TaskStatusInProgress, Progress: "60%", Data: json.RawMessage(`{}`)}
	assert.False(t, a.Equal(b))
}

func TestSnapshotEqual_DifferentData(t *testing.T) {
	a := taskSnapshot{Status: TaskStatusInProgress, Data: json.RawMessage(`{"a":1}`)}
	b := taskSnapshot{Status: TaskStatusInProgress, Data: json.RawMessage(`{"a":2}`)}
	assert.False(t, a.Equal(b))
}

func TestSnapshotEqual_NilVsEmpty(t *testing.T) {
	a := taskSnapshot{Status: TaskStatusInProgress, Data: nil}
	b := taskSnapshot{Status: TaskStatusInProgress, Data: json.RawMessage{}}
	// bytes.Equal(nil, []byte{}) == true
	assert.True(t, a.Equal(b))
}

func TestSnapshot_Roundtrip(t *testing.T) {
	task := &Task{
		Status:     TaskStatusInProgress,
		Progress:   "42%",
		StartTime:  1234,
		FinishTime: 5678,
		FailReason: "timeout",
		PrivateData: TaskPrivateData{
			ResultURL: "https://example.com/result.mp4",
		},
		Data: json.RawMessage(`{"model":"test-model"}`),
	}
	snap := task.Snapshot()
	assert.Equal(t, task.Status, snap.Status)
	assert.Equal(t, task.Progress, snap.Progress)
	assert.Equal(t, task.StartTime, snap.StartTime)
	assert.Equal(t, task.FinishTime, snap.FinishTime)
	assert.Equal(t, task.FailReason, snap.FailReason)
	assert.Equal(t, task.PrivateData.ResultURL, snap.ResultURL)
	assert.JSONEq(t, string(task.Data), string(snap.Data))
}

// ---------------------------------------------------------------------------
// UpdateWithStatus CAS — DB integration tests
// ---------------------------------------------------------------------------

func TestUpdateWithStatus_Win(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID:   "task_cas_win",
		Status:   TaskStatusInProgress,
		Progress: "50%",
		Data:     json.RawMessage(`{}`),
	}
	insertTask(t, task)

	task.Status = TaskStatusSuccess
	task.Progress = "100%"
	won, err := task.UpdateWithStatus(TaskStatusInProgress)
	require.NoError(t, err)
	assert.True(t, won)

	var reloaded Task
	require.NoError(t, DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, TaskStatusSuccess, reloaded.Status)
	assert.Equal(t, "100%", reloaded.Progress)
}

func TestUpdateWithStatus_Lose(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID: "task_cas_lose",
		Status: TaskStatusFailure,
		Data:   json.RawMessage(`{}`),
	}
	insertTask(t, task)

	task.Status = TaskStatusSuccess
	won, err := task.UpdateWithStatus(TaskStatusInProgress) // wrong fromStatus
	require.NoError(t, err)
	assert.False(t, won)

	var reloaded Task
	require.NoError(t, DB.First(&reloaded, task.ID).Error)
	assert.EqualValues(t, TaskStatusFailure, reloaded.Status) // unchanged
}

func TestUpdateWithStatus_ConcurrentWinner(t *testing.T) {
	truncateTables(t)

	task := &Task{
		TaskID: "task_cas_race",
		Status: TaskStatusInProgress,
		Quota:  1000,
		Data:   json.RawMessage(`{}`),
	}
	insertTask(t, task)

	const goroutines = 5
	wins := make([]bool, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			t := &Task{}
			*t = Task{
				ID:       task.ID,
				TaskID:   task.TaskID,
				Status:   TaskStatusSuccess,
				Progress: "100%",
				Quota:    task.Quota,
				Data:     json.RawMessage(`{}`),
			}
			t.CreatedAt = task.CreatedAt
			t.UpdatedAt = time.Now().Unix()
			won, err := t.UpdateWithStatus(TaskStatusInProgress)
			if err == nil {
				wins[idx] = won
			}
		}(i)
	}
	wg.Wait()

	winCount := 0
	for _, w := range wins {
		if w {
			winCount++
		}
	}
	assert.Equal(t, 1, winCount, "exactly one goroutine should win the CAS")
}

func TestRefundTaskQuotaAtomically_ConcurrentOnlyOnce(t *testing.T) {
	truncateTables(t)

	user := &User{Username: "refund_concurrent_user", Password: "password", Quota: 500}
	require.NoError(t, DB.Create(user).Error)
	token := &Token{
		UserId:      user.Id,
		Key:         "refund_concurrent_token",
		RemainQuota: 200,
		UsedQuota:   1000,
	}
	require.NoError(t, DB.Create(token).Error)
	task := &Task{
		TaskID: "task_refund_concurrent",
		UserId: user.Id,
		Status: TaskStatusFailure,
		Quota:  1000,
		Data:   json.RawMessage(`{}`),
	}
	insertTask(t, task)

	const goroutines = 5
	claimed := make([]bool, goroutines)
	errs := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			claimed[idx], errs[idx] = RefundTaskQuotaAtomically(
				task.ID,
				task.Quota,
				user.Id,
				0,
				token.Id,
			)
		}(i)
	}
	wg.Wait()

	claimCount := 0
	for i := range claimed {
		require.NoError(t, errs[i])
		if claimed[i] {
			claimCount++
		}
	}
	assert.Equal(t, 1, claimCount)

	var reloaded Task
	require.NoError(t, DB.First(&reloaded, task.ID).Error)
	assert.Zero(t, reloaded.Quota)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 1500, user.Quota)
	require.NoError(t, DB.First(token, token.Id).Error)
	assert.Equal(t, 1200, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}

func TestRefundTaskQuotaAtomically_NonFailureDoesNotRefund(t *testing.T) {
	truncateTables(t)

	user := &User{Username: "refund_success_user", Password: "password", Quota: 500}
	require.NoError(t, DB.Create(user).Error)
	token := &Token{
		UserId:      user.Id,
		Key:         "refund_success_token",
		RemainQuota: 200,
		UsedQuota:   300,
	}
	require.NoError(t, DB.Create(token).Error)
	task := &Task{
		TaskID: "task_refund_success",
		UserId: user.Id,
		Status: TaskStatusSuccess,
		Quota:  300,
		Data:   json.RawMessage(`{}`),
	}
	insertTask(t, task)

	claimed, err := RefundTaskQuotaAtomically(task.ID, task.Quota, user.Id, 0, token.Id)
	require.NoError(t, err)
	assert.False(t, claimed)

	require.NoError(t, DB.First(task, task.ID).Error)
	assert.Equal(t, 300, task.Quota)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 500, user.Quota)
	require.NoError(t, DB.First(token, token.Id).Error)
	assert.Equal(t, 200, token.RemainQuota)
	assert.Equal(t, 300, token.UsedQuota)
}

func TestRefundTaskQuotaAtomically_RefundsSubscription(t *testing.T) {
	truncateTables(t)

	user := &User{Username: "refund_subscription_user", Password: "password", Quota: 500}
	require.NoError(t, DB.Create(user).Error)
	subscription := &UserSubscription{
		UserId:      user.Id,
		AmountTotal: 2000,
		AmountUsed:  900,
		Status:      "active",
	}
	require.NoError(t, DB.Create(subscription).Error)
	token := &Token{
		UserId:      user.Id,
		Key:         "refund_subscription_token",
		RemainQuota: 100,
		UsedQuota:   400,
	}
	require.NoError(t, DB.Create(token).Error)
	task := &Task{
		TaskID: "task_refund_subscription",
		UserId: user.Id,
		Status: TaskStatusFailure,
		Quota:  400,
		Data:   json.RawMessage(`{}`),
	}
	insertTask(t, task)

	claimed, err := RefundTaskQuotaAtomically(task.ID, task.Quota, user.Id, subscription.Id, token.Id)
	require.NoError(t, err)
	require.True(t, claimed)

	require.NoError(t, DB.First(task, task.ID).Error)
	assert.Zero(t, task.Quota)
	require.NoError(t, DB.First(subscription, subscription.Id).Error)
	assert.EqualValues(t, 500, subscription.AmountUsed)
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 500, user.Quota, "订阅退款不应修改钱包额度")
	require.NoError(t, DB.First(token, token.Id).Error)
	assert.Equal(t, 500, token.RemainQuota)
	assert.Zero(t, token.UsedQuota)
}

func TestRefundTaskQuotaAtomically_FailureRollsBackAllChanges(t *testing.T) {
	truncateTables(t)

	user := &User{Username: "refund_rollback_user", Password: "password", Quota: 500}
	require.NoError(t, DB.Create(user).Error)
	token := &Token{
		UserId:      user.Id,
		Key:         "refund_rollback_token",
		RemainQuota: 200,
		UsedQuota:   300,
	}
	require.NoError(t, DB.Create(token).Error)
	task := &Task{
		TaskID: "task_refund_rollback",
		UserId: user.Id,
		Status: TaskStatusFailure,
		Quota:  300,
		Data:   json.RawMessage(`{}`),
	}
	insertTask(t, task)

	require.NoError(t, DB.Exec(`
		CREATE TRIGGER fail_task_refund_token_update
		BEFORE UPDATE ON tokens
		BEGIN
			SELECT RAISE(ABORT, 'forced token refund failure');
		END
	`).Error)
	t.Cleanup(func() {
		DB.Exec("DROP TRIGGER IF EXISTS fail_task_refund_token_update")
	})

	claimed, err := RefundTaskQuotaAtomically(task.ID, task.Quota, user.Id, 0, token.Id)
	require.Error(t, err)
	assert.False(t, claimed)

	require.NoError(t, DB.First(task, task.ID).Error)
	assert.Equal(t, 300, task.Quota, "退款失败后必须保留待对账额度")
	require.NoError(t, DB.First(user, user.Id).Error)
	assert.Equal(t, 500, user.Quota)
	require.NoError(t, DB.First(token, token.Id).Error)
	assert.Equal(t, 200, token.RemainQuota)
	assert.Equal(t, 300, token.UsedQuota)
}

func TestGetUnrefundedFailedTasks_FiltersLimitsAndNegativeQuota(t *testing.T) {
	truncateTables(t)

	tasks := []*Task{
		{TaskID: "failed_refundable_1", Status: TaskStatusFailure, Quota: 100, SubmitTime: TaskRefundLegacyCutoff, Data: json.RawMessage(`{}`)},
		{TaskID: "failed_refundable_2", Status: TaskStatusFailure, Quota: 200, SubmitTime: TaskRefundLegacyCutoff + 1, Data: json.RawMessage(`{}`)},
		{TaskID: "legacy_failed", Status: TaskStatusFailure, Quota: 400, SubmitTime: TaskRefundLegacyCutoff - 1, Data: json.RawMessage(`{}`)},
		{TaskID: "failed_without_quota", Status: TaskStatusFailure, Quota: 0, Data: json.RawMessage(`{}`)},
		{TaskID: "failed_negative_quota", Status: TaskStatusFailure, Quota: -100, SubmitTime: TaskRefundLegacyCutoff, Data: json.RawMessage(`{}`)},
		{TaskID: "successful_with_quota", Status: TaskStatusSuccess, Quota: 300, Data: json.RawMessage(`{}`)},
	}
	for _, task := range tasks {
		insertTask(t, task)
	}

	updatedBefore := time.Now().Unix() + 1
	found := GetUnrefundedFailedTasks(updatedBefore, 1)
	require.Len(t, found, 1)
	assert.Equal(t, tasks[0].ID, found[0].ID)

	found = GetUnrefundedFailedTasks(updatedBefore, 10)
	require.Len(t, found, 2)
	assert.Equal(t, []int64{tasks[0].ID, tasks[1].ID}, []int64{found[0].ID, found[1].ID})

	assert.Empty(t, GetUnrefundedFailedTasks(updatedBefore, 0))
}

func TestRefundTaskQuotaAtomically_RejectsNonPositiveQuota(t *testing.T) {
	truncateTables(t)

	claimed, err := RefundTaskQuotaAtomically(1, 0, 1, 0, 0)
	require.NoError(t, err)
	assert.False(t, claimed)
	claimed, err = RefundTaskQuotaAtomically(1, -1, 1, 0, 0)
	require.NoError(t, err)
	assert.False(t, claimed)
}

func TestHasTaskPollingWork_IncludesOnlyPositiveRefundableFailedTasks(t *testing.T) {
	truncateTables(t)
	assert.False(t, HasTaskPollingWork())

	legacy := &Task{
		TaskID:     "legacy_failed_work",
		Status:     TaskStatusFailure,
		Progress:   "100%",
		Quota:      500,
		SubmitTime: TaskRefundLegacyCutoff - 1,
		Data:       json.RawMessage(`{}`),
	}
	insertTask(t, legacy)
	assert.False(t, HasTaskPollingWork())

	negative := &Task{
		TaskID:     "negative_failed_work",
		Status:     TaskStatusFailure,
		Progress:   "100%",
		Quota:      -500,
		SubmitTime: TaskRefundLegacyCutoff,
		Data:       json.RawMessage(`{}`),
	}
	insertTask(t, negative)
	assert.False(t, HasTaskPollingWork())

	refundable := &Task{
		TaskID:     "refundable_failed_work",
		Status:     TaskStatusFailure,
		Progress:   "100%",
		Quota:      500,
		SubmitTime: TaskRefundLegacyCutoff,
		Data:       json.RawMessage(`{}`),
	}
	insertTask(t, refundable)
	assert.True(t, HasTaskPollingWork())
}

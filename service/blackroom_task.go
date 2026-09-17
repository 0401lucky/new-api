package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

const blackroomCandidateLimit = 1000

var (
	blackroomTaskOnce    sync.Once
	blackroomTaskRunning atomic.Bool
)

type BlackroomScanSummary struct {
	WindowStart int64 `json:"window_start"`
	WindowEnd   int64 `json:"window_end"`
	Scanned     int   `json:"scanned"`
	Banned      int   `json:"banned"`
	Updated     int   `json:"updated"`
	Skipped     int   `json:"skipped"`
	Expired     int64 `json:"expired"`
}

func StartBlackroomTask() {
	blackroomTaskOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), "blackroom task started")
			for {
				runBlackroomMaintenance(false)
				interval := blackroomTaskInterval()
				time.Sleep(interval)
			}
		})
	})
}

func blackroomTaskInterval() time.Duration {
	setting := operation_setting.GetBlackroomSetting()
	minutes := setting.CheckIntervalMinutes
	if minutes <= 0 {
		minutes = 10
	}
	return time.Duration(minutes) * time.Minute
}

func RunBlackroomScanOnce() (BlackroomScanSummary, error) {
	return runBlackroomMaintenance(true)
}

func runBlackroomMaintenance(manual bool) (BlackroomScanSummary, error) {
	if !blackroomTaskRunning.CompareAndSwap(false, true) {
		return BlackroomScanSummary{}, errors.New("小黑屋扫描正在运行")
	}
	defer blackroomTaskRunning.Store(false)

	ctx := context.Background()
	summary := BlackroomScanSummary{}

	expired, err := model.ExpireDueBlackroomBans()
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("blackroom expire task failed: %v", err))
		return summary, err
	}
	summary.Expired = expired

	setting := operation_setting.GetBlackroomSetting()
	if !setting.Enabled {
		return summary, nil
	}
	if !manual && !setting.AutoBanEnabled {
		return summary, nil
	}

	minIPCount := operation_setting.MinBlackroomRuleIPCount(setting)
	if minIPCount <= 0 {
		return summary, nil
	}

	now := common.GetTimestamp()
	windowStart := now - int64(setting.LookbackHours*3600)
	summary.WindowStart = windowStart
	summary.WindowEnd = now

	candidates, err := model.FindBlackroomAuditCandidates(windowStart, now, minIPCount, setting.MinRequests, blackroomCandidateLimit)
	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("blackroom scan query failed: %v", err))
		return summary, err
	}

	for _, candidate := range candidates {
		summary.Scanned++
		created, updated, skipped, err := handleBlackroomCandidate(setting, candidate.UserId, now)
		if err != nil {
			summary.Skipped++
			logger.LogWarn(ctx, fmt.Sprintf("blackroom candidate skipped: user_id=%d error=%v", candidate.UserId, err))
			continue
		}
		if skipped {
			summary.Skipped++
			continue
		}
		if created {
			summary.Banned++
		} else if updated {
			summary.Updated++
		}
	}

	if summary.Banned > 0 || summary.Updated > 0 || summary.Expired > 0 || common.DebugEnabled {
		logger.LogInfo(ctx, fmt.Sprintf(
			"blackroom scan done: scanned=%d banned=%d updated=%d skipped=%d expired=%d",
			summary.Scanned,
			summary.Banned,
			summary.Updated,
			summary.Skipped,
			summary.Expired,
		))
	}
	return summary, nil
}

func handleBlackroomCandidate(setting *operation_setting.BlackroomSetting, userId int, windowEnd int64) (created bool, updated bool, skipped bool, err error) {
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return false, false, false, err
	}

	existing, existingErr := model.GetActiveBlackroomBan(user.Id)
	if existingErr != nil && !errors.Is(existingErr, gorm.ErrRecordNotFound) {
		return false, false, false, existingErr
	}
	// 管理员手动封禁优先：自动判定既不覆盖也不延长它。
	if existing != nil && existing.Source == model.BlackroomBanSourceManual {
		return false, false, true, nil
	}

	decision, err := evaluateBlackroomUser(setting, user.Id, user.Username, windowEnd, existingErr != nil)
	if err != nil {
		return false, false, false, err
	}
	if decision == nil || decision.Skipped || decision.ShadowMatched {
		return false, false, true, nil
	}
	return decision.NewlyApplied, !decision.NewlyApplied, false, nil
}

func CreateManualBlackroomBan(userID int, durationHours int, permanent bool, reason string) (*model.BlackroomBan, error) {
	user, err := model.GetUserById(userID, false)
	if err != nil {
		return nil, err
	}
	if user.Role >= common.RoleAdminUser {
		return nil, errors.New("不能将管理员加入小黑屋")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "管理员手动加入小黑屋"
	}

	now := common.GetTimestamp()
	durationSeconds := int64(0)
	bannedUntil := int64(0)
	if !permanent {
		if durationHours <= 0 {
			return nil, errors.New("临时封禁时长必须大于 0 小时")
		}
		durationSeconds = int64(durationHours * 3600)
		bannedUntil = now + durationSeconds
	}

	evidenceBytes, err := common.Marshal(map[string]any{
		"type":           "manual",
		"duration_hours": durationHours,
		"permanent":      permanent,
	})
	if err != nil {
		return nil, err
	}

	ban, _, err := model.UpsertActiveBlackroomBan(model.BlackroomBanInput{
		UserId:             user.Id,
		Username:           user.Username,
		Source:             model.BlackroomBanSourceManual,
		Reason:             reason,
		Evidence:           string(evidenceBytes),
		WindowStart:        now,
		WindowEnd:          now,
		BanDurationSeconds: durationSeconds,
		BannedUntil:        bannedUntil,
	})
	return ban, err
}

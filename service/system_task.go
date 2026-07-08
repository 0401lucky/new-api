package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const logCleanupBatchSize = 1000

// 运行中的清理任务每批都会刷新 updated_at；超过该时长未更新视为宿主进程已死亡
const systemTaskStaleSeconds = int64(5 * 60)

func currentSystemTaskOwner() string {
	if common.NodeName != "" {
		return common.NodeName
	}
	return "local"
}

func marshalSystemTaskField(value map[string]any) string {
	payload, err := common.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func StartLogCleanupTask(targetTimestamp int64) (*model.SystemTask, error) {
	if targetTimestamp <= 0 {
		return nil, errors.New("target timestamp is required")
	}

	currentTask, err := model.GetCurrentSystemTask(model.SystemTaskTypeLogCleanup)
	if err != nil {
		return nil, err
	}
	if currentTask != nil {
		if common.GetTimestamp()-currentTask.UpdatedAt <= systemTaskStaleSeconds {
			return currentTask, nil
		}
		// 僵尸任务：宿主进程已重启/崩溃，标记失败后允许创建新任务
		_ = model.UpdateSystemTask(currentTask.TaskID, map[string]any{
			"status": model.SystemTaskStatusFailed,
			"error":  "task stalled: owner process no longer reporting progress",
		})
	}

	task, err := model.CreateSystemTask(model.SystemTaskTypeLogCleanup, map[string]any{
		"target_timestamp": targetTimestamp,
		"batch_size":       logCleanupBatchSize,
	})
	if err != nil {
		return nil, err
	}

	go runLogCleanupTask(task.TaskID, targetTimestamp)
	return task, nil
}

func updateLogCleanupState(taskID string, total int64, processed int64) {
	remaining := total - processed
	if remaining < 0 {
		remaining = 0
	}
	progress := 100.0
	if total > 0 {
		progress = math.Min(100, math.Max(0, float64(processed)*100/float64(total)))
	}
	_ = model.UpdateSystemTask(taskID, map[string]any{
		"state": marshalSystemTaskField(map[string]any{
			"total":     total,
			"processed": processed,
			"progress":  progress,
			"remaining": remaining,
		}),
	})
}

func runLogCleanupTask(taskID string, targetTimestamp int64) {
	ctx := context.Background()
	owner := currentSystemTaskOwner()
	_ = model.UpdateSystemTask(taskID, map[string]any{
		"status":     model.SystemTaskStatusRunning,
		"active_key": owner,
		"locked_by":  owner,
	})

	total, err := model.CountOldLog(ctx, targetTimestamp)
	if err != nil {
		_ = model.UpdateSystemTask(taskID, map[string]any{
			"status": model.SystemTaskStatusFailed,
			"error":  err.Error(),
		})
		return
	}

	processed := int64(0)
	updateLogCleanupState(taskID, total, processed)

	for {
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}

		deleted, deleteErr := model.DeleteOldLogBatch(ctx, targetTimestamp, logCleanupBatchSize)
		if deleteErr != nil {
			err = deleteErr
			break
		}
		processed += deleted
		updateLogCleanupState(taskID, total, processed)

		if deleted < int64(logCleanupBatchSize) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err != nil {
		_ = model.UpdateSystemTask(taskID, map[string]any{
			"status": model.SystemTaskStatusFailed,
			"error":  fmt.Sprintf("log cleanup failed: %v", err),
		})
		return
	}

	_ = model.UpdateSystemTask(taskID, map[string]any{
		"status": model.SystemTaskStatusSucceeded,
		"result": marshalSystemTaskField(map[string]any{
			"deleted_count": processed,
		}),
	})
}

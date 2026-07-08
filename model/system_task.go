package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	SystemTaskTypeLogCleanup = "log_cleanup"

	SystemTaskStatusPending   = "pending"
	SystemTaskStatusRunning   = "running"
	SystemTaskStatusSucceeded = "succeeded"
	SystemTaskStatusFailed    = "failed"
)

type SystemTask struct {
	ID          int64  `json:"id" gorm:"primaryKey"`
	TaskID      string `json:"task_id" gorm:"type:varchar(191);uniqueIndex;not null"`
	Type        string `json:"type" gorm:"type:varchar(64);index;not null"`
	Status      string `json:"status" gorm:"type:varchar(32);index;not null"`
	ActiveKey   string `json:"active_key,omitempty" gorm:"type:varchar(191);index"`
	Payload     string `json:"payload,omitempty" gorm:"type:text"`
	State       string `json:"state,omitempty" gorm:"type:text"`
	Result      string `json:"result,omitempty" gorm:"type:text"`
	Error       string `json:"error,omitempty" gorm:"type:text"`
	LockedBy    string `json:"locked_by,omitempty" gorm:"type:varchar(191)"`
	LockedUntil int64  `json:"locked_until,omitempty" gorm:"bigint"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint;index"`
}

func (SystemTask) TableName() string {
	return "system_tasks"
}

func CreateSystemTask(taskType string, payload map[string]any) (*SystemTask, error) {
	taskIDKey, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}

	now := common.GetTimestamp()
	task := &SystemTask{
		TaskID:    "system_task_" + taskIDKey,
		Type:      taskType,
		Status:    SystemTaskStatusPending,
		Payload:   string(payloadBytes),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := DB.Create(task).Error; err != nil {
		return nil, err
	}
	return task, nil
}

func UpdateSystemTask(taskID string, updates map[string]any) error {
	updates["updated_at"] = common.GetTimestamp()
	return DB.Model(&SystemTask{}).Where("task_id = ?", taskID).Updates(updates).Error
}

func GetSystemTaskByTaskID(taskID string) (*SystemTask, error) {
	task := &SystemTask{}
	err := DB.Where("task_id = ?", taskID).First(task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return task, nil
}

func GetCurrentSystemTask(taskType string) (*SystemTask, error) {
	task := &SystemTask{}
	err := DB.Where("type = ? AND status IN ?", taskType, []string{SystemTaskStatusPending, SystemTaskStatusRunning}).
		Order("id desc").
		First(task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return task, nil
}

func ListSystemTasks(limit int) ([]*SystemTask, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	tasks := make([]*SystemTask, 0, limit)
	err := DB.Order("id desc").Limit(limit).Find(&tasks).Error
	return tasks, err
}

package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/groupload"
)

func (t *Task) GroupLoadSlot() string {
	if bc := t.PrivateData.BillingContext; bc != nil && bc.GroupLoadSlot != "" {
		return bc.GroupLoadSlot
	}
	return "task:" + t.TaskID
}

// Recover every unfinished task, including tasks created before this feature.
// Keyset pagination avoids starving later tasks behind the poller's batch cap.
// Terminal transitions leave a short tombstone, preventing a stale recovery
// read from resurrecting a task that just completed on another instance.
func RestoreTaskGroupLoads() error {
	var after int64
	for {
		var tasks []Task
		err := DB.Select("id", "task_id", "group", "private_data").Where("id > ? AND status NOT IN ?", after, []TaskStatus{TaskStatusSuccess, TaskStatusFailure}).Order("id").Limit(200).Find(&tasks).Error
		if err != nil {
			return err
		}
		for i := range tasks {
			task := &tasks[i]
			if _, err := groupload.Register(task.Group, task.GroupLoadSlot(), 120*time.Second); err != nil {
				return err
			}
			after = task.ID
		}
		if len(tasks) < 200 {
			return nil
		}
	}
}

func SyncTaskGroupLoads() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if err := RestoreTaskGroupLoads(); err != nil {
			common.SysError("restore task group load: " + err.Error())
		}
	}
}

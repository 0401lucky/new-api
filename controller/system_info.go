package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func parseSystemJSONField(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var data map[string]any
	if err := common.UnmarshalJsonStr(raw, &data); err != nil {
		return nil
	}
	return data
}

func systemInstanceStatus(instance model.SystemInstance) string {
	if common.GetTimestamp()-instance.LastSeenAt > model.SystemInstanceStaleAfterSeconds {
		return "stale"
	}
	return "online"
}

func systemInstanceDTO(instance model.SystemInstance) gin.H {
	return gin.H{
		"node_name":           instance.NodeName,
		"status":              systemInstanceStatus(instance),
		"stale_after_seconds": model.SystemInstanceStaleAfterSeconds,
		"started_at":          instance.StartedAt,
		"last_seen_at":        instance.LastSeenAt,
		"info":                parseSystemJSONField(instance.Info),
	}
}

func ListSystemInstances(c *gin.Context) {
	instances, err := model.ListSystemInstances()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := make([]gin.H, 0, len(instances))
	for _, instance := range instances {
		data = append(data, systemInstanceDTO(instance))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

func DeleteStaleSystemInstances(c *gin.Context) {
	count, err := model.DeleteStaleSystemInstances()
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"deleted_count": count,
		},
	})
}

func DeleteStaleSystemInstance(c *gin.Context) {
	nodeName := strings.TrimSpace(c.Param("node_name"))
	if nodeName == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "参数错误",
		})
		return
	}

	count, err := model.DeleteStaleSystemInstance(nodeName)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"deleted_count": count,
		},
	})
}

func systemTaskDTO(task *model.SystemTask) gin.H {
	if task == nil {
		return nil
	}

	return gin.H{
		"id":           task.ID,
		"task_id":      task.TaskID,
		"type":         task.Type,
		"status":       task.Status,
		"active_key":   task.ActiveKey,
		"payload":      parseSystemJSONField(task.Payload),
		"state":        parseSystemJSONField(task.State),
		"result":       parseSystemJSONField(task.Result),
		"error":        task.Error,
		"locked_by":    task.LockedBy,
		"locked_until": task.LockedUntil,
		"created_at":   task.CreatedAt,
		"updated_at":   task.UpdatedAt,
	}
}

func StartLogCleanupTask(c *gin.Context) {
	targetTimestamp, _ := strconv.ParseInt(c.Query("target_timestamp"), 10, 64)
	task, err := service.StartLogCleanupTask(targetTimestamp)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    systemTaskDTO(task),
	})
}

func GetCurrentSystemTask(c *gin.Context) {
	taskType := strings.TrimSpace(c.Query("type"))
	if taskType == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "type is required",
		})
		return
	}

	task, err := model.GetCurrentSystemTask(taskType)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    systemTaskDTO(task),
	})
}

func GetSystemTask(c *gin.Context) {
	taskID := strings.TrimSpace(c.Param("task_id"))
	if taskID == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "task_id is required",
		})
		return
	}

	task, err := model.GetSystemTaskByTaskID(taskID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if task == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "task not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    systemTaskDTO(task),
	})
}

func ListSystemTasks(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	tasks, err := model.ListSystemTasks(limit)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	data := make([]gin.H, 0, len(tasks))
	for _, task := range tasks {
		data = append(data, systemTaskDTO(task))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    data,
	})
}

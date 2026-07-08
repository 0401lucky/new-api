package model

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const SystemInstanceStaleAfterSeconds int64 = 90

var (
	systemInstanceStartedAt = time.Now().Unix()
	systemNodeNameOnce      sync.Once
	systemNodeName          string
	systemNodeNameSource    string
)

type SystemInstance struct {
	ID         int64  `json:"id" gorm:"primaryKey"`
	NodeName   string `json:"node_name" gorm:"type:varchar(191);uniqueIndex;not null"`
	Info       string `json:"info" gorm:"type:text"`
	StartedAt  int64  `json:"started_at" gorm:"bigint;index"`
	LastSeenAt int64  `json:"last_seen_at" gorm:"bigint;index"`
	CreatedAt  int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt  int64  `json:"updated_at" gorm:"bigint"`
}

func (SystemInstance) TableName() string {
	return "system_instances"
}

func currentSystemNodeName() (string, string) {
	systemNodeNameOnce.Do(func() {
		if configured := strings.TrimSpace(common.NodeName); configured != "" {
			systemNodeName = configured
			systemNodeNameSource = "env"
			return
		}
		if hostname, err := os.Hostname(); err == nil && strings.TrimSpace(hostname) != "" {
			systemNodeName = strings.TrimSpace(hostname)
			systemNodeNameSource = "hostname"
			return
		}
		systemNodeName = "node-" + common.GetUUID()
		systemNodeNameSource = "generated"
	})
	return systemNodeName, systemNodeNameSource
}

func buildSystemInstanceInfo(nodeName string, source string) string {
	hostname, _ := os.Hostname()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	info := map[string]any{
		"schema_version": 1,
		"node": map[string]any{
			"name":                      nodeName,
			"source":                    source,
			"manually_configured":       source == "env",
			"should_configure_manually": source != "env",
		},
		"role": map[string]any{
			"is_master": common.IsMasterNode,
		},
		"runtime": map[string]any{
			"version":    runtime.Version(),
			"goos":       runtime.GOOS,
			"goarch":     runtime.GOARCH,
			"started_at": systemInstanceStartedAt,
		},
		"host": map[string]any{
			"hostname": hostname,
		},
		"resources": map[string]any{
			"memory": map[string]any{
				"used_bytes":   mem.Alloc,
				"system_bytes": mem.Sys,
			},
		},
	}
	payload, err := common.Marshal(info)
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func ReportCurrentSystemInstance() error {
	if DB == nil {
		return errors.New("database is not initialized")
	}

	nodeName, source := currentSystemNodeName()
	now := common.GetTimestamp()
	instance := SystemInstance{
		NodeName:   nodeName,
		Info:       buildSystemInstanceInfo(nodeName, source),
		StartedAt:  systemInstanceStartedAt,
		LastSeenAt: now,
		UpdatedAt:  now,
	}

	var existing SystemInstance
	err := DB.Where("node_name = ?", nodeName).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		instance.CreatedAt = now
		return DB.Create(&instance).Error
	}
	if err != nil {
		return err
	}

	return DB.Model(&SystemInstance{}).
		Where("id = ?", existing.ID).
		Updates(map[string]any{
			"info":         instance.Info,
			"started_at":   instance.StartedAt,
			"last_seen_at": instance.LastSeenAt,
			"updated_at":   instance.UpdatedAt,
		}).Error
}

func StartSystemInstanceHeartbeat() {
	go func() {
		if err := ReportCurrentSystemInstance(); err != nil {
			common.SysError("failed to report system instance: " + err.Error())
		}

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := ReportCurrentSystemInstance(); err != nil {
				common.SysError("failed to report system instance: " + err.Error())
			}
		}
	}()
}

func ListSystemInstances() ([]SystemInstance, error) {
	_ = ReportCurrentSystemInstance()

	instances := make([]SystemInstance, 0)
	err := DB.Order("last_seen_at desc").Find(&instances).Error
	return instances, err
}

func DeleteStaleSystemInstances() (int64, error) {
	cutoff := common.GetTimestamp() - SystemInstanceStaleAfterSeconds
	result := DB.Where("last_seen_at < ?", cutoff).Delete(&SystemInstance{})
	return result.RowsAffected, result.Error
}

func DeleteStaleSystemInstance(nodeName string) (int64, error) {
	cutoff := common.GetTimestamp() - SystemInstanceStaleAfterSeconds
	result := DB.Where("node_name = ? AND last_seen_at < ?", nodeName, cutoff).Delete(&SystemInstance{})
	return result.RowsAffected, result.Error
}

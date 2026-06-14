package model

import (
	"crypto/rand"
	"crypto/sha1"
	"math/bits"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	MaxGlobalSlots      = 1000
	MaxUserSlots        = 50
	ActiveWindowSeconds = 30
	SimHashThreshold    = 5
	activeTaskHashBytes = 64 * 1024
)

var simhashTokenSalt [16]byte

func init() {
	_, _ = rand.Read(simhashTokenSalt[:])
}

type TaskSlot struct {
	UserID    int
	Username  string
	UpdatedAt int64
	SimHash   uint64
}

type ActiveTaskSlotManager struct {
	mu          sync.RWMutex
	slots       []*TaskSlot
	userSlotIdx map[int][]int
	lruOrder    []int
}

var (
	activeTaskManager     *ActiveTaskSlotManager
	activeTaskManagerOnce sync.Once
)

func GetActiveTaskSlotManager() *ActiveTaskSlotManager {
	activeTaskManagerOnce.Do(func() {
		activeTaskManager = &ActiveTaskSlotManager{
			slots:       make([]*TaskSlot, 0, MaxGlobalSlots),
			userSlotIdx: make(map[int][]int),
			lruOrder:    make([]int, 0, MaxGlobalSlots),
		}
	})
	return activeTaskManager
}

func simhash64(data string) uint64 {
	tokens := strings.Fields(data)
	if len(tokens) == 0 {
		return 0
	}

	var vector [64]int
	for _, token := range tokens {
		hash := tokenHash64(token)
		for i := 0; i < 64; i++ {
			if (hash>>i)&1 == 1 {
				vector[i]++
			} else {
				vector[i]--
			}
		}
	}

	var out uint64
	for i := 0; i < 64; i++ {
		if vector[i] >= 0 {
			out |= 1 << i
		}
	}
	return out
}

func tokenHash64(token string) uint64 {
	hash := sha1.New()
	_, _ = hash.Write(simhashTokenSalt[:])
	_, _ = hash.Write([]byte(token))
	sum := hash.Sum(nil)
	return uint64(sum[0]) |
		uint64(sum[1])<<8 |
		uint64(sum[2])<<16 |
		uint64(sum[3])<<24 |
		uint64(sum[4])<<32 |
		uint64(sum[5])<<40 |
		uint64(sum[6])<<48 |
		uint64(sum[7])<<56
}

func hamming64(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

func (m *ActiveTaskSlotManager) RecordTask(userID int, username string, data string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().Unix()
	newHash := simhash64(data)

	userSlots := m.userSlotIdx[userID]
	for _, idx := range userSlots {
		slot := m.slots[idx]
		if hamming64(slot.SimHash, newHash) <= SimHashThreshold {
			slot.UpdatedAt = now
			slot.SimHash = newHash
			slot.Username = username
			m.moveToLRUEnd(idx)
			return
		}
	}

	if len(userSlots) >= MaxUserSlots {
		oldestIdx := m.findOldestUserSlot(userID)
		if oldestIdx >= 0 {
			m.reuseSlot(oldestIdx, userID, username, now, newHash)
			return
		}
	}

	if len(m.slots) >= MaxGlobalSlots && len(m.lruOrder) > 0 {
		m.reuseSlot(m.lruOrder[0], userID, username, now, newHash)
		return
	}

	newSlot := &TaskSlot{
		UserID:    userID,
		Username:  username,
		UpdatedAt: now,
		SimHash:   newHash,
	}
	newIdx := len(m.slots)
	m.slots = append(m.slots, newSlot)
	m.userSlotIdx[userID] = append(m.userSlotIdx[userID], newIdx)
	m.lruOrder = append(m.lruOrder, newIdx)
}

func (m *ActiveTaskSlotManager) reuseSlot(idx int, newUserID int, username string, now int64, newHash uint64) {
	oldSlot := m.slots[idx]
	oldUserID := oldSlot.UserID

	if oldUserID != newUserID {
		m.removeFromUserSlotIdx(oldUserID, idx)
		m.userSlotIdx[newUserID] = append(m.userSlotIdx[newUserID], idx)
	}

	oldSlot.UserID = newUserID
	oldSlot.Username = username
	oldSlot.UpdatedAt = now
	oldSlot.SimHash = newHash
	m.moveToLRUEnd(idx)
}

func (m *ActiveTaskSlotManager) removeFromUserSlotIdx(userID int, idx int) {
	slots := m.userSlotIdx[userID]
	for i, slotIdx := range slots {
		if slotIdx == idx {
			m.userSlotIdx[userID] = append(slots[:i], slots[i+1:]...)
			break
		}
	}
	if len(m.userSlotIdx[userID]) == 0 {
		delete(m.userSlotIdx, userID)
	}
}

func (m *ActiveTaskSlotManager) findOldestUserSlot(userID int) int {
	userSlots := m.userSlotIdx[userID]
	if len(userSlots) == 0 {
		return -1
	}

	oldestIdx := userSlots[0]
	oldestTime := m.slots[oldestIdx].UpdatedAt
	for _, idx := range userSlots[1:] {
		if m.slots[idx].UpdatedAt < oldestTime {
			oldestIdx = idx
			oldestTime = m.slots[idx].UpdatedAt
		}
	}
	return oldestIdx
}

func (m *ActiveTaskSlotManager) moveToLRUEnd(idx int) {
	for i, slotIdx := range m.lruOrder {
		if slotIdx == idx {
			m.lruOrder = append(m.lruOrder[:i], m.lruOrder[i+1:]...)
			break
		}
	}
	m.lruOrder = append(m.lruOrder, idx)
}

type UserActiveTaskCount struct {
	UserID      int    `json:"user_id"`
	Username    string `json:"username"`
	ActiveSlots int    `json:"active_slots"`
}

func (m *ActiveTaskSlotManager) GetActiveTaskRank(windowSeconds int64) []UserActiveTaskCount {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if windowSeconds <= 0 {
		windowSeconds = ActiveWindowSeconds
	}

	cutoff := time.Now().Unix() - windowSeconds
	userCounts := make(map[int]*UserActiveTaskCount)
	for _, slot := range m.slots {
		if slot.UpdatedAt < cutoff {
			continue
		}
		if _, ok := userCounts[slot.UserID]; !ok {
			userCounts[slot.UserID] = &UserActiveTaskCount{
				UserID:   slot.UserID,
				Username: slot.Username,
			}
		}
		userCounts[slot.UserID].ActiveSlots++
	}

	result := make([]UserActiveTaskCount, 0, len(userCounts))
	for _, count := range userCounts {
		result = append(result, *count)
	}
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].ActiveSlots > result[i].ActiveSlots {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

func (m *ActiveTaskSlotManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	activeCount := 0
	cutoff := time.Now().Unix() - ActiveWindowSeconds
	for _, slot := range m.slots {
		if slot.UpdatedAt >= cutoff {
			activeCount++
		}
	}

	return map[string]interface{}{
		"total_slots":      len(m.slots),
		"active_slots":     activeCount,
		"max_global_slots": MaxGlobalSlots,
		"max_user_slots":   MaxUserSlots,
		"active_users":     len(m.userSlotIdx),
		"window_seconds":   ActiveWindowSeconds,
	}
}

const (
	HighActiveTaskScanInterval  = 600
	HighActiveTaskThreshold     = 5
	HighActiveTaskWindowSeconds = 600
)

type HighActiveTaskRecord struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId      int    `json:"user_id" gorm:"index"`
	Username    string `json:"username" gorm:"type:varchar(64)"`
	ActiveSlots int    `json:"active_slots"`
	WindowSecs  int    `json:"window_secs"`
	CreatedAt   int64  `json:"created_at" gorm:"index"`
}

func (HighActiveTaskRecord) TableName() string {
	return "high_active_task_records"
}

func (m *ActiveTaskSlotManager) GetHighActiveUsers(windowSeconds int64, threshold int) []UserActiveTaskCount {
	rank := m.GetActiveTaskRank(windowSeconds)
	result := make([]UserActiveTaskCount, 0)
	for _, user := range rank {
		if user.ActiveSlots >= threshold {
			result = append(result, user)
		}
	}
	return result
}

func StartHighActiveTaskScanner() {
	go func() {
		ticker := time.NewTicker(HighActiveTaskScanInterval * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			scanAndSaveHighActiveUsers()
		}
	}()
}

func scanAndSaveHighActiveUsers() {
	manager := GetActiveTaskSlotManager()
	highActiveUsers := manager.GetHighActiveUsers(HighActiveTaskWindowSeconds, HighActiveTaskThreshold)
	if len(highActiveUsers) == 0 {
		return
	}

	now := time.Now().Unix()
	for _, user := range highActiveUsers {
		if IsAdmin(user.UserID) {
			continue
		}
		_ = DB.Create(&HighActiveTaskRecord{
			UserId:      user.UserID,
			Username:    user.Username,
			ActiveSlots: user.ActiveSlots,
			WindowSecs:  HighActiveTaskWindowSeconds,
			CreatedAt:   now,
		}).Error
	}
}

func GetHighActiveTaskHistory(startTime, endTime int64, userId int, limit int) ([]HighActiveTaskRecord, error) {
	var records []HighActiveTaskRecord
	query := DB.Model(&HighActiveTaskRecord{})
	if startTime > 0 {
		query = query.Where("created_at >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where("created_at <= ?", endTime)
	}
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if limit <= 0 {
		limit = 100
	}
	err := query.Order("created_at desc").Limit(limit).Find(&records).Error
	return records, err
}

type ModelTokenUsage struct {
	ModelName    string `json:"model_name"`
	TotalTokens  int64  `json:"total_tokens"`
	RequestCount int64  `json:"request_count"`
}

func GetUserTokenUsageByModel(userId int, startTime, endTime int64) ([]ModelTokenUsage, error) {
	var results []ModelTokenUsage
	err := DB.Table("quota_data").
		Select("model_name, SUM(token_used) as total_tokens, SUM(count) as request_count").
		Where("user_id = ? AND created_at >= ? AND created_at <= ?", userId, startTime, endTime).
		Group("model_name").
		Order("total_tokens desc").
		Scan(&results).Error
	return results, err
}

func RecordActiveTaskSlot(c interface{}, userID int, username string, modelName string) {
	if userID <= 0 {
		return
	}
	gc, ok := c.(*gin.Context)
	if !ok || gc.Request == nil || gc.Request.URL == nil {
		return
	}

	requestPath := gc.Request.URL.Path
	isChatRequest := strings.Contains(requestPath, "/chat/completions") ||
		strings.Contains(requestPath, "/v1/completions") ||
		strings.Contains(requestPath, "/v1/responses") ||
		strings.Contains(requestPath, "/v1/messages") ||
		(strings.Contains(requestPath, "/v1beta/models/") && strings.Contains(requestPath, "generateContent"))
	if !isChatRequest {
		return
	}

	data := ""
	if body, exists := gc.Get("key_request_body"); exists {
		if bodyBytes, ok := body.([]byte); ok && len(bodyBytes) > 0 {
			if len(bodyBytes) > activeTaskHashBytes {
				bodyBytes = bodyBytes[:activeTaskHashBytes]
			}
			data = string(bodyBytes)
		}
	}
	if data == "" {
		if storage, exists := gc.Get(common.KeyBodyStorage); exists && storage != nil {
			if bodyStorage, ok := storage.(common.BodyStorage); ok {
				if bodyBytes, err := bodyStorage.Bytes(); err == nil && len(bodyBytes) > 0 {
					if len(bodyBytes) > activeTaskHashBytes {
						bodyBytes = bodyBytes[:activeTaskHashBytes]
					}
					data = string(bodyBytes)
				}
			}
		}
	}
	if data == "" {
		data = modelName
	}
	GetActiveTaskSlotManager().RecordTask(userID, username, data)
}

func recordActiveTaskSlotSafe(c *gin.Context, userID int, username string, modelName string) {
	defer func() {
		if r := recover(); r != nil {
			common.SysError("failed to record active task slot")
		}
	}()
	RecordActiveTaskSlot(c, userID, username, modelName)
}

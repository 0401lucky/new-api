package model

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	modelHealthEventQueueSize = 8192
	modelHealthWorkerCount    = 4
	modelHealthUsageKey       = "model_health_usage"
)

type modelHealthWrite struct {
	db    *gorm.DB
	event ModelHealthEvent
}

var (
	modelHealthOnce    sync.Once
	modelHealthQueue   chan modelHealthWrite
	modelHealthWriteMu sync.Mutex
	modelHealthDrained = sync.NewCond(&modelHealthWriteMu)
	modelHealthPending int
)

// RecordModelHealthResult samples a classified final request, independently
// of whether its consume/error logs were enabled or successfully persisted.
func RecordModelHealthResult(c *gin.Context, modelName string, failed bool) {
	if c == nil || modelName == "" {
		return
	}
	value, _ := c.Get(modelHealthUsageKey)
	event, _ := value.(ModelHealthEvent)
	event.ModelName = modelName
	event.CreatedAt = time.Now().Unix()
	event.IsError = failed
	event.ResponseBytes = max(0, c.GetInt("response_bytes"))
	event.AssistantChars = max(0, c.GetInt("assistant_content_chars"))
	RecordModelHealthEventAsync(&event)
}

func initModelHealthWriter() {
	modelHealthQueue = make(chan modelHealthWrite, modelHealthEventQueueSize)
	for range modelHealthWorkerCount {
		gopool.Go(func() {
			for write := range modelHealthQueue {
				write.persist()
			}
		})
	}
}

func (write modelHealthWrite) persist() {
	defer func() {
		modelHealthWriteMu.Lock()
		modelHealthPending--
		if modelHealthPending == 0 {
			modelHealthDrained.Broadcast()
		}
		modelHealthWriteMu.Unlock()
	}()
	defer func() {
		if recovered := recover(); recovered != nil {
			common.SysError(fmt.Sprintf("model health writer panic: %v", recovered))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := UpsertModelHealthSlice5m(ctx, write.db, &write.event); err != nil {
		common.SysError("model health write failed: " + err.Error())
	}
}

func RecordModelHealthEventAsync(event *ModelHealthEvent) {
	if event == nil {
		return
	}
	modelHealthOnce.Do(initModelHealthWriter)
	// Capture both values before returning; a caller may reuse its event.
	write := modelHealthWrite{db: DB, event: *event}
	modelHealthWriteMu.Lock()
	modelHealthPending++
	modelHealthWriteMu.Unlock()
	select {
	case modelHealthQueue <- write:
	default:
		// Bound concurrency and apply backpressure instead of spawning writers.
		write.persist()
	}
}

// FlushModelHealthEvents drains accepted events within the shutdown deadline.
// Canceling the wait does not discard writes already accepted by the queue.
func FlushModelHealthEvents(ctx context.Context) error {
	stop := context.AfterFunc(ctx, func() {
		modelHealthWriteMu.Lock()
		modelHealthDrained.Broadcast()
		modelHealthWriteMu.Unlock()
	})
	defer stop()
	modelHealthWriteMu.Lock()
	defer modelHealthWriteMu.Unlock()
	for modelHealthPending > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		modelHealthDrained.Wait()
	}
	return nil
}

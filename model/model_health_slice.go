package model

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const modelHealthSliceSeconds = int64(300)

type ModelHealthSlice5m struct {
	SliceStartTs             int64     `json:"slice_start_ts" gorm:"primaryKey;autoIncrement:false;index:idx_model_health_request_start;index:idx_model_health_request_model,priority:1;comment:slice start unix seconds, aligned to 300s"`
	ModelName                string    `json:"model_name" gorm:"size:128;primaryKey;autoIncrement:false;default:'';index:idx_model_health_request_model,priority:2;comment:client requested model name"`
	TotalRequests            int64     `json:"total_requests" gorm:"not null;default:0;comment:events observed in this slice for this model"`
	ErrorRequests            int64     `json:"error_requests" gorm:"not null;default:0;comment:events considered failure in this slice for this model"`
	SuccessQualifiedRequests int64     `json:"success_qualified_requests" gorm:"not null;default:0;comment:successful requests meeting threshold"`
	SuccessTokens            int64     `json:"success_tokens" gorm:"not null;default:0;comment:total prompt and completion tokens from successful requests"`
	HasSuccessQualified      bool      `json:"has_success_qualified" gorm:"not null;default:false;comment:1 if any qualified success in slice"`
	MaxResponseBytes         int       `json:"max_response_bytes" gorm:"not null;default:0;comment:max response bytes observed in slice"`
	MaxCompletionTokens      int       `json:"max_completion_tokens" gorm:"not null;default:0;comment:max completion tokens observed in slice"`
	MaxAssistantChars        int       `json:"max_assistant_chars" gorm:"not null;default:0;comment:max assistant content char length observed in slice"`
	UpdatedAt                time.Time `json:"updated_at"`
}

func (ModelHealthSlice5m) TableName() string {
	// Keep the legacy attempt/consume-log counters intact for rollback. They
	// cannot be converted into final request outcomes without missing facts.
	return "model_health_request_5m"
}

type ModelHealthEvent struct {
	ModelName           string
	CreatedAt           int64
	IsError             bool
	ResponseBytes       int
	CompletionTokens    int
	SuccessTokens       int
	AssistantChars      int
	SuccessIsQualified  bool
	HasMetricsAvailable bool
}

func AlignSliceStartTs(createdAt int64) int64 {
	return createdAt - (createdAt % modelHealthSliceSeconds)
}

func IsQualifiedSuccess(responseBytes, completionTokens, assistantChars int) bool {
	return responseBytes > 1024 || completionTokens > 2 || assistantChars > 2
}

func (e *ModelHealthEvent) Normalize() error {
	if e == nil {
		return errors.New("event is nil")
	}
	if e.ModelName == "" {
		return errors.New("model_name is required")
	}
	if e.CreatedAt <= 0 {
		return errors.New("created_at must be positive")
	}
	if e.ResponseBytes < 0 || e.CompletionTokens < 0 || e.SuccessTokens < 0 || e.AssistantChars < 0 {
		return errors.New("metrics must be non-negative")
	}
	if e.IsError {
		e.SuccessTokens = 0
	}
	e.SuccessIsQualified = !e.IsError && IsQualifiedSuccess(e.ResponseBytes, e.CompletionTokens, e.AssistantChars)
	e.HasMetricsAvailable = e.ResponseBytes > 0 || e.CompletionTokens > 0 || e.AssistantChars > 0
	return nil
}

func UpsertModelHealthSlice5m(ctx context.Context, db *gorm.DB, event *ModelHealthEvent) error {
	if event == nil {
		return errors.New("event is nil")
	}
	if err := event.Normalize(); err != nil {
		return err
	}
	if db == nil {
		return errors.New("db is nil")
	}

	row := &ModelHealthSlice5m{
		SliceStartTs:             AlignSliceStartTs(event.CreatedAt),
		ModelName:                event.ModelName,
		TotalRequests:            1,
		ErrorRequests:            0,
		SuccessQualifiedRequests: 0,
		SuccessTokens:            int64(event.SuccessTokens),
		HasSuccessQualified:      event.SuccessIsQualified,
		MaxResponseBytes:         event.ResponseBytes,
		MaxCompletionTokens:      event.CompletionTokens,
		MaxAssistantChars:        event.AssistantChars,
	}

	if event.IsError {
		row.ErrorRequests = 1
	}
	if event.SuccessIsQualified {
		row.SuccessQualifiedRequests = 1
	}

	table := row.TableName()
	updates := map[string]any{
		"total_requests":             gorm.Expr(table+".total_requests + ?", row.TotalRequests),
		"error_requests":             gorm.Expr(table+".error_requests + ?", row.ErrorRequests),
		"success_qualified_requests": gorm.Expr(table+".success_qualified_requests + ?", row.SuccessQualifiedRequests),
		"success_tokens":             gorm.Expr(table+".success_tokens + ?", row.SuccessTokens),
		"has_success_qualified":      gorm.Expr(table+".has_success_qualified OR ?", row.HasSuccessQualified),
	}
	for column, value := range map[string]int{
		"max_response_bytes": row.MaxResponseBytes, "max_completion_tokens": row.MaxCompletionTokens, "max_assistant_chars": row.MaxAssistantChars,
	} {
		updates[column] = gorm.Expr(fmt.Sprintf("CASE WHEN %s.%s > ? THEN %s.%s ELSE ? END", table, column, table, column), value, value)
	}

	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "slice_start_ts"},
			{Name: "model_name"},
		},
		DoUpdates: clause.Assignments(updates),
	}).Create(row).Error
}

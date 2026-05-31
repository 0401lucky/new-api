package model

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type modelHealthSliceKey struct {
	ModelName    string `gorm:"column:model_name"`
	SliceStartTs int64  `gorm:"column:slice_start_ts"`
}

type modelHealthLogAggRow struct {
	ModelName                string `gorm:"column:model_name"`
	SliceStartTs             int64  `gorm:"column:slice_start_ts"`
	ConsumeRequests          int64  `gorm:"column:consume_requests"`
	ErrorRequests            int64  `gorm:"column:error_requests"`
	SuccessQualifiedRequests int64  `gorm:"column:success_qualified_requests"`
	SuccessTokens            int64  `gorm:"column:success_tokens"`
	MaxCompletionTokens      int    `gorm:"column:max_completion_tokens"`
}

func sliceStartExprSQLForDialect(dialectName string, column string, seconds int64) string {
	switch dialectName {
	case "mysql":
		return fmt.Sprintf("((%s DIV %d) * %d)", column, seconds, seconds)
	case "sqlite":
		return fmt.Sprintf("(CAST((%s / %d) AS INTEGER) * %d)", column, seconds, seconds)
	default:
		return fmt.Sprintf("((%s / %d) * %d)", column, seconds, seconds)
	}
}

// BackfillModelHealthSlicesFromLogs 从历史请求日志回填缺失的 5 分钟健康度分片。
// 这样老部署升级后不必等待新请求累计，也能尽快展示最近 24 小时数据。
func BackfillModelHealthSlicesFromLogs(ctx context.Context, healthDB *gorm.DB, logDB *gorm.DB, startTs int64, endTs int64) error {
	if healthDB == nil {
		return fmt.Errorf("health db is nil")
	}
	if logDB == nil {
		logDB = healthDB
	}
	if startTs <= 0 || endTs <= 0 || endTs <= startTs {
		return fmt.Errorf("invalid time range")
	}

	var existing []modelHealthSliceKey
	if err := healthDB.WithContext(ctx).
		Model(&ModelHealthSlice5m{}).
		Select("model_name, slice_start_ts").
		Where("slice_start_ts >= ? AND slice_start_ts < ?", startTs, endTs).
		Find(&existing).Error; err != nil {
		return err
	}

	existingKeys := make(map[modelHealthSliceKey]struct{}, len(existing))
	for _, row := range existing {
		existingKeys[row] = struct{}{}
	}

	sliceExpr := sliceStartExprSQLForDialect(dbDialectName(logDB), "created_at", modelHealthSliceSeconds)
	var rows []modelHealthLogAggRow
	if err := logDB.WithContext(ctx).
		Model(&Log{}).
		Select(fmt.Sprintf(`
model_name as model_name,
%s as slice_start_ts,
SUM(CASE WHEN type = ? THEN 1 ELSE 0 END) as consume_requests,
SUM(CASE WHEN type = ? THEN 1 ELSE 0 END) as error_requests,
SUM(CASE WHEN type = ? AND completion_tokens > 2 THEN 1 ELSE 0 END) as success_qualified_requests,
SUM(CASE WHEN type = ? THEN prompt_tokens + completion_tokens ELSE 0 END) as success_tokens,
MAX(CASE WHEN type = ? THEN completion_tokens ELSE 0 END) as max_completion_tokens`, sliceExpr),
			LogTypeConsume,
			LogTypeError,
			LogTypeConsume,
			LogTypeConsume,
			LogTypeConsume,
		).
		Where("created_at >= ? AND created_at < ?", startTs, endTs).
		Where("model_name <> ''").
		Where("type IN ?", []int{LogTypeConsume, LogTypeError}).
		Group("model_name, " + sliceExpr).
		Scan(&rows).Error; err != nil {
		return err
	}

	toCreate := make([]ModelHealthSlice5m, 0, len(rows))
	for _, row := range rows {
		key := modelHealthSliceKey{ModelName: row.ModelName, SliceStartTs: row.SliceStartTs}
		if _, ok := existingKeys[key]; ok {
			continue
		}
		totalRequests := row.ConsumeRequests + row.ErrorRequests
		if row.ModelName == "" || row.SliceStartTs <= 0 || totalRequests <= 0 {
			continue
		}
		toCreate = append(toCreate, ModelHealthSlice5m{
			SliceStartTs:             row.SliceStartTs,
			ModelName:                row.ModelName,
			TotalRequests:            totalRequests,
			ErrorRequests:            row.ErrorRequests,
			SuccessQualifiedRequests: row.SuccessQualifiedRequests,
			SuccessTokens:            row.SuccessTokens,
			HasSuccessQualified:      row.SuccessQualifiedRequests > 0,
			MaxCompletionTokens:      row.MaxCompletionTokens,
		})
	}
	if len(toCreate) == 0 {
		return nil
	}

	return healthDB.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "slice_start_ts"},
				{Name: "model_name"},
			},
			DoNothing: true,
		}).
		CreateInBatches(toCreate, 200).Error
}

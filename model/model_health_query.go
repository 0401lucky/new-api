package model

import (
	"fmt"

	"gorm.io/gorm"
)

type ModelHealthHourlyStat struct {
	ModelName       string  `json:"model_name"`
	HourStartTs     int64   `json:"hour_start_ts"`
	SuccessSlices   int64   `json:"success_slices"`
	TotalSlices     int64   `json:"total_slices"`
	SuccessRate     float64 `json:"success_rate"`
	TotalRequests   int64   `json:"total_requests"`
	ErrorRequests   int64   `json:"error_requests"`
	SuccessRequests int64   `json:"success_requests"`
	SuccessTokens   int64   `json:"success_tokens"`
}

func hourStartExprSQL(db *gorm.DB) string {
	return hourStartExprSQLForDialect(dbDialectName(db))
}

func hourStartExprSQLForDialect(dialectName string) string {
	switch dialectName {
	case "mysql":
		return "((slice_start_ts DIV 3600) * 3600)"
	case "sqlite":
		return "(CAST((slice_start_ts / 3600) AS INTEGER) * 3600)"
	default:
		return "((slice_start_ts / 3600) * 3600)"
	}
}

func successSliceExprSQL() string {
	return "CASE WHEN has_success_qualified THEN 1 ELSE 0 END"
}

func successRateExprSQL() string {
	return fmt.Sprintf("CASE WHEN COUNT(*) = 0 THEN 0 ELSE (1.0 * SUM(%s)) / COUNT(*) END", successSliceExprSQL())
}

func GetModelHealthHourlyStats(db *gorm.DB, modelName string, startHourTs int64, endHourTs int64) ([]ModelHealthHourlyStat, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if modelName == "" {
		return nil, fmt.Errorf("model_name is required")
	}
	if startHourTs <= 0 || endHourTs <= 0 || endHourTs <= startHourTs {
		return nil, fmt.Errorf("invalid hour range")
	}

	var rows []ModelHealthHourlyStat
	err := db.Table((&ModelHealthSlice5m{}).TableName()).
		Select(fmt.Sprintf(`
model_name as model_name,
%s as hour_start_ts,
SUM(%s) as success_slices,
COUNT(*) as total_slices,
%s as success_rate,
SUM(total_requests) as total_requests,
SUM(error_requests) as error_requests,
SUM(total_requests) - SUM(error_requests) as success_requests,
SUM(success_tokens) as success_tokens`, hourStartExprSQL(db), successSliceExprSQL(), successRateExprSQL())).
		Where("model_name = ?", modelName).
		Where("slice_start_ts >= ? AND slice_start_ts < ?", startHourTs, endHourTs).
		Group("model_name, hour_start_ts").
		Order("hour_start_ts ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func GetAllModelsHealthHourlyStats(db *gorm.DB, startHourTs int64, endHourTs int64) ([]ModelHealthHourlyStat, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if startHourTs <= 0 || endHourTs <= 0 || endHourTs <= startHourTs {
		return nil, fmt.Errorf("invalid hour range")
	}

	var rows []ModelHealthHourlyStat
	err := db.Table((&ModelHealthSlice5m{}).TableName()).
		Select(fmt.Sprintf(`
model_name as model_name,
%s as hour_start_ts,
SUM(%s) as success_slices,
COUNT(*) as total_slices,
%s as success_rate,
SUM(total_requests) as total_requests,
SUM(error_requests) as error_requests,
SUM(total_requests) - SUM(error_requests) as success_requests,
SUM(success_tokens) as success_tokens`, hourStartExprSQL(db), successSliceExprSQL(), successRateExprSQL())).
		Where("slice_start_ts >= ? AND slice_start_ts < ?", startHourTs, endHourTs).
		Group("model_name, hour_start_ts").
		Order("model_name ASC, hour_start_ts ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func dbDialectName(db *gorm.DB) string {
	if db == nil || db.Dialector == nil {
		return ""
	}
	return db.Dialector.Name()
}

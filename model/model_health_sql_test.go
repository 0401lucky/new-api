package model

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestConflictValueExprForDialect(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		column  string
		want    string
	}{
		{name: "postgres", dialect: "postgres", column: "total_requests", want: "EXCLUDED.total_requests"},
		{name: "mysql", dialect: "mysql", column: "total_requests", want: "VALUES(total_requests)"},
		{name: "sqlite", dialect: "sqlite", column: "total_requests", want: "excluded.total_requests"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := conflictValueExprForDialect(tt.dialect, tt.column); got != tt.want {
				t.Fatalf("conflictValueExprForDialect(%q, %q) = %q, want %q", tt.dialect, tt.column, got, tt.want)
			}
		})
	}
}

func TestMaxMetricExprForDialect(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		column  string
		want    string
	}{
		{name: "postgres", dialect: "postgres", column: "max_response_bytes", want: "GREATEST(max_response_bytes, EXCLUDED.max_response_bytes)"},
		{name: "mysql", dialect: "mysql", column: "max_response_bytes", want: "GREATEST(max_response_bytes, VALUES(max_response_bytes))"},
		{name: "sqlite", dialect: "sqlite", column: "max_response_bytes", want: "MAX(max_response_bytes, excluded.max_response_bytes)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maxMetricExprForDialect(tt.dialect, tt.column); got != tt.want {
				t.Fatalf("maxMetricExprForDialect(%q, %q) = %q, want %q", tt.dialect, tt.column, got, tt.want)
			}
		})
	}
}

func TestHourStartExprSQLForDialect(t *testing.T) {
	tests := []struct {
		name    string
		dialect string
		want    string
	}{
		{name: "postgres", dialect: "postgres", want: "((slice_start_ts / 3600) * 3600)"},
		{name: "mysql", dialect: "mysql", want: "((slice_start_ts DIV 3600) * 3600)"},
		{name: "sqlite", dialect: "sqlite", want: "(CAST((slice_start_ts / 3600) AS INTEGER) * 3600)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hourStartExprSQLForDialect(tt.dialect); got != tt.want {
				t.Fatalf("hourStartExprSQLForDialect(%q) = %q, want %q", tt.dialect, got, tt.want)
			}
		})
	}
}

func TestSuccessRateExprSQLUsesBooleanSafeAggregation(t *testing.T) {
	expr := successRateExprSQL()
	if !strings.Contains(expr, "CASE WHEN has_success_qualified THEN 1 ELSE 0 END") {
		t.Fatalf("successRateExprSQL should use CASE aggregation, got %q", expr)
	}
	if strings.Contains(expr, "SUM(has_success_qualified)") {
		t.Fatalf("successRateExprSQL should not sum boolean directly, got %q", expr)
	}
}

func TestModelHealthSuccessTokensAggregated(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	if err := db.AutoMigrate(&ModelHealthSlice5m{}); err != nil {
		t.Fatalf("failed to migrate model health table: %v", err)
	}

	successEvent := &ModelHealthEvent{
		ModelName:        "gpt-test",
		CreatedAt:        3601,
		IsError:          false,
		ResponseBytes:    2048,
		CompletionTokens: 8,
		SuccessTokens:    42,
	}
	if err := UpsertModelHealthSlice5m(context.Background(), db, successEvent); err != nil {
		t.Fatalf("failed to upsert success event: %v", err)
	}

	errorEvent := &ModelHealthEvent{
		ModelName:     "gpt-test",
		CreatedAt:     3610,
		IsError:       true,
		SuccessTokens: 99,
	}
	if err := UpsertModelHealthSlice5m(context.Background(), db, errorEvent); err != nil {
		t.Fatalf("failed to upsert error event: %v", err)
	}

	rows, err := GetAllModelsHealthHourlyStats(db, 3600, 7200)
	if err != nil {
		t.Fatalf("failed to query hourly stats: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one row, got %d", len(rows))
	}
	if rows[0].SuccessTokens != 42 {
		t.Fatalf("expected success tokens 42, got %d", rows[0].SuccessTokens)
	}
	if rows[0].TotalRequests != 2 || rows[0].ErrorRequests != 1 {
		t.Fatalf("unexpected request counters: total=%d error=%d", rows[0].TotalRequests, rows[0].ErrorRequests)
	}
}

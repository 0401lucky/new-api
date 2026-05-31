package common

import (
	"strings"
	"testing"
)

func TestWriteDataStringKeepsSSETerminator(t *testing.T) {
	var sb strings.Builder

	if err := writeData(checkWriter(&sb), "data: hello"); err != nil {
		t.Fatalf("writeData returned error: %v", err)
	}

	if got := sb.String(); got != "data: hello\n\n" {
		t.Fatalf("unexpected SSE payload: %q", got)
	}
}

func TestWriteDataNonStringDoesNotPanic(t *testing.T) {
	var sb strings.Builder

	payload := map[string]any{"message": "hello"}
	if err := writeData(checkWriter(&sb), payload); err != nil {
		t.Fatalf("writeData returned error: %v", err)
	}

	if got := sb.String(); got == "" {
		t.Fatal("expected serialized non-string payload")
	}
}

func TestWriteDataNilDoesNotPanic(t *testing.T) {
	var sb strings.Builder

	if err := writeData(checkWriter(&sb), nil); err != nil {
		t.Fatalf("writeData returned error: %v", err)
	}

	if got := sb.String(); got != "<nil>" {
		t.Fatalf("unexpected nil payload: %q", got)
	}
}

package doctor

import (
	"testing"
	"time"
)

func TestCheckTokenExpiryReturnsResult(t *testing.T) {
	// checkTokenExpiry should always return a result (pass or fail)
	results := checkTokenExpiry()
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 result, got %d", len(results))
	}
	if results[0].Name != "token-expiry" {
		t.Fatalf("expected name 'token-expiry', got %q", results[0].Name)
	}
	if results[0].Message == "" {
		t.Fatal("expected non-empty message")
	}
}

func TestFormatDurationLocal(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{10 * time.Second, "10s"},
		{90 * time.Second, "1m"},
		{65 * time.Minute, "1h5m"},
		{25 * time.Hour, "1d1h"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatDuration(tt.d)
			if result != tt.expected {
				t.Fatalf("formatDuration(%v) = %q, want %q", tt.d, result, tt.expected)
			}
		})
	}
}

package videojobs

import (
	"os"
	"testing"
)

func TestLoadUSDtoCNYRate(t *testing.T) {
	prev := os.Getenv("VIDEO_JOB_USD_TO_CNY_RATE")
	defer os.Setenv("VIDEO_JOB_USD_TO_CNY_RATE", prev)

	_ = os.Unsetenv("VIDEO_JOB_USD_TO_CNY_RATE")
	if got := loadUSDtoCNYRate(); got <= 0 {
		t.Fatalf("expected positive default rate, got %v", got)
	}

	if err := os.Setenv("VIDEO_JOB_USD_TO_CNY_RATE", "6.99"); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	if got := loadUSDtoCNYRate(); got != 6.99 {
		t.Fatalf("expected 6.99, got %v", got)
	}

	if err := os.Setenv("VIDEO_JOB_USD_TO_CNY_RATE", "invalid"); err != nil {
		t.Fatalf("set env failed: %v", err)
	}
	if got := loadUSDtoCNYRate(); got <= 0 {
		t.Fatalf("expected fallback rate, got %v", got)
	}
}

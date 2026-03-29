package handlers

import (
	"testing"
	"time"
)

func TestExtractAdminVideoImageSplitBackfillInstanceFromRunID(t *testing.T) {
	tests := []struct {
		name  string
		runID string
		want  string
	}{
		{name: "empty", runID: "", want: ""},
		{name: "legacy", runID: "split-backfill-123", want: ""},
		{name: "new format", runID: "split-backfill-123-node-a-1001", want: "node-a-1001"},
		{name: "invalid prefix", runID: "backfill-123-node-a-1001", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractAdminVideoImageSplitBackfillInstanceFromRunID(tc.runID)
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestBuildAdminVideoImageSplitBackfillLeaseStatus(t *testing.T) {
	now := time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)
	hb := now.Add(-2 * time.Minute)
	status := adminVideoImageSplitBackfillStatusResponse{
		Running:   true,
		RunID:     "split-backfill-123-node-a-1001",
		StartedAt: &hb,
	}

	lease := buildAdminVideoImageSplitBackfillLeaseStatus(status, now)
	if lease.TimeoutSeconds <= 0 {
		t.Fatalf("timeout seconds should be positive, got %d", lease.TimeoutSeconds)
	}
	if lease.ExpiresAt == nil {
		t.Fatalf("expected expires_at")
	}
	if lease.CanTakeover {
		t.Fatalf("lease should not be takeoverable before timeout")
	}

	expiredNow := now.Add(adminVideoImageSplitBackfillLeaseTimeout + time.Second)
	expiredLease := buildAdminVideoImageSplitBackfillLeaseStatus(status, expiredNow)
	if !expiredLease.CanTakeover {
		t.Fatalf("lease should be takeoverable after timeout")
	}
}

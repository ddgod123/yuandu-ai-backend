package handlers

import "testing"

func TestParseGIFHealthWindowHours(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{name: "default", raw: "", want: 24},
		{name: "normal", raw: "72", want: 72},
		{name: "lower bound", raw: "1", want: 1},
		{name: "upper bound", raw: "168", want: 168},
		{name: "invalid text", raw: "abc", wantErr: true},
		{name: "too small", raw: "0", wantErr: true},
		{name: "too large", raw: "999", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseGIFHealthWindowHours(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected window hours: got=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestResolveAdminGIFHealth(t *testing.T) {
	if got := resolveAdminGIFHealth(nil); got != "green" {
		t.Fatalf("expected green, got %s", got)
	}
	if got := resolveAdminGIFHealth([]AdminGIFHealthAlert{{Level: "warn"}}); got != "yellow" {
		t.Fatalf("expected yellow, got %s", got)
	}
	if got := resolveAdminGIFHealth([]AdminGIFHealthAlert{{Level: "critical"}}); got != "red" {
		t.Fatalf("expected red, got %s", got)
	}
}

func TestBuildAdminGIFHealthAlerts(t *testing.T) {
	out := AdminGIFHealthReportResponse{
		Jobs: AdminGIFHealthJobsSummary{
			JobsTotal:  20,
			DoneRate:   0.55,
			FailedRate: 0.35,
		},
		Path: AdminGIFHealthPathSummary{
			Total:             20,
			NewPathStrictRate: 0.2,
		},
		Consistency: AdminGIFHealthConsistencySummary{
			DoneWithoutMainOutput: 2,
		},
		Integrity: AdminGIFHealthIntegritySummary{
			InvalidDimension: 1,
		},
		LoopTune: AdminGIFHealthLoopTuneSummary{
			Samples:      20,
			FallbackRate: 0.8,
		},
	}

	alerts := buildAdminGIFHealthAlerts(out, defaultGIFHealthAlertThresholdSettings())
	if len(alerts) == 0 {
		t.Fatalf("expected alerts, got none")
	}
	if got := resolveAdminGIFHealth(alerts); got != "red" {
		t.Fatalf("expected red health, got %s", got)
	}
}

func TestBuildAdminGIFHealthAlerts_UsesCustomThresholds(t *testing.T) {
	out := AdminGIFHealthReportResponse{
		Jobs: AdminGIFHealthJobsSummary{
			JobsTotal: 20,
			DoneRate:  0.70,
		},
	}
	thresholds := defaultGIFHealthAlertThresholdSettings()
	thresholds.GIFHealthDoneRateWarn = 0.65
	thresholds.GIFHealthDoneRateCritical = 0.50

	alerts := buildAdminGIFHealthAlerts(out, thresholds)
	for _, item := range alerts {
		if item.Code == "done_rate_warn" || item.Code == "done_rate_low" {
			t.Fatalf("expected no done-rate alert under custom thresholds, got %+v", alerts)
		}
	}
}

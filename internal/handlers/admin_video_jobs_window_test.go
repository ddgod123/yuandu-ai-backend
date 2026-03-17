package handlers

import (
	"testing"
	"time"
)

func TestParseVideoJobsOverviewWindow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantLabel string
		wantDur   time.Duration
		wantErr   bool
	}{
		{name: "default empty", input: "", wantLabel: "24h", wantDur: 24 * time.Hour},
		{name: "24h", input: "24h", wantLabel: "24h", wantDur: 24 * time.Hour},
		{name: "7d", input: "7d", wantLabel: "7d", wantDur: 7 * 24 * time.Hour},
		{name: "30d", input: "30d", wantLabel: "30d", wantDur: 30 * 24 * time.Hour},
		{name: "alias month", input: "month", wantLabel: "30d", wantDur: 30 * 24 * time.Hour},
		{name: "invalid", input: "2h", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotLabel, gotDur, err := parseVideoJobsOverviewWindow(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotLabel != tc.wantLabel {
				t.Fatalf("label mismatch: got %q want %q", gotLabel, tc.wantLabel)
			}
			if gotDur != tc.wantDur {
				t.Fatalf("duration mismatch: got %s want %s", gotDur, tc.wantDur)
			}
		})
	}
}

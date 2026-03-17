package handlers

import "testing"

func TestParseVideoJobReviewStatusFilter(t *testing.T) {
	got := parseVideoJobReviewStatusFilter("deliver,reject,deliver,unknown, need_manual_review ")
	if len(got) != 3 {
		t.Fatalf("expected 3 statuses, got %d (%v)", len(got), got)
	}
	if got[0] != "deliver" || got[1] != "reject" || got[2] != "need_manual_review" {
		t.Fatalf("unexpected filter parse result: %v", got)
	}
}

func TestParseBoolQueryWithDefault(t *testing.T) {
	tests := []struct {
		raw      string
		fallback bool
		want     bool
	}{
		{raw: "", fallback: true, want: true},
		{raw: "", fallback: false, want: false},
		{raw: "1", fallback: false, want: true},
		{raw: "0", fallback: true, want: false},
		{raw: "true", fallback: false, want: true},
		{raw: "false", fallback: true, want: false},
		{raw: "invalid", fallback: true, want: true},
		{raw: "invalid", fallback: false, want: false},
	}
	for _, tc := range tests {
		got := parseBoolQueryWithDefault(tc.raw, tc.fallback)
		if got != tc.want {
			t.Fatalf("raw=%q fallback=%v want=%v got=%v", tc.raw, tc.fallback, tc.want, got)
		}
	}
}

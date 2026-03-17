package handlers

import "testing"

func TestNormalizeFeedbackIntegrityAlertThresholdSettings_DefaultFallback(t *testing.T) {
	got := normalizeFeedbackIntegrityAlertThresholdSettings(FeedbackIntegrityAlertThresholdSettings{})
	def := defaultFeedbackIntegrityAlertThresholdSettings()
	if got != def {
		t.Fatalf("expected defaults, got %+v", got)
	}
}

func TestNormalizeFeedbackIntegrityAlertThresholdSettings_OrderGuard(t *testing.T) {
	got := normalizeFeedbackIntegrityAlertThresholdSettings(FeedbackIntegrityAlertThresholdSettings{
		FeedbackIntegrityOutputCoverageRateWarn:           0.9,
		FeedbackIntegrityOutputCoverageRateCritical:       0.96,
		FeedbackIntegrityOutputResolvedRateWarn:           0.92,
		FeedbackIntegrityOutputResolvedRateCritical:       0.97,
		FeedbackIntegrityOutputJobConsistencyRateWarn:     0.95,
		FeedbackIntegrityOutputJobConsistencyRateCritical: 0.99,
		FeedbackIntegrityTopPickConflictUsersWarn:         3,
		FeedbackIntegrityTopPickConflictUsersCritical:     1,
	})

	if !(got.FeedbackIntegrityOutputCoverageRateCritical < got.FeedbackIntegrityOutputCoverageRateWarn) {
		t.Fatalf("expected coverage critical < warn, got %+v", got)
	}
	if !(got.FeedbackIntegrityOutputResolvedRateCritical < got.FeedbackIntegrityOutputResolvedRateWarn) {
		t.Fatalf("expected resolved critical < warn, got %+v", got)
	}
	if !(got.FeedbackIntegrityOutputJobConsistencyRateCritical < got.FeedbackIntegrityOutputJobConsistencyRateWarn) {
		t.Fatalf("expected consistency critical < warn, got %+v", got)
	}
	if !(got.FeedbackIntegrityTopPickConflictUsersCritical > got.FeedbackIntegrityTopPickConflictUsersWarn) {
		t.Fatalf("expected top_pick critical > warn, got %+v", got)
	}
}

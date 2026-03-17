package videojobs

import "strings"

const (
	GIFCandidateRejectReasonLowEmotion         = "low_emotion"
	GIFCandidateRejectReasonLowConfidence      = "low_confidence"
	GIFCandidateRejectReasonDuplicate          = "duplicate_candidate"
	GIFCandidateRejectReasonBlurLow            = "blur_low"
	GIFCandidateRejectReasonSizeBudgetExceeded = "size_budget_exceeded"
	GIFCandidateRejectReasonLoopPoor           = "loop_poor"
)

func KnownGIFCandidateRejectReasons() []string {
	return []string{
		GIFCandidateRejectReasonLowEmotion,
		GIFCandidateRejectReasonLowConfidence,
		GIFCandidateRejectReasonDuplicate,
		GIFCandidateRejectReasonBlurLow,
		GIFCandidateRejectReasonSizeBudgetExceeded,
		GIFCandidateRejectReasonLoopPoor,
	}
}

func IsKnownGIFCandidateRejectReason(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case GIFCandidateRejectReasonLowEmotion,
		GIFCandidateRejectReasonLowConfidence,
		GIFCandidateRejectReasonDuplicate,
		GIFCandidateRejectReasonBlurLow,
		GIFCandidateRejectReasonSizeBudgetExceeded,
		GIFCandidateRejectReasonLoopPoor:
		return true
	default:
		return false
	}
}

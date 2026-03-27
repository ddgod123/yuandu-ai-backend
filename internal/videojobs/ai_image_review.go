package videojobs

import (
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func UpsertVideoJobImageAIReview(db *gorm.DB, row models.VideoJobImageAIReview) error {
	if db == nil || row.JobID == 0 {
		return nil
	}
	row.TargetFormat = NormalizeRequestedFormat(row.TargetFormat)
	if row.TargetFormat == "" {
		row.TargetFormat = "png"
	}
	row.Stage = strings.TrimSpace(strings.ToLower(row.Stage))
	if row.Stage == "" {
		row.Stage = "ai3"
	}
	row.Recommendation = strings.TrimSpace(strings.ToLower(row.Recommendation))
	if row.Recommendation == "" {
		row.Recommendation = "deliver"
	}
	if len(row.SummaryJSON) == 0 {
		row.SummaryJSON = datatypes.JSON([]byte("{}"))
	}
	if len(row.Metadata) == 0 {
		row.Metadata = datatypes.JSON([]byte("{}"))
	}

	now := time.Now()
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}, {Name: "target_format"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"user_id":                       row.UserID,
			"stage":                         row.Stage,
			"recommendation":                row.Recommendation,
			"reviewed_outputs":              row.ReviewedOutputs,
			"deliver_count":                 row.DeliverCount,
			"reject_count":                  row.RejectCount,
			"manual_review_count":           row.ManualReviewCount,
			"hard_gate_reject_count":        row.HardGateRejectCount,
			"hard_gate_manual_review_count": row.HardGateManualReviewCount,
			"candidate_budget":              row.CandidateBudget,
			"effective_duration_sec":        row.EffectiveDurationSec,
			"quality_fallback":              row.QualityFallback,
			"quality_selector_version":      row.QualitySelectorVersion,
			"summary_note":                  row.SummaryNote,
			"summary_json":                  row.SummaryJSON,
			"metadata":                      row.Metadata,
			"updated_at":                    now,
		}),
	}).Create(&row).Error
	if err != nil && isMissingTableError(err, "video_job_image_ai_reviews") {
		return nil
	}
	return err
}

package videojobs

import (
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	VideoJobAI1PlanSchemaV1           = "ai1_executable_plan_v1"
	VideoJobAI1PlanStatusGenerated    = "generated"
	VideoJobAI1PlanStatusAwaitingUser = "awaiting_user_confirm"
	VideoJobAI1PlanStatusConfirmed    = "confirmed"
)

func UpsertVideoJobAI1Plan(db *gorm.DB, row models.VideoJobAI1Plan) error {
	if db == nil || row.JobID == 0 {
		return nil
	}
	row.RequestedFormat = strings.ToLower(strings.TrimSpace(row.RequestedFormat))
	if row.SchemaVersion == "" {
		row.SchemaVersion = VideoJobAI1PlanSchemaV1
	}
	if row.Status == "" {
		row.Status = VideoJobAI1PlanStatusGenerated
	}
	if len(row.PlanJSON) == 0 {
		row.PlanJSON = datatypes.JSON([]byte("{}"))
	}

	now := time.Now()
	err := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"user_id":           row.UserID,
			"requested_format":  row.RequestedFormat,
			"schema_version":    row.SchemaVersion,
			"status":            row.Status,
			"source_prompt":     row.SourcePrompt,
			"plan_json":         row.PlanJSON,
			"model_provider":    row.ModelProvider,
			"model_name":        row.ModelName,
			"prompt_version":    row.PromptVersion,
			"fallback_used":     row.FallbackUsed,
			"confirmed_by_user": row.ConfirmedByUser,
			"confirmed_at":      row.ConfirmedAt,
			"updated_at":        now,
		}),
	}).Create(&row).Error
	if err != nil && isMissingTableError(err, "video_job_ai1_plans") {
		return nil
	}
	return err
}

func ConfirmVideoJobAI1Plan(db *gorm.DB, jobID, userID uint64, at time.Time) error {
	if db == nil || jobID == 0 {
		return nil
	}
	if at.IsZero() {
		at = time.Now()
	}
	updates := map[string]interface{}{
		"status":            VideoJobAI1PlanStatusConfirmed,
		"confirmed_by_user": true,
		"confirmed_at":      at,
		"updated_at":        at,
	}
	query := db.Model(&models.VideoJobAI1Plan{}).Where("job_id = ?", jobID)
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	err := query.Updates(updates).Error
	if err != nil && isMissingTableError(err, "video_job_ai1_plans") {
		return nil
	}
	return err
}

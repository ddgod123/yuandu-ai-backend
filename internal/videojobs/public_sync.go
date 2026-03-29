package videojobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertPublicVideoImageJob mirrors legacy archive.video_jobs row into routed public.video_image_jobs(_format) table.
func UpsertPublicVideoImageJob(tx *gorm.DB, legacy models.VideoJob) error {
	if tx == nil || legacy.ID == 0 {
		return nil
	}
	row := models.VideoImageJobPublic{
		ID:              legacy.ID,
		UserID:          legacy.UserID,
		Title:           strings.TrimSpace(legacy.Title),
		SourceVideoKey:  strings.TrimSpace(legacy.SourceVideoKey),
		RequestedFormat: requestedFormatFromLegacy(legacy.OutputFormats),
		Status:          strings.TrimSpace(legacy.Status),
		Stage:           strings.TrimSpace(legacy.Stage),
		Progress:        legacy.Progress,
		Options:         normalizeJSON(legacy.Options),
		Metrics:         normalizeJSON(legacy.Metrics),
		ErrorMessage:    strings.TrimSpace(legacy.ErrorMessage),
		StartedAt:       legacy.StartedAt,
		FinishedAt:      legacy.FinishedAt,
		CreatedAt:       legacy.CreatedAt,
		UpdatedAt:       legacy.UpdatedAt,
	}
	if row.Options == nil {
		row.Options = datatypes.JSON([]byte("{}"))
	}
	if row.Metrics == nil {
		row.Metrics = datatypes.JSON([]byte("{}"))
	}
	return writePublicVideoImageWithSplitFallback(tx, publicVideoImageJobsTable, row.RequestedFormat, func(tableName string) error {
		return upsertPublicVideoImageJobRow(tx, tableName, row)
	})
}

func upsertPublicVideoImageJobRow(tx *gorm.DB, tableName string, row models.VideoImageJobPublic) error {
	if tx == nil {
		return nil
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return nil
	}
	return tx.Table(tableName).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"user_id":          row.UserID,
			"title":            row.Title,
			"source_video_key": row.SourceVideoKey,
			"requested_format": row.RequestedFormat,
			"status":           row.Status,
			"stage":            row.Stage,
			"progress":         row.Progress,
			"options":          row.Options,
			"metrics":          row.Metrics,
			"error_message":    row.ErrorMessage,
			"started_at":       row.StartedAt,
			"finished_at":      row.FinishedAt,
			"updated_at":       row.UpdatedAt,
		}),
	}).Create(&row).Error
}

// SyncPublicVideoImageJobUpdates applies partial legacy updates into routed public.video_image_jobs(_format) table.
func SyncPublicVideoImageJobUpdates(db *gorm.DB, jobID uint64, updates map[string]interface{}) error {
	if db == nil || jobID == 0 {
		return nil
	}
	mapped := mapLegacyVideoJobUpdates(updates)
	if len(mapped) == 0 {
		return nil
	}
	mapped["updated_at"] = time.Now()

	requestedFormat := resolvePublicVideoImageRequestedFormat(db, jobID)
	targetTables := resolvePublicVideoImageWriteTables(publicVideoImageJobsTable, requestedFormat)
	var rowsAffected int64
	for _, tableName := range targetTables {
		tableName = strings.TrimSpace(tableName)
		if tableName == "" {
			continue
		}
		if tableName != publicVideoImageJobsTable && !publicVideoImageTableExists(db, tableName) {
			fallback := db.Table(publicVideoImageJobsTable).Where("id = ?", jobID).Updates(mapped)
			if fallback.Error != nil {
				return fallback.Error
			}
			rowsAffected += fallback.RowsAffected
			continue
		}
		result := db.Table(tableName).Where("id = ?", jobID).Updates(mapped)
		if result.Error != nil {
			if tableName != publicVideoImageJobsTable && (isMissingTableError(result.Error, tableName) || isMissingTableError(result.Error, tableOnlyName(tableName))) {
				fallback := db.Table(publicVideoImageJobsTable).Where("id = ?", jobID).Updates(mapped)
				if fallback.Error != nil {
					return fallback.Error
				}
				rowsAffected += fallback.RowsAffected
				continue
			}
			return result.Error
		}
		rowsAffected += result.RowsAffected
	}
	if rowsAffected > 0 {
		return nil
	}

	var legacy models.VideoJob
	if err := db.Where("id = ?", jobID).First(&legacy).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	return UpsertPublicVideoImageJob(db, legacy)
}

func resolvePublicVideoImageRequestedFormat(db *gorm.DB, jobID uint64) string {
	if db == nil || jobID == 0 {
		return ""
	}
	if format := lookupPublicVideoImageRequestedFormatFromSplitTables(db, jobID); format != "" {
		return format
	}
	var legacy models.VideoJob
	if err := db.Select("output_formats").Where("id = ?", jobID).First(&legacy).Error; err == nil {
		if format := normalizePublicVideoImageSplitFormat(requestedFormatFromLegacyLoose(legacy.OutputFormats)); format != "" {
			return format
		}
	}
	var row struct {
		RequestedFormat string `gorm:"column:requested_format"`
	}
	if err := db.Table(publicVideoImageJobsTable).Select("requested_format").Where("id = ?", jobID).First(&row).Error; err == nil {
		if format := normalizePublicVideoImageSplitFormat(row.RequestedFormat); format != "" {
			return format
		}
	}
	return ""
}

func lookupPublicVideoImageRequestedFormatFromSplitTables(db *gorm.DB, jobID uint64) string {
	if db == nil || jobID == 0 {
		return ""
	}
	for _, format := range publicVideoImageSplitFormats {
		tableName := resolvePublicVideoImageJobsTable(format)
		if !publicVideoImageTableExists(db, tableName) {
			continue
		}
		var row struct {
			RequestedFormat string `gorm:"column:requested_format"`
		}
		err := db.Table(tableName).Select("requested_format").Where("id = ?", jobID).First(&row).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || isMissingTableError(err, tableName) || isMissingTableError(err, tableOnlyName(tableName)) {
				continue
			}
			continue
		}
		if format := normalizePublicVideoImageSplitFormat(row.RequestedFormat); format != "" {
			return format
		}
		return normalizePublicVideoImageSplitFormat(format)
	}
	return ""
}

func CreatePublicVideoImageEvent(tx *gorm.DB, event models.VideoJobEvent) error {
	if tx == nil || event.JobID == 0 {
		return nil
	}
	row := models.VideoImageEventPublic{
		JobID:    event.JobID,
		Level:    strings.TrimSpace(event.Level),
		Stage:    strings.TrimSpace(event.Stage),
		Message:  strings.TrimSpace(event.Message),
		Metadata: normalizeJSON(event.Metadata),
	}
	if row.Metadata == nil {
		row.Metadata = datatypes.JSON([]byte("{}"))
	}
	requestedFormat := resolvePublicVideoImageRequestedFormat(tx, event.JobID)
	return writePublicVideoImageWithSplitFallback(tx, publicVideoImageEventsTable, requestedFormat, func(tableName string) error {
		return tx.Table(tableName).Create(&row).Error
	})
}

func CreatePublicVideoImageFeedback(db *gorm.DB, entry models.VideoImageFeedbackPublic) error {
	if db == nil || entry.JobID == 0 {
		return nil
	}
	return forEachPublicVideoImageFeedbackTableByJob(db, entry.JobID, func(tableName string) error {
		return db.Table(tableName).Create(&entry).Error
	})
}

func DeletePublicVideoImageFeedbackByJobUserAction(db *gorm.DB, jobID uint64, userID uint64, action string) error {
	if db == nil || jobID == 0 || userID == 0 {
		return nil
	}
	action = strings.TrimSpace(strings.ToLower(action))
	return forEachPublicVideoImageFeedbackTableByJob(db, jobID, func(tableName string) error {
		return db.Table(tableName).
			Where("job_id = ? AND user_id = ? AND LOWER(COALESCE(action, '')) = ?", jobID, userID, action).
			Delete(&models.VideoImageFeedbackPublic{}).Error
	})
}

func UpsertPublicVideoImageOutputByArtifact(tx *gorm.DB, artifact models.VideoJobArtifact) error {
	if tx == nil || artifact.JobID == 0 {
		return nil
	}
	key := strings.TrimSpace(artifact.QiniuKey)
	if key == "" {
		return nil
	}

	var legacyJob models.VideoJob
	if err := tx.Select("id", "user_id", "output_formats").Where("id = ?", artifact.JobID).First(&legacyJob).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	format := inferOutputFormatFromArtifact(artifact, legacyJob.OutputFormats)
	role := inferOutputRoleFromArtifact(artifact)
	score, frameIndex := extractArtifactNumericSignals(artifact.Metadata)
	gifTune := extractGIFLoopTuneSignals(artifact.Metadata, format, role)
	proposalID, err := resolveArtifactOutputProposalID(tx, artifact.JobID, artifact.Metadata)
	if err != nil {
		return err
	}

	row := models.VideoImageOutputPublic{
		JobID:                       artifact.JobID,
		UserID:                      legacyJob.UserID,
		Format:                      format,
		FileRole:                    role,
		ObjectKey:                   key,
		MimeType:                    strings.TrimSpace(artifact.MimeType),
		SizeBytes:                   artifact.SizeBytes,
		Width:                       artifact.Width,
		Height:                      artifact.Height,
		DurationMs:                  artifact.DurationMs,
		FrameIndex:                  frameIndex,
		ProposalID:                  proposalID,
		Score:                       score,
		GIFLoopTuneApplied:          gifTune.Applied,
		GIFLoopTuneEffectiveApplied: gifTune.EffectiveApplied,
		GIFLoopTuneFallbackToBase:   gifTune.FallbackToBase,
		GIFLoopTuneScore:            gifTune.Score,
		GIFLoopTuneLoopClosure:      gifTune.LoopClosure,
		GIFLoopTuneMotionMean:       gifTune.MotionMean,
		GIFLoopTuneEffectiveSec:     gifTune.EffectiveSec,
		SHA256:                      "",
		IsPrimary:                   role == "main",
		Metadata:                    normalizeJSON(artifact.Metadata),
	}
	if row.Metadata == nil {
		row.Metadata = datatypes.JSON([]byte("{}"))
	}
	requestedFormat := resolvePublicVideoImageRequestedFormat(tx, artifact.JobID)
	if requestedFormat == "" {
		requestedFormat = inferRequestedFormatFromArtifactKey(key)
	}
	if requestedFormat == "" {
		requestedFormat = normalizePublicVideoImageSplitFormat(format)
	}
	if requestedFormat == "" {
		requestedFormat = normalizePublicVideoImageSplitFormat(requestedFormatFromLegacy(legacyJob.OutputFormats))
	}
	outputTables := resolvePublicVideoImageWriteTables(publicVideoImageOutputsTable, requestedFormat)
	if err := writePublicVideoImageWithSplitFallback(tx, publicVideoImageOutputsTable, requestedFormat, func(tableName string) error {
		return upsertPublicVideoImageOutputRow(tx, tableName, row)
	}); err != nil {
		return err
	}
	outputTables = resolvePublicVideoImageExistingWriteTables(tx, publicVideoImageOutputsTable, requestedFormat)

	var synced models.VideoImageOutputPublic
	for _, tableName := range outputTables {
		if err := tx.
			Table(tableName).
			Select("id", "job_id", "format", "file_role", "size_bytes", "width", "height", "duration_ms", "proposal_id", "score",
				"gif_loop_tune_loop_closure", "gif_loop_tune_motion_mean", "metadata").
			Where("object_key = ?", row.ObjectKey).
			First(&synced).Error; err == nil {
			_ = UpsertGIFEvaluationByPublicOutput(tx, synced)
			break
		}
	}

	if row.Format == "zip" || row.FileRole == "package" {
		pkg := models.VideoImagePackagePublic{
			JobID:        artifact.JobID,
			UserID:       legacyJob.UserID,
			ZipObjectKey: key,
			ZipName:      path.Base(key),
			ZipSizeBytes: artifact.SizeBytes,
			FileCount:    0,
			Manifest:     row.Metadata,
		}
		if err := writePublicVideoImageWithSplitFallback(tx, publicVideoImagePackagesTable, requestedFormat, func(tableName string) error {
			return upsertPublicVideoImagePackageRow(tx, tableName, pkg)
		}); err != nil {
			return err
		}
	}

	return nil
}

func upsertPublicVideoImageOutputRow(tx *gorm.DB, tableName string, row models.VideoImageOutputPublic) error {
	if tx == nil {
		return nil
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return nil
	}
	proposalExpr := fmt.Sprintf("COALESCE(EXCLUDED.proposal_id, %s.proposal_id)", tableName)
	return tx.Table(tableName).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "object_key"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"job_id":                          row.JobID,
			"user_id":                         row.UserID,
			"format":                          row.Format,
			"file_role":                       row.FileRole,
			"mime_type":                       row.MimeType,
			"size_bytes":                      row.SizeBytes,
			"width":                           row.Width,
			"height":                          row.Height,
			"duration_ms":                     row.DurationMs,
			"frame_index":                     row.FrameIndex,
			"proposal_id":                     gorm.Expr(proposalExpr),
			"score":                           row.Score,
			"gif_loop_tune_applied":           row.GIFLoopTuneApplied,
			"gif_loop_tune_effective_applied": row.GIFLoopTuneEffectiveApplied,
			"gif_loop_tune_fallback_to_base":  row.GIFLoopTuneFallbackToBase,
			"gif_loop_tune_score":             row.GIFLoopTuneScore,
			"gif_loop_tune_loop_closure":      row.GIFLoopTuneLoopClosure,
			"gif_loop_tune_motion_mean":       row.GIFLoopTuneMotionMean,
			"gif_loop_tune_effective_sec":     row.GIFLoopTuneEffectiveSec,
			"is_primary":                      row.IsPrimary,
			"metadata":                        row.Metadata,
		}),
	}).Create(&row).Error
}

func upsertPublicVideoImagePackageRow(tx *gorm.DB, tableName string, pkg models.VideoImagePackagePublic) error {
	if tx == nil {
		return nil
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return nil
	}
	return tx.Table(tableName).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"user_id":        pkg.UserID,
			"zip_object_key": pkg.ZipObjectKey,
			"zip_name":       pkg.ZipName,
			"zip_size_bytes": pkg.ZipSizeBytes,
			"manifest":       pkg.Manifest,
		}),
	}).Create(&pkg).Error
}

func tableOnlyName(tableName string) string {
	value := strings.TrimSpace(tableName)
	if value == "" {
		return ""
	}
	if idx := strings.LastIndex(value, "."); idx >= 0 && idx+1 < len(value) {
		return value[idx+1:]
	}
	return value
}

func mapLegacyVideoJobUpdates(updates map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, 8)
	for key, value := range updates {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "status", "stage", "progress", "options", "metrics", "error_message", "started_at", "finished_at", "title", "source_video_key":
			out[key] = value
		}
	}
	return out
}

func writePublicVideoImageWithSplitFallback(
	db *gorm.DB,
	baseTable string,
	requestedFormat string,
	fn func(tableName string) error,
) error {
	if db == nil || fn == nil {
		return nil
	}
	baseTable = strings.TrimSpace(baseTable)
	if baseTable == "" {
		return nil
	}
	targetTables := resolvePublicVideoImageWriteTables(baseTable, requestedFormat)
	if len(targetTables) == 0 {
		targetTables = []string{baseTable}
	}
	for _, tableName := range targetTables {
		tableName = strings.TrimSpace(tableName)
		if tableName == "" {
			continue
		}
		if tableName != baseTable && !publicVideoImageTableExists(db, tableName) {
			return fn(baseTable)
		}
		if err := fn(tableName); err != nil {
			if tableName != baseTable && (isMissingTableError(err, tableName) || isMissingTableError(err, tableOnlyName(tableName))) {
				return fn(baseTable)
			}
			return err
		}
	}
	return nil
}

func resolvePublicVideoImageExistingWriteTables(db *gorm.DB, baseTable string, requestedFormat string) []string {
	baseTable = strings.TrimSpace(baseTable)
	if baseTable == "" {
		return nil
	}
	target := resolvePublicVideoImageWriteTables(baseTable, requestedFormat)
	if len(target) == 0 {
		return []string{baseTable}
	}
	out := make([]string, 0, len(target))
	for _, tableName := range target {
		tableName = strings.TrimSpace(tableName)
		if tableName == "" {
			continue
		}
		if db == nil {
			out = append(out, tableName)
			continue
		}
		if tableName != baseTable && !publicVideoImageTableExists(db, tableName) {
			out = append(out, baseTable)
			return dedupeTableNames(out)
		}
		probe := map[string]interface{}{}
		probeErr := db.Table(tableName).Select("1 AS v").Limit(1).Take(&probe).Error
		if probeErr != nil && !errors.Is(probeErr, gorm.ErrRecordNotFound) {
			if tableName != baseTable && (isMissingTableError(probeErr, tableName) || isMissingTableError(probeErr, tableOnlyName(tableName))) {
				out = append(out, baseTable)
				return dedupeTableNames(out)
			}
			continue
		}
		out = append(out, tableName)
	}
	if len(out) == 0 {
		out = append(out, baseTable)
	}
	return dedupeTableNames(out)
}

func dedupeTableNames(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, tableName := range in {
		tableName = strings.TrimSpace(tableName)
		if tableName == "" {
			continue
		}
		if _, ok := seen[tableName]; ok {
			continue
		}
		seen[tableName] = struct{}{}
		out = append(out, tableName)
	}
	return out
}

func publicVideoImageTableExists(db *gorm.DB, tableName string) bool {
	if db == nil {
		return false
	}
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return false
	}
	dialect := strings.ToLower(strings.TrimSpace(db.Dialector.Name()))
	switch dialect {
	case "postgres", "postgresql":
		var exists bool
		if err := db.Raw("SELECT to_regclass(?) IS NOT NULL", tableName).Scan(&exists).Error; err == nil {
			return exists
		}
	case "sqlite":
		var count int64
		schemaName, rawTableName := splitSchemaAndTableName(tableName)
		if schemaName == "" {
			schemaName = "main"
		}
		if !isSafeSQLiteSchemaName(schemaName) {
			return false
		}
		sql := fmt.Sprintf("SELECT COUNT(1) FROM %s.sqlite_master WHERE type = 'table' AND name = ?", schemaName)
		if err := db.Raw(sql, rawTableName).Scan(&count).Error; err == nil {
			return count > 0
		}
	}
	if db.Migrator().HasTable(tableName) {
		return true
	}
	return db.Migrator().HasTable(tableOnlyName(tableName))
}

func splitSchemaAndTableName(tableName string) (schemaName string, rawTableName string) {
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return "", ""
	}
	if idx := strings.LastIndex(tableName, "."); idx >= 0 && idx+1 < len(tableName) {
		return strings.TrimSpace(tableName[:idx]), strings.TrimSpace(tableName[idx+1:])
	}
	return "", tableName
}

func isSafeSQLiteSchemaName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func requestedFormatFromLegacy(raw string) string {
	return requestedFormatFromLegacyWithDefault(raw, true)
}

func requestedFormatFromLegacyLoose(raw string) string {
	return requestedFormatFromLegacyWithDefault(raw, false)
}

func requestedFormatFromLegacyWithDefault(raw string, fallbackGIF bool) string {
	items := strings.Split(strings.TrimSpace(raw), ",")
	for _, item := range items {
		clean := strings.ToLower(strings.TrimSpace(item))
		if clean != "" {
			return clean
		}
	}
	if !fallbackGIF {
		return ""
	}
	return "gif"
}

func inferRequestedFormatFromArtifactKey(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return ""
	}
	parts := strings.Split(strings.ReplaceAll(key, "\\", "/"), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] != "f" {
			continue
		}
		if format := normalizePublicVideoImageSplitFormat(parts[i+1]); format != "" {
			return format
		}
	}
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] != "outputs" {
			continue
		}
		if format := normalizePublicVideoImageSplitFormat(parts[i+1]); format != "" {
			return format
		}
	}
	fileName := strings.ToLower(path.Base(key))
	for _, candidate := range publicVideoImageSplitFormats {
		if strings.Contains(fileName, "_"+candidate+"_") || strings.HasSuffix(fileName, "_"+candidate+".zip") {
			if format := normalizePublicVideoImageSplitFormat(candidate); format != "" {
				return format
			}
		}
	}
	return ""
}

func normalizeJSON(raw datatypes.JSON) datatypes.JSON {
	if len(raw) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return raw
}

func resolveArtifactOutputProposalID(tx *gorm.DB, jobID uint64, raw datatypes.JSON) (*uint64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil
	}
	if directID := uint64(toFloatFromAny(payload["proposal_id"])); directID > 0 {
		directID := directID
		return &directID, nil
	}
	proposalRank := int(toFloatFromAny(payload["proposal_rank"]))
	if proposalRank <= 0 || tx == nil || jobID == 0 {
		return nil, nil
	}
	type proposalRow struct {
		ID uint64 `gorm:"column:id"`
	}
	var row proposalRow
	err := tx.Model(&models.VideoJobGIFAIProposal{}).
		Select("id").
		Where("job_id = ? AND proposal_rank = ?", jobID, proposalRank).
		Order("id ASC").
		Limit(1).
		Find(&row).Error
	if err != nil {
		if isMissingTableError(err, "video_job_gif_ai_proposals") {
			return nil, nil
		}
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil
	}
	resolvedID := row.ID
	return &resolvedID, nil
}

func inferOutputFormatFromArtifact(artifact models.VideoJobArtifact, fallbackFormats string) string {
	if parsed := parseFormatFromArtifactMetadata(artifact.Metadata); parsed != "" {
		return parsed
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(strings.TrimSpace(artifact.QiniuKey)), "."))
	switch ext {
	case "gif", "webp", "jpg", "jpeg", "png", "svg", "zip", "mov", "mp4":
		if ext == "jpeg" {
			return "jpg"
		}
		return ext
	}
	fallback := requestedFormatFromLegacy(fallbackFormats)
	if fallback == "" {
		return "gif"
	}
	return fallback
}

func parseFormatFromArtifactMetadata(raw datatypes.JSON) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if format, ok := payload["format"].(string); ok {
		format = strings.ToLower(strings.TrimSpace(format))
		if format == "jpeg" {
			format = "jpg"
		}
		return format
	}
	return ""
}

func inferOutputRoleFromArtifact(artifact models.VideoJobArtifact) string {
	typ := strings.ToLower(strings.TrimSpace(artifact.Type))
	switch typ {
	case "live_cover", "poster":
		return "cover"
	case "live_video":
		return "live_video"
	case "live_package":
		return "package"
	case "frame", "clip":
		return "main"
	default:
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(artifact.QiniuKey)), ".zip") {
			return "package"
		}
		if typ == "" {
			return "main"
		}
		return typ
	}
}

func extractArtifactNumericSignals(raw datatypes.JSON) (float64, int) {
	if len(raw) == 0 {
		return 0, 0
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, 0
	}
	score := toFloatFromAny(payload["score"])
	if score == 0 {
		score = toFloatFromAny(payload["cover_score"])
	}
	idx := int(toFloatFromAny(payload["index"]))
	if idx == 0 {
		idx = int(toFloatFromAny(payload["window_index"]))
	}
	return score, idx
}

type gifLoopTuneSignals struct {
	Applied          bool
	EffectiveApplied bool
	FallbackToBase   bool
	Score            float64
	LoopClosure      float64
	MotionMean       float64
	EffectiveSec     float64
}

func extractGIFLoopTuneSignals(raw datatypes.JSON, format string, role string) gifLoopTuneSignals {
	format = strings.ToLower(strings.TrimSpace(format))
	role = strings.ToLower(strings.TrimSpace(role))
	if format != "gif" || role != "main" || len(raw) == 0 {
		return gifLoopTuneSignals{}
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return gifLoopTuneSignals{}
	}
	nested, ok := payload["gif_loop_tune"].(map[string]interface{})
	if !ok {
		return gifLoopTuneSignals{}
	}
	s := gifLoopTuneSignals{
		Applied:          toBoolFromAny(nested["applied"]),
		EffectiveApplied: toBoolFromAny(nested["effective_applied"]),
		FallbackToBase:   toBoolFromAny(nested["fallback_to_base"]),
		Score:            toFloatFromAny(nested["score"]),
		LoopClosure:      toFloatFromAny(nested["loop_closure"]),
		MotionMean:       toFloatFromAny(nested["motion_mean"]),
		EffectiveSec:     toFloatFromAny(nested["effective_sec"]),
	}
	if s.EffectiveSec <= 0 {
		s.EffectiveSec = toFloatFromAny(nested["duration_sec"])
	}
	return s
}

func toBoolFromAny(v interface{}) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		lower := strings.ToLower(strings.TrimSpace(val))
		return lower == "1" || lower == "true" || lower == "yes"
	case float64:
		return val > 0
	case int:
		return val > 0
	case int64:
		return val > 0
	default:
		return false
	}
}

func toFloatFromAny(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case json.Number:
		f, _ := val.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f
	default:
		return 0
	}
}

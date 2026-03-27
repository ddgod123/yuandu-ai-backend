package videojobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"

	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/gorm"
)

const (
	GIFRerenderErrorInvalidInput     = "invalid_input"
	GIFRerenderErrorJobNotFound      = "job_not_found"
	GIFRerenderErrorCollectionMiss   = "collection_not_found"
	GIFRerenderErrorProposalNotFound = "proposal_not_found"
	GIFRerenderErrorJobNotDone       = "job_not_done"
	GIFRerenderErrorSourceDeleted    = "source_video_deleted"
	GIFRerenderErrorSourceMissing    = "source_video_unavailable"
	GIFRerenderErrorAlreadyRendered  = "proposal_already_rendered"
	GIFRerenderErrorRenderFailed     = "render_failed"
	GIFRerenderErrorPersistFailed    = "persist_failed"
)

type GIFRerenderError struct {
	Code    string
	Message string
	Err     error
}

func (e *GIFRerenderError) Error() string {
	if e == nil {
		return ""
	}
	msg := strings.TrimSpace(e.Message)
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	if msg == "" {
		msg = "gif rerender failed"
	}
	if strings.TrimSpace(e.Code) == "" {
		return msg
	}
	return e.Code + ": " + msg
}

func (e *GIFRerenderError) Unwrap() error { return e.Err }

func newGIFRerenderError(code, message string, err error) error {
	return &GIFRerenderError{
		Code:    strings.TrimSpace(code),
		Message: strings.TrimSpace(message),
		Err:     err,
	}
}

type GIFRerenderRequest struct {
	JobID        uint64
	ProposalID   uint64
	ProposalRank int
	Force        bool
	Trigger      string
	ActorID      uint64
	ActorRole    string
}

type GIFRerenderResult struct {
	JobID             uint64  `json:"job_id"`
	CollectionID      uint64  `json:"collection_id"`
	ProposalID        uint64  `json:"proposal_id"`
	ProposalRank      int     `json:"proposal_rank"`
	CandidateID       *uint64 `json:"candidate_id,omitempty"`
	OutputID          uint64  `json:"output_id"`
	EmojiID           uint64  `json:"emoji_id"`
	ArtifactID        uint64  `json:"artifact_id"`
	OutputObjectKey   string  `json:"output_object_key"`
	DisplayOrder      int     `json:"display_order"`
	WindowStartSec    float64 `json:"window_start_sec"`
	WindowEndSec      float64 `json:"window_end_sec"`
	WindowDurationSec float64 `json:"window_duration_sec"`
	SizeBytes         int64   `json:"size_bytes"`
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	ZipInvalidated    bool    `json:"zip_invalidated"`
	CostBeforeCNY     float64 `json:"cost_before_cny"`
	CostAfterCNY      float64 `json:"cost_after_cny"`
	CostDeltaCNY      float64 `json:"cost_delta_cny"`
}

// RerenderGIFByProposal performs an on-demand GIF re-render for one AI proposal.
// It is intended for admin/ops tooling and keeps output status as need_manual_review by default.
func (p *Processor) RerenderGIFByProposal(ctx context.Context, req GIFRerenderRequest) (*GIFRerenderResult, error) {
	if p == nil || p.db == nil || p.qiniu == nil {
		return nil, newGIFRerenderError(GIFRerenderErrorInvalidInput, "processor unavailable", nil)
	}
	if req.JobID == 0 || (req.ProposalID == 0 && req.ProposalRank <= 0) {
		return nil, newGIFRerenderError(GIFRerenderErrorInvalidInput, "job_id and proposal_id/proposal_rank required", nil)
	}

	var job models.VideoJob
	if err := p.db.
		Select("id", "user_id", "title", "source_video_key", "status", "stage", "output_formats", "options", "metrics", "asset_domain", "result_collection_id").
		Where("id = ?", req.JobID).
		First(&job).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, newGIFRerenderError(GIFRerenderErrorJobNotFound, "job not found", err)
		}
		return nil, newGIFRerenderError(GIFRerenderErrorPersistFailed, "load job failed", err)
	}
	if job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
		return nil, newGIFRerenderError(GIFRerenderErrorCollectionMiss, "job result collection missing", nil)
	}
	if strings.TrimSpace(strings.ToLower(job.Status)) != models.VideoJobStatusDone {
		return nil, newGIFRerenderError(GIFRerenderErrorJobNotDone, "job is not done", nil)
	}
	if strings.TrimSpace(job.SourceVideoKey) == "" {
		return nil, newGIFRerenderError(GIFRerenderErrorSourceMissing, "source video key empty", nil)
	}
	if sourceVideoDeleted(parseJSONMap(job.Metrics)) {
		return nil, newGIFRerenderError(GIFRerenderErrorSourceDeleted, "source video already deleted", nil)
	}
	if !containsString(normalizeOutputFormats(job.OutputFormats), "gif") {
		return nil, newGIFRerenderError(GIFRerenderErrorInvalidInput, "job is not gif format", nil)
	}

	var proposal models.VideoJobGIFAIProposal
	var proposalErr error
	if req.ProposalID > 0 {
		proposalErr = p.db.
			Where("id = ? AND job_id = ?", req.ProposalID, job.ID).
			First(&proposal).Error
	} else {
		proposalErr = p.db.
			Where("job_id = ? AND proposal_rank = ?", job.ID, req.ProposalRank).
			Order("id ASC").
			First(&proposal).Error
	}
	if proposalErr != nil {
		if errors.Is(proposalErr, gorm.ErrRecordNotFound) || isMissingTableError(proposalErr, "video_job_gif_ai_proposals") {
			return nil, newGIFRerenderError(GIFRerenderErrorProposalNotFound, "proposal not found", proposalErr)
		}
		return nil, newGIFRerenderError(GIFRerenderErrorPersistFailed, "load proposal failed", proposalErr)
	}
	if proposal.EndSec <= proposal.StartSec {
		return nil, newGIFRerenderError(GIFRerenderErrorInvalidInput, "proposal window invalid", nil)
	}

	assetDomain := strings.ToLower(strings.TrimSpace(job.AssetDomain))
	if assetDomain == "" {
		assetDomain = models.VideoJobAssetDomainVideo
	}
	var collection models.Collection
	if assetDomain == models.VideoJobAssetDomainVideo {
		var videoCollection models.VideoAssetCollection
		if err := p.db.Select("id", "title", "qiniu_prefix", "cover_url", "latest_zip_key", "latest_zip_name", "latest_zip_size").
			Where("id = ?", *job.ResultCollectionID).
			First(&videoCollection).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, newGIFRerenderError(GIFRerenderErrorCollectionMiss, "collection not found", err)
			}
			return nil, newGIFRerenderError(GIFRerenderErrorPersistFailed, "load collection failed", err)
		}
		collection = models.Collection{
			ID:            videoCollection.ID,
			Title:         videoCollection.Title,
			QiniuPrefix:   videoCollection.QiniuPrefix,
			CoverURL:      videoCollection.CoverURL,
			LatestZipKey:  videoCollection.LatestZipKey,
			LatestZipName: videoCollection.LatestZipName,
			LatestZipSize: videoCollection.LatestZipSize,
		}
	} else {
		if err := p.db.Select("id", "title", "qiniu_prefix", "cover_url", "latest_zip_key", "latest_zip_name", "latest_zip_size").
			Where("id = ?", *job.ResultCollectionID).
			First(&collection).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, newGIFRerenderError(GIFRerenderErrorCollectionMiss, "collection not found", err)
			}
			return nil, newGIFRerenderError(GIFRerenderErrorPersistFailed, "load collection failed", err)
		}
	}

	if !req.Force {
		var renderedCount int64
		if err := p.db.Model(&models.VideoImageOutputPublic{}).
			Where("job_id = ? AND format = ? AND file_role = ? AND proposal_id = ?", job.ID, "gif", "main", proposal.ID).
			Count(&renderedCount).Error; err == nil && renderedCount > 0 {
			return nil, newGIFRerenderError(GIFRerenderErrorAlreadyRendered, "proposal already rendered", nil)
		}
	}

	proposalID := proposal.ID
	candidateID, _ := p.resolveCandidateIDForProposal(job.ID, proposalID, proposal.ProposalRank)
	trigger := strings.TrimSpace(req.Trigger)
	if trigger == "" {
		trigger = "admin_manual"
	}
	actorRole := strings.TrimSpace(req.ActorRole)
	if actorRole == "" {
		actorRole = "admin"
	}
	p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", "gif rerender started", map[string]interface{}{
		"trigger":       trigger,
		"actor_role":    actorRole,
		"actor_id":      req.ActorID,
		"proposal_id":   proposalID,
		"proposal_rank": proposal.ProposalRank,
		"force":         req.Force,
	})

	costBefore := p.loadJobEstimatedCost(job.ID)
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("video-job-%d-rerender-*", job.ID))
	if err != nil {
		return nil, newGIFRerenderError(GIFRerenderErrorRenderFailed, "create temp dir failed", err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source")
	if ext := strings.TrimSpace(filepath.Ext(job.SourceVideoKey)); ext != "" {
		sourcePath += ext
	} else {
		sourcePath += ".mp4"
	}
	if err := p.downloadObjectByKey(ctx, job.SourceVideoKey, sourcePath); err != nil {
		return nil, newGIFRerenderError(GIFRerenderErrorSourceMissing, "download source video failed", err)
	}

	meta, err := probeVideo(ctx, sourcePath)
	if err != nil {
		return nil, newGIFRerenderError(GIFRerenderErrorRenderFailed, "probe source video failed", err)
	}
	startSec, endSec := clampHighlightWindow(proposal.StartSec, proposal.EndSec, meta.DurationSec)
	if endSec-startSec < 0.5 {
		return nil, newGIFRerenderError(GIFRerenderErrorInvalidInput, "proposal window too short after clamp", nil)
	}

	qualitySettings := p.loadQualitySettings()
	options := parseJobOptions(job.Options)
	options = applyAnimatedProfileDefaults(options, []string{"gif"}, qualitySettings)

	prefix := strings.Trim(strings.TrimSpace(collection.QiniuPrefix), "/")
	if prefix == "" {
		if resolved, rErr := p.resolveCollectionPrefix(job); rErr == nil {
			prefix = resolved
		} else {
			return nil, newGIFRerenderError(GIFRerenderErrorPersistFailed, "resolve collection prefix failed", rErr)
		}
	}
	windowIndex, err := p.resolveUniqueGIFWindowIndex(job.ID, prefix)
	if err != nil {
		return nil, newGIFRerenderError(GIFRerenderErrorPersistFailed, "resolve rerender window index failed", err)
	}

	window := highlightCandidate{
		StartSec:     startSec,
		EndSec:       endSec,
		Score:        proposal.BaseScore,
		Reason:       strings.TrimSpace(proposal.ProposalReason),
		ProposalRank: proposal.ProposalRank,
		ProposalID:   &proposalID,
		CandidateID:  candidateID,
	}
	if strings.TrimSpace(window.Reason) == "" {
		window.Reason = "ai_proposal_rerender"
	}

	outputDir := filepath.Join(tmpDir, "animated")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, newGIFRerenderError(GIFRerenderErrorRenderFailed, "create animated output dir failed", err)
	}
	uploader := qiniustorage.NewFormUploader(p.qiniu.Cfg)
	rendered := p.processAnimatedTask(
		ctx,
		sourcePath,
		outputDir,
		prefix,
		meta,
		options,
		qualitySettings,
		uploader,
		animatedTask{
			WindowIndex: windowIndex,
			Window:      window,
			Format:      "gif",
			Order:       windowIndex,
		},
	)
	if rendered.UnsupportedReason != "" {
		return nil, newGIFRerenderError(GIFRerenderErrorRenderFailed, rendered.UnsupportedReason, nil)
	}
	if rendered.Err != nil {
		deleteQiniuKeysByPrefix(p.qiniu, rendered.UploadedKeys)
		return nil, newGIFRerenderError(GIFRerenderErrorRenderFailed, "render gif failed", rendered.Err)
	}
	if strings.TrimSpace(rendered.FileKey) == "" {
		deleteQiniuKeysByPrefix(p.qiniu, rendered.UploadedKeys)
		return nil, newGIFRerenderError(GIFRerenderErrorRenderFailed, "render output key empty", nil)
	}

	now := time.Now()
	oldZipKey := strings.Trim(strings.TrimSpace(collection.LatestZipKey), "/")
	zipInvalidated := oldZipKey != ""
	result := &GIFRerenderResult{
		JobID:             job.ID,
		CollectionID:      collection.ID,
		ProposalID:        proposal.ID,
		ProposalRank:      proposal.ProposalRank,
		CandidateID:       candidateID,
		OutputObjectKey:   rendered.FileKey,
		WindowStartSec:    roundTo(startSec, 3),
		WindowEndSec:      roundTo(endSec, 3),
		WindowDurationSec: roundTo(endSec-startSec, 3),
		Width:             rendered.Width,
		Height:            rendered.Height,
		SizeBytes:         rendered.SizeBytes,
		CostBeforeCNY:     costBefore,
		ZipInvalidated:    zipInvalidated,
	}

	updatedMetrics := parseJSONMap(job.Metrics)
	metricsSnapshot := mapFromAny(updatedMetrics["gif_rerender_v1"])
	metricsSnapshot["version"] = "v1"
	metricsSnapshot["count"] = intFromAny(metricsSnapshot["count"]) + 1
	metricsSnapshot["last_at"] = now.Format(time.RFC3339)
	metricsSnapshot["last_trigger"] = trigger
	metricsSnapshot["last_actor_role"] = actorRole
	metricsSnapshot["last_actor_id"] = req.ActorID
	metricsSnapshot["last_proposal_id"] = proposal.ID
	metricsSnapshot["last_proposal_rank"] = proposal.ProposalRank
	metricsSnapshot["last_output_key"] = rendered.FileKey
	metricsSnapshot["zip_invalidated"] = zipInvalidated
	if candidateID != nil && *candidateID > 0 {
		metricsSnapshot["last_candidate_id"] = *candidateID
	}
	updatedMetrics["gif_rerender_v1"] = metricsSnapshot
	metricsJSON := mustJSON(updatedMetrics)

	txErr := p.db.Transaction(func(tx *gorm.DB) error {
		displayOrder, err := nextEmojiDisplayOrder(tx, collection.ID, assetDomain)
		if err != nil {
			return err
		}

		if assetDomain == models.VideoJobAssetDomainVideo {
			emoji := models.VideoAssetEmoji{
				CollectionID: collection.ID,
				Title:        buildAnimatedEmojiTitle(collection.Title, windowIndex, "gif"),
				FileURL:      rendered.FileKey,
				ThumbURL:     rendered.ThumbKey,
				Format:       "gif",
				Width:        rendered.Width,
				Height:       rendered.Height,
				SizeBytes:    rendered.SizeBytes,
				DisplayOrder: displayOrder,
				Status:       "active",
			}
			if err := upsertVideoAssetEmojiByCollectionFile(tx, &emoji); err != nil {
				return err
			}
			result.EmojiID = emoji.ID
		} else {
			emoji := models.Emoji{
				CollectionID: collection.ID,
				Title:        buildAnimatedEmojiTitle(collection.Title, windowIndex, "gif"),
				FileURL:      rendered.FileKey,
				ThumbURL:     rendered.ThumbKey,
				Format:       "gif",
				Width:        rendered.Width,
				Height:       rendered.Height,
				SizeBytes:    rendered.SizeBytes,
				DisplayOrder: displayOrder,
				Status:       "active",
			}
			if err := upsertEmojiByCollectionFile(tx, &emoji); err != nil {
				return err
			}
			result.EmojiID = emoji.ID
		}
		result.DisplayOrder = displayOrder

		artifactID := uint64(0)
		for _, payload := range rendered.Artifacts {
			metadata := mapFromAny(payload.Metadata)
			metadata["rerender"] = true
			metadata["rerender_at"] = now.Format(time.RFC3339)
			metadata["rerender_trigger"] = trigger
			metadata["rerender_actor_role"] = actorRole
			metadata["rerender_actor_id"] = req.ActorID
			metadata["proposal_id"] = proposal.ID
			metadata["proposal_rank"] = proposal.ProposalRank
			if candidateID != nil && *candidateID > 0 {
				metadata["candidate_id"] = *candidateID
			}
			artifact := models.VideoJobArtifact{
				JobID:      job.ID,
				Type:       payload.Type,
				QiniuKey:   payload.Key,
				MimeType:   payload.MimeType,
				SizeBytes:  payload.SizeBytes,
				Width:      payload.Width,
				Height:     payload.Height,
				DurationMs: payload.DurationMs,
				Metadata:   mustJSON(metadata),
			}
			if err := upsertVideoJobArtifactByJobKey(tx, &artifact); err != nil {
				return err
			}
			if payload.Type == "clip" {
				artifactID = artifact.ID
			}
		}
		result.ArtifactID = artifactID

		var output models.VideoImageOutputPublic
		if err := tx.Select("id").Where("object_key = ?", rendered.FileKey).First(&output).Error; err != nil {
			return err
		}
		result.OutputID = output.ID

		review := models.VideoJobGIFAIReview{
			JobID:               job.ID,
			UserID:              job.UserID,
			OutputID:            &output.ID,
			ProposalID:          &proposal.ID,
			Provider:            "system",
			Model:               "admin_rerender_v1",
			Endpoint:            "admin/video-jobs/rerender-gif",
			PromptVersion:       "v1",
			FinalRecommendation: "need_manual_review",
			SemanticVerdict:     0,
			DiagnosticReason:    "admin rerender output pending review",
			SuggestedAction:     "manual_review_required",
			Metadata: mustJSON(map[string]interface{}{
				"rerender":      true,
				"trigger":       trigger,
				"actor_role":    actorRole,
				"actor_id":      req.ActorID,
				"proposal_id":   proposal.ID,
				"proposal_rank": proposal.ProposalRank,
			}),
			RawResponse: mustJSON(map[string]interface{}{
				"source":  "admin_rerender",
				"trigger": trigger,
			}),
		}
		if err := tx.Create(&review).Error; err != nil && !isMissingTableError(err, "video_job_gif_ai_reviews") {
			return err
		}

		var activeCount int64
		if assetDomain == models.VideoJobAssetDomainVideo {
			if err := tx.Model(&models.VideoAssetEmoji{}).
				Where("collection_id = ? AND status = ?", collection.ID, "active").
				Count(&activeCount).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&models.Emoji{}).
				Where("collection_id = ? AND status = ?", collection.ID, "active").
				Count(&activeCount).Error; err != nil {
				return err
			}
		}

		collectionUpdates := map[string]interface{}{
			"file_count":      int(activeCount),
			"latest_zip_key":  "",
			"latest_zip_name": "",
			"latest_zip_size": int64(0),
			"latest_zip_at":   nil,
		}
		if strings.TrimSpace(collection.CoverURL) == "" {
			collectionUpdates["cover_url"] = rendered.ThumbKey
		}
		if assetDomain == models.VideoJobAssetDomainVideo {
			if err := tx.Model(&models.VideoAssetCollection{}).Where("id = ?", collection.ID).Updates(collectionUpdates).Error; err != nil {
				return err
			}
			if err := tx.Where("collection_id = ?", collection.ID).Delete(&models.VideoAssetCollectionZip{}).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Model(&models.Collection{}).Where("id = ?", collection.ID).Updates(collectionUpdates).Error; err != nil {
				return err
			}
			if err := tx.Where("collection_id = ?", collection.ID).Delete(&models.CollectionZip{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("job_id = ? AND type = ?", job.ID, "package").Delete(&models.VideoJobArtifact{}).Error; err != nil {
			return err
		}
		for _, tableName := range PublicVideoImageOutputsMirrorTables() {
			if err := tx.Table(tableName).
				Where("job_id = ? AND (format = ? OR file_role = ?)", job.ID, "zip", "package").
				Delete(nil).Error; err != nil {
				if isMissingTableError(err, tableName) {
					continue
				}
				return err
			}
		}
		for _, tableName := range PublicVideoImagePackagesMirrorTables() {
			if err := tx.Table(tableName).Where("job_id = ?", job.ID).Delete(nil).Error; err != nil {
				if isMissingTableError(err, tableName) {
					continue
				}
				return err
			}
		}
		if err := tx.Model(&models.VideoJob{}).Where("id = ?", job.ID).Update("metrics", metricsJSON).Error; err != nil {
			return err
		}
		return nil
	})
	if txErr != nil {
		deleteQiniuKeysByPrefix(p.qiniu, rendered.UploadedKeys)
		return nil, newGIFRerenderError(GIFRerenderErrorPersistFailed, "persist rerender outputs failed", txErr)
	}

	if syncErr := SyncPublicVideoImageJobUpdates(p.db, job.ID, map[string]interface{}{"metrics": metricsJSON}); syncErr != nil {
		p.appendJobEvent(job.ID, models.VideoJobStageIndexing, "warn", "sync rerender metrics to public failed", map[string]interface{}{
			"error": syncErr.Error(),
		})
	}
	if oldZipKey != "" {
		if err := deleteQiniuKey(p.qiniu, oldZipKey); err != nil {
			p.appendJobEvent(job.ID, models.VideoJobStageUploading, "warn", "rerender stale zip delete failed", map[string]interface{}{
				"zip_key": oldZipKey,
				"error":   err.Error(),
			})
		}
	}

	p.syncJobCost(job.ID)
	p.syncGIFBaseline(job.ID)
	result.CostAfterCNY = p.loadJobEstimatedCost(job.ID)
	result.CostDeltaCNY = roundTo(result.CostAfterCNY-result.CostBeforeCNY, 6)

	p.appendJobEvent(job.ID, models.VideoJobStageUploading, "info", "gif rerender completed", map[string]interface{}{
		"proposal_id":     proposal.ID,
		"proposal_rank":   proposal.ProposalRank,
		"candidate_id":    valueOrNilUint64(candidateID),
		"output_id":       result.OutputID,
		"artifact_id":     result.ArtifactID,
		"emoji_id":        result.EmojiID,
		"output_object":   result.OutputObjectKey,
		"display_order":   result.DisplayOrder,
		"window_start":    result.WindowStartSec,
		"window_end":      result.WindowEndSec,
		"cost_before_cny": result.CostBeforeCNY,
		"cost_after_cny":  result.CostAfterCNY,
		"cost_delta_cny":  result.CostDeltaCNY,
		"zip_invalidated": result.ZipInvalidated,
		"trigger":         trigger,
		"actor_role":      actorRole,
		"actor_id":        req.ActorID,
	})

	return result, nil
}

func (p *Processor) resolveCandidateIDForProposal(jobID, proposalID uint64, proposalRank int) (*uint64, error) {
	if p == nil || p.db == nil || jobID == 0 {
		return nil, nil
	}
	type candidateRow struct {
		ID uint64 `gorm:"column:id"`
	}
	var row candidateRow

	if proposalID > 0 {
		err := p.db.Model(&models.VideoJobGIFCandidate{}).
			Select("id").
			Where("job_id = ? AND feature_json ->> 'proposal_id' = ?", jobID, strconv.FormatUint(proposalID, 10)).
			Order("id ASC").
			Limit(1).
			Find(&row).Error
		if err != nil {
			if isMissingTableError(err, "video_job_gif_candidates") {
				return nil, nil
			}
			return nil, err
		}
		if row.ID > 0 {
			id := row.ID
			return &id, nil
		}
	}
	if proposalRank <= 0 {
		return nil, nil
	}
	row = candidateRow{}
	err := p.db.Model(&models.VideoJobGIFCandidate{}).
		Select("id").
		Where("job_id = ? AND final_rank = ?", jobID, proposalRank).
		Order("id ASC").
		Limit(1).
		Find(&row).Error
	if err != nil {
		if isMissingTableError(err, "video_job_gif_candidates") {
			return nil, nil
		}
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil
	}
	id := row.ID
	return &id, nil
}

func (p *Processor) resolveUniqueGIFWindowIndex(jobID uint64, prefix string) (int, error) {
	if p == nil || p.db == nil {
		return 0, errors.New("processor unavailable")
	}
	base := int(time.Now().Unix()%1000000) + 1
	if base < 1 {
		base = 1
	}
	for i := 0; i < 50; i++ {
		index := base + i
		key := buildVideoImageOutputObjectKey(prefix, "gif", fmt.Sprintf("clip_%02d.gif", index))
		var count int64
		if err := p.db.Model(&models.VideoImageOutputPublic{}).Where("object_key = ?", key).Count(&count).Error; err != nil {
			return 0, err
		}
		if count == 0 {
			return index, nil
		}
	}
	return 0, errors.New("unable to allocate unique gif rerender index")
}

func (p *Processor) loadJobEstimatedCost(jobID uint64) float64 {
	if p == nil || p.db == nil || jobID == 0 {
		return 0
	}
	var row models.VideoJobCost
	if err := p.db.Select("estimated_cost").Where("job_id = ?", jobID).First(&row).Error; err != nil {
		return 0
	}
	return roundTo(row.EstimatedCost, 6)
}

func nextEmojiDisplayOrder(tx *gorm.DB, collectionID uint64, assetDomain string) (int, error) {
	if tx == nil || collectionID == 0 {
		return 1, nil
	}
	type orderRow struct {
		MaxOrder int `gorm:"column:max_order"`
	}
	var row orderRow
	if strings.ToLower(strings.TrimSpace(assetDomain)) == models.VideoJobAssetDomainVideo {
		if err := tx.Model(&models.VideoAssetEmoji{}).
			Select("COALESCE(MAX(display_order), 0) AS max_order").
			Where("collection_id = ? AND status = ?", collectionID, "active").
			Scan(&row).Error; err != nil {
			return 0, err
		}
	} else {
		if err := tx.Model(&models.Emoji{}).
			Select("COALESCE(MAX(display_order), 0) AS max_order").
			Where("collection_id = ? AND status = ?", collectionID, "active").
			Scan(&row).Error; err != nil {
			return 0, err
		}
	}
	next := row.MaxOrder + 1
	if next < 1 {
		next = 1
	}
	return next, nil
}

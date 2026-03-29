package videojobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

func (p *Processor) processUnified(ctx context.Context, jobID uint64) error {
	return p.processUnifiedWithLane(ctx, jobID, "")
}

func (p *Processor) processUnifiedWithLane(ctx context.Context, jobID uint64, lane string) error {
	executionLane := strings.ToLower(strings.TrimSpace(lane))
	if executionLane == "" {
		executionLane = "auto"
	}

	var job models.VideoJob
	if err := p.db.First(&job, jobID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: video job not found", asynq.SkipRetry)
		}
		return err
	}
	if job.Status == models.VideoJobStatusDone || job.Status == models.VideoJobStatusCancelled {
		return nil
	}
	if recovered, err := p.recoverCompletedJobFromExistingResult(&job); err != nil {
		return err
	} else if recovered {
		return nil
	}
	if strings.TrimSpace(job.SourceVideoKey) == "" {
		return permanentError{err: errors.New("source video key is empty")}
	}
	preOptions := parseJSONMap(job.Options)
	if isFlowAwaitingAI1Confirm(preOptions) {
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "waiting for user confirmation before continuing", map[string]interface{}{
			"flow_mode": "ai1_confirm",
		})
		return nil
	}

	now := time.Now()
	acquired, err := p.acquireVideoJobRun(job.ID, now)
	if err != nil {
		return fmt.Errorf("acquire video job run: %w", err)
	}
	if !acquired {
		p.appendJobEvent(job.ID, models.VideoJobStageQueued, "info", "skip duplicated processing trigger", nil)
		return nil
	}
	p.appendJobEvent(job.ID, models.VideoJobStagePreprocessing, "info", "execution lane resolved", map[string]interface{}{
		"execution_lane":   executionLane,
		"requested_format": PrimaryRequestedFormat(job.OutputFormats),
	})
	p.appendJobEvent(job.ID, models.VideoJobStagePreprocessing, "info", "video job started", nil)

	if p.isJobCancelled(job.ID) {
		p.appendJobEvent(job.ID, models.VideoJobStageCancelled, "info", "job cancelled before processing", nil)
		p.syncJobCost(job.ID)
		p.syncJobPointSettlement(job.ID, models.VideoJobStatusCancelled)
		p.cleanupSourceVideo(job.ID, "cancelled")
		return nil
	}

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return permanentError{err: errors.New("ffmpeg not found in PATH")}
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return permanentError{err: errors.New("ffprobe not found in PATH")}
	}

	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("video-job-%d-*", job.ID))
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.mp4")
	optionsPayload := parseJSONMap(job.Options)
	expectedSourceSizeBytes := sourceInt64FromAny(optionsPayload["source_video_size_bytes"])
	sourceReadability, err := p.downloadObjectByKeyWithReadability(ctx, job.SourceVideoKey, sourcePath, expectedSourceSizeBytes)
	if err != nil {
		p.persistSourceReadability(job.ID, job.Metrics, sourceReadability)
		p.appendJobEvent(job.ID, models.VideoJobStagePreprocessing, "warn", "source video readability failed", sourceReadability)
		var readErr *sourceReadError
		if errors.As(err, &readErr) && readErr != nil && readErr.Permanent {
			return permanentError{err: fmt.Errorf("download source video: %w", readErr)}
		}
		return fmt.Errorf("download source video: %w", err)
	}
	if used, ok := sourceReadability["used_fallback"].(bool); ok && used {
		p.appendJobEvent(job.ID, models.VideoJobStagePreprocessing, "info", "source video readability fallback used", sourceReadability)
	}

	meta, err := probeVideo(ctx, sourcePath)
	if err != nil {
		return fmt.Errorf("probe video: %w", err)
	}
	sourceInfo, _ := os.Stat(sourcePath)

	metrics := map[string]interface{}{
		"duration_sec": meta.DurationSec,
		"width":        meta.Width,
		"height":       meta.Height,
		"fps":          meta.FPS,
	}
	if len(sourceReadability) > 0 {
		metrics["source_video_readability_v1"] = sourceReadability
	}
	if sourceInfo != nil && sourceInfo.Size() > 0 {
		metrics["source_video_size_bytes"] = sourceInfo.Size()
	}
	gifSubStages := map[string]map[string]interface{}{
		gifSubStageBriefing:  {"status": "pending"},
		gifSubStagePlanning:  {"status": "pending"},
		gifSubStageScoring:   {"status": "pending"},
		gifSubStageReviewing: {"status": "pending"},
	}
	metrics["gif_pipeline_sub_stages_v1"] = gifSubStages

	markGIFSubStageRunning := func(name string, detail map[string]interface{}) time.Time {
		started := time.Now()
		stageDetail := map[string]interface{}{
			"status":      "running",
			"started_at":  started.Format(time.RFC3339),
			"finished_at": "",
			"duration_ms": int64(0),
		}
		for k, v := range detail {
			stageDetail[k] = v
		}
		gifSubStages[name] = stageDetail
		return started
	}
	markGIFSubStageDone := func(name string, started time.Time, status string, detail map[string]interface{}) {
		finalStatus := strings.ToLower(strings.TrimSpace(status))
		if finalStatus == "" {
			finalStatus = "done"
		}
		stageDetail := gifSubStages[name]
		if stageDetail == nil {
			stageDetail = map[string]interface{}{}
		}
		if _, ok := stageDetail["started_at"]; !ok {
			if !started.IsZero() {
				stageDetail["started_at"] = started.Format(time.RFC3339)
			} else {
				stageDetail["started_at"] = time.Now().Format(time.RFC3339)
			}
		}
		finished := time.Now()
		stageDetail["status"] = finalStatus
		stageDetail["finished_at"] = finished.Format(time.RFC3339)
		if !started.IsZero() {
			stageDetail["duration_ms"] = clampDurationMillis(started)
		}
		for k, v := range detail {
			stageDetail[k] = v
		}
		gifSubStages[name] = stageDetail
	}
	markGIFSubStageSkipped := func(name string, reason string) {
		stageDetail := gifSubStages[name]
		if stageDetail == nil {
			stageDetail = map[string]interface{}{}
		}
		stageDetail["status"] = "skipped"
		stageDetail["reason"] = strings.TrimSpace(reason)
		if _, ok := stageDetail["started_at"]; !ok {
			stageDetail["started_at"] = ""
		}
		if _, ok := stageDetail["finished_at"]; !ok {
			stageDetail["finished_at"] = ""
		}
		if _, ok := stageDetail["duration_ms"]; !ok {
			stageDetail["duration_ms"] = int64(0)
		}
		gifSubStages[name] = stageDetail
	}
	hasGIFSubStageFinalStatus := func(name string) bool {
		stageDetail := gifSubStages[name]
		if stageDetail == nil {
			return false
		}
		status := strings.ToLower(strings.TrimSpace(stringFromAny(stageDetail["status"])))
		switch status {
		case "done", "degraded", "failed", "skipped":
			return true
		default:
			return false
		}
	}

	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageAnalyzing,
		"progress": 30,
		"metrics":  mustJSON(metrics),
	})
	p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "video metadata analyzed", metrics)

	if p.isJobCancelled(job.ID) {
		p.appendJobEvent(job.ID, models.VideoJobStageCancelled, "info", "job cancelled during analyzing", nil)
		p.syncJobCost(job.ID)
		p.syncJobPointSettlement(job.ID, models.VideoJobStatusCancelled)
		p.cleanupSourceVideo(job.ID, "cancelled")
		return nil
	}

	options := parseJobOptions(job.Options)
	qualitySettings := p.loadQualitySettings()
	requestedFormats := normalizeOutputFormats(job.OutputFormats)
	flowMode := normalizeVideoFlowMode(stringFromAny(optionsPayload["flow_mode"]))
	ai1Confirmed := boolFromAny(optionsPayload["ai1_confirmed"])
	ai1PauseConsumed := boolFromAny(optionsPayload["ai1_pause_consumed"])
	pipelineMode := gifPipelineModeStandard
	pipelineModeDecision := gifPipelineModeDecision{
		Mode:          gifPipelineModeStandard,
		RequestedMode: normalizeRequestedGIFPipelineMode(stringFromAny(optionsPayload["gif_pipeline_mode"])),
		Reason:        "non_gif_job",
		EnableAI1:     false,
		EnableAI2:     false,
		EnableAI3:     false,
	}
	if containsString(requestedFormats, "gif") {
		pipelineModeDecision = resolveGIFPipelineMode(job, meta, optionsPayload, qualitySettings)
		pipelineMode = pipelineModeDecision.Mode
	}
	metrics["gif_pipeline_mode_v1"] = map[string]interface{}{
		"resolved_mode":       pipelineModeDecision.Mode,
		"requested_mode":      pipelineModeDecision.RequestedMode,
		"reason":              pipelineModeDecision.Reason,
		"enable_ai1":          pipelineModeDecision.EnableAI1,
		"enable_ai2":          pipelineModeDecision.EnableAI2,
		"enable_ai3":          pipelineModeDecision.EnableAI3,
		"priority":            strings.TrimSpace(strings.ToLower(job.Priority)),
		"duration_sec":        roundTo(meta.DurationSec, 3),
		"short_video_max_sec": roundTo(qualitySettings.GIFPipelineShortVideoMaxSec, 3),
		"long_video_min_sec":  roundTo(qualitySettings.GIFPipelineLongVideoMinSec, 3),
		"short_video_mode":    qualitySettings.GIFPipelineShortVideoMode,
		"default_mode":        qualitySettings.GIFPipelineDefaultMode,
		"long_video_mode":     qualitySettings.GIFPipelineLongVideoMode,
		"requested_format": func() string {
			if len(requestedFormats) == 0 {
				return ""
			}
			return strings.Join(requestedFormats, ",")
		}(),
	}
	p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "gif pipeline mode resolved", map[string]interface{}{
		"resolved_mode":       pipelineModeDecision.Mode,
		"requested_mode":      pipelineModeDecision.RequestedMode,
		"reason":              pipelineModeDecision.Reason,
		"enable_ai1":          pipelineModeDecision.EnableAI1,
		"enable_ai2":          pipelineModeDecision.EnableAI2,
		"enable_ai3":          pipelineModeDecision.EnableAI3,
		"short_video_max_sec": roundTo(qualitySettings.GIFPipelineShortVideoMaxSec, 3),
		"long_video_min_sec":  roundTo(qualitySettings.GIFPipelineLongVideoMinSec, 3),
		"short_video_mode":    qualitySettings.GIFPipelineShortVideoMode,
		"default_mode":        qualitySettings.GIFPipelineDefaultMode,
		"long_video_mode":     qualitySettings.GIFPipelineLongVideoMode,
	})
	qualitySettings, qualityOverrides := applyQualityProfileOverridesFromOptions(qualitySettings, optionsPayload, requestedFormats)
	if len(qualityOverrides) > 0 {
		metrics["quality_profile_overrides_applied"] = qualityOverrides
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "quality profile overrides applied", map[string]interface{}{
			"overrides": qualityOverrides,
		})
	}
	sceneTags := inferSceneTags(job.Title, job.SourceVideoKey, requestedFormats)
	if len(sceneTags) > 0 {
		metrics["scene_tags_v1"] = sceneTags
	}
	if shouldPauseAtAI1(flowMode, ai1Confirmed, ai1PauseConsumed) && !containsString(requestedFormats, "gif") {
		directive, directorSnapshot, directorErr := p.requestAIGIFPromptDirective(ctx, job, sourcePath, meta, highlightSuggestion{}, qualitySettings)
		aiReply := buildGenericAI1Reply(job.Title, requestedFormats, meta)
		eventMeta := map[string]interface{}{
			"sub_stage":    gifSubStageBriefing,
			"flow_mode":    flowMode,
			"duration_sec": roundTo(meta.DurationSec, 3),
			"width":        meta.Width,
			"height":       meta.Height,
			"fps":          roundTo(meta.FPS, 3),
		}
		if directorErr != nil {
			metrics["highlight_ai_director_v1"] = map[string]interface{}{
				"applied": false,
				"error":   directorErr.Error(),
				"mode":    pipelineMode,
			}
			eventMeta["error"] = directorErr.Error()
		} else {
			metrics["highlight_ai_director_v1"] = directorSnapshot
			if directive != nil {
				aiReply = strings.TrimSpace(buildAIDirectorNaturalReply(directive))
				if aiReply == "" {
					aiReply = buildGenericAI1Reply(job.Title, requestedFormats, meta)
				}
				eventMeta["business_goal"] = strings.TrimSpace(directive.BusinessGoal)
				eventMeta["audience"] = strings.TrimSpace(directive.Audience)
				eventMeta["must_capture"] = directive.MustCapture
				eventMeta["avoid"] = directive.Avoid
				eventMeta["risk_flags"] = directive.RiskFlags
				eventMeta["quality_weights"] = directive.QualityWeights
				eventMeta["style_direction"] = strings.TrimSpace(directive.StyleDirection)
				eventMeta["directive_text"] = strings.TrimSpace(directive.DirectiveText)
				eventMeta["clip_count_min"] = directive.ClipCountMin
				eventMeta["clip_count_max"] = directive.ClipCountMax
				eventMeta["loop_preference"] = roundTo(directive.LoopPreference, 4)
			}
		}
		eventMeta["ai_reply"] = aiReply
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "ai1 preview generated", map[string]interface{}{
			"sub_stage":       eventMeta["sub_stage"],
			"flow_mode":       eventMeta["flow_mode"],
			"ai_reply":        eventMeta["ai_reply"],
			"duration_sec":    eventMeta["duration_sec"],
			"width":           eventMeta["width"],
			"height":          eventMeta["height"],
			"fps":             eventMeta["fps"],
			"error":           eventMeta["error"],
			"business_goal":   eventMeta["business_goal"],
			"audience":        eventMeta["audience"],
			"must_capture":    eventMeta["must_capture"],
			"avoid":           eventMeta["avoid"],
			"style_direction": eventMeta["style_direction"],
			"directive_text":  eventMeta["directive_text"],
			"clip_count_min":  eventMeta["clip_count_min"],
			"clip_count_max":  eventMeta["clip_count_max"],
		})
		p.persistAI1Plan(job, requestedFormats, flowMode, ai1Confirmed, meta, "ai1_preview_generated", eventMeta, directorSnapshot)
		if p.pauseForAI1Confirmation(job.ID, metrics, optionsPayload, map[string]interface{}{
			"flow_mode": flowMode,
			"stage":     "ai1_preview_generated",
		}) {
			return nil
		}
	}
	options = applyAnimatedProfileDefaults(options, requestedFormats, qualitySettings)
	if options.CropW <= 0 || options.CropH <= 0 {
		autoCrop, applied, cropErr := detectAutoLetterboxCrop(ctx, sourcePath, meta)
		switch {
		case cropErr != nil:
			metrics["auto_crop_v1"] = map[string]interface{}{
				"enabled": true,
				"applied": false,
				"error":   cropErr.Error(),
			}
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "auto letterbox crop detection failed", map[string]interface{}{
				"error": cropErr.Error(),
			})
		case applied:
			options.CropX = autoCrop.CropX
			options.CropY = autoCrop.CropY
			options.CropW = autoCrop.CropW
			options.CropH = autoCrop.CropH
			optionsPayload["crop_x"] = options.CropX
			optionsPayload["crop_y"] = options.CropY
			optionsPayload["crop_w"] = options.CropW
			optionsPayload["crop_h"] = options.CropH
			optionsPayload["auto_crop_v1"] = autoCrop
			metrics["auto_crop_v1"] = autoCrop
			p.updateVideoJob(job.ID, map[string]interface{}{
				"options": mustJSON(optionsPayload),
			})
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "auto letterbox crop applied", map[string]interface{}{
				"crop_x":            autoCrop.CropX,
				"crop_y":            autoCrop.CropY,
				"crop_w":            autoCrop.CropW,
				"crop_h":            autoCrop.CropH,
				"confidence":        autoCrop.Confidence,
				"removed_area_rate": roundTo(autoCrop.RemovedAreaRate, 4),
			})
		default:
			metrics["auto_crop_v1"] = map[string]interface{}{
				"enabled": true,
				"applied": false,
			}
		}
	}
	manualClipWindow := options.StartSec > 0 || options.EndSec > 0
	var highlightPlan *highlightSuggestion
	if options.AutoHighlight && options.StartSec <= 0 && options.EndSec <= 0 {
		var (
			suggestion        highlightSuggestion
			directorDirective *gifAIDirectiveProfile
		)

		applyLocalHighlightFallback := func(reason string) {
			if suggestion.Selected != nil {
				return
			}
			localSuggestion, localErr := suggestHighlightWindow(ctx, sourcePath, meta, qualitySettings)
			if localErr != nil {
				metrics["highlight_local_fallback_v1"] = map[string]interface{}{
					"used":   false,
					"reason": reason,
					"error":  localErr.Error(),
				}
				p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "local highlight fallback failed", map[string]interface{}{
					"reason": reason,
					"error":  localErr.Error(),
				})
				return
			}
			if localSuggestion.Selected == nil {
				metrics["highlight_local_fallback_v1"] = map[string]interface{}{
					"used":   false,
					"reason": reason,
					"error":  "local highlight produced empty selection",
				}
				return
			}
			suggestion = localSuggestion
			metrics["highlight_local_fallback_v1"] = map[string]interface{}{
				"used":           true,
				"reason":         reason,
				"selected_start": localSuggestion.Selected.StartSec,
				"selected_end":   localSuggestion.Selected.EndSec,
				"selected_score": localSuggestion.Selected.Score,
			}
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "local highlight fallback applied", map[string]interface{}{
				"reason":    reason,
				"start_sec": localSuggestion.Selected.StartSec,
				"end_sec":   localSuggestion.Selected.EndSec,
				"score":     localSuggestion.Selected.Score,
			})
		}

		if pipelineModeDecision.EnableAI1 {
			briefingStarted := markGIFSubStageRunning(gifSubStageBriefing, map[string]interface{}{
				"entry_stage": models.VideoJobStageAnalyzing,
			})
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "gif sub-stage briefing started", map[string]interface{}{
				"sub_stage": gifSubStageBriefing,
				"status":    "running",
				"mode":      pipelineMode,
			})
			resolvedDirective, directorSnapshot, directorErr := p.requestAIGIFPromptDirective(ctx, job, sourcePath, meta, highlightSuggestion{}, qualitySettings)
			if directorErr != nil {
				metrics["highlight_ai_director_v1"] = map[string]interface{}{
					"enabled":        directorSnapshot["enabled"],
					"provider":       directorSnapshot["provider"],
					"model":          directorSnapshot["model"],
					"prompt_version": directorSnapshot["prompt_version"],
					"applied":        false,
					"error":          directorErr.Error(),
					"mode":           pipelineMode,
				}
				markGIFSubStageDone(gifSubStageBriefing, briefingStarted, "degraded", map[string]interface{}{
					"error":    directorErr.Error(),
					"fallback": true,
					"mode":     pipelineMode,
				})
				p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "ai director unavailable; fallback to default planner context", map[string]interface{}{
					"error":     directorErr.Error(),
					"sub_stage": gifSubStageBriefing,
					"mode":      pipelineMode,
				})
			} else {
				directorDirective = resolvedDirective
				metrics["highlight_ai_director_v1"] = directorSnapshot
				markGIFSubStageDone(gifSubStageBriefing, briefingStarted, "done", map[string]interface{}{
					"applied": true,
					"mode":    pipelineMode,
				})
				ai1Event := map[string]interface{}{
					"business_goal":  directorSnapshot["business_goal"],
					"clip_count_min": directorSnapshot["clip_count_min"],
					"clip_count_max": directorSnapshot["clip_count_max"],
					"sub_stage":      gifSubStageBriefing,
					"mode":           pipelineMode,
				}
				if resolvedDirective != nil {
					ai1Event["audience"] = strings.TrimSpace(resolvedDirective.Audience)
					ai1Event["must_capture"] = resolvedDirective.MustCapture
					ai1Event["avoid"] = resolvedDirective.Avoid
					ai1Event["risk_flags"] = resolvedDirective.RiskFlags
					ai1Event["quality_weights"] = resolvedDirective.QualityWeights
					ai1Event["style_direction"] = strings.TrimSpace(resolvedDirective.StyleDirection)
					ai1Event["directive_text"] = strings.TrimSpace(resolvedDirective.DirectiveText)
					ai1Event["loop_preference"] = roundTo(resolvedDirective.LoopPreference, 4)
					ai1Event["ai_reply"] = strings.TrimSpace(buildAIDirectorNaturalReply(resolvedDirective))
				}
				p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "ai director prompt pack generated", ai1Event)
				p.persistAI1Plan(job, requestedFormats, flowMode, ai1Confirmed, meta, gifSubStageBriefing, ai1Event, directorSnapshot)
				if shouldPauseAtAI1(flowMode, ai1Confirmed, ai1PauseConsumed) {
					if p.pauseForAI1Confirmation(job.ID, metrics, optionsPayload, map[string]interface{}{
						"flow_mode": flowMode,
						"stage":     gifSubStageBriefing,
					}) {
						return nil
					}
				}
			}
		} else {
			markGIFSubStageSkipped(gifSubStageBriefing, "pipeline_mode_"+pipelineMode+"_director_skipped")
			metrics["highlight_ai_director_v1"] = map[string]interface{}{
				"enabled": false,
				"applied": false,
				"mode":    pipelineMode,
				"reason":  "pipeline_mode_director_skipped",
			}
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "gif sub-stage briefing skipped by pipeline mode", map[string]interface{}{
				"sub_stage": gifSubStageBriefing,
				"mode":      pipelineMode,
			})
		}

		if pipelineModeDecision.EnableAI2 {
			planningStarted := markGIFSubStageRunning(gifSubStagePlanning, map[string]interface{}{
				"entry_stage": models.VideoJobStageAnalyzing,
			})
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "gif sub-stage planning started", map[string]interface{}{
				"sub_stage": gifSubStagePlanning,
				"status":    "running",
				"mode":      pipelineMode,
			})
			plannerSuggestion, plannerSnapshot, plannerErr := p.requestAIGIFPlannerSuggestion(ctx, job, sourcePath, meta, suggestion, directorDirective, qualitySettings)
			if plannerErr != nil {
				metrics["highlight_ai_planner_v1"] = map[string]interface{}{
					"enabled":        plannerSnapshot["enabled"],
					"provider":       plannerSnapshot["provider"],
					"model":          plannerSnapshot["model"],
					"prompt_version": plannerSnapshot["prompt_version"],
					"applied":        false,
					"error":          plannerErr.Error(),
					"mode":           pipelineMode,
				}
				markGIFSubStageDone(gifSubStagePlanning, planningStarted, "degraded", map[string]interface{}{
					"error": plannerErr.Error(),
					"mode":  pipelineMode,
				})
				p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "ai planner unavailable; fallback to local highlight", map[string]interface{}{
					"error":     plannerErr.Error(),
					"sub_stage": gifSubStagePlanning,
					"mode":      pipelineMode,
				})
				applyLocalHighlightFallback("ai_planner_error")
			} else {
				suggestion = plannerSuggestion
				metrics["highlight_ai_planner_v1"] = plannerSnapshot
				markGIFSubStageDone(gifSubStagePlanning, planningStarted, "done", map[string]interface{}{
					"applied": true,
					"mode":    pipelineMode,
				})
				p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "ai planner suggestion applied", map[string]interface{}{
					"selected_start_sec": suggestion.Selected.StartSec,
					"selected_end_sec":   suggestion.Selected.EndSec,
					"selected_score":     suggestion.Selected.Score,
					"selected_count":     len(suggestion.Candidates),
					"score_formula":      stringFromAny(plannerSnapshot["planner_score_formula"]),
					"scoring_summary_v1": mapFromAny(plannerSnapshot["scoring_summary_v1"]),
					"sub_stage":          gifSubStagePlanning,
					"mode":               pipelineMode,
				})
			}
		} else {
			markGIFSubStageSkipped(gifSubStagePlanning, "pipeline_mode_"+pipelineMode+"_planner_skipped")
			metrics["highlight_ai_planner_v1"] = map[string]interface{}{
				"enabled": false,
				"applied": false,
				"mode":    pipelineMode,
				"reason":  "pipeline_mode_planner_skipped",
			}
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "gif sub-stage planning skipped by pipeline mode", map[string]interface{}{
				"sub_stage": gifSubStagePlanning,
				"mode":      pipelineMode,
			})
			applyLocalHighlightFallback("pipeline_mode_planner_skipped")
		}

		if suggestion.Selected == nil {
			applyLocalHighlightFallback("planner_empty")
		}
		plannerPrimary := strings.EqualFold(strings.TrimSpace(suggestion.Strategy), "ai_semantic_planner")

		feedbackMetrics := map[string]interface{}{
			"enabled": qualitySettings.HighlightFeedbackEnabled,
			"group":   "off",
		}
		if suggestion.Selected != nil && qualitySettings.HighlightFeedbackEnabled {
			feedbackMetrics["rollout_percent"] = qualitySettings.HighlightFeedbackRollout
			feedbackMetrics["negative_guard_enabled"] = qualitySettings.HighlightNegativeGuardEnabled
			feedbackMetrics["negative_guard_threshold"] = roundTo(qualitySettings.HighlightNegativeGuardThreshold, 4)
			feedbackMetrics["negative_guard_min_weight"] = roundTo(qualitySettings.HighlightNegativeGuardMinWeight, 4)
			feedbackMetrics["negative_guard_penalty_scale"] = roundTo(qualitySettings.HighlightNegativePenaltyScale, 4)
			feedbackMetrics["negative_guard_penalty_weight"] = roundTo(qualitySettings.HighlightNegativePenaltyWeight, 4)
			if inTreatment := shouldApplyFeedbackRerank(job.ID, qualitySettings); !inTreatment {
				feedbackMetrics["group"] = "control"
			} else {
				feedbackMetrics["group"] = "treatment"
				profile, profileErr := p.loadUserHighlightFeedbackProfile(job.UserID, 80, qualitySettings)
				if profileErr != nil {
					p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "load highlight feedback profile failed", map[string]interface{}{
						"error": profileErr.Error(),
					})
					feedbackMetrics["error"] = profileErr.Error()
				} else if plannerPrimary {
					feedbackMetrics["advisory_only"] = true
					feedbackMetrics["applied"] = false
					feedbackMetrics["reason"] = "ai2_primary_preserved"
					feedbackMetrics["engaged_jobs"] = profile.EngagedJobs
					feedbackMetrics["weighted_signals"] = roundTo(profile.WeightedSignals, 2)
					feedbackMetrics["avg_signal_weight"] = roundTo(profile.AverageSignalWeight, 2)
					feedbackMetrics["public_positive_signals"] = roundTo(profile.PublicPositiveSignals, 2)
					feedbackMetrics["public_negative_signals"] = roundTo(profile.PublicNegativeSignals, 2)
					feedbackMetrics["preferred_center"] = roundTo(profile.PreferredCenter, 4)
					feedbackMetrics["preferred_duration"] = roundTo(profile.PreferredDuration, 4)
					feedbackMetrics["reason_preference"] = profile.ReasonPreference
					feedbackMetrics["reason_negative_guard"] = profile.ReasonNegativeGuard
					feedbackMetrics["scene_preference"] = profile.ScenePreference
				} else if reranked, applied := applyHighlightFeedbackProfile(suggestion, meta.DurationSec, profile, qualitySettings); applied {
					beforeSelected := suggestion.Selected
					beforeCandidates := append([]highlightCandidate{}, suggestion.Candidates...)
					suggestion = reranked
					p.persistGIFRerankLogs(job.ID, job.UserID, beforeCandidates, suggestion.Candidates, profile)
					feedbackMetrics["applied"] = true
					feedbackMetrics["engaged_jobs"] = profile.EngagedJobs
					feedbackMetrics["weighted_signals"] = roundTo(profile.WeightedSignals, 2)
					feedbackMetrics["avg_signal_weight"] = roundTo(profile.AverageSignalWeight, 2)
					feedbackMetrics["public_positive_signals"] = roundTo(profile.PublicPositiveSignals, 2)
					feedbackMetrics["public_negative_signals"] = roundTo(profile.PublicNegativeSignals, 2)
					feedbackMetrics["preferred_center"] = roundTo(profile.PreferredCenter, 4)
					feedbackMetrics["preferred_duration"] = roundTo(profile.PreferredDuration, 4)
					feedbackMetrics["reason_preference"] = profile.ReasonPreference
					feedbackMetrics["reason_negative_guard"] = profile.ReasonNegativeGuard
					feedbackMetrics["scene_preference"] = profile.ScenePreference
					feedbackMetrics["selected_before"] = beforeSelected
					feedbackMetrics["selected_after"] = suggestion.Selected
					feedbackMetrics["candidate_count_after"] = len(suggestion.Candidates)
					p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "highlight candidates reranked by feedback profile", map[string]interface{}{
						"engaged_jobs":       profile.EngagedJobs,
						"weighted_signals":   roundTo(profile.WeightedSignals, 2),
						"selected_start_sec": suggestion.Selected.StartSec,
						"selected_end_sec":   suggestion.Selected.EndSec,
						"selected_score":     suggestion.Selected.Score,
					})
				} else {
					feedbackMetrics["applied"] = false
					feedbackMetrics["engaged_jobs"] = profile.EngagedJobs
					feedbackMetrics["weighted_signals"] = roundTo(profile.WeightedSignals, 2)
					feedbackMetrics["public_positive_signals"] = roundTo(profile.PublicPositiveSignals, 2)
					feedbackMetrics["public_negative_signals"] = roundTo(profile.PublicNegativeSignals, 2)
					feedbackMetrics["reason_negative_guard"] = profile.ReasonNegativeGuard
				}
			}
		}
		metrics["highlight_feedback_v1"] = feedbackMetrics

		if suggestion.Selected != nil {
			highlightPlan = &suggestion
			metrics["highlight_v1"] = suggestion
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "highlight scorer selected clip window", map[string]interface{}{
				"start_sec": suggestion.Selected.StartSec,
				"end_sec":   suggestion.Selected.EndSec,
				"score":     suggestion.Selected.Score,
			})

			options.StartSec = highlightPlan.Selected.StartSec
			options.EndSec = highlightPlan.Selected.EndSec
			optionsPayload["start_sec"] = options.StartSec
			optionsPayload["end_sec"] = options.EndSec
			optionsPayload["highlight_selected"] = highlightPlan.Selected
			p.updateVideoJob(job.ID, map[string]interface{}{
				"options": mustJSON(optionsPayload),
			})

			if plannerPrimary {
				metrics["highlight_cloud_fallback"] = map[string]interface{}{
					"enabled": false,
					"used":    false,
					"reason":  "ai2_primary_preserved",
				}
			} else if p.shouldUseCloudHighlightFallback(suggestion) {
				cloudSuggestion, cloudErr := p.requestCloudHighlightFallback(ctx, job.ID, job.UserID, sourcePath, meta, suggestion, qualitySettings)
				if cloudErr != nil {
					metrics["highlight_cloud_fallback"] = map[string]interface{}{
						"enabled": true,
						"used":    false,
						"error":   cloudErr.Error(),
					}
					p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "cloud highlight fallback failed", map[string]interface{}{
						"error": cloudErr.Error(),
					})
				} else if cloudSuggestion.Selected != nil {
					highlightPlan = &cloudSuggestion
					options.StartSec = cloudSuggestion.Selected.StartSec
					options.EndSec = cloudSuggestion.Selected.EndSec
					optionsPayload["start_sec"] = options.StartSec
					optionsPayload["end_sec"] = options.EndSec
					optionsPayload["highlight_selected"] = cloudSuggestion.Selected
					optionsPayload["highlight_source"] = "cloud_fallback"
					p.updateVideoJob(job.ID, map[string]interface{}{
						"options": mustJSON(optionsPayload),
					})
					metrics["highlight_cloud_fallback"] = map[string]interface{}{
						"enabled":        true,
						"used":           true,
						"start_sec":      cloudSuggestion.Selected.StartSec,
						"end_sec":        cloudSuggestion.Selected.EndSec,
						"score":          cloudSuggestion.Selected.Score,
						"candidate_size": len(cloudSuggestion.Candidates),
					}
					p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "cloud highlight fallback applied", map[string]interface{}{
						"start_sec": options.StartSec,
						"end_sec":   options.EndSec,
						"score":     cloudSuggestion.Selected.Score,
					})
				}
			}
		} else {
			metrics["highlight_v1"] = map[string]interface{}{
				"version": "v1",
				"enabled": true,
				"applied": false,
				"error":   "no_highlight_window_selected",
			}
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "highlight planning produced no selected window", nil)
		}
	}
	if !hasGIFSubStageFinalStatus(gifSubStageBriefing) {
		reason := "auto_highlight_disabled_or_no_selected_window"
		if !containsString(normalizeOutputFormats(job.OutputFormats), "gif") {
			reason = "non_gif_job"
		}
		markGIFSubStageSkipped(gifSubStageBriefing, reason)
	}
	if !hasGIFSubStageFinalStatus(gifSubStagePlanning) {
		reason := "auto_highlight_disabled_or_no_selected_window"
		if !containsString(normalizeOutputFormats(job.OutputFormats), "gif") {
			reason = "non_gif_job"
		}
		markGIFSubStageSkipped(gifSubStagePlanning, reason)
	}

	highlightCandidates := make([]highlightCandidate, 0)
	highlightCandidatePool := make([]highlightCandidate, 0)
	if highlightPlan != nil {
		scoringStarted := markGIFSubStageRunning(gifSubStageScoring, map[string]interface{}{
			"entry_stage": models.VideoJobStageAnalyzing,
		})
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "gif sub-stage scoring started", map[string]interface{}{
			"sub_stage": gifSubStageScoring,
			"status":    "running",
		})
		if err := p.persistGIFHighlightCandidates(ctx, sourcePath, meta, options, job.ID, *highlightPlan, qualitySettings); err != nil {
			highlightCandidates = append(highlightCandidates, highlightPlan.Candidates...)
			if len(highlightPlan.All) > 0 {
				highlightCandidatePool = append(highlightCandidatePool, highlightPlan.All...)
			} else {
				highlightCandidatePool = append(highlightCandidatePool, highlightPlan.Candidates...)
			}
			metrics["gif_candidates_v1"] = map[string]interface{}{
				"persisted":            false,
				"error":                err.Error(),
				"max_outputs":          qualitySettings.GIFCandidateMaxOutputs,
				"confidence_threshold": qualitySettings.GIFCandidateConfidenceThreshold,
				"dedup_iou_threshold":  qualitySettings.GIFCandidateDedupIOUThreshold,
			}
			markGIFSubStageDone(gifSubStageScoring, scoringStarted, "degraded", map[string]interface{}{
				"error": err.Error(),
			})
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "persist gif highlight candidates failed", map[string]interface{}{
				"error":     err.Error(),
				"sub_stage": gifSubStageScoring,
			})
		} else {
			highlightPlan.Candidates = p.attachGIFCandidateBindings(job.ID, highlightPlan.Candidates)
			highlightPlan.All = p.attachGIFCandidateBindings(job.ID, highlightPlan.All)
			highlightCandidates = append(highlightCandidates, highlightPlan.Candidates...)
			if len(highlightPlan.All) > 0 {
				highlightCandidatePool = append(highlightCandidatePool, highlightPlan.All...)
			} else {
				highlightCandidatePool = append(highlightCandidatePool, highlightPlan.Candidates...)
			}
			withCandidateID := 0
			withProposalID := 0
			for _, item := range highlightPlan.Candidates {
				if item.CandidateID != nil && *item.CandidateID > 0 {
					withCandidateID++
				}
				if item.ProposalID != nil && *item.ProposalID > 0 {
					withProposalID++
				}
			}
			metrics["gif_candidates_v1"] = map[string]interface{}{
				"persisted":                  true,
				"candidate_count":            len(highlightPlan.All),
				"selected_count":             len(highlightPlan.Candidates),
				"selected_with_candidate_id": withCandidateID,
				"selected_with_proposal_id":  withProposalID,
				"strategy":                   highlightPlan.Strategy,
				"version":                    highlightPlan.Version,
				"max_outputs":                qualitySettings.GIFCandidateMaxOutputs,
				"confidence_threshold":       qualitySettings.GIFCandidateConfidenceThreshold,
				"dedup_iou_threshold":        qualitySettings.GIFCandidateDedupIOUThreshold,
			}
			markGIFSubStageDone(gifSubStageScoring, scoringStarted, "done", map[string]interface{}{
				"selected_count": len(highlightPlan.Candidates),
			})
		}
	} else {
		markGIFSubStageSkipped(gifSubStageScoring, "no_highlight_plan")
	}
	extractOptions := applyStillProfileDefaults(options, requestedFormats, qualitySettings)

	frameDir := filepath.Join(tmpDir, "frames")
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return fmt.Errorf("create frame dir: %w", err)
	}

	effectiveDurationSec := effectiveSampleDuration(meta, options)
	candidateBudget := qualitySelectionCandidateBudget(extractOptions.MaxStatic)
	interval := chooseFrameInterval(effectiveDurationSec, extractOptions.FrameIntervalSec, candidateBudget)

	framePaths := make([]string, 0, candidateBudget)
	multiIntervals := make([]float64, 0, qualitySettings.GIFCandidateMaxOutputs)
	if options.AutoHighlight && !manualClipWindow && highlightPlan != nil && len(highlightPlan.Candidates) > 1 {
		paths, intervals, err := extractFramesByHighlightCandidates(ctx, sourcePath, frameDir, meta, extractOptions, highlightPlan.Candidates, candidateBudget, qualitySettings)
		if err != nil {
			p.appendJobEvent(job.ID, models.VideoJobStageRendering, "warn", "multi-window highlight extraction failed; fallback to selected window", map[string]interface{}{
				"error": err.Error(),
			})
		} else if len(paths) > 0 {
			framePaths = paths
			multiIntervals = intervals
			p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", "multi-window highlight extraction applied", map[string]interface{}{
				"windows": len(intervals),
				"frames":  len(paths),
			})
		}
	}
	if len(framePaths) == 0 {
		if err := extractFrames(ctx, sourcePath, frameDir, meta, extractOptions, interval, qualitySettings); err != nil {
			return fmt.Errorf("extract frames: %w", err)
		}
		paths, err := collectFramePaths(frameDir, candidateBudget)
		if err != nil {
			return fmt.Errorf("collect frames: %w", err)
		}
		framePaths = paths
	}
	if len(framePaths) == 0 {
		return permanentError{err: errors.New("no frames extracted from video")}
	}
	optimizedFramePaths, qualityReport := optimizeFramePathsForQuality(framePaths, extractOptions.MaxStatic, qualitySettings)
	if len(optimizedFramePaths) > 0 {
		framePaths = optimizedFramePaths
	}
	metrics["frame_quality"] = qualityReport
	metrics["quality_settings"] = map[string]interface{}{
		"min_brightness":                          qualitySettings.MinBrightness,
		"max_brightness":                          qualitySettings.MaxBrightness,
		"blur_threshold_factor":                   qualitySettings.BlurThresholdFactor,
		"duplicate_hamming_threshold":             qualitySettings.DuplicateHammingThreshold,
		"gif_default_fps":                         qualitySettings.GIFDefaultFPS,
		"gif_default_max_colors":                  qualitySettings.GIFDefaultMaxColors,
		"gif_default_dither_mode":                 qualitySettings.GIFDitherMode,
		"gif_target_size_kb":                      qualitySettings.GIFTargetSizeKB,
		"gif_gifsicle_enabled":                    qualitySettings.GIFGifsicleEnabled,
		"gif_gifsicle_level":                      qualitySettings.GIFGifsicleLevel,
		"gif_gifsicle_skip_below_kb":              qualitySettings.GIFGifsicleSkipBelowKB,
		"gif_gifsicle_min_gain_ratio":             qualitySettings.GIFGifsicleMinGainRatio,
		"gif_loop_tune_enabled":                   qualitySettings.GIFLoopTuneEnabled,
		"gif_loop_tune_min_enable":                qualitySettings.GIFLoopTuneMinEnableSec,
		"gif_loop_tune_min_improve":               qualitySettings.GIFLoopTuneMinImprovement,
		"gif_loop_tune_motion_target":             qualitySettings.GIFLoopTuneMotionTarget,
		"gif_loop_tune_prefer_sec":                qualitySettings.GIFLoopTunePreferDuration,
		"gif_candidate_max_outputs":               qualitySettings.GIFCandidateMaxOutputs,
		"gif_candidate_long_video_max_outputs":    qualitySettings.GIFCandidateLongVideoMaxOutputs,
		"gif_candidate_ultra_video_max_outputs":   qualitySettings.GIFCandidateUltraVideoMaxOutputs,
		"gif_candidate_conf_threshold":            qualitySettings.GIFCandidateConfidenceThreshold,
		"gif_candidate_dedup_iou":                 qualitySettings.GIFCandidateDedupIOUThreshold,
		"gif_render_selection_version":            gifRenderSelectionVersion,
		"gif_render_budget_normal_mult":           qualitySettings.GIFRenderBudgetNormalMultiplier,
		"gif_render_budget_long_mult":             qualitySettings.GIFRenderBudgetLongMultiplier,
		"gif_render_budget_ultra_mult":            qualitySettings.GIFRenderBudgetUltraMultiplier,
		"gif_medium_video_threshold_sec":          qualitySettings.GIFDurationTierMediumSec,
		"gif_long_video_threshold_sec":            qualitySettings.GIFDurationTierLongSec,
		"gif_ultra_video_threshold_sec":           qualitySettings.GIFDurationTierUltraSec,
		"gif_segment_timeout_min_sec":             qualitySettings.GIFSegmentTimeoutMinSec,
		"gif_segment_timeout_max_sec":             qualitySettings.GIFSegmentTimeoutMaxSec,
		"gif_segment_timeout_fallback_cap_sec":    qualitySettings.GIFSegmentTimeoutFallbackCapSec,
		"gif_segment_timeout_emergency_cap_sec":   qualitySettings.GIFSegmentTimeoutEmergencyCapSec,
		"gif_segment_timeout_last_resort_cap_sec": qualitySettings.GIFSegmentTimeoutLastResortCapSec,
		"webp_target_size_kb":                     qualitySettings.WebPTargetSizeKB,
		"jpg_target_size_kb":                      qualitySettings.JPGTargetSizeKB,
		"png_target_size_kb":                      qualitySettings.PNGTargetSizeKB,
		"duplicate_backtrack_frames":              qualitySettings.DuplicateBacktrackFrames,
		"fallback_blur_relax_factor":              qualitySettings.FallbackBlurRelaxFactor,
		"fallback_hamming_threshold":              qualitySettings.FallbackHammingThreshold,
		"quality_min_keep_base":                   qualitySettings.MinKeepBase,
		"quality_min_keep_ratio":                  qualitySettings.MinKeepRatio,
		"still_min_blur_score":                    qualitySettings.StillMinBlurScore,
		"still_min_exposure_score":                qualitySettings.StillMinExposureScore,
		"still_min_width":                         qualitySettings.StillMinWidth,
		"still_min_height":                        qualitySettings.StillMinHeight,
		"quality_analysis_workers":                qualitySettings.QualityAnalysisWorkers,
		"upload_concurrency":                      qualitySettings.UploadConcurrency,
		"gif_profile":                             qualitySettings.GIFProfile,
		"webp_profile":                            qualitySettings.WebPProfile,
		"live_profile":                            qualitySettings.LiveProfile,
		"jpg_profile":                             qualitySettings.JPGProfile,
		"png_profile":                             qualitySettings.PNGProfile,
		"still_clarity_enhance":                   shouldApplyStillClarityEnhancement(meta, extractOptions, qualitySettings),
		"quality_candidate_budget":                candidateBudget,
	}
	if len(multiIntervals) > 0 {
		interval = averageFloat(multiIntervals)
		metrics["highlight_multi_window"] = map[string]interface{}{
			"enabled":       true,
			"window_count":  len(multiIntervals),
			"intervals_sec": roundFloatSlice(multiIntervals, 3),
		}
	}

	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageRendering,
		"progress": 55,
	})
	p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", "frame extraction completed", map[string]interface{}{
		"frames":                    len(framePaths),
		"quality_blur_reject":       qualityReport.RejectedBlur,
		"quality_bright_reject":     qualityReport.RejectedBrightness,
		"quality_exposure_reject":   qualityReport.RejectedExposure,
		"quality_resolution_reject": qualityReport.RejectedResolution,
		"quality_still_blur_reject": qualityReport.RejectedStillBlurGate,
		"quality_dup_reject":        qualityReport.RejectedNearDuplicate,
		"quality_fallback":          qualityReport.FallbackApplied,
	})

	if p.isJobCancelled(job.ID) {
		p.appendJobEvent(job.ID, models.VideoJobStageCancelled, "info", "job cancelled after rendering", nil)
		p.syncJobCost(job.ID)
		p.syncJobPointSettlement(job.ID, models.VideoJobStatusCancelled)
		p.cleanupSourceVideo(job.ID, "cancelled")
		return nil
	}

	// 进入候选窗口渲染阶段，避免在实际仍在渲染时对外显示为 uploading/70。
	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageRendering,
		"progress": 70,
	})
	p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", "animated render pipeline started", map[string]interface{}{
		"entry_progress": 70,
	})

	animatedWindows := make([]highlightCandidate, 0, 6)
	gifRenderSelectionSnapshot := map[string]interface{}{
		"version": gifRenderSelectionVersion,
		"enabled": false,
		"reason":  "non_gif_job",
	}
	if containsString(requestedFormats, "gif") {
		renderCandidatePool := highlightCandidates
		if len(highlightCandidatePool) > len(renderCandidatePool) {
			renderCandidatePool = highlightCandidatePool
		}
		preferredMaxOutputs := len(renderCandidatePool)
		animatedWindows, gifRenderSelectionSnapshot = resolveOutputClipWindows(meta, options, renderCandidatePool, qualitySettings, preferredMaxOutputs)
		p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", "gif render windows selected", map[string]interface{}{
			"version":                 gifRenderSelectionVersion,
			"candidate_pool_count":    intFromAny(gifRenderSelectionSnapshot["candidate_pool_count"]),
			"selected_window_count":   intFromAny(gifRenderSelectionSnapshot["selected_window_count"]),
			"duration_tier":           stringFromAny(gifRenderSelectionSnapshot["duration_tier"]),
			"confidence_threshold":    roundTo(floatFromAny(gifRenderSelectionSnapshot["confidence_threshold"]), 4),
			"estimated_selected_kb":   roundTo(floatFromAny(gifRenderSelectionSnapshot["estimated_selected_kb"]), 2),
			"estimated_budget_limit":  roundTo(floatFromAny(gifRenderSelectionSnapshot["estimated_budget_limit_kb"]), 2),
			"dropped_low_confidence":  intFromAny(gifRenderSelectionSnapshot["dropped_low_confidence"]),
			"dropped_size_budget":     intFromAny(gifRenderSelectionSnapshot["dropped_size_budget"]),
			"dropped_by_output_limit": intFromAny(gifRenderSelectionSnapshot["dropped_output_limit"]),
			"fallback_applied":        boolFromAny(gifRenderSelectionSnapshot["fallback_applied"]),
			"fallback_reason":         stringFromAny(gifRenderSelectionSnapshot["fallback_reason"]),
		})
	}
	metrics["gif_render_selection_v1"] = gifRenderSelectionSnapshot

	resultCollectionID, totalFrames, uploadedKeys, generatedFormats, packageOutcome, err := p.persistJobResults(
		ctx,
		job,
		framePaths,
		sourcePath,
		meta,
		options,
		highlightCandidates,
		animatedWindows,
		qualitySettings,
	)
	if err != nil {
		deleteQiniuKeysByPrefix(p.qiniu, uploadedKeys)
		return err
	}

	// 渲染产物已落库，进入上传/收尾（含 zip 与 AI3 复审）阶段。
	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageUploading,
		"progress": 88,
	})

	metrics["static_count"] = totalFrames
	metrics["output_formats_requested"] = normalizeOutputFormats(job.OutputFormats)
	metrics["output_formats"] = generatedFormats
	metrics["result_collection_id"] = resultCollectionID
	metrics["asset_domain"] = models.VideoJobAssetDomainVideo
	metrics["edit_options"] = jobOptionsMetrics(options, interval)
	metrics["effective_duration_sec"] = roundTo(effectiveDurationSec, 3)
	metrics["package_zip_status"] = packageOutcome.Status
	metrics["package_zip_attempts"] = packageOutcome.Attempts
	metrics["package_zip_retry_count"] = packageOutcome.RetryCount
	if packageOutcome.Key != "" {
		metrics["package_zip_key"] = packageOutcome.Key
	}
	if packageOutcome.Name != "" {
		metrics["package_zip_name"] = packageOutcome.Name
	}
	if packageOutcome.SizeBytes > 0 {
		metrics["package_zip_size_bytes"] = packageOutcome.SizeBytes
	}
	if packageOutcome.Error != "" {
		metrics["package_zip_error"] = packageOutcome.Error
	}
	judgeMetricSnapshot := aiGIFJudgeRunSnapshot{}
	if containsString(generatedFormats, "gif") && pipelineModeDecision.EnableAI3 {
		reviewingStarted := markGIFSubStageRunning(gifSubStageReviewing, map[string]interface{}{
			"entry_stage": models.VideoJobStageUploading,
		})
		p.appendJobEvent(job.ID, models.VideoJobStageUploading, "info", "gif sub-stage reviewing started", normalizeVideoJobAIUsageMetadata(map[string]interface{}{
			"sub_stage": gifSubStageReviewing,
			"status":    "running",
		}))
		judgeSnapshot, judgeErr := p.runAIGIFJudgeReview(ctx, job, qualitySettings)
		judgeMetricSnapshot = decodeAIGIFJudgeRunSnapshot(judgeSnapshot)
		if judgeErr != nil {
			judgeMetricSnapshot.Applied = false
			judgeMetricSnapshot.Error = judgeErr.Error()
			metrics["gif_ai_judge_v1"] = judgeMetricSnapshot
			markGIFSubStageDone(gifSubStageReviewing, reviewingStarted, "degraded", map[string]interface{}{
				"error": judgeErr.Error(),
			})
			p.appendJobEvent(job.ID, models.VideoJobStageUploading, "warn", "gif ai judge failed", normalizeVideoJobAIUsageMetadata(aiGIFJudgeFailedEvent{
				SubStage: gifSubStageReviewing,
				Error:    judgeErr.Error(),
				Judge:    judgeMetricSnapshot,
			}))
		} else {
			metrics["gif_ai_judge_v1"] = judgeMetricSnapshot
			markGIFSubStageDone(gifSubStageReviewing, reviewingStarted, "done", map[string]interface{}{
				"applied": true,
			})
			p.appendJobEvent(job.ID, models.VideoJobStageUploading, "info", "gif ai judge completed", normalizeVideoJobAIUsageMetadata(aiGIFJudgeCompletedEvent{
				SubStage: gifSubStageReviewing,
				Judge:    judgeMetricSnapshot,
			}))
		}
	} else if containsString(generatedFormats, "gif") {
		markGIFSubStageSkipped(gifSubStageReviewing, "pipeline_mode_"+pipelineMode+"_judge_skipped")
		judgeMetricSnapshot = aiGIFJudgeRunSnapshot{
			Enabled: false,
			Applied: false,
			Mode:    pipelineMode,
			Reason:  "pipeline_mode_judge_skipped",
		}
		metrics["gif_ai_judge_v1"] = judgeMetricSnapshot
		p.appendJobEvent(job.ID, models.VideoJobStageUploading, "info", "gif sub-stage reviewing skipped by pipeline mode", normalizeVideoJobAIUsageMetadata(aiGIFJudgeSkippedEvent{
			SubStage: gifSubStageReviewing,
			Mode:     pipelineMode,
		}))
	} else {
		markGIFSubStageSkipped(gifSubStageReviewing, "gif_not_generated")
	}
	if containsString(generatedFormats, "gif") {
		reason := "disabled_ai3_final_review_authoritative"
		if !pipelineModeDecision.EnableAI3 {
			reason = "disabled_pipeline_mode_" + pipelineMode
		} else if !judgeMetricSnapshot.Applied {
			reason = "disabled_ai3_unavailable"
		}
		deliverFallback := aiGIFDeliverFallbackResult{
			Attempted:     false,
			Applied:       false,
			Reason:        reason,
			TriggerReason: "not_applicable",
			Policy:        "ai3_final_review_authoritative",
		}
		metrics["gif_deliver_fallback_v1"] = deliverFallback
		judgeMetricSnapshot.DeliverFallbackApplied = false
		judgeMetricSnapshot.DeliverFallbackReason = reason
		judgeMetricSnapshot.DeliverFallbackTriggerReason = "not_applicable"
		metrics["gif_ai_judge_v1"] = judgeMetricSnapshot
		p.appendJobEvent(job.ID, models.VideoJobStageUploading, "info", "gif deliver fallback disabled", normalizeVideoJobAIUsageMetadata(aiGIFDeliverFallbackDisabledEvent{
			Reason: reason,
			Policy: "ai3_final_review_authoritative",
		}))
	}
	gifSubStageStatus := map[string]string{
		gifSubStageBriefing:  strings.ToLower(strings.TrimSpace(stringFromAny(mapFromAny(gifSubStages[gifSubStageBriefing])["status"]))),
		gifSubStagePlanning:  strings.ToLower(strings.TrimSpace(stringFromAny(mapFromAny(gifSubStages[gifSubStagePlanning])["status"]))),
		gifSubStageScoring:   strings.ToLower(strings.TrimSpace(stringFromAny(mapFromAny(gifSubStages[gifSubStageScoring])["status"]))),
		gifSubStageReviewing: strings.ToLower(strings.TrimSpace(stringFromAny(mapFromAny(gifSubStages[gifSubStageReviewing])["status"]))),
	}
	metrics["gif_pipeline_sub_stage_status_v1"] = gifSubStageStatus

	finishedAt := time.Now()
	p.updateVideoJob(job.ID, map[string]interface{}{
		"status":               models.VideoJobStatusDone,
		"stage":                models.VideoJobStageDone,
		"progress":             100,
		"asset_domain":         models.VideoJobAssetDomainVideo,
		"result_collection_id": resultCollectionID,
		"metrics":              mustJSON(metrics),
		"error_message":        "",
		"finished_at":          finishedAt,
	})
	p.appendJobEvent(job.ID, models.VideoJobStageDone, "info", "video job completed", map[string]interface{}{
		"collection_id":       resultCollectionID,
		"static_count":        totalFrames,
		"package_zip_status":  packageOutcome.Status,
		"package_zip_attempt": packageOutcome.Attempts,
		"gif_pipeline_status": gifSubStageStatus,
	})
	if packageOutcome.Status == packageZipStatusFailed {
		p.appendJobEvent(job.ID, models.VideoJobStageDone, "warn", "video job completed without zip package", map[string]interface{}{
			"attempts": packageOutcome.Attempts,
			"error":    packageOutcome.Error,
		})
	}
	p.syncJobCost(job.ID)
	p.syncJobPointSettlement(job.ID, models.VideoJobStatusDone)
	p.syncGIFBaseline(job.ID)
	p.cleanupSourceVideo(job.ID, "done")
	return nil
}

func normalizeVideoFlowMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ai1_confirm":
		return "ai1_confirm"
	default:
		return "direct"
	}
}

func isFlowAwaitingAI1Confirm(options map[string]interface{}) bool {
	if normalizeVideoFlowMode(stringFromAny(options["flow_mode"])) != "ai1_confirm" {
		return false
	}
	return boolFromAny(options["ai1_pending"]) && !boolFromAny(options["ai1_confirmed"])
}

func shouldPauseAtAI1(flowMode string, ai1Confirmed, ai1PauseConsumed bool) bool {
	if flowMode != "ai1_confirm" {
		return false
	}
	if ai1Confirmed {
		return false
	}
	if ai1PauseConsumed {
		return false
	}
	return true
}

const ai1NeedClarifyConfidenceThreshold = 0.62

func buildGenericAI1Reply(title string, requestedFormats []string, meta videoProbeMeta) string {
	formatLabel := "-"
	if len(requestedFormats) > 0 {
		formatLabel = strings.ToUpper(strings.Join(requestedFormats, "/"))
	}
	titleText := strings.TrimSpace(title)
	if titleText == "" {
		titleText = "未提供额外提示词"
	}
	return fmt.Sprintf(
		"我先做了首轮识别：目标输出格式 %s；视频约 %.1f 秒，分辨率 %dx%d，帧率 %.1ffps。你的需求描述是“%s”。如果方向OK，请确认继续后续生成。",
		formatLabel,
		meta.DurationSec,
		meta.Width,
		meta.Height,
		meta.FPS,
		titleText,
	)
}

func (p *Processor) pauseForAI1Confirmation(jobID uint64, metrics map[string]interface{}, optionsPayload map[string]interface{}, extra map[string]interface{}) bool {
	if p == nil || p.db == nil || jobID == 0 {
		return false
	}
	if optionsPayload == nil {
		optionsPayload = map[string]interface{}{}
	}
	if metrics == nil {
		metrics = map[string]interface{}{}
	}
	optionsPayload["ai1_pending"] = true
	optionsPayload["ai1_confirmed"] = false
	optionsPayload["ai1_pause_consumed"] = true
	optionsPayload["ai1_paused_at"] = time.Now().Format(time.RFC3339)
	optionsPayload["flow_mode"] = "ai1_confirm"
	if pauseReason := strings.TrimSpace(stringFromAny(extra["pause_reason"])); pauseReason != "" {
		optionsPayload["ai1_pause_reason"] = pauseReason
	}
	metrics["ai1_confirm_flow_v1"] = map[string]interface{}{
		"pending": true,
		"status":  "awaiting_user_confirm",
	}
	updates := map[string]interface{}{
		"status":   models.VideoJobStatusQueued,
		"stage":    models.VideoJobStageAwaitingAI1,
		"progress": 38,
		"options":  mustJSON(optionsPayload),
		"metrics":  mustJSON(metrics),
	}
	p.updateVideoJob(jobID, updates)
	_ = SyncPublicVideoImageJobUpdates(p.db, jobID, updates)

	metadata := map[string]interface{}{
		"flow_mode": "ai1_confirm",
		"status":    "awaiting_user_confirm",
	}
	for k, v := range extra {
		metadata[k] = v
	}
	p.appendJobEvent(jobID, models.VideoJobStageAwaitingAI1, "info", "ai1 waiting user confirmation", metadata)
	return true
}

func buildAIDirectorNaturalReply(directive *gifAIDirectiveProfile) string {
	if directive == nil {
		return ""
	}
	if text := strings.TrimSpace(directive.DirectiveText); text != "" {
		return text
	}

	parts := make([]string, 0, 6)
	if goal := strings.TrimSpace(directive.BusinessGoal); goal != "" {
		parts = append(parts, "目标："+goal)
	}
	if audience := strings.TrimSpace(directive.Audience); audience != "" {
		parts = append(parts, "受众："+audience)
	}
	if len(directive.MustCapture) > 0 {
		parts = append(parts, "重点："+strings.Join(directive.MustCapture, "、"))
	}
	if len(directive.Avoid) > 0 {
		parts = append(parts, "规避："+strings.Join(directive.Avoid, "、"))
	}
	if directive.ClipCountMin > 0 || directive.ClipCountMax > 0 {
		parts = append(parts, fmt.Sprintf("候选数量：%d~%d", directive.ClipCountMin, directive.ClipCountMax))
	}
	if style := strings.TrimSpace(directive.StyleDirection); style != "" {
		parts = append(parts, "风格："+style)
	}
	return strings.Join(parts, "；")
}

func resolveAI1PlanStatus(flowMode string, ai1Confirmed bool) string {
	if ai1Confirmed {
		return VideoJobAI1PlanStatusConfirmed
	}
	if normalizeVideoFlowMode(flowMode) == "ai1_confirm" {
		return VideoJobAI1PlanStatusAwaitingUser
	}
	return VideoJobAI1PlanStatusGenerated
}

func firstRequestedFormat(formats []string) string {
	for _, item := range formats {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		if value == "jpeg" {
			value = "jpg"
		}
		return value
	}
	return ""
}

func resolveVideoJobSourcePrompt(job models.VideoJob) string {
	options := parseJSONMap(job.Options)
	if prompt := strings.TrimSpace(stringFromAny(options["user_prompt"])); prompt != "" {
		return prompt
	}
	return strings.TrimSpace(job.Title)
}

func cloneMapStringKey(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func buildAI1PlanDirective(eventMeta map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{
		"business_goal",
		"audience",
		"must_capture",
		"avoid",
		"style_direction",
		"directive_text",
		"clip_count_min",
		"clip_count_max",
		"candidate_hints",
	} {
		value, ok := eventMeta[key]
		if !ok || value == nil {
			continue
		}
		if text, isText := value.(string); isText && strings.TrimSpace(text) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func buildAI1PlanDirectorSnapshot(snapshot map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, key := range []string{
		"provider",
		"model",
		"prompt_version",
		"status",
		"fallback_used",
		"applied",
		"target_format",
		"director_input_mode_applied",
		"director_input_source",
		"response_shape",
		"sample_frame_manifest_v1",
		"sample_frame_previews_v1",
		"error",
	} {
		value, ok := snapshot[key]
		if !ok || value == nil {
			continue
		}
		if text, isText := value.(string); isText && strings.TrimSpace(text) == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func buildAI1DetectedTags(eventMeta map[string]interface{}, requestedFormats []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	appendTag := func(raw string) {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			return
		}
		if !strings.HasPrefix(tag, "#") {
			tag = "#" + tag
		}
		if _, exists := seen[tag]; exists {
			return
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}

	appendTag(strings.ToUpper(firstRequestedFormat(requestedFormats)))
	appendTag(strings.TrimSpace(stringFromAny(eventMeta["business_goal"])))
	appendTag(strings.TrimSpace(stringFromAny(eventMeta["style_direction"])))

	for _, item := range stringSliceFromAny(eventMeta["must_capture"]) {
		appendTag(item)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func buildAI1IntentUnderstanding(eventMeta map[string]interface{}, requestedFormats []string, meta videoProbeMeta) string {
	parts := make([]string, 0, 5)
	if goal := strings.TrimSpace(stringFromAny(eventMeta["business_goal"])); goal != "" {
		parts = append(parts, "目标为 "+goal)
	}
	if audience := strings.TrimSpace(stringFromAny(eventMeta["audience"])); audience != "" {
		parts = append(parts, "受众侧重 "+audience)
	}
	if mustCapture := stringSliceFromAny(eventMeta["must_capture"]); len(mustCapture) > 0 {
		parts = append(parts, "重点抓取 "+strings.Join(mustCapture, "、"))
	}
	if avoid := stringSliceFromAny(eventMeta["avoid"]); len(avoid) > 0 {
		parts = append(parts, "规避 "+strings.Join(avoid, "、"))
	}
	if len(parts) > 0 {
		return strings.Join(parts, "；")
	}
	return strings.TrimSpace(buildGenericAI1Reply("", requestedFormats, meta))
}

func buildAI1StrategySummary(eventMeta map[string]interface{}, executablePlan map[string]interface{}) string {
	parts := make([]string, 0, 4)
	mode := strings.TrimSpace(stringFromAny(executablePlan["mode"]))
	if mode == "" {
		mode = strings.TrimSpace(stringFromAny(eventMeta["mode"]))
	}
	if mode != "" {
		parts = append(parts, "执行模式："+mode)
	}
	if focus := mapFromAny(executablePlan["focus_window"]); len(focus) > 0 {
		start := roundTo(floatFromAny(focus["start_sec"]), 3)
		end := roundTo(floatFromAny(focus["end_sec"]), 3)
		if end > start {
			parts = append(parts, fmt.Sprintf("重点窗口：%.3fs~%.3fs", start, end))
		}
	}
	if text := strings.TrimSpace(stringFromAny(eventMeta["focus_window_source"])); text != "" {
		parts = append(parts, "窗口来源："+text)
	}
	if minCount := intFromAny(eventMeta["clip_count_min"]); minCount > 0 {
		maxCount := intFromAny(eventMeta["clip_count_max"])
		if maxCount < minCount {
			maxCount = minCount
		}
		parts = append(parts, fmt.Sprintf("候选数量：%d~%d", minCount, maxCount))
	}
	if len(parts) == 0 {
		if text := strings.TrimSpace(stringFromAny(eventMeta["directive_text"])); text != "" {
			return text
		}
		return "已完成首轮意图解析，并将按可执行计划进入下一阶段。"
	}
	return strings.Join(parts, "；")
}

func buildAI1RiskWarning(eventMeta map[string]interface{}, meta videoProbeMeta) map[string]interface{} {
	hasRisk := false
	message := ""
	if errText := strings.TrimSpace(stringFromAny(eventMeta["error"])); errText != "" {
		hasRisk = true
		message = errText
	}
	if !hasRisk && meta.Width > 0 && meta.Height > 0 && meta.Width*meta.Height <= 640*360 {
		hasRisk = true
		message = "检测到输入分辨率较低，输出清晰度可能受限。"
	}
	return map[string]interface{}{
		"has_risk": hasRisk,
		"message":  strings.TrimSpace(message),
	}
}

func estimateAI1Confidence(
	eventMeta map[string]interface{},
	executablePlan map[string]interface{},
	requestedFormats []string,
	meta videoProbeMeta,
) float64 {
	confidence := 0.86
	if strings.TrimSpace(stringFromAny(eventMeta["error"])) != "" {
		confidence -= 0.42
	}
	if value, exists := eventMeta["director_applied"]; exists && !boolFromAny(value) {
		confidence -= 0.12
	}
	if strings.TrimSpace(stringFromAny(eventMeta["business_goal"])) == "" {
		confidence -= 0.10
	}
	if len(stringSliceFromAny(eventMeta["must_capture"])) == 0 {
		confidence -= 0.08
	}
	if len(stringSliceFromAny(eventMeta["avoid"])) == 0 {
		confidence -= 0.06
	}
	if meta.Width > 0 && meta.Height > 0 && meta.Width*meta.Height <= 640*360 {
		confidence -= 0.12
	}
	if meta.DurationSec >= 180 {
		confidence -= 0.06
	}
	if focus := mapFromAny(executablePlan["focus_window"]); len(focus) > 0 {
		if floatFromAny(focus["end_sec"]) > floatFromAny(focus["start_sec"]) {
			confidence += 0.05
		}
	}
	if score := floatFromAny(eventMeta["local_focus_score"]); score > 0 {
		confidence += clampFloat(score*0.05, 0.01, 0.06)
	}
	format := firstRequestedFormat(requestedFormats)
	if format == "png" || format == "jpg" || format == "webp" {
		confidence += 0.02
	}
	return roundTo(clampFloat(confidence, 0.05, 0.98), 3)
}

func buildAI1ClarifyQuestions(
	eventMeta map[string]interface{},
	executablePlan map[string]interface{},
	requestedFormats []string,
	meta videoProbeMeta,
	confidence float64,
	hasRisk bool,
) []string {
	out := make([]string, 0, 4)
	appendQuestion := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" || containsString(out, text) {
			return
		}
		out = append(out, text)
	}

	if strings.TrimSpace(stringFromAny(eventMeta["business_goal"])) == "" {
		appendQuestion("你更希望抓取哪类画面（例如进球瞬间、人物表情、特定动作）？")
	}
	if len(stringSliceFromAny(eventMeta["must_capture"])) == 0 {
		appendQuestion("是否有必须保留的主体、动作或关键词？")
	}
	if len(stringSliceFromAny(eventMeta["avoid"])) == 0 {
		appendQuestion("是否有明确要避开的内容（如黑场、模糊、水印）？")
	}
	if meta.DurationSec >= 120 && len(mapFromAny(executablePlan["focus_window"])) == 0 {
		appendQuestion("视频较长，是否限定一个优先处理的时间范围？")
	}
	if hasRisk && meta.Width > 0 && meta.Height > 0 && meta.Width*meta.Height <= 640*360 {
		appendQuestion("检测到清晰度较低，是否接受画质增强后继续输出？")
	}
	if confidence < ai1NeedClarifyConfidenceThreshold && len(out) == 0 {
		appendQuestion("当前意图理解置信度较低，是否补充更具体的处理要求？")
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return out
}

func estimateAI1ETASeconds(meta videoProbeMeta, requestedFormats []string, eventMeta map[string]interface{}) int {
	format := firstRequestedFormat(requestedFormats)
	base := 22.0
	switch format {
	case "gif":
		base = 38
	case "png", "jpg", "webp":
		base = 20
	}
	if meta.DurationSec > 0 {
		base += clampFloat(meta.DurationSec*0.35, 0, 70)
	}
	longSide := meta.Width
	if meta.Height > longSide {
		longSide = meta.Height
	}
	switch {
	case longSide >= 1440:
		base += 16
	case longSide >= 1080:
		base += 10
	case longSide >= 720:
		base += 5
	}
	if strings.TrimSpace(stringFromAny(eventMeta["error"])) != "" {
		base += 8
	}
	return int(clampFloat(base, 15, 180))
}

func buildAI1UserFeedbackV2(
	eventMeta map[string]interface{},
	requestedFormats []string,
	meta videoProbeMeta,
	executablePlan map[string]interface{},
) map[string]interface{} {
	summary := strings.TrimSpace(stringFromAny(eventMeta["ai_reply"]))
	if summary == "" {
		summary = strings.TrimSpace(buildGenericAI1Reply("", requestedFormats, meta))
	}
	riskWarning := buildAI1RiskWarning(eventMeta, meta)
	hasRisk := boolFromAny(riskWarning["has_risk"])
	confidence := estimateAI1Confidence(eventMeta, executablePlan, requestedFormats, meta)
	clarifyQuestions := buildAI1ClarifyQuestions(eventMeta, executablePlan, requestedFormats, meta, confidence, hasRisk)
	if len(clarifyQuestions) == 0 {
		clarifyQuestions = []string{}
	}
	interactiveAction := "proceed"
	if confidence < ai1NeedClarifyConfidenceThreshold || len(clarifyQuestions) > 0 || (hasRisk && strings.TrimSpace(stringFromAny(eventMeta["error"])) != "") {
		interactiveAction = "need_clarify"
	}

	out := map[string]interface{}{
		"schema_version":       AI1UserFeedbackSchemaV2,
		"summary":              summary,
		"intent_understanding": buildAI1IntentUnderstanding(eventMeta, requestedFormats, meta),
		"strategy_summary":     buildAI1StrategySummary(eventMeta, executablePlan),
		"detected_tags":        buildAI1DetectedTags(eventMeta, requestedFormats),
		"risk_warning":         riskWarning,
		"confidence":           confidence,
		"clarify_questions":    clarifyQuestions,
		"estimated_eta_seconds": estimateAI1ETASeconds(
			meta,
			requestedFormats,
			eventMeta,
		),
		"interactive_action": interactiveAction,
		"video_meta": map[string]interface{}{
			"duration_sec": roundTo(meta.DurationSec, 3),
			"width":        meta.Width,
			"height":       meta.Height,
			"fps":          roundTo(meta.FPS, 3),
		},
	}
	return out
}

func buildAI2DirectiveV2(
	schemaVersion string,
	requestedFormats []string,
	executablePlan map[string]interface{},
	eventMeta map[string]interface{},
) map[string]interface{} {
	targetFormat := strings.ToLower(strings.TrimSpace(stringFromAny(executablePlan["target_format"])))
	if targetFormat == "" {
		targetFormat = firstRequestedFormat(requestedFormats)
	}
	targetFormat = NormalizeRequestedFormat(targetFormat)
	objective := strings.TrimSpace(stringFromAny(eventMeta["business_goal"]))
	if objective == "" {
		switch targetFormat {
		case "gif":
			objective = "extract_high_value_clips"
		default:
			objective = "extract_high_quality_frames"
		}
	}

	mustCapture := stringSliceFromAny(executablePlan["must_capture"])
	if len(mustCapture) == 0 {
		mustCapture = stringSliceFromAny(eventMeta["must_capture"])
	}
	avoid := stringSliceFromAny(executablePlan["avoid"])
	if len(avoid) == 0 {
		avoid = stringSliceFromAny(eventMeta["avoid"])
	}
	styleDirection := strings.TrimSpace(stringFromAny(executablePlan["style_direction"]))
	if styleDirection == "" {
		styleDirection = strings.TrimSpace(stringFromAny(eventMeta["style_direction"]))
	}

	visualFocusArea := "auto"
	if focus := mapFromAny(executablePlan["focus_window"]); len(focus) > 0 {
		if floatFromAny(focus["end_sec"]) > floatFromAny(focus["start_sec"]) {
			visualFocusArea = "center"
		}
	}

	subjectTracking := "auto"
	for _, item := range mustCapture {
		text := strings.ToLower(strings.TrimSpace(item))
		if strings.Contains(text, "人物") || strings.Contains(text, "主角") || strings.Contains(text, "person") || strings.Contains(text, "subject") {
			subjectTracking = "primary_subject_auto"
			break
		}
	}

	maxBlurTolerance := "medium"
	switch targetFormat {
	case "png", "jpg", "webp":
		maxBlurTolerance = "low"
	}

	rhythmTrajectory := "start_peak_fade"
	if targetFormat == "gif" || targetFormat == "live" {
		rhythmTrajectory = "loop"
	}

	out := map[string]interface{}{
		"schema_version":      AI2DirectiveSchemaV2,
		"instruction_version": strings.TrimSpace(schemaVersion),
		"target_format":       targetFormat,
		"objective":           objective,
		"sampling_plan":       cloneMapStringKey(executablePlan),
		"must_capture":        mustCapture,
		"avoid":               avoid,
		"style_direction":     styleDirection,
		"visual_focus_area":   visualFocusArea,
		"subject_tracking":    subjectTracking,
		"technical_reject": map[string]interface{}{
			"max_blur_tolerance": maxBlurTolerance,
			"avoid_watermarks":   true,
			"avoid_extreme_dark": true,
		},
		"rhythm_trajectory": rhythmTrajectory,
		"source":            "ai1_executable_plan",
	}
	out["quality_weights"] = normalizeDirectiveQualityWeights(map[string]float64{})
	if len(executablePlan) == 0 {
		out["source"] = "ai1_directive"
	}
	if qualityWeights := normalizeAI2QualityWeightsAny(eventMeta["quality_weights"]); len(qualityWeights) > 0 {
		out["quality_weights"] = qualityWeights
	}
	if riskFlags := normalizeAI2RiskFlags(stringSliceFromAny(eventMeta["risk_flags"])); len(riskFlags) > 0 {
		out["risk_flags"] = riskFlags
	}
	if clipMin := intFromAny(eventMeta["clip_count_min"]); clipMin > 0 {
		out["clip_count_min"] = clipMin
	}
	if clipMax := intFromAny(eventMeta["clip_count_max"]); clipMax > 0 {
		out["clip_count_max"] = clipMax
	}
	if text := strings.TrimSpace(stringFromAny(eventMeta["directive_text"])); text != "" {
		out["planner_instruction_text"] = text
	}
	return out
}

func buildAI1OutputV2(
	schemaVersion string,
	requestedFormats []string,
	meta videoProbeMeta,
	eventMeta map[string]interface{},
	executablePlan map[string]interface{},
	trace map[string]interface{},
) map[string]interface{} {
	userFeedback := buildAI1UserFeedbackV2(eventMeta, requestedFormats, meta, executablePlan)
	ai2Directive := buildAI2DirectiveV2(schemaVersion, requestedFormats, executablePlan, eventMeta)
	return map[string]interface{}{
		"schema_version": AI1OutputSchemaV2,
		"user_feedback":  userFeedback,
		"ai2_directive":  ai2Directive,
		"trace":          cloneMapStringKey(trace),
	}
}

func appendRepairItem(items []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return items
	}
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}

func normalizeAI1InteractiveAction(raw string, hasSystemError bool) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "proceed", "need_clarify":
		return value
	}
	if hasSystemError {
		return "need_clarify"
	}
	return "proceed"
}

func normalizeAI1VisualFocusArea(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "auto", "center", "lower_third", "upper_half", "full_frame":
		return value
	default:
		return "auto"
	}
}

func normalizeAI1RhythmTrajectory(raw string, targetFormat string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "loop", "sudden_impact", "start_peak_fade":
		return value
	}
	target := NormalizeRequestedFormat(targetFormat)
	if target == "gif" || target == "live" {
		return "loop"
	}
	return "start_peak_fade"
}

func normalizeAI1MaxBlurTolerance(raw string, targetFormat string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "low", "medium", "high":
		return value
	}
	target := NormalizeRequestedFormat(targetFormat)
	switch target {
	case "png", "jpg", "webp":
		return "low"
	default:
		return "medium"
	}
}

func validateAndRepairAI1UserFeedbackV2(
	raw map[string]interface{},
	eventMeta map[string]interface{},
	requestedFormats []string,
	meta videoProbeMeta,
	executablePlan map[string]interface{},
) (map[string]interface{}, []string) {
	repairs := make([]string, 0, 8)
	defaultValue := buildAI1UserFeedbackV2(eventMeta, requestedFormats, meta, executablePlan)
	if len(raw) == 0 {
		return cloneMapStringKey(defaultValue), appendRepairItem(repairs, "user_feedback.rebuild")
	}
	out := cloneMapStringKey(raw)

	if strings.TrimSpace(stringFromAny(out["schema_version"])) != AI1UserFeedbackSchemaV2 {
		out["schema_version"] = AI1UserFeedbackSchemaV2
		repairs = appendRepairItem(repairs, "user_feedback.schema_version")
	}
	for _, key := range []string{"summary", "intent_understanding", "strategy_summary"} {
		if strings.TrimSpace(stringFromAny(out[key])) == "" {
			out[key] = strings.TrimSpace(stringFromAny(defaultValue[key]))
			repairs = appendRepairItem(repairs, "user_feedback."+key)
		}
	}

	riskWarning := mapFromAny(out["risk_warning"])
	if len(riskWarning) == 0 {
		riskWarning = cloneMapStringKey(mapFromAny(defaultValue["risk_warning"]))
		repairs = appendRepairItem(repairs, "user_feedback.risk_warning")
	}
	if _, ok := riskWarning["has_risk"]; !ok {
		riskWarning["has_risk"] = boolFromAny(mapFromAny(defaultValue["risk_warning"])["has_risk"])
		repairs = appendRepairItem(repairs, "user_feedback.risk_warning.has_risk")
	} else {
		riskWarning["has_risk"] = boolFromAny(riskWarning["has_risk"])
	}
	if _, ok := riskWarning["message"]; !ok {
		riskWarning["message"] = strings.TrimSpace(stringFromAny(mapFromAny(defaultValue["risk_warning"])["message"]))
		repairs = appendRepairItem(repairs, "user_feedback.risk_warning.message")
	} else {
		riskWarning["message"] = strings.TrimSpace(stringFromAny(riskWarning["message"]))
	}
	out["risk_warning"] = riskWarning

	defaultConfidence := floatFromAny(defaultValue["confidence"])
	if defaultConfidence <= 0 {
		defaultConfidence = 0.8
	}
	confidence := floatFromAny(out["confidence"])
	if confidence <= 0 {
		confidence = defaultConfidence
		repairs = appendRepairItem(repairs, "user_feedback.confidence")
	}
	clampedConfidence := roundTo(clampFloat(confidence, 0, 1), 3)
	if roundTo(confidence, 3) != clampedConfidence {
		repairs = appendRepairItem(repairs, "user_feedback.confidence.clamped")
	}
	out["confidence"] = clampedConfidence

	clarifyQuestions := normalizeStringSlice(stringSliceFromAny(out["clarify_questions"]), 4)
	if _, exists := out["clarify_questions"]; !exists {
		repairs = appendRepairItem(repairs, "user_feedback.clarify_questions")
	}
	if len(clarifyQuestions) == 0 {
		clarifyQuestions = buildAI1ClarifyQuestions(
			eventMeta,
			executablePlan,
			requestedFormats,
			meta,
			clampedConfidence,
			boolFromAny(riskWarning["has_risk"]),
		)
		if len(clarifyQuestions) > 0 {
			repairs = appendRepairItem(repairs, "user_feedback.clarify_questions.auto_generated")
		}
	}

	defaultETA := intFromAny(defaultValue["estimated_eta_seconds"])
	eta := intFromAny(out["estimated_eta_seconds"])
	if eta <= 0 {
		eta = defaultETA
		repairs = appendRepairItem(repairs, "user_feedback.estimated_eta_seconds")
	}
	clampedETA := int(clampFloat(float64(eta), 15, 240))
	if clampedETA != eta {
		repairs = appendRepairItem(repairs, "user_feedback.estimated_eta_seconds.clamped")
	}
	out["estimated_eta_seconds"] = clampedETA

	hasSystemError := strings.TrimSpace(stringFromAny(eventMeta["error"])) != ""
	action := normalizeAI1InteractiveAction(stringFromAny(out["interactive_action"]), hasSystemError)
	shouldNeedClarify := hasSystemError || clampedConfidence < ai1NeedClarifyConfidenceThreshold || len(clarifyQuestions) > 0
	if shouldNeedClarify && action != "need_clarify" {
		action = "need_clarify"
		repairs = appendRepairItem(repairs, "user_feedback.interactive_action")
	} else if !shouldNeedClarify && action != "proceed" {
		action = "proceed"
		repairs = appendRepairItem(repairs, "user_feedback.interactive_action")
	} else if action != strings.ToLower(strings.TrimSpace(stringFromAny(out["interactive_action"]))) {
		repairs = appendRepairItem(repairs, "user_feedback.interactive_action")
	}
	out["interactive_action"] = action
	if action == "need_clarify" && len(clarifyQuestions) == 0 {
		clarifyQuestions = []string{"请补充你希望优先抓取的关键画面或动作。"}
		repairs = appendRepairItem(repairs, "user_feedback.clarify_questions.need_clarify_default")
	}
	if len(clarifyQuestions) == 0 {
		clarifyQuestions = []string{}
	}
	out["clarify_questions"] = clarifyQuestions

	if len(stringSliceFromAny(out["detected_tags"])) == 0 {
		out["detected_tags"] = buildAI1DetectedTags(eventMeta, requestedFormats)
		repairs = appendRepairItem(repairs, "user_feedback.detected_tags")
	}

	videoMeta := mapFromAny(out["video_meta"])
	if len(videoMeta) == 0 {
		videoMeta = cloneMapStringKey(mapFromAny(defaultValue["video_meta"]))
		repairs = appendRepairItem(repairs, "user_feedback.video_meta")
	}
	if intFromAny(videoMeta["width"]) <= 0 && meta.Width > 0 {
		videoMeta["width"] = meta.Width
		repairs = appendRepairItem(repairs, "user_feedback.video_meta.width")
	}
	if intFromAny(videoMeta["height"]) <= 0 && meta.Height > 0 {
		videoMeta["height"] = meta.Height
		repairs = appendRepairItem(repairs, "user_feedback.video_meta.height")
	}
	if floatFromAny(videoMeta["duration_sec"]) <= 0 && meta.DurationSec > 0 {
		videoMeta["duration_sec"] = roundTo(meta.DurationSec, 3)
		repairs = appendRepairItem(repairs, "user_feedback.video_meta.duration_sec")
	}
	out["video_meta"] = videoMeta

	return out, repairs
}

func validateAndRepairAI2DirectiveV2(
	raw map[string]interface{},
	schemaVersion string,
	requestedFormats []string,
	executablePlan map[string]interface{},
	eventMeta map[string]interface{},
) (map[string]interface{}, []string) {
	repairs := make([]string, 0, 8)
	defaultValue := buildAI2DirectiveV2(schemaVersion, requestedFormats, executablePlan, eventMeta)
	if len(raw) == 0 {
		return cloneMapStringKey(defaultValue), appendRepairItem(repairs, "ai2_directive.rebuild")
	}
	out := cloneMapStringKey(raw)

	if strings.TrimSpace(stringFromAny(out["schema_version"])) != AI2DirectiveSchemaV2 {
		out["schema_version"] = AI2DirectiveSchemaV2
		repairs = appendRepairItem(repairs, "ai2_directive.schema_version")
	}

	if strings.TrimSpace(stringFromAny(out["instruction_version"])) == "" {
		out["instruction_version"] = strings.TrimSpace(stringFromAny(defaultValue["instruction_version"]))
		repairs = appendRepairItem(repairs, "ai2_directive.instruction_version")
	}

	targetFormat := NormalizeRequestedFormat(strings.TrimSpace(stringFromAny(out["target_format"])))
	if targetFormat == "" {
		targetFormat = NormalizeRequestedFormat(strings.TrimSpace(stringFromAny(defaultValue["target_format"])))
	}
	if targetFormat == "" {
		targetFormat = NormalizeRequestedFormat(firstRequestedFormat(requestedFormats))
	}
	if targetFormat == "" {
		targetFormat = "png"
	}
	if targetFormat != strings.ToLower(strings.TrimSpace(stringFromAny(out["target_format"]))) {
		repairs = appendRepairItem(repairs, "ai2_directive.target_format")
	}
	out["target_format"] = targetFormat

	if strings.TrimSpace(stringFromAny(out["objective"])) == "" {
		out["objective"] = strings.TrimSpace(stringFromAny(defaultValue["objective"]))
		repairs = appendRepairItem(repairs, "ai2_directive.objective")
	}

	samplingPlan := mapFromAny(out["sampling_plan"])
	if len(samplingPlan) == 0 {
		samplingPlan = cloneMapStringKey(executablePlan)
		if len(samplingPlan) == 0 {
			samplingPlan = cloneMapStringKey(mapFromAny(defaultValue["sampling_plan"]))
		}
		repairs = appendRepairItem(repairs, "ai2_directive.sampling_plan")
	}
	out["sampling_plan"] = samplingPlan

	visualFocusArea := normalizeAI1VisualFocusArea(stringFromAny(out["visual_focus_area"]))
	if visualFocusArea != strings.ToLower(strings.TrimSpace(stringFromAny(out["visual_focus_area"]))) {
		repairs = appendRepairItem(repairs, "ai2_directive.visual_focus_area")
	}
	out["visual_focus_area"] = visualFocusArea

	if strings.TrimSpace(stringFromAny(out["subject_tracking"])) == "" {
		out["subject_tracking"] = strings.TrimSpace(stringFromAny(defaultValue["subject_tracking"]))
		repairs = appendRepairItem(repairs, "ai2_directive.subject_tracking")
	}

	technicalReject := mapFromAny(out["technical_reject"])
	if len(technicalReject) == 0 {
		technicalReject = cloneMapStringKey(mapFromAny(defaultValue["technical_reject"]))
		repairs = appendRepairItem(repairs, "ai2_directive.technical_reject")
	}
	blurTolerance := normalizeAI1MaxBlurTolerance(stringFromAny(technicalReject["max_blur_tolerance"]), targetFormat)
	if blurTolerance != strings.ToLower(strings.TrimSpace(stringFromAny(technicalReject["max_blur_tolerance"]))) {
		repairs = appendRepairItem(repairs, "ai2_directive.technical_reject.max_blur_tolerance")
	}
	technicalReject["max_blur_tolerance"] = blurTolerance
	if _, ok := technicalReject["avoid_watermarks"]; !ok {
		technicalReject["avoid_watermarks"] = true
		repairs = appendRepairItem(repairs, "ai2_directive.technical_reject.avoid_watermarks")
	} else {
		technicalReject["avoid_watermarks"] = boolFromAny(technicalReject["avoid_watermarks"])
	}
	if _, ok := technicalReject["avoid_extreme_dark"]; !ok {
		technicalReject["avoid_extreme_dark"] = true
		repairs = appendRepairItem(repairs, "ai2_directive.technical_reject.avoid_extreme_dark")
	} else {
		technicalReject["avoid_extreme_dark"] = boolFromAny(technicalReject["avoid_extreme_dark"])
	}
	out["technical_reject"] = technicalReject

	qualityWeights := normalizeAI2QualityWeightsAny(out["quality_weights"])
	if len(qualityWeights) == 0 {
		qualityWeights = normalizeAI2QualityWeightsAny(defaultValue["quality_weights"])
		repairs = appendRepairItem(repairs, "ai2_directive.quality_weights")
	}
	if len(qualityWeights) == 0 {
		qualityWeights = normalizeDirectiveQualityWeights(map[string]float64{})
		repairs = appendRepairItem(repairs, "ai2_directive.quality_weights.defaulted")
	}
	out["quality_weights"] = qualityWeights

	riskFlags := normalizeAI2RiskFlags(stringSliceFromAny(out["risk_flags"]))
	defaultRiskFlags := normalizeAI2RiskFlags(stringSliceFromAny(defaultValue["risk_flags"]))
	if len(riskFlags) == 0 && len(defaultRiskFlags) > 0 {
		riskFlags = defaultRiskFlags
		repairs = appendRepairItem(repairs, "ai2_directive.risk_flags")
	}
	if len(riskFlags) > 0 {
		out["risk_flags"] = riskFlags
	}

	rhythm := normalizeAI1RhythmTrajectory(stringFromAny(out["rhythm_trajectory"]), targetFormat)
	if rhythm != strings.ToLower(strings.TrimSpace(stringFromAny(out["rhythm_trajectory"]))) {
		repairs = appendRepairItem(repairs, "ai2_directive.rhythm_trajectory")
	}
	out["rhythm_trajectory"] = rhythm

	if strings.TrimSpace(stringFromAny(out["source"])) == "" {
		out["source"] = strings.TrimSpace(stringFromAny(defaultValue["source"]))
		repairs = appendRepairItem(repairs, "ai2_directive.source")
	}
	return out, repairs
}

func hasRequiredAI1UserFeedbackV2(raw map[string]interface{}) bool {
	if len(raw) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromAny(raw["summary"])) == "" {
		return false
	}
	if strings.TrimSpace(stringFromAny(raw["intent_understanding"])) == "" {
		return false
	}
	if strings.TrimSpace(stringFromAny(raw["strategy_summary"])) == "" {
		return false
	}
	action := normalizeAI1InteractiveAction(stringFromAny(raw["interactive_action"]), false)
	if action != strings.ToLower(strings.TrimSpace(stringFromAny(raw["interactive_action"]))) {
		return false
	}
	risk := mapFromAny(raw["risk_warning"])
	if len(risk) == 0 {
		return false
	}
	if _, ok := risk["has_risk"]; !ok {
		return false
	}
	if _, exists := raw["confidence"]; !exists {
		return false
	}
	confidence := floatFromAny(raw["confidence"])
	if confidence < 0 || confidence > 1 {
		return false
	}
	questions := stringSliceFromAny(raw["clarify_questions"])
	if _, exists := raw["clarify_questions"]; !exists {
		return false
	}
	if action == "need_clarify" && len(questions) == 0 {
		return false
	}
	return true
}

func hasRequiredAI2DirectiveV2(raw map[string]interface{}) bool {
	if len(raw) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromAny(raw["schema_version"])) != AI2DirectiveSchemaV2 {
		return false
	}
	if strings.TrimSpace(stringFromAny(raw["target_format"])) == "" {
		return false
	}
	if strings.TrimSpace(stringFromAny(raw["objective"])) == "" {
		return false
	}
	if len(mapFromAny(raw["sampling_plan"])) == 0 {
		return false
	}
	technicalReject := mapFromAny(raw["technical_reject"])
	if len(technicalReject) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromAny(technicalReject["max_blur_tolerance"])) == "" {
		return false
	}
	if len(normalizeAI2QualityWeightsAny(raw["quality_weights"])) == 0 {
		return false
	}
	return true
}

func hasRequiredAI1OutputV2(raw map[string]interface{}) bool {
	if len(raw) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromAny(raw["schema_version"])) != AI1OutputSchemaV2 {
		return false
	}
	if !hasRequiredAI1UserFeedbackV2(mapFromAny(raw["user_feedback"])) {
		return false
	}
	if !hasRequiredAI2DirectiveV2(mapFromAny(raw["ai2_directive"])) {
		return false
	}
	if len(mapFromAny(raw["trace"])) == 0 {
		return false
	}
	return true
}

func buildSafeAI1OutputV2(
	schemaVersion string,
	requestedFormats []string,
	meta videoProbeMeta,
	eventMeta map[string]interface{},
	executablePlan map[string]interface{},
	baseTrace map[string]interface{},
	reason string,
	repairItems []string,
) map[string]interface{} {
	safe := buildAI1OutputV2(schemaVersion, requestedFormats, meta, eventMeta, executablePlan, baseTrace)
	userFeedback, userRepairs := validateAndRepairAI1UserFeedbackV2(
		mapFromAny(safe["user_feedback"]),
		eventMeta,
		requestedFormats,
		meta,
		executablePlan,
	)
	ai2Directive, directiveRepairs := validateAndRepairAI2DirectiveV2(
		mapFromAny(safe["ai2_directive"]),
		schemaVersion,
		requestedFormats,
		executablePlan,
		eventMeta,
	)
	for _, item := range userRepairs {
		repairItems = appendRepairItem(repairItems, item)
	}
	for _, item := range directiveRepairs {
		repairItems = appendRepairItem(repairItems, item)
	}
	repairItems = appendRepairItem(repairItems, "fallback.safe_envelope")

	trace := mapFromAny(safe["trace"])
	if len(trace) == 0 {
		trace = cloneMapStringKey(baseTrace)
	}
	if len(trace) == 0 {
		trace = map[string]interface{}{}
	}
	trace["contract_repaired"] = true
	trace["repair_items"] = repairItems
	if strings.TrimSpace(reason) != "" {
		trace["repair_reason"] = strings.TrimSpace(reason)
	}

	safe["schema_version"] = AI1OutputSchemaV2
	safe["user_feedback"] = userFeedback
	safe["ai2_directive"] = ai2Directive
	safe["trace"] = trace
	return safe
}

func validateAndRepairAI1OutputV2(
	raw map[string]interface{},
	schemaVersion string,
	requestedFormats []string,
	meta videoProbeMeta,
	eventMeta map[string]interface{},
	executablePlan map[string]interface{},
	baseTrace map[string]interface{},
) map[string]interface{} {
	if len(raw) == 0 {
		return buildSafeAI1OutputV2(
			schemaVersion,
			requestedFormats,
			meta,
			eventMeta,
			executablePlan,
			baseTrace,
			"empty_output",
			nil,
		)
	}

	out := cloneMapStringKey(raw)
	repairs := make([]string, 0, 8)

	if strings.TrimSpace(stringFromAny(out["schema_version"])) != AI1OutputSchemaV2 {
		out["schema_version"] = AI1OutputSchemaV2
		repairs = appendRepairItem(repairs, "schema_version")
	}

	userFeedback, userRepairs := validateAndRepairAI1UserFeedbackV2(
		mapFromAny(out["user_feedback"]),
		eventMeta,
		requestedFormats,
		meta,
		executablePlan,
	)
	for _, item := range userRepairs {
		repairs = appendRepairItem(repairs, item)
	}
	out["user_feedback"] = userFeedback

	ai2Directive, directiveRepairs := validateAndRepairAI2DirectiveV2(
		mapFromAny(out["ai2_directive"]),
		schemaVersion,
		requestedFormats,
		executablePlan,
		eventMeta,
	)
	for _, item := range directiveRepairs {
		repairs = appendRepairItem(repairs, item)
	}
	out["ai2_directive"] = ai2Directive

	trace := mapFromAny(out["trace"])
	if len(trace) == 0 {
		trace = cloneMapStringKey(baseTrace)
		repairs = appendRepairItem(repairs, "trace.rebuild")
	}
	for _, key := range []string{"event_stage", "flow_mode", "sub_stage", "mode", "error", "director_input_mode", "director_input_source"} {
		if strings.TrimSpace(stringFromAny(trace[key])) != "" {
			continue
		}
		value := strings.TrimSpace(stringFromAny(baseTrace[key]))
		if value == "" {
			continue
		}
		trace[key] = value
		repairs = appendRepairItem(repairs, "trace."+key)
	}
	out["trace"] = trace

	if !hasRequiredAI1OutputV2(out) {
		return buildSafeAI1OutputV2(
			schemaVersion,
			requestedFormats,
			meta,
			eventMeta,
			executablePlan,
			baseTrace,
			"required_fields_missing_after_repair",
			repairs,
		)
	}

	if len(repairs) > 0 {
		trace["contract_repaired"] = true
		trace["repair_items"] = repairs
		out["trace"] = trace
	}
	return out
}

func (p *Processor) persistAI1Plan(
	job models.VideoJob,
	requestedFormats []string,
	flowMode string,
	ai1Confirmed bool,
	meta videoProbeMeta,
	eventStage string,
	eventMeta map[string]interface{},
	directorSnapshot map[string]interface{},
) {
	if p == nil || p.db == nil || job.ID == 0 {
		return
	}
	requestedFormat := firstRequestedFormat(requestedFormats)
	sourcePrompt := resolveVideoJobSourcePrompt(job)
	aiReply := strings.TrimSpace(stringFromAny(eventMeta["ai_reply"]))

	provider := strings.TrimSpace(stringFromAny(directorSnapshot["provider"]))
	model := strings.TrimSpace(stringFromAny(directorSnapshot["model"]))
	promptVersion := strings.TrimSpace(stringFromAny(directorSnapshot["prompt_version"]))
	fallbackUsed := boolFromAny(directorSnapshot["fallback_used"]) || strings.TrimSpace(stringFromAny(eventMeta["error"])) != ""

	trace := map[string]interface{}{
		"event_stage": strings.TrimSpace(eventStage),
		"flow_mode":   normalizeVideoFlowMode(flowMode),
		"sub_stage":   strings.TrimSpace(stringFromAny(eventMeta["sub_stage"])),
		"mode":        strings.TrimSpace(stringFromAny(eventMeta["mode"])),
		"error":       strings.TrimSpace(stringFromAny(eventMeta["error"])),
	}
	if inputMode := strings.TrimSpace(stringFromAny(directorSnapshot["director_input_mode_applied"])); inputMode != "" {
		trace["director_input_mode"] = inputMode
	}
	if inputSource := strings.TrimSpace(stringFromAny(directorSnapshot["director_input_source"])); inputSource != "" {
		trace["director_input_source"] = inputSource
	}

	ai1OutputV2 := buildAI1OutputV2(
		VideoJobAI1PlanSchemaV1,
		requestedFormats,
		meta,
		eventMeta,
		map[string]interface{}{},
		trace,
	)
	ai1OutputV2 = validateAndRepairAI1OutputV2(
		ai1OutputV2,
		VideoJobAI1PlanSchemaV1,
		requestedFormats,
		meta,
		eventMeta,
		map[string]interface{}{},
		trace,
	)
	traceV2 := mapFromAny(ai1OutputV2["trace"])
	if len(traceV2) > 0 {
		trace = traceV2
	}
	userFeedbackV2 := mapFromAny(ai1OutputV2["user_feedback"])
	ai2DirectiveV2 := mapFromAny(ai1OutputV2["ai2_directive"])

	planPayload := map[string]interface{}{
		"plan_revision":     1,
		"schema_version":    VideoJobAI1PlanSchemaV1,
		"requested_format":  requestedFormat,
		"requested_formats": requestedFormats,
		"flow_mode":         normalizeVideoFlowMode(flowMode),
		"source_prompt":     sourcePrompt,
		"ai_reply":          aiReply,
		"source_meta": map[string]interface{}{
			"duration_sec": roundTo(meta.DurationSec, 3),
			"width":        meta.Width,
			"height":       meta.Height,
			"fps":          roundTo(meta.FPS, 3),
		},
		"directive":         buildAI1PlanDirective(eventMeta),
		"event_meta":        cloneMapStringKey(eventMeta),
		"trace":             trace,
		"director_snapshot": buildAI1PlanDirectorSnapshot(directorSnapshot),
		"ai1_output_v2":     ai1OutputV2,
		"ai1_output_v1": map[string]interface{}{
			"schema_version":  VideoJobAI1PlanSchemaV1,
			"user_reply":      userFeedbackV2,
			"ai2_instruction": ai2DirectiveV2,
		},
	}

	status := resolveAI1PlanStatus(flowMode, ai1Confirmed)
	row := models.VideoJobAI1Plan{
		JobID:           job.ID,
		UserID:          job.UserID,
		RequestedFormat: requestedFormat,
		SchemaVersion:   VideoJobAI1PlanSchemaV1,
		Status:          status,
		SourcePrompt:    sourcePrompt,
		PlanJSON:        mustJSON(planPayload),
		ModelProvider:   provider,
		ModelName:       model,
		PromptVersion:   promptVersion,
		FallbackUsed:    fallbackUsed,
		ConfirmedByUser: ai1Confirmed,
	}
	if ai1Confirmed {
		now := time.Now()
		row.ConfirmedAt = &now
	}
	if err := UpsertVideoJobAI1Plan(p.db, row); err != nil {
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "ai1 plan persistence failed", map[string]interface{}{
			"error": err.Error(),
		})
	}
}

func (p *Processor) persistSourceReadability(jobID uint64, currentMetrics []byte, readability map[string]interface{}) {
	if p == nil || p.db == nil || jobID == 0 || len(readability) == 0 {
		return
	}
	metrics := parseJSONMap(currentMetrics)
	metrics["source_video_readability_v1"] = readability
	p.updateVideoJob(jobID, map[string]interface{}{
		"metrics": mustJSON(metrics),
	})
}

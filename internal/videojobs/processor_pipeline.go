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

func (p *Processor) process(ctx context.Context, jobID uint64) error {
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

	now := time.Now()
	acquired, err := p.acquireVideoJobRun(job.ID, now)
	if err != nil {
		return fmt.Errorf("acquire video job run: %w", err)
	}
	if !acquired {
		p.appendJobEvent(job.ID, models.VideoJobStageQueued, "info", "skip duplicated processing trigger", nil)
		return nil
	}
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
	sourceReadability, err := p.downloadObjectByKeyWithReadability(ctx, job.SourceVideoKey, sourcePath)
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
	optionsPayload := parseJSONMap(job.Options)
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
				p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "ai director prompt pack generated", map[string]interface{}{
					"business_goal":  directorSnapshot["business_goal"],
					"clip_count_min": directorSnapshot["clip_count_min"],
					"clip_count_max": directorSnapshot["clip_count_max"],
					"sub_stage":      gifSubStageBriefing,
					"mode":           pipelineMode,
				})
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

			if p.shouldUseCloudHighlightFallback(suggestion) {
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
		if err := p.persistGIFHighlightCandidates(ctx, sourcePath, meta, job.ID, *highlightPlan, qualitySettings); err != nil {
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
	if containsString(generatedFormats, "gif") && pipelineModeDecision.EnableAI3 {
		reviewingStarted := markGIFSubStageRunning(gifSubStageReviewing, map[string]interface{}{
			"entry_stage": models.VideoJobStageUploading,
		})
		p.appendJobEvent(job.ID, models.VideoJobStageUploading, "info", "gif sub-stage reviewing started", map[string]interface{}{
			"sub_stage": gifSubStageReviewing,
			"status":    "running",
		})
		judgeSnapshot, judgeErr := p.runAIGIFJudgeReview(ctx, job, qualitySettings)
		if judgeErr != nil {
			metrics["gif_ai_judge_v1"] = map[string]interface{}{
				"enabled":        judgeSnapshot["enabled"],
				"provider":       judgeSnapshot["provider"],
				"model":          judgeSnapshot["model"],
				"prompt_version": judgeSnapshot["prompt_version"],
				"applied":        false,
				"error":          judgeErr.Error(),
			}
			markGIFSubStageDone(gifSubStageReviewing, reviewingStarted, "degraded", map[string]interface{}{
				"error": judgeErr.Error(),
			})
			p.appendJobEvent(job.ID, models.VideoJobStageUploading, "warn", "gif ai judge failed", map[string]interface{}{
				"error":     judgeErr.Error(),
				"sub_stage": gifSubStageReviewing,
			})
		} else {
			metrics["gif_ai_judge_v1"] = judgeSnapshot
			markGIFSubStageDone(gifSubStageReviewing, reviewingStarted, "done", map[string]interface{}{
				"applied": true,
			})
			judgeEvent := map[string]interface{}{"sub_stage": gifSubStageReviewing}
			for k, v := range judgeSnapshot {
				judgeEvent[k] = v
			}
			p.appendJobEvent(job.ID, models.VideoJobStageUploading, "info", "gif ai judge completed", judgeEvent)
		}
	} else if containsString(generatedFormats, "gif") {
		markGIFSubStageSkipped(gifSubStageReviewing, "pipeline_mode_"+pipelineMode+"_judge_skipped")
		metrics["gif_ai_judge_v1"] = map[string]interface{}{
			"enabled": false,
			"applied": false,
			"mode":    pipelineMode,
			"reason":  "pipeline_mode_judge_skipped",
		}
		p.appendJobEvent(job.ID, models.VideoJobStageUploading, "info", "gif sub-stage reviewing skipped by pipeline mode", map[string]interface{}{
			"sub_stage": gifSubStageReviewing,
			"mode":      pipelineMode,
		})
	} else {
		markGIFSubStageSkipped(gifSubStageReviewing, "gif_not_generated")
	}
	if containsString(generatedFormats, "gif") {
		judgeMetric := mapFromAny(metrics["gif_ai_judge_v1"])
		shouldAttemptDeliverFallback := pipelineModeDecision.EnableAI3 && boolFromAny(judgeMetric["applied"])
		if shouldAttemptDeliverFallback {
			triggerReason := "judge_no_deliver"
			fallbackContext := map[string]interface{}{
				"pipeline_mode": pipelineMode,
				"ai3_enabled":   pipelineModeDecision.EnableAI3,
				"ai3_applied":   boolFromAny(judgeMetric["applied"]),
				"ai3_reason":    stringFromAny(judgeMetric["reason"]),
				"ai3_error":     stringFromAny(judgeMetric["error"]),
			}
			deliverFallback, fallbackErr := p.ensureAIGIFDeliverFallback(job, triggerReason, fallbackContext)
			if fallbackErr != nil {
				deliverFallback["error"] = fallbackErr.Error()
				p.appendJobEvent(job.ID, models.VideoJobStageUploading, "warn", "gif deliver fallback failed", map[string]interface{}{
					"error": fallbackErr.Error(),
				})
			}
			metrics["gif_deliver_fallback_v1"] = deliverFallback
			if len(judgeMetric) > 0 {
				judgeMetric["deliver_fallback_applied"] = boolFromAny(deliverFallback["applied"])
				judgeMetric["deliver_fallback_reason"] = stringFromAny(deliverFallback["reason"])
				judgeMetric["deliver_fallback_trigger_reason"] = stringFromAny(deliverFallback["trigger_reason"])
				if outputID := intFromAny(deliverFallback["selected_output_id"]); outputID > 0 {
					judgeMetric["deliver_fallback_output_id"] = outputID
				}
				metrics["gif_ai_judge_v1"] = judgeMetric
			}
			if boolFromAny(deliverFallback["applied"]) {
				p.appendJobEvent(job.ID, models.VideoJobStageUploading, "info", "gif deliver fallback applied", map[string]interface{}{
					"output_id":               intFromAny(deliverFallback["selected_output_id"]),
					"reason":                  stringFromAny(deliverFallback["reason"]),
					"trigger_reason":          stringFromAny(deliverFallback["trigger_reason"]),
					"previous_recommendation": stringFromAny(deliverFallback["previous_recommendation"]),
				})
			}
		} else {
			reason := "fallback_blocked_without_ai3_review"
			switch {
			case !pipelineModeDecision.EnableAI3:
				reason = "fallback_blocked_pipeline_mode_" + pipelineMode
			case !boolFromAny(judgeMetric["applied"]):
				reason = "fallback_blocked_ai3_unavailable"
			}
			deliverFallback := map[string]interface{}{
				"attempted":      false,
				"applied":        false,
				"reason":         reason,
				"trigger_reason": "not_applicable",
				"policy":         "require_ai3_review_before_auto_deliver",
			}
			metrics["gif_deliver_fallback_v1"] = deliverFallback
			if len(judgeMetric) > 0 {
				judgeMetric["deliver_fallback_applied"] = false
				judgeMetric["deliver_fallback_reason"] = reason
				judgeMetric["deliver_fallback_trigger_reason"] = "not_applicable"
				metrics["gif_ai_judge_v1"] = judgeMetric
			}
			p.appendJobEvent(job.ID, models.VideoJobStageUploading, "info", "gif deliver fallback skipped", map[string]interface{}{
				"reason": reason,
				"policy": "require_ai3_review_before_auto_deliver",
			})
		}
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

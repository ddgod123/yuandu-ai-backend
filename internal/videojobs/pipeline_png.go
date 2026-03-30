package videojobs

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"emoji/internal/models"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type imageAI1ExecutablePlan struct {
	SchemaVersion    string
	Schema           map[string]interface{}
	Executable       map[string]interface{}
	EventMeta        map[string]interface{}
	DirectorSnapshot map[string]interface{}
}

func (p *Processor) processImagePipeline(ctx context.Context, jobID uint64) error {
	return p.processImagePipelineCore(ctx, jobID, "")
}

func (p *Processor) processPNGPipeline(ctx context.Context, jobID uint64) error {
	return p.processImagePipelineCore(ctx, jobID, "png")
}

func (p *Processor) processJPGPipeline(ctx context.Context, jobID uint64) error {
	return p.processImagePipelineCore(ctx, jobID, "jpg")
}

func (p *Processor) processWebPPipeline(ctx context.Context, jobID uint64) error {
	return p.processImagePipelineCore(ctx, jobID, "webp")
}

func (p *Processor) processLivePipeline(ctx context.Context, jobID uint64) error {
	return p.processImagePipelineCore(ctx, jobID, "live")
}

func (p *Processor) processMP4Pipeline(ctx context.Context, jobID uint64) error {
	return p.processImagePipelineCore(ctx, jobID, "mp4")
}

// processImagePipeline is the dedicated execution lane for still/image-first jobs
// (png/jpg/webp/live/mp4). It is intentionally decoupled from GIF-specific
// AI2/AI3 stages and metrics.
func (p *Processor) processImagePipelineCore(ctx context.Context, jobID uint64, forcedFormat string) error {
	forcedFormat = NormalizeRequestedFormat(forcedFormat)
	executionLane := resolveImageExecutionLane(forcedFormat)

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
	userRequestedMaxStatic := intFromAny(preOptions["user_requested_max_static"]) > 0
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

	requestedFormats := normalizeOutputFormats(job.OutputFormats)
	primaryFormat := firstRequestedFormat(requestedFormats)
	if forcedFormat != "" {
		if primaryFormat == "" {
			primaryFormat = forcedFormat
			requestedFormats = []string{forcedFormat}
		} else if primaryFormat != forcedFormat {
			p.appendJobEvent(job.ID, models.VideoJobStagePreprocessing, "warn", "image pipeline format override applied", map[string]interface{}{
				"requested_format": primaryFormat,
				"forced_format":    forcedFormat,
			})
			primaryFormat = forcedFormat
			requestedFormats = []string{forcedFormat}
		}
	}
	executionLane = resolveImageExecutionLane(primaryFormat)
	metricPrefix := resolveImagePipelineMetricPrefix(primaryFormat)
	pipelineMetricKey := fmt.Sprintf("%s_pipeline_v1", metricPrefix)
	stageStatusKey := fmt.Sprintf("%s_pipeline_stage_status_v1", metricPrefix)
	ai1MetricKey := fmt.Sprintf("%s_ai1_plan_v1", metricPrefix)
	ai2MetricKey := fmt.Sprintf("%s_ai2_instruction_v1", metricPrefix)
	ai3MetricKey := fmt.Sprintf("%s_ai3_review_v1", metricPrefix)
	extractionMetricKey := fmt.Sprintf("%s_extraction_v1", metricPrefix)

	p.appendJobEvent(job.ID, models.VideoJobStagePreprocessing, "info", "execution lane resolved", map[string]interface{}{
		"execution_lane":   executionLane,
		"requested_format": primaryFormat,
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

	pipelineStageStatus := map[string]string{
		"ai1":        "pending",
		"ai2":        "pending",
		"worker":     "pending",
		"ai3":        "pending",
		"extraction": "pending",
		"delivery":   "pending",
	}
	metrics := map[string]interface{}{
		"duration_sec": meta.DurationSec,
		"width":        meta.Width,
		"height":       meta.Height,
		"fps":          meta.FPS,
		pipelineMetricKey: map[string]interface{}{
			"execution_lane":   executionLane,
			"requested_format": primaryFormat,
			"version":          "v1",
		},
		stageStatusKey: pipelineStageStatus,
	}
	if len(sourceReadability) > 0 {
		metrics["source_video_readability_v1"] = sourceReadability
	}
	if sourceInfo != nil && sourceInfo.Size() > 0 {
		metrics["source_video_size_bytes"] = sourceInfo.Size()
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
	flowMode := normalizeVideoFlowMode(stringFromAny(optionsPayload["flow_mode"]))
	ai1Confirmed := boolFromAny(optionsPayload["ai1_confirmed"])
	ai1PauseConsumed := boolFromAny(optionsPayload["ai1_pause_consumed"])

	qualitySettings, qualityOverrides := applyQualityProfileOverridesFromOptions(qualitySettings, optionsPayload, requestedFormats)
	if len(qualityOverrides) > 0 {
		metrics["quality_profile_overrides_applied"] = qualityOverrides
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "quality profile overrides applied", map[string]interface{}{
			"overrides": qualityOverrides,
		})
	}
	if sceneTags := inferSceneTags(job.Title, job.SourceVideoKey, requestedFormats); len(sceneTags) > 0 {
		metrics["scene_tags_v1"] = sceneTags
	}

	options = applyStillProfileDefaults(options, requestedFormats, qualitySettings)
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

	existingPlan := mapFromAny(optionsPayload["ai1_executable_plan_v1"])
	schemaVersion := strings.TrimSpace(stringFromAny(optionsPayload["ai1_plan_schema_version"]))
	if schemaVersion == "" {
		schemaVersion = resolveImageAI1PlanSchemaVersion(primaryFormat)
	}

	planGenerated := false
	planMeta := map[string]interface{}{}
	directorSnapshot := map[string]interface{}{}
	if len(existingPlan) == 0 {
		pipelineStageStatus["ai1"] = "running"
		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", imagePipelineEventMessage(primaryFormat, "ai1 planning started"), map[string]interface{}{
			"flow_mode":        flowMode,
			"requested_format": primaryFormat,
		})
		plan := p.buildImageAI1ExecutablePlan(ctx, job, sourcePath, meta, requestedFormats, options, qualitySettings)
		existingPlan = plan.Executable
		planMeta = plan.EventMeta
		directorSnapshot = plan.DirectorSnapshot
		schemaVersion = plan.SchemaVersion
		planGenerated = true

		optionsPayload["ai1_executable_plan_v1"] = existingPlan
		optionsPayload["ai1_plan_schema_version"] = schemaVersion
		optionsPayload["ai1_plan_generated"] = true
		optionsPayload["ai1_plan_generated_at"] = time.Now().Format(time.RFC3339)
		if len(directorSnapshot) > 0 {
			optionsPayload["ai1_director_snapshot_v1"] = directorSnapshot
		}
		if len(planMeta) > 0 {
			optionsPayload["ai1_event_meta_v1"] = planMeta
		}

		p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", imagePipelineEventMessage(primaryFormat, "ai1 preview generated"), planMeta)
		p.persistImageAI1Plan(job, requestedFormats, flowMode, ai1Confirmed, meta, "ai1_preview_generated", planMeta, directorSnapshot, schemaVersion, plan.Schema, existingPlan)
		pipelineStageStatus["ai1"] = "done"
	} else {
		pipelineStageStatus["ai1"] = "reused"
		planMeta = mapFromAny(optionsPayload["ai1_event_meta_v1"])
		directorSnapshot = mapFromAny(optionsPayload["ai1_director_snapshot_v1"])
	}

	appliedPlanPatch := applyImageAI1ExecutablePlan(existingPlan, meta, &options, optionsPayload, schemaVersion)
	optionsUpdated := len(appliedPlanPatch) > 0 || planGenerated

	ai1Metric := map[string]interface{}{
		"schema_version":   schemaVersion,
		"generated":        planGenerated,
		"reused":           !planGenerated,
		"flow_mode":        flowMode,
		"requested_format": primaryFormat,
		"applied_patch":    appliedPlanPatch,
		"status":           pipelineStageStatus["ai1"],
	}
	if len(planMeta) > 0 {
		ai1Metric["event_meta"] = planMeta
		if strategyTrace := mapFromAny(planMeta["strategy_profile_trace_v1"]); len(strategyTrace) > 0 {
			optionsPayload["ai1_strategy_profile_trace_v1"] = strategyTrace
			optionsUpdated = true
		}
		if sceneGuard := mapFromAny(planMeta["advanced_scene_guard_v1"]); len(sceneGuard) > 0 {
			optionsPayload["ai1_advanced_scene_guard_v1"] = sceneGuard
			optionsUpdated = true
		}
		if overrideReport := mapFromAny(planMeta["strategy_override_report_v1"]); len(overrideReport) > 0 {
			optionsPayload["ai1_strategy_override_report_v1"] = overrideReport
			optionsUpdated = true
		}
	}
	if len(directorSnapshot) > 0 {
		ai1Metric["director_snapshot"] = buildAI1PlanDirectorSnapshot(directorSnapshot)
	}
	metrics[ai1MetricKey] = ai1Metric

	needClarifyPause := shouldPauseForAI1NeedClarify(planMeta, ai1Confirmed, ai1PauseConsumed)
	if shouldPauseAtAI1(flowMode, ai1Confirmed, ai1PauseConsumed) || needClarifyPause {
		pipelineStageStatus["ai2"] = "awaiting_user_confirm"
		pipelineStageStatus["worker"] = "awaiting_user_confirm"
		pipelineStageStatus["ai3"] = "awaiting_user_confirm"
		pipelineStageStatus["extraction"] = "awaiting_user_confirm"
		pipelineStageStatus["delivery"] = "awaiting_user_confirm"
		pauseReason := "flow_mode_ai1_confirm"
		if needClarifyPause {
			pauseReason = "interactive_action_need_clarify"
		}
		if p.pauseForAI1Confirmation(job.ID, metrics, optionsPayload, map[string]interface{}{
			"flow_mode":    flowMode,
			"stage":        "ai1_preview_generated",
			"pause_reason": pauseReason,
		}) {
			return nil
		}
	}

	ai2Instruction := buildImageAI2Instruction(schemaVersion, existingPlan, planMeta)
	ai2Guidance := resolveImageAI2Guidance(primaryFormat, ai2Instruction, planMeta)
	if postprocess := mapFromAny(ai2Instruction["postprocess"]); len(postprocess) > 0 {
		optionsPayload["ai2_postprocess_v1"] = postprocess
	}
	if advancedOptions := mapFromAny(ai2Instruction["advanced_options"]); len(advancedOptions) > 0 {
		optionsPayload["ai1_advanced_options_v1"] = advancedOptions
	}
	if strategyProfile := mapFromAny(ai2Instruction["strategy_profile"]); len(strategyProfile) > 0 {
		optionsPayload["ai1_strategy_profile_v1"] = strategyProfile
	}
	if strategyTrace := mapFromAny(planMeta["strategy_profile_trace_v1"]); len(strategyTrace) > 0 {
		optionsPayload["ai1_strategy_profile_trace_v1"] = strategyTrace
	}
	if sceneGuard := mapFromAny(planMeta["advanced_scene_guard_v1"]); len(sceneGuard) > 0 {
		optionsPayload["ai1_advanced_scene_guard_v1"] = sceneGuard
	}
	if overrideReport := mapFromAny(planMeta["strategy_override_report_v1"]); len(overrideReport) > 0 {
		optionsPayload["ai1_strategy_override_report_v1"] = overrideReport
	}
	if len(ai2Guidance.QualityWeights) > 0 {
		optionsPayload["ai2_quality_weights_v1"] = ai2Guidance.QualityWeights
	}
	if len(ai2Guidance.RiskFlags) > 0 {
		optionsPayload["ai2_risk_flags_v1"] = ai2Guidance.RiskFlags
	}
	optionsPayload["ai2_technical_reject_v1"] = map[string]interface{}{
		"max_blur_tolerance": ai2Guidance.MaxBlurTolerance,
		"avoid_watermarks":   ai2Guidance.AvoidWatermarks,
		"avoid_extreme_dark": ai2Guidance.AvoidExtremeDark,
	}
	if len(ai2Instruction) > 0 {
		optionsPayload["ai2_instruction_v1"] = ai2Instruction
		optionsPayload["ai2_instruction_generated_at"] = time.Now().Format(time.RFC3339)
		optionsUpdated = true
	}
	optionsPayload["ai2_guidance_v1"] = ai2Guidance.toMetricsMap()
	optionsUpdated = true
	pipelineStageStatus["ai2"] = "running"
	p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "sub-stage planning started", map[string]interface{}{
		"requested_format": primaryFormat,
		"source":           "ai1_executable_plan",
	})
	ai2EventMeta := map[string]interface{}{
		"requested_format": strings.ToUpper(primaryFormat),
		"mode":             strings.TrimSpace(stringFromAny(existingPlan["mode"])),
		"selected_count":   intFromAny(existingPlan["target_count"]),
		"selected_score":   roundTo(floatFromAny(planMeta["local_focus_score"]), 4),
	}
	if focus := mapFromAny(existingPlan["focus_window"]); len(focus) > 0 {
		ai2EventMeta["selected_start_sec"] = roundTo(floatFromAny(focus["start_sec"]), 3)
		ai2EventMeta["selected_end_sec"] = roundTo(floatFromAny(focus["end_sec"]), 3)
	}
	if objective := strings.TrimSpace(stringFromAny(ai2Instruction["objective"])); objective != "" {
		ai2EventMeta["objective"] = objective
	}
	if operatorIdentity := strings.TrimSpace(stringFromAny(ai2Instruction["operator_identity"])); operatorIdentity != "" {
		ai2EventMeta["operator_identity"] = operatorIdentity
	}
	if candidateCountBias := mapFromAny(ai2Instruction["candidate_count_bias"]); len(candidateCountBias) > 0 {
		ai2EventMeta["candidate_count_bias"] = candidateCountBias
	}
	if advanced := mapFromAny(ai2Instruction["advanced_options"]); len(advanced) > 0 {
		ai2EventMeta["scene"] = strings.TrimSpace(stringFromAny(advanced["scene"]))
		ai2EventMeta["visual_focus"] = stringSliceFromAny(advanced["visual_focus"])
		ai2EventMeta["enable_matting"] = boolFromAny(advanced["enable_matting"])
	}
	if profile := mapFromAny(ai2Instruction["strategy_profile"]); len(profile) > 0 {
		ai2EventMeta["scene_label"] = strings.TrimSpace(stringFromAny(profile["scene_label"]))
	}
	if len(ai2Guidance.QualityWeights) > 0 {
		ai2EventMeta["quality_weights"] = ai2Guidance.QualityWeights
	}
	if len(ai2Guidance.RiskFlags) > 0 {
		ai2EventMeta["risk_flags"] = ai2Guidance.RiskFlags
	}
	ai2EventMeta["max_blur_tolerance"] = ai2Guidance.MaxBlurTolerance
	p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "ai planner suggestion applied", ai2EventMeta)
	pipelineStageStatus["ai2"] = "done"
	metrics[ai2MetricKey] = ai2Instruction
	metrics[fmt.Sprintf("%s_ai2_guidance_v1", metricPrefix)] = ai2Guidance.toMetricsMap()

	extractOptions := applyStillProfileDefaults(options, requestedFormats, qualitySettings)
	workerQualitySettings, extractOptions, workerStrategy := applyImageAI2WorkerStrategy(qualitySettings, extractOptions, ai2Guidance)
	extractOptions, outputCountPolicy := applyPNGMainlineOutputCountPolicy(extractOptions, primaryFormat, meta, ai2Guidance, userRequestedMaxStatic)
	extractOptions, coverageWindowPolicy := applyPNGMainlineCoverageWindowPolicy(extractOptions, primaryFormat, meta, ai2Guidance, userRequestedMaxStatic)
	workerStrategy["output_count_policy_v1"] = outputCountPolicy
	workerStrategy["coverage_window_policy_v1"] = coverageWindowPolicy
	metrics[fmt.Sprintf("%s_worker_strategy_v1", metricPrefix)] = workerStrategy
	optionsPayload["ai2_worker_strategy_v1"] = workerStrategy
	if boolFromAny(outputCountPolicy["applied"]) {
		optionsPayload["max_static"] = extractOptions.MaxStatic
	}
	if boolFromAny(coverageWindowPolicy["applied"]) {
		optionsPayload["start_sec"] = extractOptions.StartSec
		optionsPayload["end_sec"] = extractOptions.EndSec
		optionsPayload["frame_interval_sec"] = extractOptions.FrameIntervalSec
	}
	if outputCountPolicy != nil {
		optionsPayload["png_output_count_policy_v1"] = outputCountPolicy
	}
	if coverageWindowPolicy != nil {
		optionsPayload["png_coverage_window_policy_v1"] = coverageWindowPolicy
	}
	optionsUpdated = true

	if optionsUpdated {
		p.updateVideoJob(job.ID, map[string]interface{}{
			"options": mustJSON(optionsPayload),
		})
	}

	pipelineStageStatus["worker"] = "running"
	pipelineStageStatus["extraction"] = "running"
	p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", "worker risk strategy applied", workerStrategy)

	frameDir := filepath.Join(tmpDir, "frames")
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return fmt.Errorf("create frame dir: %w", err)
	}

	effectiveDurationSec := effectiveSampleDuration(meta, options)
	candidateBudget := qualitySelectionCandidateBudget(extractOptions.MaxStatic)
	interval := chooseFrameInterval(effectiveDurationSec, extractOptions.FrameIntervalSec, candidateBudget)

	if err := extractFrames(ctx, sourcePath, frameDir, meta, extractOptions, interval, workerQualitySettings); err != nil {
		return fmt.Errorf("extract frames: %w", err)
	}
	framePaths, err := collectFramePaths(frameDir, candidateBudget)
	if err != nil {
		return fmt.Errorf("collect frames: %w", err)
	}
	if len(framePaths) == 0 {
		return permanentError{err: errors.New("no frames extracted from video")}
	}

	optimizedFramePaths, qualityReport := optimizeFramePathsForQualityWithGuidance(framePaths, extractOptions.MaxStatic, workerQualitySettings, ai2Guidance)
	if len(optimizedFramePaths) > 0 {
		framePaths = optimizedFramePaths
	}
	framePaths, ai2LLMRerankReport := p.maybeApplyPNGAI2LLMRerank(ctx, job, primaryFormat, framePaths, qualityReport, ai2Guidance, "pre_enhance")
	if len(ai2LLMRerankReport) > 0 {
		metrics[fmt.Sprintf("%s_ai2_llm_rerank_v1", metricPrefix)] = ai2LLMRerankReport
	}

	framePaths, faceEnhancementReport := p.maybeApplyPNGAliyunFaceEnhancement(ctx, job, primaryFormat, framePaths, ai2Guidance)
	if len(faceEnhancementReport) > 0 {
		metrics[fmt.Sprintf("%s_worker_face_enhancement_v1", metricPrefix)] = faceEnhancementReport
	}

	framePaths, superResolutionReport := p.maybeApplyPNGAliyunSuperResolution(ctx, job, primaryFormat, framePaths)
	if len(superResolutionReport) > 0 {
		metrics[fmt.Sprintf("%s_worker_super_resolution_v1", metricPrefix)] = superResolutionReport
	}

	postEnhanceFramePaths, postEnhanceQualityReport := rerankEnhancedFramePaths(framePaths, workerQualitySettings, ai2Guidance)
	if len(postEnhanceFramePaths) > 0 {
		framePaths = postEnhanceFramePaths
		qualityReport = postEnhanceQualityReport
	}
	if postEnhanceQualityReport.TotalFrames > 0 {
		metrics[fmt.Sprintf("%s_post_enhance_quality_v1", metricPrefix)] = postEnhanceQualityReport
	}
	framePaths, ai2LLMRerankPostEnhanceReport := p.maybeApplyPNGAI2LLMRerank(ctx, job, primaryFormat, framePaths, qualityReport, ai2Guidance, "post_enhance")
	if len(ai2LLMRerankPostEnhanceReport) > 0 {
		metrics[fmt.Sprintf("%s_ai2_llm_rerank_post_enhance_v1", metricPrefix)] = ai2LLMRerankPostEnhanceReport
	}
	framePaths, finalQualityGuardReport := applyPNGFinalQualityGuards(framePaths, workerQualitySettings, ai2Guidance)
	if len(finalQualityGuardReport) > 0 {
		metrics[fmt.Sprintf("%s_final_quality_guard_v1", metricPrefix)] = finalQualityGuardReport
	}

	metrics["frame_quality"] = qualityReport
	qualitySettingsMetric := map[string]interface{}{
		"min_brightness":              workerQualitySettings.MinBrightness,
		"max_brightness":              workerQualitySettings.MaxBrightness,
		"blur_threshold_factor":       workerQualitySettings.BlurThresholdFactor,
		"duplicate_hamming_threshold": workerQualitySettings.DuplicateHammingThreshold,
		"still_min_blur_score":        workerQualitySettings.StillMinBlurScore,
		"still_min_exposure_score":    workerQualitySettings.StillMinExposureScore,
		"still_min_width":             workerQualitySettings.StillMinWidth,
		"still_min_height":            workerQualitySettings.StillMinHeight,
		"jpg_profile":                 workerQualitySettings.JPGProfile,
		"png_profile":                 workerQualitySettings.PNGProfile,
		"webp_profile":                workerQualitySettings.WebPProfile,
		"live_profile":                workerQualitySettings.LiveProfile,
		"png_target_size_kb":          workerQualitySettings.PNGTargetSizeKB,
		"jpg_target_size_kb":          workerQualitySettings.JPGTargetSizeKB,
		"webp_target_size_kb":         workerQualitySettings.WebPTargetSizeKB,
		"quality_candidate_budget":    candidateBudget,
		"still_clarity_enhance":       shouldApplyStillClarityEnhancement(meta, extractOptions, workerQualitySettings),
		"ai2_quality_weights":         ai2Guidance.QualityWeights,
		"ai2_risk_flags":              ai2Guidance.RiskFlags,
		"ai2_max_blur_tolerance":      ai2Guidance.MaxBlurTolerance,
		"ai2_selection_policy":        qualityReport.SelectionPolicy,
		"ai2_llm_rerank_mode":         stringFromAny(ai2LLMRerankReport["mode"]),
		"ai2_llm_rerank_status":       stringFromAny(ai2LLMRerankReport["status"]),
		"ai2_llm_rerank_applied":      boolFromAny(ai2LLMRerankReport["applied"]),
		"ai2_llm_rerank_post_mode":    stringFromAny(ai2LLMRerankPostEnhanceReport["mode"]),
		"ai2_llm_rerank_post_status":  stringFromAny(ai2LLMRerankPostEnhanceReport["status"]),
		"ai2_llm_rerank_post_applied": boolFromAny(ai2LLMRerankPostEnhanceReport["applied"]),
		"output_count_policy_status":  stringFromAny(outputCountPolicy["status"]),
		"output_count_policy_applied": boolFromAny(outputCountPolicy["applied"]),
		"output_count_policy_before":  intFromAny(outputCountPolicy["before_max_static"]),
		"output_count_policy_after":   intFromAny(outputCountPolicy["after_max_static"]),
		"coverage_window_status":      stringFromAny(coverageWindowPolicy["status"]),
		"coverage_window_applied":     boolFromAny(coverageWindowPolicy["applied"]),
		"coverage_window_before_sec":  roundTo(floatFromAny(coverageWindowPolicy["window_duration_before_sec"]), 3),
		"coverage_window_after_sec":   roundTo(floatFromAny(coverageWindowPolicy["window_duration_after_sec"]), 3),
		"final_quality_guard_status":  stringFromAny(finalQualityGuardReport["status"]),
		"final_quality_guard_applied": boolFromAny(finalQualityGuardReport["applied"]),
		"final_quality_guard_output":  intFromAny(finalQualityGuardReport["output_count"]),
	}
	metrics[fmt.Sprintf("%s_quality_settings_v1", metricPrefix)] = qualitySettingsMetric
	metrics[extractionMetricKey] = map[string]interface{}{
		"frame_count":             len(framePaths),
		"candidate_count":         len(qualityReport.CandidateScores),
		"candidate_budget":        candidateBudget,
		"interval_sec":            roundTo(interval, 3),
		"effective_duration":      roundTo(effectiveDurationSec, 3),
		"selector_version":        qualityReport.SelectorVersion,
		"scoring_mode":            qualityReport.ScoringMode,
		"selection_policy":        qualityReport.SelectionPolicy,
		"must_capture_hits":       countFrameCandidateMustCaptureHits(qualityReport.CandidateScores),
		"avoid_hits":              countFrameCandidateAvoidHits(qualityReport.CandidateScores),
		"llm_rerank_mode":         stringFromAny(ai2LLMRerankReport["mode"]),
		"llm_rerank_status":       stringFromAny(ai2LLMRerankReport["status"]),
		"llm_rerank_applied":      boolFromAny(ai2LLMRerankReport["applied"]),
		"llm_rerank_post_mode":    stringFromAny(ai2LLMRerankPostEnhanceReport["mode"]),
		"llm_rerank_post_status":  stringFromAny(ai2LLMRerankPostEnhanceReport["status"]),
		"llm_rerank_post_applied": boolFromAny(ai2LLMRerankPostEnhanceReport["applied"]),
		"final_guard_status":      stringFromAny(finalQualityGuardReport["status"]),
		"final_guard_applied":     boolFromAny(finalQualityGuardReport["applied"]),
		"final_guard_output":      intFromAny(finalQualityGuardReport["output_count"]),
	}

	pipelineStageStatus["worker"] = "done"
	pipelineStageStatus["extraction"] = "done"
	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageRendering,
		"progress": 55,
		"metrics":  mustJSON(metrics),
	})
	p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", "frame extraction completed", map[string]interface{}{
		"frames":                      len(framePaths),
		"quality_blur_reject":         qualityReport.RejectedBlur,
		"quality_bright_reject":       qualityReport.RejectedBrightness,
		"quality_exposure_reject":     qualityReport.RejectedExposure,
		"quality_resolution_reject":   qualityReport.RejectedResolution,
		"quality_still_blur_reject":   qualityReport.RejectedStillBlurGate,
		"quality_watermark_reject":    qualityReport.RejectedWatermark,
		"quality_dup_reject":          qualityReport.RejectedNearDuplicate,
		"quality_fallback":            qualityReport.FallbackApplied,
		"quality_selector_version":    qualityReport.SelectorVersion,
		"quality_scoring_mode":        qualityReport.ScoringMode,
		"quality_selection_policy":    qualityReport.SelectionPolicy,
		"ai2_llm_rerank_mode":         stringFromAny(ai2LLMRerankReport["mode"]),
		"ai2_llm_rerank_status":       stringFromAny(ai2LLMRerankReport["status"]),
		"ai2_llm_rerank_applied":      boolFromAny(ai2LLMRerankReport["applied"]),
		"ai2_llm_rerank_post_mode":    stringFromAny(ai2LLMRerankPostEnhanceReport["mode"]),
		"ai2_llm_rerank_post_status":  stringFromAny(ai2LLMRerankPostEnhanceReport["status"]),
		"ai2_llm_rerank_post_applied": boolFromAny(ai2LLMRerankPostEnhanceReport["applied"]),
		"face_enhance_mode":           stringFromAny(faceEnhancementReport["mode"]),
		"face_enhance_attempted":      intFromAny(faceEnhancementReport["attempted"]),
		"face_enhance_succeeded":      intFromAny(faceEnhancementReport["succeeded"]),
		"face_enhance_replaced":       intFromAny(faceEnhancementReport["replaced"]),
		"face_enhance_total_cost_cny": roundTo(floatFromAny(faceEnhancementReport["total_cost_cny"]), 6),
		"face_enhance_cost_capped":    boolFromAny(faceEnhancementReport["cost_capped"]),
		"superres_mode":               stringFromAny(superResolutionReport["mode"]),
		"superres_attempted":          intFromAny(superResolutionReport["attempted"]),
		"superres_succeeded":          intFromAny(superResolutionReport["succeeded"]),
		"superres_replaced":           intFromAny(superResolutionReport["replaced"]),
		"superres_total_cost_cny":     roundTo(floatFromAny(superResolutionReport["total_cost_cny"]), 6),
		"superres_cost_capped":        boolFromAny(superResolutionReport["cost_capped"]),
		"final_guard_status":          stringFromAny(finalQualityGuardReport["status"]),
		"final_guard_applied":         boolFromAny(finalQualityGuardReport["applied"]),
		"final_guard_output":          intFromAny(finalQualityGuardReport["output_count"]),
	})

	if p.isJobCancelled(job.ID) {
		p.appendJobEvent(job.ID, models.VideoJobStageCancelled, "info", "job cancelled after rendering", nil)
		p.syncJobCost(job.ID)
		p.syncJobPointSettlement(job.ID, models.VideoJobStatusCancelled)
		p.cleanupSourceVideo(job.ID, "cancelled")
		return nil
	}

	pipelineStageStatus["ai3"] = "running"
	p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", "sub-stage reviewing started", map[string]interface{}{
		"requested_format": primaryFormat,
		"review_type":      "image_quality_gate",
	})
	ai3Review := buildImageAI3ReviewSummary(primaryFormat, qualityReport, len(framePaths), candidateBudget, effectiveDurationSec)
	if err := p.persistImageAI3Review(job, primaryFormat, ai3Review); err != nil {
		p.appendJobEvent(job.ID, models.VideoJobStageRendering, "warn", "ai image review persistence failed", map[string]interface{}{
			"requested_format": primaryFormat,
			"error":            err.Error(),
		})
	}
	metrics[ai3MetricKey] = ai3Review
	p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", "ai judge completed", ai3Review)
	pipelineStageStatus["ai3"] = "done"

	pipelineStageStatus["delivery"] = "running"
	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageRendering,
		"progress": 70,
	})
	p.appendJobEvent(job.ID, models.VideoJobStageRendering, "info", imagePipelineEventMessage(primaryFormat, "output render pipeline started"), map[string]interface{}{
		"entry_progress": 70,
	})

	highlightCandidates := make([]highlightCandidate, 0)
	animatedWindows := make([]highlightCandidate, 0)
	resultCollectionID, totalFrames, uploadedKeys, generatedFormats, packageOutcome, err := p.persistJobResults(
		ctx,
		job,
		framePaths,
		sourcePath,
		meta,
		options,
		highlightCandidates,
		animatedWindows,
		workerQualitySettings,
	)
	if err != nil {
		deleteQiniuKeysByPrefix(p.qiniu, uploadedKeys)
		return err
	}

	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageUploading,
		"progress": 88,
	})

	metrics["static_count"] = totalFrames
	metrics["output_formats_requested"] = requestedFormats
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
	pipelineStageStatus["delivery"] = "done"
	metrics[stageStatusKey] = pipelineStageStatus

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
		fmt.Sprintf("%s_pipeline_status_v1", metricPrefix): pipelineStageStatus,
	})
	if packageOutcome.Status == packageZipStatusFailed {
		p.appendJobEvent(job.ID, models.VideoJobStageDone, "warn", "video job completed without zip package", map[string]interface{}{
			"attempts": packageOutcome.Attempts,
			"error":    packageOutcome.Error,
		})
	}
	p.syncJobCost(job.ID)
	p.syncJobPointSettlement(job.ID, models.VideoJobStatusDone)
	p.cleanupSourceVideo(job.ID, "done")
	return nil
}

func resolveImagePipelineMetricPrefix(primaryFormat string) string {
	switch NormalizeRequestedFormat(primaryFormat) {
	case "png":
		return "png"
	case "jpg":
		return "jpg"
	case "webp":
		return "webp"
	case "live":
		return "live"
	case "mp4":
		return "mp4"
	default:
		return "image"
	}
}

func resolveImageExecutionLane(primaryFormat string) string {
	switch NormalizeRequestedFormat(primaryFormat) {
	case "png":
		return "png"
	case "jpg":
		return "jpg"
	case "webp":
		return "webp"
	case "live":
		return "live"
	case "mp4":
		return "mp4"
	default:
		return "image"
	}
}

func resolveImageAI1PlanSchemaVersion(primaryFormat string) string {
	switch NormalizeRequestedFormat(primaryFormat) {
	case "png":
		return VideoJobAI1PlanSchemaPNGV1
	default:
		return VideoJobAI1PlanSchemaImageV1
	}
}

func imagePipelineEventMessage(primaryFormat, suffix string) string {
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return ""
	}
	if NormalizeRequestedFormat(primaryFormat) == "png" {
		return "png " + suffix
	}
	return suffix
}

func buildImageAI3ReviewSummary(
	primaryFormat string,
	qualityReport frameQualityReport,
	reviewedOutputs int,
	candidateBudget int,
	effectiveDurationSec float64,
) map[string]interface{} {
	if reviewedOutputs < 0 {
		reviewedOutputs = 0
	}
	deliverCount := reviewedOutputs
	rejectCount := 0
	if qualityReport.TotalFrames > 0 {
		rejectCount = qualityReport.TotalFrames - reviewedOutputs
		if rejectCount < 0 {
			rejectCount = 0
		}
	}

	recommendation := "deliver"
	summaryNote := "质量筛选通过，输出可交付。"
	if reviewedOutputs == 0 {
		recommendation = "need_manual_review"
		summaryNote = "未筛选到可交付帧，建议人工复核源视频质量。"
	} else if qualityReport.FallbackApplied {
		recommendation = "deliver_with_fallback"
		summaryNote = "已启用质量回退策略补足样本，建议抽检结果。"
	}

	return map[string]interface{}{
		"requested_format":                strings.ToUpper(NormalizeRequestedFormat(primaryFormat)),
		"reviewed_outputs":                reviewedOutputs,
		"deliver_count":                   deliverCount,
		"keep_internal_count":             0,
		"reject_count":                    rejectCount,
		"manual_review_count":             0,
		"hard_gate_reject_count":          0,
		"hard_gate_manual_review_count":   0,
		"recommendation":                  recommendation,
		"candidate_budget":                candidateBudget,
		"effective_duration_sec":          roundTo(effectiveDurationSec, 3),
		"quality_report_selector_version": qualityReport.SelectorVersion,
		"quality_fallback":                qualityReport.FallbackApplied,
		"summary": map[string]interface{}{
			"note": summaryNote,
		},
	}
}

func (p *Processor) persistImageAI3Review(job models.VideoJob, primaryFormat string, summary map[string]interface{}) error {
	if p == nil || p.db == nil || job.ID == 0 || len(summary) == 0 {
		return nil
	}
	reviewSummary := mapFromAny(summary["summary"])
	row := models.VideoJobImageAIReview{
		JobID:                     job.ID,
		UserID:                    job.UserID,
		TargetFormat:              NormalizeRequestedFormat(primaryFormat),
		Stage:                     "ai3",
		Recommendation:            strings.TrimSpace(strings.ToLower(stringFromAny(summary["recommendation"]))),
		ReviewedOutputs:           intFromAny(summary["reviewed_outputs"]),
		DeliverCount:              intFromAny(summary["deliver_count"]),
		RejectCount:               intFromAny(summary["reject_count"]),
		ManualReviewCount:         intFromAny(summary["manual_review_count"]),
		HardGateRejectCount:       intFromAny(summary["hard_gate_reject_count"]),
		HardGateManualReviewCount: intFromAny(summary["hard_gate_manual_review_count"]),
		CandidateBudget:           intFromAny(summary["candidate_budget"]),
		EffectiveDurationSec:      roundTo(floatFromAny(summary["effective_duration_sec"]), 3),
		QualityFallback:           boolFromAny(summary["quality_fallback"]),
		QualitySelectorVersion:    strings.TrimSpace(stringFromAny(summary["quality_report_selector_version"])),
		SummaryNote:               strings.TrimSpace(stringFromAny(reviewSummary["note"])),
		SummaryJSON:               mustJSON(reviewSummary),
		Metadata:                  mustJSON(summary),
	}
	return UpsertVideoJobImageAIReview(p.db, row)
}

func sanitizeTextList(items []string, maxItems int) []string {
	if len(items) == 0 || maxItems <= 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, minInt(len(items), maxItems))
	for _, item := range items {
		text := strings.TrimSpace(item)
		if text == "" {
			continue
		}
		normalized := strings.ToLower(text)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, text)
		if len(out) >= maxItems {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clampImageTargetCount(value int) int {
	if value <= 0 {
		return 24
	}
	if value > 80 {
		return 80
	}
	return value
}

func applyPNGMainlineOutputCountPolicy(
	options jobOptions,
	primaryFormat string,
	meta videoProbeMeta,
	guidance imageAI2Guidance,
	userPinned bool,
) (jobOptions, map[string]interface{}) {
	report := map[string]interface{}{
		"schema_version": "png_output_count_policy_v1",
		"format":         NormalizeRequestedFormat(primaryFormat),
	}
	if report["format"] != "png" {
		report["status"] = "skipped_not_png"
		return options, report
	}

	before := clampImageTargetCount(options.MaxStatic)
	report["before_max_static"] = before
	report["user_pinned"] = userPinned
	if userPinned {
		report["status"] = "skipped_user_pinned"
		report["after_max_static"] = before
		report["applied"] = false
		return options, report
	}

	enabled := parseEnvBool("PNG_MAINLINE_OUTPUT_COUNT_GUARD_ENABLED", true)
	report["enabled"] = enabled
	if !enabled {
		report["status"] = "disabled"
		report["after_max_static"] = before
		report["applied"] = false
		return options, report
	}

	dynamicFloor := resolvePNGCoverageMinOutputCount(meta.DurationSec, guidance)
	configFloor := clampInt(envIntOrDefault("PNG_MAINLINE_OUTPUT_COUNT_MIN", 12), 4, 64)
	targetMin := configFloor
	if dynamicFloor > targetMin {
		targetMin = dynamicFloor
	}
	targetMax := clampInt(envIntOrDefault("PNG_MAINLINE_OUTPUT_COUNT_MAX", 36), targetMin, 80)
	after := before
	if after < targetMin {
		after = targetMin
	}
	if after > targetMax {
		after = targetMax
	}
	after = clampImageTargetCount(after)

	report["dynamic_floor"] = dynamicFloor
	report["config_floor"] = configFloor
	report["target_min"] = targetMin
	report["target_max"] = targetMax
	report["after_max_static"] = after
	report["applied"] = after != before
	if after != before {
		options.MaxStatic = after
		report["status"] = "applied"
		return options, report
	}
	report["status"] = "unchanged"
	return options, report
}

func resolvePNGCoverageMinOutputCount(durationSec float64, guidance imageAI2Guidance) int {
	minCount := 10
	switch {
	case durationSec <= 0:
		minCount = 10
	case durationSec <= 6:
		minCount = 6
	case durationSec <= 12:
		minCount = 10
	case durationSec <= 24:
		minCount = 14
	case durationSec <= 45:
		minCount = 18
	default:
		minCount = 22
	}
	if guidance.SelectionPolicy == "ai2_scene_diversity_first" || guidance.SelectionPolicy == "scene_diversity_first" {
		minCount += 2
	}
	if hasVisualFocus(guidance.VisualFocus, "action") || hasVisualFocus(guidance.VisualFocus, "vibe") {
		minCount += 2
	}
	if guidance.Scene == AdvancedScenarioXiaohongshu {
		minCount++
	}
	return clampInt(minCount, 4, 64)
}

func applyPNGMainlineCoverageWindowPolicy(
	options jobOptions,
	primaryFormat string,
	meta videoProbeMeta,
	guidance imageAI2Guidance,
	userPinned bool,
) (jobOptions, map[string]interface{}) {
	report := map[string]interface{}{
		"schema_version": "png_coverage_window_policy_v1",
		"format":         NormalizeRequestedFormat(primaryFormat),
		"user_pinned":    userPinned,
	}
	if report["format"] != "png" {
		report["status"] = "skipped_not_png"
		return options, report
	}
	if userPinned {
		report["status"] = "skipped_user_pinned"
		return options, report
	}
	enabled := parseEnvBool("PNG_MAINLINE_COVERAGE_WINDOW_GUARD_ENABLED", true)
	report["enabled"] = enabled
	if !enabled {
		report["status"] = "disabled"
		return options, report
	}

	windowStart, windowDuration := resolveClipWindow(meta, options)
	totalDuration := maxFloat(meta.DurationSec, 0)
	report["window_start_sec"] = roundTo(windowStart, 3)
	report["window_duration_before_sec"] = roundTo(windowDuration, 3)
	report["total_duration_sec"] = roundTo(totalDuration, 3)
	report["before_start_sec"] = roundTo(options.StartSec, 3)
	report["before_end_sec"] = roundTo(options.EndSec, 3)
	report["before_interval_sec"] = roundTo(options.FrameIntervalSec, 3)
	report["before_max_static"] = clampImageTargetCount(options.MaxStatic)
	if windowDuration <= 0 {
		report["status"] = "no_focus_window"
		report["window_duration_after_sec"] = roundTo(effectiveSampleDuration(meta, options), 3)
		return options, report
	}

	targetCount := clampImageTargetCount(options.MaxStatic)
	if targetCount < clampInt(envIntOrDefault("PNG_MAINLINE_COVERAGE_WINDOW_TARGET_MIN", 12), 6, 80) {
		report["status"] = "skipped_target_count_low"
		report["window_duration_after_sec"] = roundTo(windowDuration, 3)
		return options, report
	}

	interval := options.FrameIntervalSec
	if interval <= 0 {
		interval = resolveImagePlanInterval(0, targetCount, meta, windowStart, windowStart+windowDuration)
	}
	if interval <= 0 {
		interval = 0.8
	}
	windowCandidateEstimate := int(math.Floor(windowDuration/interval)) + 1
	if windowCandidateEstimate < 1 {
		windowCandidateEstimate = 1
	}
	requiredCoverageRatio := clampFloat(parseFloat(firstNonEmptyString(os.Getenv("PNG_MAINLINE_COVERAGE_WINDOW_REQUIRED_RATIO"), "0.75")), 0.45, 0.98)
	if guidance.SelectionPolicy == "ai2_scene_diversity_first" || guidance.SelectionPolicy == "scene_diversity_first" {
		requiredCoverageRatio = clampFloat(requiredCoverageRatio+0.1, 0.5, 0.99)
	}
	requiredMinCandidates := int(math.Ceil(float64(targetCount) * requiredCoverageRatio))
	if requiredMinCandidates < 4 {
		requiredMinCandidates = 4
	}
	report["window_candidate_estimate"] = windowCandidateEstimate
	report["required_candidate_min"] = requiredMinCandidates
	report["required_coverage_ratio"] = roundTo(requiredCoverageRatio, 3)

	windowRatio := 0.0
	if totalDuration > 0 {
		windowRatio = clampZeroOne(windowDuration / totalDuration)
	}
	report["window_ratio"] = roundTo(windowRatio, 3)

	windowRatioMin := clampFloat(parseFloat(firstNonEmptyString(os.Getenv("PNG_MAINLINE_COVERAGE_WINDOW_MIN_RATIO"), "0.45")), 0.2, 0.95)
	report["window_ratio_min"] = roundTo(windowRatioMin, 3)

	if windowCandidateEstimate >= requiredMinCandidates && (totalDuration <= 0 || windowRatio >= windowRatioMin) {
		report["status"] = "kept_focus_window"
		report["window_duration_after_sec"] = roundTo(windowDuration, 3)
		report["applied"] = false
		return options, report
	}

	options.StartSec = 0
	options.EndSec = 0
	options.FrameIntervalSec = 0
	report["status"] = "broaden_to_full_video"
	report["applied"] = true
	report["after_start_sec"] = 0
	report["after_end_sec"] = 0
	report["after_interval_sec"] = 0
	report["window_duration_after_sec"] = roundTo(effectiveSampleDuration(meta, options), 3)
	return options, report
}

func resolveImageFocusWindow(meta videoProbeMeta, localSuggestion highlightSuggestion) (startSec float64, endSec float64, source string) {
	if localSuggestion.Selected == nil {
		return 0, 0, ""
	}
	startSec = localSuggestion.Selected.StartSec
	endSec = localSuggestion.Selected.EndSec
	if meta.DurationSec > 0 {
		if startSec < 0 {
			startSec = 0
		}
		if endSec > meta.DurationSec {
			endSec = meta.DurationSec
		}
	}
	if endSec <= startSec {
		return 0, 0, ""
	}
	return roundTo(startSec, 3), roundTo(endSec, 3), "local_highlight"
}

func resolveImagePlanInterval(preferred float64, targetCount int, meta videoProbeMeta, focusStart, focusEnd float64) float64 {
	if preferred > 0 {
		return roundTo(clampFloat(preferred, 0.2, 8.0), 3)
	}
	duration := meta.DurationSec
	if focusEnd > focusStart {
		duration = focusEnd - focusStart
	}
	if duration <= 0 {
		duration = 8
	}
	if targetCount <= 0 {
		targetCount = 24
	}
	interval := duration / float64(targetCount)
	if interval <= 0 {
		interval = 1
	}
	return roundTo(clampFloat(interval, 0.2, 8.0), 3)
}

func buildImageAI1ExecutableSchema(schemaVersion string, requestedFormat string) map[string]interface{} {
	return map[string]interface{}{
		"$schema":        "https://json-schema.org/draft/2020-12/schema",
		"title":          "AI1 Image Executable Plan",
		"type":           "object",
		"schema_version": schemaVersion,
		"required":       []string{"target_format", "mode", "target_count", "frame_interval_sec"},
		"properties": map[string]interface{}{
			"target_format": map[string]interface{}{
				"type":    "string",
				"default": requestedFormat,
			},
			"mode": map[string]interface{}{
				"type": "string",
				"enum": []string{"uniform_sampling", "focus_window"},
			},
			"target_count": map[string]interface{}{
				"type":    "integer",
				"minimum": 1,
				"maximum": 80,
			},
			"frame_interval_sec": map[string]interface{}{
				"type":    "number",
				"minimum": 0.2,
				"maximum": 8,
			},
			"focus_window": map[string]interface{}{
				"type": []string{"object", "null"},
				"properties": map[string]interface{}{
					"start_sec": map[string]interface{}{"type": "number", "minimum": 0},
					"end_sec":   map[string]interface{}{"type": "number", "minimum": 0},
					"source":    map[string]interface{}{"type": "string"},
				},
				"required": []string{"start_sec", "end_sec"},
			},
			"must_capture": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"avoid":        map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
			"style_direction": map[string]interface{}{
				"type": "string",
			},
		},
	}
}

func buildImageAI1UserReply(eventMeta map[string]interface{}, meta videoProbeMeta, requestedFormats []string) map[string]interface{} {
	formatLabel := strings.ToUpper(firstRequestedFormat(requestedFormats))
	if formatLabel == "" {
		formatLabel = "PNG"
	}
	summary := strings.TrimSpace(stringFromAny(eventMeta["ai_reply"]))
	if summary == "" {
		summary = buildGenericAI1Reply("", requestedFormats, meta)
	}
	out := map[string]interface{}{
		"summary":          summary,
		"requested_format": formatLabel,
		"video_meta": map[string]interface{}{
			"duration_sec": roundTo(meta.DurationSec, 3),
			"width":        meta.Width,
			"height":       meta.Height,
			"fps":          roundTo(meta.FPS, 3),
		},
	}
	intent := map[string]interface{}{}
	for _, key := range []string{"business_goal", "audience", "style_direction", "must_capture", "avoid"} {
		if value, ok := eventMeta[key]; ok && value != nil {
			intent[key] = value
		}
	}
	if len(intent) > 0 {
		out["understood_intent"] = intent
	}
	assumptions := make([]string, 0, 2)
	if text := strings.TrimSpace(stringFromAny(eventMeta["focus_window_source"])); text != "" {
		assumptions = append(assumptions, "已根据视频分析启用重点时间窗："+text)
	}
	if len(assumptions) > 0 {
		out["assumptions"] = assumptions
	}
	if errText := strings.TrimSpace(stringFromAny(eventMeta["error"])); errText != "" {
		out["risk_notice"] = []string{errText}
	}
	if advanced := mapFromAny(eventMeta["advanced_options_v1"]); len(advanced) > 0 {
		out["advanced_options"] = advanced
	}
	if strategyProfile := mapFromAny(eventMeta["strategy_profile_v1"]); len(strategyProfile) > 0 {
		out["applied_strategy_profile"] = strategyProfile
	}
	if strategyTrace := mapFromAny(eventMeta["strategy_profile_trace_v1"]); len(strategyTrace) > 0 {
		out["applied_strategy_trace"] = strategyTrace
	}
	return out
}

func buildImageAI2Instruction(schemaVersion string, executablePlan map[string]interface{}, eventMeta map[string]interface{}) map[string]interface{} {
	targetFormat := strings.ToLower(strings.TrimSpace(stringFromAny(executablePlan["target_format"])))
	targetFormat = NormalizeRequestedFormat(targetFormat)
	if targetFormat == "" {
		targetFormat = "png"
	}
	qualityWeights := normalizeAI2QualityWeightsAny(eventMeta["quality_weights"])
	if len(qualityWeights) == 0 {
		qualityWeights = normalizeDirectiveQualityWeights(map[string]float64{})
	}
	riskFlags := normalizeAI2RiskFlags(stringSliceFromAny(eventMeta["risk_flags"]))
	visualFocusArea := "auto"
	if focus := mapFromAny(executablePlan["focus_window"]); len(focus) > 0 {
		if floatFromAny(focus["end_sec"]) > floatFromAny(focus["start_sec"]) {
			visualFocusArea = "center"
		}
	}
	subjectTracking := "auto"
	for _, item := range stringSliceFromAny(executablePlan["must_capture"]) {
		if strings.Contains(item, "人物") || strings.Contains(item, "主角") || strings.Contains(item, "person") || strings.Contains(item, "subject") {
			subjectTracking = "primary_subject_auto"
			break
		}
	}
	maxBlurTolerance := normalizeAI1MaxBlurTolerance("", targetFormat)
	technicalReject := map[string]interface{}{
		"max_blur_tolerance": maxBlurTolerance,
		"avoid_watermarks":   true,
		"avoid_extreme_dark": true,
	}
	if eventTechnicalReject := mapFromAny(eventMeta["technical_reject"]); len(eventTechnicalReject) > 0 {
		for key, value := range eventTechnicalReject {
			technicalReject[key] = value
		}
	}
	rhythmTrajectory := normalizeAI1RhythmTrajectory("", targetFormat)
	strategyProfile := mapFromAny(eventMeta["strategy_profile_v1"])
	advancedOptions := mapFromAny(eventMeta["advanced_options_v1"])
	postprocess := mapFromAny(eventMeta["postprocess"])
	operatorIdentity := strings.TrimSpace(firstNonEmptyString(
		stringFromAny(eventMeta["operator_identity"]),
		stringFromAny(strategyProfile["operator_identity"]),
	))
	candidateCountBias := mapFromAny(eventMeta["candidate_count_bias"])
	if len(candidateCountBias) == 0 {
		candidateCountBias = mapFromAny(strategyProfile["candidate_count_bias"])
	}
	if len(candidateCountBias) == 0 {
		candidateCountBias = mapFromAny(executablePlan["candidate_count_bias"])
	}
	if len(candidateCountBias) > 0 {
		minCount, maxCount := normalizeCandidateCountBias(
			intFromAny(candidateCountBias["min"]),
			intFromAny(candidateCountBias["max"]),
			targetFormat,
		)
		candidateCountBias = map[string]interface{}{
			"min": minCount,
			"max": maxCount,
		}
	}
	out := map[string]interface{}{
		"schema_version":      AI2DirectiveSchemaV2,
		"instruction_version": schemaVersion,
		"target_format":       targetFormat,
		"objective":           strings.TrimSpace(stringFromAny(eventMeta["business_goal"])),
		"sampling_plan":       cloneMapStringKey(executablePlan),
		"quality_weights":     qualityWeights,
		"risk_flags":          riskFlags,
		"visual_focus_area":   visualFocusArea,
		"subject_tracking":    subjectTracking,
		"technical_reject":    technicalReject,
		"rhythm_trajectory":   rhythmTrajectory,
		"source":              "ai1_executable_plan",
	}
	if out["objective"] == "" {
		out["objective"] = "extract_high_quality_frames"
	}
	if operatorIdentity != "" {
		out["operator_identity"] = operatorIdentity
	}
	if len(candidateCountBias) > 0 {
		out["candidate_count_bias"] = candidateCountBias
	}
	if len(strategyProfile) > 0 {
		out["strategy_profile"] = strategyProfile
		if strategyTechnicalReject := mapFromAny(strategyProfile["technical_reject"]); len(strategyTechnicalReject) > 0 {
			mergedReject := mapFromAny(out["technical_reject"])
			for key, value := range strategyTechnicalReject {
				mergedReject[key] = value
			}
			mergedReject["max_blur_tolerance"] = normalizeAI1MaxBlurTolerance(
				stringFromAny(mergedReject["max_blur_tolerance"]),
				targetFormat,
			)
			if _, exists := mergedReject["avoid_watermarks"]; !exists {
				mergedReject["avoid_watermarks"] = true
			}
			if _, exists := mergedReject["avoid_extreme_dark"]; !exists {
				mergedReject["avoid_extreme_dark"] = true
			}
			out["technical_reject"] = mergedReject
		}
	}
	if mergedReject := mapFromAny(out["technical_reject"]); len(mergedReject) > 0 {
		mergedReject["max_blur_tolerance"] = normalizeAI1MaxBlurTolerance(
			stringFromAny(mergedReject["max_blur_tolerance"]),
			targetFormat,
		)
		if _, exists := mergedReject["avoid_watermarks"]; !exists {
			mergedReject["avoid_watermarks"] = true
		}
		if _, exists := mergedReject["avoid_extreme_dark"]; !exists {
			mergedReject["avoid_extreme_dark"] = true
		}
		out["technical_reject"] = mergedReject
	}
	if len(advancedOptions) > 0 {
		out["advanced_options"] = advancedOptions
	}
	if len(postprocess) > 0 {
		out["postprocess"] = postprocess
	}
	for _, key := range []string{"must_capture", "avoid", "style_direction"} {
		if value, ok := executablePlan[key]; ok && value != nil {
			out[key] = value
		}
	}
	return out
}

func (p *Processor) buildImageAI1ExecutablePlan(
	ctx context.Context,
	job models.VideoJob,
	sourcePath string,
	meta videoProbeMeta,
	requestedFormats []string,
	options jobOptions,
	qualitySettings QualitySettings,
) imageAI1ExecutablePlan {
	requestedFormat := firstRequestedFormat(requestedFormats)
	if requestedFormat == "" {
		requestedFormat = "png"
	}
	requestedFormat = NormalizeRequestedFormat(requestedFormat)
	schemaVersion := resolveImageAI1PlanSchemaVersion(requestedFormat)

	plan := imageAI1ExecutablePlan{
		SchemaVersion: schemaVersion,
		Schema:        buildImageAI1ExecutableSchema(schemaVersion, requestedFormat),
		Executable:    map[string]interface{}{},
		EventMeta: map[string]interface{}{
			"sub_stage":        "ai1_planning",
			"requested_format": requestedFormat,
			"target_format":    requestedFormat,
			"duration_sec":     roundTo(meta.DurationSec, 3),
			"width":            meta.Width,
			"height":           meta.Height,
			"fps":              roundTo(meta.FPS, 3),
		},
		DirectorSnapshot: map[string]interface{}{},
	}
	optionsPayload := parseJSONMap(job.Options)
	advancedOptions := ParseVideoJobAdvancedOptions(optionsPayload["ai1_advanced_options_v1"])
	advancedOptions, advancedSceneGuard := clampPNGMainlineAdvancedOptions(requestedFormat, advancedOptions)
	storedStrategyProfile := ParseStrategyProfileFromAny(optionsPayload["ai1_strategy_profile_v1"])
	strategyProfile, strategyTrace := p.resolveVideoJobAI1StrategyProfileWithOverrides(requestedFormat, advancedOptions, storedStrategyProfile)
	plan.EventMeta["advanced_options_v1"] = AdvancedOptionsToMap(advancedOptions)
	if len(advancedSceneGuard) > 0 {
		plan.EventMeta["advanced_scene_guard_v1"] = advancedSceneGuard
	}
	plan.EventMeta["strategy_profile_v1"] = StrategyProfileToMap(strategyProfile)
	if len(strategyTrace) > 0 {
		plan.EventMeta["strategy_profile_trace_v1"] = strategyTrace
	}
	if text := strings.TrimSpace(strategyProfile.BusinessGoal); text != "" {
		plan.EventMeta["business_goal"] = text
	}
	if text := strings.TrimSpace(strategyProfile.Audience); text != "" {
		plan.EventMeta["audience"] = text
	}
	if text := strings.TrimSpace(strategyProfile.OperatorIdentity); text != "" {
		plan.EventMeta["operator_identity"] = text
	}
	if text := strings.TrimSpace(strategyProfile.StyleDirection); text != "" {
		plan.EventMeta["style_direction"] = text
	}
	if strategyProfile.CandidateCountMin > 0 || strategyProfile.CandidateCountMax > 0 {
		minCount, maxCount := normalizeCandidateCountBias(
			strategyProfile.CandidateCountMin,
			strategyProfile.CandidateCountMax,
			requestedFormat,
		)
		plan.EventMeta["candidate_count_bias"] = map[string]interface{}{
			"min": minCount,
			"max": maxCount,
		}
	}
	if len(strategyProfile.MustCaptureBias) > 0 {
		plan.EventMeta["must_capture"] = strategyProfile.MustCaptureBias
	}
	if len(strategyProfile.AvoidBias) > 0 {
		plan.EventMeta["avoid"] = strategyProfile.AvoidBias
	}
	if len(strategyProfile.QualityWeights) > 0 {
		plan.EventMeta["quality_weights"] = strategyProfile.QualityWeights
	}
	if len(strategyProfile.RiskFlags) > 0 {
		plan.EventMeta["risk_flags"] = strategyProfile.RiskFlags
	}
	if hint := strings.TrimSpace(strategyProfile.DirectiveHint); hint != "" {
		plan.EventMeta["directive_text"] = hint
	}
	if advancedOptions.EnableMatting {
		plan.EventMeta["postprocess"] = map[string]interface{}{
			"enable_matting": true,
			"type":           "portrait_cutout",
			"output_alpha":   true,
		}
	}

	localSuggestion, localErr := suggestHighlightWindow(ctx, sourcePath, meta, qualitySettings)
	if localErr != nil {
		plan.EventMeta["local_focus_error"] = localErr.Error()
	} else if localSuggestion.Selected != nil {
		plan.EventMeta["local_focus_start_sec"] = roundTo(localSuggestion.Selected.StartSec, 3)
		plan.EventMeta["local_focus_end_sec"] = roundTo(localSuggestion.Selected.EndSec, 3)
		plan.EventMeta["local_focus_score"] = roundTo(localSuggestion.Selected.Score, 4)
	}

	directive, directorSnapshot, directorErr := p.requestAIGIFPromptDirective(ctx, job, sourcePath, meta, localSuggestion, qualitySettings)
	if len(directorSnapshot) > 0 {
		plan.DirectorSnapshot = directorSnapshot
	}

	aiReply := buildGenericAI1Reply(job.Title, requestedFormats, meta)
	if directorErr != nil {
		plan.EventMeta["error"] = directorErr.Error()
		plan.EventMeta["director_applied"] = false
	} else {
		plan.EventMeta["director_applied"] = true
	}
	if directive != nil {
		if reply := strings.TrimSpace(buildAIDirectorNaturalReply(directive)); reply != "" {
			aiReply = reply
		}
		if text := strings.TrimSpace(directive.BusinessGoal); text != "" {
			plan.EventMeta["business_goal"] = text
		}
		if text := strings.TrimSpace(directive.Audience); text != "" {
			plan.EventMeta["audience"] = text
		}
		if list := sanitizeTextList(directive.MustCapture, 12); len(list) > 0 {
			plan.EventMeta["must_capture"] = list
		}
		if list := sanitizeTextList(directive.Avoid, 12); len(list) > 0 {
			plan.EventMeta["avoid"] = list
		}
		if list := sanitizeTextList(directive.RiskFlags, 12); len(list) > 0 {
			plan.EventMeta["risk_flags"] = list
		}
		if len(directive.QualityWeights) > 0 {
			plan.EventMeta["quality_weights"] = directive.QualityWeights
		}
		if directive.LoopPreference > 0 {
			plan.EventMeta["loop_preference"] = roundTo(directive.LoopPreference, 4)
		}
		if text := strings.TrimSpace(directive.StyleDirection); text != "" {
			plan.EventMeta["style_direction"] = text
		}
		if text := strings.TrimSpace(directive.DirectiveText); text != "" {
			plan.EventMeta["directive_text"] = text
		}
		if directive.ClipCountMin > 0 {
			plan.EventMeta["clip_count_min"] = directive.ClipCountMin
		}
		if directive.ClipCountMax > 0 {
			plan.EventMeta["clip_count_max"] = directive.ClipCountMax
		}
	}
	if len(strategyProfile.MustCaptureBias) > 0 {
		plan.EventMeta["must_capture"] = mergeTextHints(stringSliceFromAny(plan.EventMeta["must_capture"]), strategyProfile.MustCaptureBias, 16)
	}
	if len(strategyProfile.AvoidBias) > 0 {
		plan.EventMeta["avoid"] = mergeTextHints(stringSliceFromAny(plan.EventMeta["avoid"]), strategyProfile.AvoidBias, 16)
	}
	if style := strings.TrimSpace(stringFromAny(plan.EventMeta["style_direction"])); style == "" && strategyProfile.StyleDirection != "" {
		plan.EventMeta["style_direction"] = strategyProfile.StyleDirection
	}
	if identity := strings.TrimSpace(stringFromAny(plan.EventMeta["operator_identity"])); identity == "" && strategyProfile.OperatorIdentity != "" {
		plan.EventMeta["operator_identity"] = strategyProfile.OperatorIdentity
	}
	if audience := strings.TrimSpace(stringFromAny(plan.EventMeta["audience"])); audience == "" && strategyProfile.Audience != "" {
		plan.EventMeta["audience"] = strategyProfile.Audience
	}
	if goal := strings.TrimSpace(stringFromAny(plan.EventMeta["business_goal"])); goal == "" && strategyProfile.BusinessGoal != "" {
		plan.EventMeta["business_goal"] = strategyProfile.BusinessGoal
	}
	if hint := strings.TrimSpace(stringFromAny(plan.EventMeta["directive_text"])); hint == "" && strategyProfile.DirectiveHint != "" {
		plan.EventMeta["directive_text"] = strategyProfile.DirectiveHint
	}
	plan.EventMeta["risk_flags"] = mergeRiskFlags(
		stringSliceFromAny(plan.EventMeta["risk_flags"]),
		strategyProfile.RiskFlags,
	)
	plan.EventMeta["quality_weights"] = blendQualityWeights(
		normalizeAI2QualityWeightsAny(plan.EventMeta["quality_weights"]),
		strategyProfile.QualityWeights,
		0.7,
	)
	if advancedOptions.EnableMatting {
		plan.EventMeta["risk_flags"] = mergeRiskFlags(stringSliceFromAny(plan.EventMeta["risk_flags"]), []string{"matting_requested"})
		plan.EventMeta["must_capture"] = mergeTextHints(stringSliceFromAny(plan.EventMeta["must_capture"]), []string{"主体边缘清晰", "人物主体完整"}, 16)
	}
	if overrideReport := applyImageAI1StrategyHardOverrides(plan.EventMeta, strategyProfile, requestedFormat); len(overrideReport) > 0 {
		plan.EventMeta["strategy_override_report_v1"] = overrideReport
	}
	if strategyProfile.SceneLabel != "" && strategyProfile.Scene != AdvancedScenarioDefault {
		aiReply = strings.TrimSpace(aiReply + " 已按「" + strategyProfile.SceneLabel + "」策略组优化。")
	}
	plan.EventMeta["ai_reply"] = aiReply

	targetCount := clampImageTargetCount(options.MaxStatic)
	if bias := mapFromAny(plan.EventMeta["candidate_count_bias"]); len(bias) > 0 {
		biasMin, biasMax := normalizeCandidateCountBias(
			intFromAny(bias["min"]),
			intFromAny(bias["max"]),
			requestedFormat,
		)
		if targetCount > biasMax {
			targetCount = biasMax
		}
		if targetCount < biasMin {
			targetCount = biasMin
		}
	}
	if maxHint := intFromAny(plan.EventMeta["clip_count_max"]); maxHint > 0 {
		hintTarget := clampImageTargetCount(maxHint * 2)
		if hintTarget > 0 && hintTarget < targetCount {
			targetCount = hintTarget
		}
	}

	focusStart, focusEnd, focusSource := resolveImageFocusWindow(meta, localSuggestion)
	mode := "uniform_sampling"
	if focusEnd > focusStart {
		mode = "focus_window"
	}
	intervalSec := resolveImagePlanInterval(options.FrameIntervalSec, targetCount, meta, focusStart, focusEnd)

	plan.Executable = map[string]interface{}{
		"target_format":      requestedFormat,
		"mode":               mode,
		"target_count":       targetCount,
		"frame_interval_sec": intervalSec,
	}
	if advanced := mapFromAny(plan.EventMeta["advanced_options_v1"]); len(advanced) > 0 {
		plan.Executable["advanced_options"] = advanced
	}
	if strategy := mapFromAny(plan.EventMeta["strategy_profile_v1"]); len(strategy) > 0 {
		plan.Executable["strategy_profile"] = strategy
	}
	if candidateCountBias := mapFromAny(plan.EventMeta["candidate_count_bias"]); len(candidateCountBias) > 0 {
		plan.Executable["candidate_count_bias"] = candidateCountBias
	}
	if postprocess := mapFromAny(plan.EventMeta["postprocess"]); len(postprocess) > 0 {
		plan.Executable["postprocess"] = postprocess
	}
	if mode == "focus_window" {
		plan.Executable["focus_window"] = map[string]interface{}{
			"start_sec": focusStart,
			"end_sec":   focusEnd,
			"source":    focusSource,
		}
		plan.EventMeta["selected_start_sec"] = focusStart
		plan.EventMeta["selected_end_sec"] = focusEnd
		plan.EventMeta["focus_window_source"] = focusSource
	}
	if list := stringSliceFromAny(plan.EventMeta["must_capture"]); len(list) > 0 {
		plan.Executable["must_capture"] = list
	}
	if list := stringSliceFromAny(plan.EventMeta["avoid"]); len(list) > 0 {
		plan.Executable["avoid"] = list
	}
	if style := strings.TrimSpace(stringFromAny(plan.EventMeta["style_direction"])); style != "" {
		plan.Executable["style_direction"] = style
	}
	if operatorIdentity := strings.TrimSpace(stringFromAny(plan.EventMeta["operator_identity"])); operatorIdentity != "" {
		plan.Executable["operator_identity"] = operatorIdentity
	}
	userFeedback := buildAI1UserFeedbackV2(plan.EventMeta, requestedFormats, meta, plan.Executable)
	interactiveAction := strings.ToLower(strings.TrimSpace(stringFromAny(userFeedback["interactive_action"])))
	if interactiveAction == "" {
		interactiveAction = "proceed"
	}
	plan.EventMeta["interactive_action"] = interactiveAction
	plan.EventMeta["clarify_questions"] = stringSliceFromAny(userFeedback["clarify_questions"])
	plan.EventMeta["ai1_confidence"] = roundTo(floatFromAny(userFeedback["confidence"]), 4)
	return plan
}

func shouldPauseForAI1NeedClarify(eventMeta map[string]interface{}, ai1Confirmed, ai1PauseConsumed bool) bool {
	if ai1Confirmed || ai1PauseConsumed {
		return false
	}
	if len(eventMeta) == 0 {
		return false
	}
	action := strings.ToLower(strings.TrimSpace(stringFromAny(eventMeta["interactive_action"])))
	return action == "need_clarify"
}

func clampPNGMainlineAdvancedOptions(
	requestedFormat string,
	options VideoJobAdvancedOptions,
) (VideoJobAdvancedOptions, map[string]interface{}) {
	normalized := NormalizeVideoJobAdvancedOptions(options)
	if NormalizeRequestedFormat(requestedFormat) != "png" {
		return normalized, nil
	}

	allowedScenes := map[string]struct{}{
		AdvancedScenarioDefault:     {},
		AdvancedScenarioXiaohongshu: {},
	}
	scene := NormalizeAdvancedScenario(normalized.Scene)
	if _, ok := allowedScenes[scene]; ok {
		normalized.Scene = scene
		return normalized, nil
	}

	before := scene
	if before == "" {
		before = AdvancedScenarioDefault
	}
	normalized.Scene = AdvancedScenarioDefault
	report := map[string]interface{}{
		"version":         "png_mainline_scene_guard_v1",
		"target_format":   "png",
		"requested_scene": before,
		"applied_scene":   normalized.Scene,
		"allowed_scenes": []string{
			AdvancedScenarioDefault,
			AdvancedScenarioXiaohongshu,
		},
		"reason": "png_mainline_only_default_xiaohongshu",
	}
	return normalized, report
}

func applyImageAI1StrategyHardOverrides(
	eventMeta map[string]interface{},
	strategyProfile VideoJobAI1StrategyProfile,
	requestedFormat string,
) map[string]interface{} {
	if len(eventMeta) == 0 {
		return nil
	}
	overrides := make([]map[string]interface{}, 0, 6)
	addOverride := func(field string, before interface{}, after interface{}, action string, reason string) {
		overrides = append(overrides, map[string]interface{}{
			"field":  strings.TrimSpace(field),
			"before": before,
			"after":  after,
			"action": strings.TrimSpace(action),
			"reason": strings.TrimSpace(reason),
		})
	}
	applyLock := func(field string, lockedValue string, reason string, forceWhenDifferent bool) {
		lockedValue = strings.TrimSpace(lockedValue)
		if lockedValue == "" {
			return
		}
		current := strings.TrimSpace(stringFromAny(eventMeta[field]))
		if current == "" {
			eventMeta[field] = lockedValue
			addOverride(field, current, lockedValue, "fill_empty", reason)
			return
		}
		if forceWhenDifferent && !strings.EqualFold(current, lockedValue) {
			eventMeta[field] = lockedValue
			addOverride(field, current, lockedValue, "force_override", reason)
		}
	}
	applyWeightsLock := func(field string, lockedValue map[string]float64, reason string, forceWhenDifferent bool) {
		if !hasPositiveQualityWeights(lockedValue) {
			return
		}
		target := normalizeDirectiveQualityWeights(lockedValue)
		current := normalizeAI2QualityWeightsAny(eventMeta[field])
		if len(current) == 0 {
			eventMeta[field] = target
			addOverride(field, current, target, "fill_empty", reason)
			return
		}
		if forceWhenDifferent && !isSameQualityWeights(current, target) {
			eventMeta[field] = target
			addOverride(field, current, target, "force_override", reason)
		}
	}

	normalizedFormat := NormalizeRequestedFormat(strings.TrimSpace(requestedFormat))
	scene := NormalizeAdvancedScenario(strategyProfile.Scene)
	applyLock("style_direction", strategyProfile.StyleDirection, "scene_strategy_lock_style_direction", true)
	if scene != "" && scene != AdvancedScenarioDefault {
		applyLock("business_goal", strategyProfile.BusinessGoal, "scene_strategy_lock_business_goal", true)
		applyLock("audience", strategyProfile.Audience, "scene_strategy_lock_audience", true)
	}
	applyLock("operator_identity", strategyProfile.OperatorIdentity, "scene_strategy_fill_operator_identity", false)
	if normalizedFormat == "png" && (scene == AdvancedScenarioDefault || scene == AdvancedScenarioXiaohongshu) {
		applyWeightsLock("quality_weights", strategyProfile.QualityWeights, "png_mainline_scene_lock_quality_weights", true)
	}

	report := map[string]interface{}{
		"version":        "ai1_strategy_override_v1",
		"lock_source":    "strategy_profile_v1",
		"target_format":  normalizedFormat,
		"scene":          scene,
		"scene_label":    strings.TrimSpace(strategyProfile.SceneLabel),
		"overrides":      overrides,
		"override_count": len(overrides),
		"has_override":   len(overrides) > 0,
	}
	return report
}

func isSameQualityWeights(a, b map[string]float64) bool {
	a = normalizeDirectiveQualityWeights(a)
	b = normalizeDirectiveQualityWeights(b)
	for _, key := range []string{"semantic", "clarity", "loop", "efficiency"} {
		if roundTo(a[key], 4) != roundTo(b[key], 4) {
			return false
		}
	}
	return true
}

func applyImageAI1ExecutablePlan(
	executable map[string]interface{},
	meta videoProbeMeta,
	options *jobOptions,
	optionsPayload map[string]interface{},
	schemaVersion string,
) map[string]interface{} {
	if options == nil {
		return map[string]interface{}{}
	}
	if optionsPayload == nil {
		optionsPayload = map[string]interface{}{}
	}
	if len(executable) == 0 {
		return map[string]interface{}{}
	}

	applied := map[string]interface{}{}
	targetCount := clampImageTargetCount(intFromAny(executable["target_count"]))
	if targetCount > 0 {
		options.MaxStatic = targetCount
		optionsPayload["max_static"] = targetCount
		applied["max_static"] = targetCount
	}

	focusStart := 0.0
	focusEnd := 0.0
	if focus := mapFromAny(executable["focus_window"]); len(focus) > 0 {
		focusStart = floatFromAny(focus["start_sec"])
		focusEnd = floatFromAny(focus["end_sec"])
		if meta.DurationSec > 0 {
			focusStart, focusEnd = clampHighlightWindow(focusStart, focusEnd, meta.DurationSec)
		}
		if focusEnd > focusStart {
			options.StartSec = roundTo(focusStart, 3)
			options.EndSec = roundTo(focusEnd, 3)
			optionsPayload["start_sec"] = options.StartSec
			optionsPayload["end_sec"] = options.EndSec
			applied["start_sec"] = options.StartSec
			applied["end_sec"] = options.EndSec
		}
	}

	interval := floatFromAny(executable["frame_interval_sec"])
	if interval <= 0 {
		interval = resolveImagePlanInterval(options.FrameIntervalSec, targetCount, meta, focusStart, focusEnd)
	}
	interval = roundTo(clampFloat(interval, 0.2, 8), 3)
	options.FrameIntervalSec = interval
	optionsPayload["frame_interval_sec"] = interval
	applied["frame_interval_sec"] = interval

	if mode := strings.TrimSpace(stringFromAny(executable["mode"])); mode != "" {
		optionsPayload["ai1_plan_mode"] = mode
		applied["mode"] = mode
	}

	optionsPayload["ai1_plan_applied"] = true
	optionsPayload["ai1_plan_applied_at"] = time.Now().Format(time.RFC3339)
	optionsPayload["ai1_executable_plan_v1"] = executable
	if schemaVersion != "" {
		optionsPayload["ai1_plan_schema_version"] = schemaVersion
	}
	return applied
}

func (p *Processor) persistImageAI1Plan(
	job models.VideoJob,
	requestedFormats []string,
	flowMode string,
	ai1Confirmed bool,
	meta videoProbeMeta,
	eventStage string,
	eventMeta map[string]interface{},
	directorSnapshot map[string]interface{},
	schemaVersion string,
	executableSchema map[string]interface{},
	executablePlan map[string]interface{},
) {
	if p == nil || p.db == nil || job.ID == 0 {
		return
	}
	if schemaVersion == "" {
		schemaVersion = resolveImageAI1PlanSchemaVersion(firstRequestedFormat(requestedFormats))
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
		"error":       strings.TrimSpace(stringFromAny(eventMeta["error"])),
	}
	if inputMode := strings.TrimSpace(stringFromAny(directorSnapshot["director_input_mode_applied"])); inputMode != "" {
		trace["director_input_mode"] = inputMode
	}
	if inputSource := strings.TrimSpace(stringFromAny(directorSnapshot["director_input_source"])); inputSource != "" {
		trace["director_input_source"] = inputSource
	}

	ai1OutputV2 := buildAI1OutputV2(
		schemaVersion,
		requestedFormats,
		meta,
		eventMeta,
		executablePlan,
		trace,
	)
	ai1OutputV2 = validateAndRepairAI1OutputV2(
		ai1OutputV2,
		schemaVersion,
		requestedFormats,
		meta,
		eventMeta,
		executablePlan,
		trace,
	)
	traceV2 := mapFromAny(ai1OutputV2["trace"])
	if len(traceV2) > 0 {
		trace = traceV2
	}
	userFeedbackV2 := mapFromAny(ai1OutputV2["user_feedback"])
	ai2DirectiveV2 := mapFromAny(ai1OutputV2["ai2_directive"])
	if len(userFeedbackV2) == 0 {
		userFeedbackV2 = buildImageAI1UserReply(eventMeta, meta, requestedFormats)
	}
	if len(ai2DirectiveV2) == 0 {
		ai2DirectiveV2 = buildImageAI2Instruction(schemaVersion, executablePlan, eventMeta)
	}

	planPayload := map[string]interface{}{
		"plan_revision":     1,
		"schema_version":    schemaVersion,
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
		"executable_schema": cloneMapStringKey(executableSchema),
		"executable_plan":   cloneMapStringKey(executablePlan),
		"ai1_output_v2":     ai1OutputV2,
		"ai1_output_v1": map[string]interface{}{
			"schema_version":  schemaVersion,
			"user_reply":      userFeedbackV2,
			"ai2_instruction": ai2DirectiveV2,
		},
	}

	status := resolveAI1PlanStatus(flowMode, ai1Confirmed)
	row := models.VideoJobAI1Plan{
		JobID:           job.ID,
		UserID:          job.UserID,
		RequestedFormat: requestedFormat,
		SchemaVersion:   schemaVersion,
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

func countFrameCandidateMustCaptureHits(rows []frameQualityCandidateScore) int {
	if len(rows) == 0 {
		return 0
	}
	total := 0
	for _, row := range rows {
		total += len(row.MustCaptureHits)
	}
	return total
}

func countFrameCandidateAvoidHits(rows []frameQualityCandidateScore) int {
	if len(rows) == 0 {
		return 0
	}
	total := 0
	for _, row := range rows {
		total += len(row.AvoidHits)
	}
	return total
}

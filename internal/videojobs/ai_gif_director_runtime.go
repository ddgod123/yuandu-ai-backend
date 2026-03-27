package videojobs

import (
	"context"
	"encoding/json"
	"strings"

	"emoji/internal/models"
)

type aiGIFDirectorRuntime struct {
	ctx             context.Context
	job             models.VideoJob
	sourcePath      string
	meta            videoProbeMeta
	qualitySettings QualitySettings
	targetFormat    string

	operatorInstructionRaw        string
	operatorInstructionRendered   string
	operatorInstructionSchema     map[string]interface{}
	operatorInstructionRenderMode string
	operatorVersion               string
	operatorSource                string
	operatorEnabled               bool

	directorInputModeRequested string
	directorInputModeApplied   string
	directorInputSource        string

	sourceVideoURL      string
	sourceVideoURLError string
	frameSamplingError  string
	frameSamples        []aiDirectorFrameSample
	frameManifest       []aiGIFFrameManifestEntry
}

func (p *Processor) loadAIGIFDirectorFrameSamples(rt *aiGIFDirectorRuntime) {
	if p == nil || rt == nil {
		return
	}
	if len(rt.frameSamples) > 0 || rt.frameSamplingError != "" {
		return
	}
	samples, err := sampleAIDirectorFrames(rt.ctx, rt.sourcePath, rt.meta, 6)
	if err != nil {
		rt.frameSamplingError = err.Error()
		rt.frameSamples = nil
		rt.frameManifest = nil
		return
	}
	rt.frameSamples = samples
	rt.frameManifest = buildAIGIFFrameManifest(rt.frameSamples)
}

func (p *Processor) resolveAIGIFDirectorSourceVideoURL(rt *aiGIFDirectorRuntime) bool {
	if p == nil || rt == nil {
		return false
	}
	sourceKey := strings.TrimSpace(rt.job.SourceVideoKey)
	if sourceKey == "" {
		rt.sourceVideoURLError = "source_video_key missing"
		return false
	}
	videoURL, err := p.buildObjectReadURL(sourceKey)
	if err != nil {
		rt.sourceVideoURLError = err.Error()
		return false
	}
	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" {
		rt.sourceVideoURLError = "empty source video url"
		return false
	}
	rt.sourceVideoURL = videoURL
	return true
}

func (p *Processor) resolveAIGIFDirectorInputMode(rt *aiGIFDirectorRuntime) {
	if p == nil || rt == nil {
		return
	}
	switch rt.directorInputModeRequested {
	case "frames":
		p.loadAIGIFDirectorFrameSamples(rt)
		rt.directorInputModeApplied = "frames"
		rt.directorInputSource = "frame_manifest"
	case "full_video":
		if p.resolveAIGIFDirectorSourceVideoURL(rt) {
			rt.directorInputModeApplied = "full_video"
			rt.directorInputSource = "full_video_url"
		} else {
			p.loadAIGIFDirectorFrameSamples(rt)
			rt.directorInputModeApplied = "frames"
			rt.directorInputSource = "full_video_fallback_frame_manifest"
		}
	default: // hybrid
		if p.resolveAIGIFDirectorSourceVideoURL(rt) {
			rt.directorInputModeApplied = "full_video"
			rt.directorInputSource = "full_video_url"
		} else {
			p.loadAIGIFDirectorFrameSamples(rt)
			rt.directorInputModeApplied = "frames"
			rt.directorInputSource = "hybrid_fallback_frame_manifest"
		}
	}
}

func (p *Processor) buildAIGIFDirectorPayloads(rt *aiGIFDirectorRuntime) (aiGIFDirectorModelPayload, map[string]interface{}) {
	if p == nil || rt == nil {
		return aiGIFDirectorModelPayload{}, map[string]interface{}{}
	}
	sourcePrompt := resolveVideoJobSourcePrompt(rt.job)
	modelPayload := aiGIFDirectorModelPayload{
		SchemaVersion: "ai1_input_v2",
		Task: aiGIFDirectorTaskPayload{
			AssetGoal:          resolveAIDirectorAssetGoal(rt.targetFormat),
			BusinessScene:      "social_spread",
			DeliveryGoal:       resolveAIDirectorDeliveryGoal(rt.targetFormat),
			OptimizationTarget: resolveAIDirectorOptimizationTarget(rt.qualitySettings.GIFProfile),
			CostSensitivity:    resolveAIDirectorCostSensitivity(rt.qualitySettings.GIFTargetSizeKB),
			HardConstraints:    resolveAIDirectorTaskConstraints(rt.meta, rt.qualitySettings),
			OperatorInstruction: aiGIFDirectorOperatorInstructionMeta{
				Enabled: rt.operatorEnabled,
				Version: rt.operatorVersion,
			},
			RequestedFormat: rt.targetFormat,
		},
		Source: aiGIFDirectorSourcePayload{
			Title:          strings.TrimSpace(rt.job.Title),
			UserPrompt:     sourcePrompt,
			SourceVideoKey: strings.TrimSpace(rt.job.SourceVideoKey),
			DurationSec:    roundTo(rt.meta.DurationSec, 3),
			Width:          rt.meta.Width,
			Height:         rt.meta.Height,
			FPS:            roundTo(rt.meta.FPS, 3),
			AspectRatio:    resolveAIDirectorSourceAspectRatio(rt.meta.Width, rt.meta.Height),
			Orientation:    resolveAIDirectorSourceOrientation(rt.meta.Width, rt.meta.Height),
			InputMode:      rt.directorInputModeApplied,
			FrameRefs:      rt.frameManifest,
		},
		RiskHints: resolveAIDirectorRiskHints(rt.meta),
	}
	if rt.sourceVideoURL != "" && strings.EqualFold(rt.directorInputModeApplied, "full_video") {
		modelPayload.Source.VideoSourceKind = "full_video_url_attached_in_content_part"
	}

	debugPayload := map[string]interface{}{
		"job_id":                      rt.job.ID,
		"title":                       strings.TrimSpace(rt.job.Title),
		"source_prompt":               sourcePrompt,
		"duration_sec":                roundTo(rt.meta.DurationSec, 3),
		"width":                       rt.meta.Width,
		"height":                      rt.meta.Height,
		"fps":                         roundTo(rt.meta.FPS, 3),
		"gif_profile":                 strings.ToLower(strings.TrimSpace(rt.qualitySettings.GIFProfile)),
		"gif_target_size_kb":          rt.qualitySettings.GIFTargetSizeKB,
		"source_input_mode_requested": rt.directorInputModeRequested,
		"source_input_mode_applied":   rt.directorInputModeApplied,
		"source_input_type":           rt.directorInputSource,
		"frame_count":                 len(rt.frameManifest),
		"frame_manifest":              rt.frameManifest,
		"source_video_url_available":  rt.sourceVideoURL != "",
		"source_video_url_error":      rt.sourceVideoURLError,
		"operator_instruction": map[string]interface{}{
			"enabled":       rt.operatorEnabled,
			"version":       rt.operatorVersion,
			"render_mode":   rt.operatorInstructionRenderMode,
			"text":          rt.operatorInstructionRendered,
			"raw_text":      rt.operatorInstructionRaw,
			"schema_fields": rt.operatorInstructionSchema,
		},
	}
	if rt.sourceVideoURL != "" {
		debugPayload["source_video_url"] = rt.sourceVideoURL
	}
	return modelPayload, debugPayload
}

func buildAIGIFDirectorUserParts(rt *aiGIFDirectorRuntime, modelPayload aiGIFDirectorModelPayload) ([]openAICompatContentPart, []byte) {
	userBytes, _ := json.Marshal(modelPayload)
	parts := make([]openAICompatContentPart, 0, len(rt.frameSamples)+2)
	parts = append(parts, openAICompatContentPart{
		Type: "text",
		Text: string(userBytes),
	})
	if rt.sourceVideoURL != "" && strings.EqualFold(rt.directorInputModeApplied, "full_video") {
		parts = append(parts, openAICompatContentPart{
			Type: "video_url",
			VideoURL: &openAICompatVideoURL{
				URL: rt.sourceVideoURL,
			},
		})
		return parts, userBytes
	}
	for _, item := range rt.frameSamples {
		if strings.TrimSpace(item.DataURL) == "" {
			continue
		}
		parts = append(parts, openAICompatContentPart{
			Type: "image_url",
			ImageURL: &openAICompatImageURL{
				URL: item.DataURL,
			},
		})
	}
	return parts, userBytes
}

func buildAIGIFDirectivePersistContext(
	rt *aiGIFDirectorRuntime,
	cfg aiModelCallConfig,
	modelPayload aiGIFDirectorModelPayload,
	status string,
	fallbackUsed bool,
	raw map[string]interface{},
	fallbackReason string,
) aiGIFDirectivePersistContext {
	if rt == nil {
		return aiGIFDirectivePersistContext{
			Status:       status,
			FallbackUsed: fallbackUsed,
		}
	}
	return aiGIFDirectivePersistContext{
		Status:              status,
		FallbackUsed:        fallbackUsed,
		BriefVersion:        resolveAIDirectiveBriefVersion(rt.operatorVersion, cfg.PromptVersion),
		ModelVersion:        resolveAIDirectiveModelVersion(cfg.Model, raw),
		InputContext:        modelPayload,
		OperatorEnabled:     rt.operatorEnabled,
		OperatorInstruction: rt.operatorInstructionRendered,
		OperatorVersion:     rt.operatorVersion,
		FallbackReason:      fallbackReason,
	}
}

func (p *Processor) callAndRecordAIGIFDirector(
	rt *aiGIFDirectorRuntime,
	systemPrompt string,
	attempt int,
	cfg aiModelCallConfig,
	modelPayload aiGIFDirectorModelPayload,
	debugPayload map[string]interface{},
	modelPayloadBytes []byte,
	parts []openAICompatContentPart,
	fixedPromptVersion string,
	fixedPromptSource string,
	contractTailVersion string,
) (string, cloudHighlightUsage, map[string]interface{}, int64, error) {
	if p == nil || rt == nil {
		return "", cloudHighlightUsage{}, nil, 0, nil
	}
	modelText, usage, rawResp, durationMs, err := p.callOpenAICompatJSONChatWithUserParts(rt.ctx, cfg, systemPrompt, parts)
	debugPayloadBytes, _ := json.Marshal(debugPayload)
	usageMetadata := aiGIFDirectorUsageMetadata{
		Attempt:                        attempt,
		PromptVersion:                  cfg.PromptVersion,
		FixedPromptVersion:             fixedPromptVersion,
		FixedPromptSource:              fixedPromptSource,
		FixedPromptContractVersion:     contractTailVersion,
		CandidateSource:                rt.directorInputSource,
		DirectorInputModeRequested:     rt.directorInputModeRequested,
		DirectorInputModeApplied:       rt.directorInputModeApplied,
		FrameCount:                     len(rt.frameSamples),
		FrameSamplingError:             rt.frameSamplingError,
		SourceVideoURLAvailable:        rt.sourceVideoURL != "",
		SourceVideoURLError:            rt.sourceVideoURLError,
		OperatorInstructionEnabled:     rt.operatorEnabled,
		OperatorInstructionVersion:     rt.operatorVersion,
		OperatorInstructionRawLen:      len(rt.operatorInstructionRaw),
		OperatorInstructionLen:         len(rt.operatorInstructionRendered),
		OperatorInstructionRenderedLen: len(rt.operatorInstructionRendered),
		OperatorInstructionRenderMode:  rt.operatorInstructionRenderMode,
		OperatorInstructionSource:      rt.operatorSource,
		OperatorInstructionSchema:      rt.operatorInstructionSchema,
		DirectorPayloadSchemaVersion:   modelPayload.SchemaVersion,
		DirectorModelPayloadV2:         modelPayload,
		DirectorModelPayloadBytes:      len(modelPayloadBytes),
		DirectorDebugContextV1:         debugPayload,
		DirectorDebugContextBytes:      len(debugPayloadBytes),
		DirectorInputPayloadV1:         debugPayload,
		DirectorInputPayloadBytes:      len(debugPayloadBytes),
		SystemPromptText:               systemPrompt,
		UserPartsShapeV1:               summarizeOpenAICompatContentParts(parts),
	}
	status := "ok"
	errText := ""
	if err != nil {
		status = "error"
		errText = err.Error()
	}
	p.recordVideoJobAIUsage(videoJobAIUsageInput{
		JobID:             rt.job.ID,
		UserID:            rt.job.UserID,
		Stage:             gifAIDirectorStage,
		Provider:          cfg.Provider,
		Model:             cfg.Model,
		Endpoint:          cfg.Endpoint,
		InputTokens:       usage.InputTokens,
		OutputTokens:      usage.OutputTokens,
		CachedInputTokens: usage.CachedInputTokens,
		ImageTokens:       usage.ImageTokens,
		VideoTokens:       usage.VideoTokens,
		AudioSeconds:      usage.AudioSeconds,
		RequestDurationMs: durationMs,
		RequestStatus:     status,
		RequestError:      errText,
		Metadata:          usageMetadata,
	})
	return modelText, usage, rawResp, durationMs, err
}

package videojobs

import (
	"encoding/json"
	"strings"

	"emoji/internal/models"
)

type aiGIFFrameManifestEntry struct {
	Index        int     `json:"index"`
	TimestampSec float64 `json:"timestamp_sec"`
	Bytes        int     `json:"bytes"`
}

func buildAIGIFFrameManifest(samples []aiDirectorFrameSample) []aiGIFFrameManifestEntry {
	if len(samples) == 0 {
		return nil
	}
	out := make([]aiGIFFrameManifestEntry, 0, len(samples))
	for _, item := range samples {
		out = append(out, aiGIFFrameManifestEntry{
			Index:        item.Index,
			TimestampSec: roundTo(item.TimestampSec, 3),
			Bytes:        item.Bytes,
		})
	}
	return out
}

type aiGIFDirectorOperatorInstructionMeta struct {
	Enabled bool   `json:"enabled"`
	Version string `json:"version,omitempty"`
}

type aiGIFDirectorTaskConstraints struct {
	TargetCountMin          int     `json:"target_count_min"`
	TargetCountMax          int     `json:"target_count_max"`
	PreferredDurationSecMin float64 `json:"duration_sec_min"`
	PreferredDurationSecMax float64 `json:"duration_sec_max"`
	RenderBudget            string  `json:"render_budget,omitempty"`
	InferenceBudget         string  `json:"inference_budget,omitempty"`
	MaxCandidateHints       int     `json:"max_candidate_hints,omitempty"`
	Profile                 string  `json:"profile,omitempty"`
}

type aiGIFDirectorTaskPayload struct {
	AssetGoal           string                               `json:"asset_goal"`
	BusinessScene       string                               `json:"business_scene"`
	DeliveryGoal        string                               `json:"delivery_goal"`
	OptimizationTarget  string                               `json:"optimization_target"`
	CostSensitivity     string                               `json:"cost_sensitivity"`
	HardConstraints     aiGIFDirectorTaskConstraints         `json:"hard_constraints"`
	OperatorInstruction aiGIFDirectorOperatorInstructionMeta `json:"operator_instruction"`
	RequestedFormat     string                               `json:"requested_format"`
}

type aiGIFDirectorSourcePayload struct {
	Title           string                    `json:"title"`
	UserPrompt      string                    `json:"user_prompt"`
	SourceVideoKey  string                    `json:"source_video_key"`
	DurationSec     float64                   `json:"duration_sec"`
	Width           int                       `json:"width"`
	Height          int                       `json:"height"`
	FPS             float64                   `json:"fps"`
	AspectRatio     string                    `json:"aspect_ratio,omitempty"`
	Orientation     string                    `json:"orientation,omitempty"`
	InputMode       string                    `json:"input_mode"`
	FrameRefs       []aiGIFFrameManifestEntry `json:"frame_refs,omitempty"`
	VideoSourceKind string                    `json:"video_source_kind,omitempty"`
}

type aiGIFDirectorModelPayload struct {
	SchemaVersion string                     `json:"schema_version"`
	Task          aiGIFDirectorTaskPayload   `json:"task"`
	Source        aiGIFDirectorSourcePayload `json:"source"`
	RiskHints     []string                   `json:"risk_hints,omitempty"`
}

type aiGIFPlannerHardConstraintPolicy struct {
	BaseTopN        int     `json:"base_top_n"`
	AISuggestedTopN int     `json:"ai_suggested_top_n"`
	AllowedTopN     int     `json:"allowed_top_n"`
	AppliedTopN     int     `json:"applied_top_n"`
	OverrideEnabled bool    `json:"override_enabled"`
	ExpandRatio     float64 `json:"expand_ratio"`
	AbsoluteCap     int     `json:"absolute_cap"`
	ClampReason     string  `json:"clamp_reason,omitempty"`
	DurationTier    string  `json:"duration_tier"`
}

type aiGIFPlannerInputPayload struct {
	JobID                uint64                           `json:"job_id"`
	Title                string                           `json:"title"`
	DurationSec          float64                          `json:"duration_sec"`
	Width                int                              `json:"width"`
	Height               int                              `json:"height"`
	FPS                  float64                          `json:"fps"`
	TargetTopN           int                              `json:"target_top_n"`
	TargetWindowSec      float64                          `json:"target_window_sec"`
	FrameCount           int                              `json:"frame_count"`
	FrameManifest        []aiGIFFrameManifestEntry        `json:"frame_manifest,omitempty"`
	HardConstraintPolicy aiGIFPlannerHardConstraintPolicy `json:"hard_constraint_policy"`
	Director             *gifAIDirectiveProfile           `json:"director,omitempty"`
}

type aiGIFJudgeInputPayload struct {
	JobID      uint64           `json:"job_id"`
	SampleSize int              `json:"sample_size"`
	Outputs    []gifJudgeSample `json:"outputs"`
}

type aiGIFDirectorUsageMetadata struct {
	Attempt                        int                       `json:"attempt"`
	PromptVersion                  string                    `json:"prompt_version"`
	FixedPromptVersion             string                    `json:"fixed_prompt_version"`
	FixedPromptSource              string                    `json:"fixed_prompt_source"`
	FixedPromptContractVersion     string                    `json:"fixed_prompt_contract_version"`
	CandidateSource                string                    `json:"candidate_source"`
	DirectorInputModeRequested     string                    `json:"director_input_mode_requested"`
	DirectorInputModeApplied       string                    `json:"director_input_mode_applied"`
	FrameCount                     int                       `json:"frame_count"`
	FrameSamplingError             string                    `json:"frame_sampling_error,omitempty"`
	SourceVideoURLAvailable        bool                      `json:"source_video_url_available"`
	SourceVideoURLError            string                    `json:"source_video_url_error,omitempty"`
	OperatorInstructionEnabled     bool                      `json:"operator_instruction_enabled"`
	OperatorInstructionVersion     string                    `json:"operator_instruction_version"`
	OperatorInstructionRawLen      int                       `json:"operator_instruction_raw_len"`
	OperatorInstructionLen         int                       `json:"operator_instruction_len"`
	OperatorInstructionRenderedLen int                       `json:"operator_instruction_rendered_len"`
	OperatorInstructionRenderMode  string                    `json:"operator_instruction_render_mode"`
	OperatorInstructionSource      string                    `json:"operator_instruction_source"`
	OperatorInstructionSchema      map[string]interface{}    `json:"operator_instruction_schema,omitempty"`
	DirectorPayloadSchemaVersion   string                    `json:"director_payload_schema_version"`
	DirectorModelPayloadV2         aiGIFDirectorModelPayload `json:"director_model_payload_v2"`
	DirectorModelPayloadBytes      int                       `json:"director_model_payload_bytes"`
	DirectorDebugContextV1         map[string]interface{}    `json:"director_debug_context_v1"`
	DirectorDebugContextBytes      int                       `json:"director_debug_context_bytes"`
	DirectorInputPayloadV1         map[string]interface{}    `json:"director_input_payload_v1"`
	DirectorInputPayloadBytes      int                       `json:"director_input_payload_bytes"`
	SystemPromptText               string                    `json:"system_prompt_text"`
	UserPartsShapeV1               []map[string]interface{}  `json:"user_parts_shape_v1"`
}

type aiGIFPlannerUsageMetadata struct {
	PromptVersion             string                   `json:"prompt_version"`
	PromptTemplateVersion     string                   `json:"prompt_template_version"`
	PromptTemplateSource      string                   `json:"prompt_template_source"`
	TargetTopN                int                      `json:"target_top_n"`
	TargetTopNBase            int                      `json:"target_top_n_base"`
	TargetTopNAISuggested     int                      `json:"target_top_n_ai_suggested"`
	TargetTopNAllowed         int                      `json:"target_top_n_allowed"`
	TargetTopNApplied         int                      `json:"target_top_n_applied"`
	TargetTopNOverrideEnabled bool                     `json:"target_top_n_override_enabled"`
	TargetTopNExpandRatio     float64                  `json:"target_top_n_expand_ratio"`
	TargetTopNAbsoluteCap     int                      `json:"target_top_n_absolute_cap"`
	TargetTopNClampReason     string                   `json:"target_top_n_clamp_reason,omitempty"`
	TargetTopNDurationTier    string                   `json:"target_top_n_duration_tier"`
	CandidateSource           string                   `json:"candidate_source"`
	FrameCount                int                      `json:"frame_count"`
	FrameSamplingError        string                   `json:"frame_sampling_error,omitempty"`
	DirectorApplied           bool                     `json:"director_applied"`
	PlannerInputPayloadV1     aiGIFPlannerInputPayload `json:"planner_input_payload_v1"`
	PlannerInputPayloadBytes  int                      `json:"planner_input_payload_bytes"`
}

type aiGIFJudgeUsageMetadata struct {
	PromptVersion           string                 `json:"prompt_version"`
	PromptTemplateVersion   string                 `json:"prompt_template_version"`
	PromptTemplateSource    string                 `json:"prompt_template_source"`
	SampleSize              int                    `json:"sample_size"`
	JudgeInputSchemaVersion string                 `json:"judge_input_schema_version"`
	JudgeInputPayloadV1     aiGIFJudgeInputPayload `json:"judge_input_payload_v1"`
	JudgeInputPayloadBytes  int                    `json:"judge_input_payload_bytes"`
}

type aiGIFDirectiveRowMetadata struct {
	MustCaptureCount           int    `json:"must_capture_count"`
	AvoidCount                 int    `json:"avoid_count"`
	RiskFlagsCount             int    `json:"risk_flags_count"`
	OperatorInstructionEnabled bool   `json:"operator_instruction_enabled"`
	OperatorInstructionVersion string `json:"operator_instruction_version,omitempty"`
	OperatorInstructionLen     int    `json:"operator_instruction_len"`
	FallbackReason             string `json:"fallback_reason,omitempty"`
}

type aiGIFJudgeHardGateMetadata struct {
	Applied       bool     `json:"applied"`
	Blocked       bool     `json:"blocked"`
	From          string   `json:"from,omitempty"`
	To            string   `json:"to,omitempty"`
	ReasonCodes   []string `json:"reason_codes,omitempty"`
	ReasonSummary string   `json:"reason_summary,omitempty"`
}

type aiGIFJudgeReviewRowMetadata struct {
	ProposalRank int                        `json:"proposal_rank"`
	OutputScore  float64                    `json:"output_score"`
	EvalOverall  float64                    `json:"eval_overall"`
	EvalLoop     float64                    `json:"eval_loop"`
	EvalClarity  float64                    `json:"eval_clarity"`
	WindowStart  float64                    `json:"window_start"`
	WindowEnd    float64                    `json:"window_end"`
	HardGate     aiGIFJudgeHardGateMetadata `json:"hard_gate"`
}

type aiGIFDeliverFallbackMetadata struct {
	Applied                bool                   `json:"applied"`
	Reason                 string                 `json:"reason"`
	TriggerReason          string                 `json:"trigger_reason"`
	PreviousRecommendation string                 `json:"previous_recommendation"`
	SelectedReviewStatus   string                 `json:"selected_review_status,omitempty"`
	SelectedOutputScore    float64                `json:"selected_output_score"`
	SelectedEvalOverall    float64                `json:"selected_eval_overall"`
	SelectedEvalClarity    float64                `json:"selected_eval_clarity"`
	SelectedEvalLoop       float64                `json:"selected_eval_loop"`
	SelectedIsPrimary      bool                   `json:"selected_is_primary"`
	AppliedAt              string                 `json:"applied_at"`
	Context                map[string]interface{} `json:"context,omitempty"`
}

type aiGIFDeliverFallbackRawResponse struct {
	Type                   string                 `json:"type"`
	TriggerReason          string                 `json:"trigger_reason"`
	OutputID               uint64                 `json:"output_id"`
	PreviousRecommendation string                 `json:"previous_recommendation"`
	SelectedReviewStatus   string                 `json:"selected_review_status,omitempty"`
	Context                map[string]interface{} `json:"context,omitempty"`
}

type aiGIFJudgeRunSnapshot struct {
	Enabled                      bool                   `json:"enabled"`
	Provider                     string                 `json:"provider,omitempty"`
	Model                        string                 `json:"model,omitempty"`
	PromptVersion                string                 `json:"prompt_version,omitempty"`
	Mode                         string                 `json:"mode,omitempty"`
	Reason                       string                 `json:"reason,omitempty"`
	Applied                      bool                   `json:"applied"`
	Error                        string                 `json:"error,omitempty"`
	ReviewedOutputs              int                    `json:"reviewed_outputs,omitempty"`
	DeliverCount                 int                    `json:"deliver_count,omitempty"`
	KeepInternalCount            int                    `json:"keep_internal_count,omitempty"`
	RejectCount                  int                    `json:"reject_count,omitempty"`
	ManualReviewCount            int                    `json:"manual_review_count,omitempty"`
	HardGateApplied              int                    `json:"hard_gate_applied,omitempty"`
	HardGateRejectCount          int                    `json:"hard_gate_reject_count,omitempty"`
	HardGateManualReviewCount    int                    `json:"hard_gate_manual_review_count,omitempty"`
	DeliverFallbackApplied       bool                   `json:"deliver_fallback_applied"`
	DeliverFallbackReason        string                 `json:"deliver_fallback_reason,omitempty"`
	DeliverFallbackTriggerReason string                 `json:"deliver_fallback_trigger_reason,omitempty"`
	Summary                      map[string]interface{} `json:"summary,omitempty"`
}

type aiGIFDeliverFallbackResult struct {
	Attempted              bool   `json:"attempted"`
	Applied                bool   `json:"applied"`
	Reason                 string `json:"reason,omitempty"`
	TriggerReason          string `json:"trigger_reason,omitempty"`
	Policy                 string `json:"policy,omitempty"`
	Error                  string `json:"error,omitempty"`
	SampleCount            int    `json:"sample_count,omitempty"`
	ReviewCount            int    `json:"review_count,omitempty"`
	DeliverCountBefore     int    `json:"deliver_count_before,omitempty"`
	DeliverCountAfter      int    `json:"deliver_count_after,omitempty"`
	SelectedOutputID       uint64 `json:"selected_output_id,omitempty"`
	PreviousRecommendation string `json:"previous_recommendation,omitempty"`
}

type aiGIFJudgeCompletedEvent struct {
	SubStage string                `json:"sub_stage"`
	Judge    aiGIFJudgeRunSnapshot `json:"judge"`
}

type aiGIFJudgeFailedEvent struct {
	SubStage string                `json:"sub_stage"`
	Error    string                `json:"error"`
	Judge    aiGIFJudgeRunSnapshot `json:"judge"`
}

type aiGIFJudgeSkippedEvent struct {
	SubStage string `json:"sub_stage"`
	Mode     string `json:"mode"`
}

type aiGIFDeliverFallbackDisabledEvent struct {
	Reason string `json:"reason"`
	Policy string `json:"policy"`
}

func decodeAIGIFJudgeRunSnapshot(raw map[string]interface{}) aiGIFJudgeRunSnapshot {
	if len(raw) == 0 {
		return aiGIFJudgeRunSnapshot{}
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return aiGIFJudgeRunSnapshot{}
	}
	var out aiGIFJudgeRunSnapshot
	if err := json.Unmarshal(body, &out); err != nil {
		return aiGIFJudgeRunSnapshot{}
	}
	return out
}

func buildAIGIFPlannerInputPayload(
	job models.VideoJob,
	meta videoProbeMeta,
	topNDecision aiGIFPlannerTopNDecision,
	frameManifest []aiGIFFrameManifestEntry,
	directive *gifAIDirectiveProfile,
) aiGIFPlannerInputPayload {
	payload := aiGIFPlannerInputPayload{
		JobID:           job.ID,
		Title:           strings.TrimSpace(job.Title),
		DurationSec:     roundTo(meta.DurationSec, 3),
		Width:           meta.Width,
		Height:          meta.Height,
		FPS:             roundTo(meta.FPS, 3),
		TargetTopN:      topNDecision.AppliedTopN,
		TargetWindowSec: roundTo(chooseHighlightDuration(meta.DurationSec), 3),
		FrameCount:      len(frameManifest),
		FrameManifest:   frameManifest,
		HardConstraintPolicy: aiGIFPlannerHardConstraintPolicy{
			BaseTopN:        topNDecision.BaseTopN,
			AISuggestedTopN: topNDecision.AISuggestedTopN,
			AllowedTopN:     topNDecision.AllowedTopN,
			AppliedTopN:     topNDecision.AppliedTopN,
			OverrideEnabled: topNDecision.OverrideEnabled,
			ExpandRatio:     roundTo(topNDecision.ExpandRatio, 4),
			AbsoluteCap:     topNDecision.AbsoluteCap,
			ClampReason:     topNDecision.ClampReason,
			DurationTier:    topNDecision.DurationTier,
		},
	}
	if directive != nil {
		payload.Director = directive
	}
	return payload
}

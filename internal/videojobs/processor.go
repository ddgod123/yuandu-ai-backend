package videojobs

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	imagejpeg "image/jpeg"
	imagepng "image/png"
	"io"
	"math"
	"math/big"
	"math/bits"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"emoji/internal/config"
	"emoji/internal/models"
	"emoji/internal/storage"

	"github.com/hibiken/asynq"
	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/sync/errgroup"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	defaultVideoJobTimeout = 20 * time.Minute
	defaultHighlightTopN   = 3
	gifLongVideoThreshold  = 120.0
	gifUltraVideoThreshold = 240.0
	gifRenderTimeoutMin    = 30 * time.Second
	gifRenderTimeoutMax    = 120 * time.Second
	gifSubStageBriefing    = "briefing"
	gifSubStagePlanning    = "planning"
	gifSubStageScoring     = "scoring"
	gifSubStageReviewing   = "reviewing"
)

const downloadCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
const downloadCodeLength = 8

type Processor struct {
	db         *gorm.DB
	qiniu      *storage.QiniuClient
	cfg        config.Config
	httpClient *http.Client
}

type scenePoint struct {
	PtsSec float64 `json:"pts_sec"`
	Score  float64 `json:"score"`
}

type highlightCandidate struct {
	StartSec     float64 `json:"start_sec"`
	EndSec       float64 `json:"end_sec"`
	Score        float64 `json:"score"`
	SceneScore   float64 `json:"scene_score,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	ProposalRank int     `json:"proposal_rank,omitempty"`
	ProposalID   *uint64 `json:"proposal_id,omitempty"`
	CandidateID  *uint64 `json:"candidate_id,omitempty"`
}

type highlightSuggestion struct {
	Version    string               `json:"version"`
	Strategy   string               `json:"strategy"`
	Selected   *highlightCandidate  `json:"selected,omitempty"`
	Candidates []highlightCandidate `json:"candidates,omitempty"`
	All        []highlightCandidate `json:"all_candidates,omitempty"`
}

type highlightFeedbackProfile struct {
	EngagedJobs           int                `json:"engaged_jobs"`
	WeightedSignals       float64            `json:"weighted_signals"`
	PreferredCenter       float64            `json:"preferred_center"`
	PreferredDuration     float64            `json:"preferred_duration"`
	ReasonPreference      map[string]float64 `json:"reason_preference,omitempty"`
	ReasonNegativeGuard   map[string]float64 `json:"reason_negative_guard,omitempty"`
	ScenePreference       map[string]float64 `json:"scene_preference,omitempty"`
	AverageSignalWeight   float64            `json:"average_signal_weight"`
	PublicPositiveSignals float64            `json:"public_positive_signals"`
	PublicNegativeSignals float64            `json:"public_negative_signals"`
}

type publicFeedbackSignalSummary struct {
	TotalWeight    float64
	DeltaWeight    float64
	PositiveWeight float64
	NegativeWeight float64
	TotalCount     int64
	Details        []publicFeedbackSignalDetail
}

type publicFeedbackSignalDetail struct {
	OutputID uint64
	Action   string
	Weight   float64
	SceneTag string
	StartSec float64
	EndSec   float64
	Reason   string
}

type cloudHighlightFallbackConfig struct {
	Enabled   bool
	URL       string
	Token     string
	MinScore  float64
	Timeout   time.Duration
	TopN      int
	Threshold float64
}

type cloudHighlightRequest struct {
	DurationSec     float64              `json:"duration_sec"`
	Width           int                  `json:"width"`
	Height          int                  `json:"height"`
	FPS             float64              `json:"fps"`
	TargetDuration  float64              `json:"target_duration_sec"`
	TopN            int                  `json:"top_n"`
	SceneThreshold  float64              `json:"scene_threshold"`
	ScenePoints     []scenePoint         `json:"scene_points,omitempty"`
	LocalCandidates []highlightCandidate `json:"local_candidates,omitempty"`
}

type cloudHighlightResponse struct {
	Candidates []highlightCandidate `json:"candidates"`
	Selected   *highlightCandidate  `json:"selected,omitempty"`
	Usage      *cloudHighlightUsage `json:"usage,omitempty"`
}

type cloudHighlightUsage struct {
	InputTokens       int64   `json:"input_tokens,omitempty"`
	OutputTokens      int64   `json:"output_tokens,omitempty"`
	CachedInputTokens int64   `json:"cached_input_tokens,omitempty"`
	ImageTokens       int64   `json:"image_tokens,omitempty"`
	VideoTokens       int64   `json:"video_tokens,omitempty"`
	AudioSeconds      float64 `json:"audio_seconds,omitempty"`
	PromptTokens      int64   `json:"prompt_tokens,omitempty"`
	CompletionTokens  int64   `json:"completion_tokens,omitempty"`
}

type cloudHighlightGenericResponse struct {
	Usage *cloudHighlightUsage `json:"usage,omitempty"`
}

type frameQualitySample struct {
	Index        int     `json:"index"`
	Path         string  `json:"path"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	Brightness   float64 `json:"brightness"`
	BlurScore    float64 `json:"blur_score"`
	SubjectScore float64 `json:"subject_score"`
	Exposure     float64 `json:"exposure_score"`
	MotionScore  float64 `json:"motion_score"`
	QualityScore float64 `json:"quality_score"`
	SceneID      int     `json:"scene_id"`
	Hash         uint64  `json:"-"`
}

type frameQualityReport struct {
	TotalFrames           int      `json:"total_frames"`
	KeptFrames            int      `json:"kept_frames"`
	RejectedBlur          int      `json:"rejected_blur"`
	RejectedBrightness    int      `json:"rejected_brightness"`
	RejectedExposure      int      `json:"rejected_exposure"`
	RejectedResolution    int      `json:"rejected_resolution"`
	RejectedStillBlurGate int      `json:"rejected_still_blur_gate"`
	RejectedNearDuplicate int      `json:"rejected_near_duplicate"`
	BlurThreshold         float64  `json:"blur_threshold"`
	SceneCutThreshold     float64  `json:"scene_cut_threshold"`
	SceneCount            int      `json:"scene_count"`
	AvgKeptScore          float64  `json:"avg_kept_score"`
	SelectorVersion       string   `json:"selector_version"`
	FallbackApplied       bool     `json:"fallback_applied"`
	KeptSample            []string `json:"kept_sample,omitempty"`
}

type autoCropSuggestion struct {
	CropX           int     `json:"crop_x"`
	CropY           int     `json:"crop_y"`
	CropW           int     `json:"crop_w"`
	CropH           int     `json:"crop_h"`
	MatchCount      int     `json:"match_count"`
	TotalMatches    int     `json:"total_matches"`
	Confidence      float64 `json:"confidence"`
	SampleStartSec  float64 `json:"sample_start_sec"`
	SampleDuration  float64 `json:"sample_duration_sec"`
	RemovedAreaRate float64 `json:"removed_area_rate"`
}

type unsupportedOutputFormatError struct {
	Format string
	Reason string
}

func (e unsupportedOutputFormatError) Error() string {
	return fmt.Sprintf("output format %s is unsupported: %s", e.Format, e.Reason)
}

var (
	ffmpegEncodersOnce sync.Once
	ffmpegEncoderSet   map[string]struct{}
	ffmpegEncodersErr  error
)

func NewProcessor(db *gorm.DB, qiniu *storage.QiniuClient, cfg config.Config) *Processor {
	return &Processor{
		db:    db,
		qiniu: qiniu,
		cfg:   cfg,
		httpClient: &http.Client{
			Timeout: defaultVideoJobTimeout,
		},
	}
}

func (p *Processor) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TaskTypeProcessVideoJob, p.HandleProcessVideoJob)
}

func (p *Processor) loadQualitySettings() QualitySettings {
	settings := DefaultQualitySettings()
	if p == nil || p.db == nil {
		return settings
	}

	var row models.VideoQualitySetting
	if err := p.db.First(&row, 1).Error; err != nil {
		return settings
	}

	return NormalizeQualitySettings(QualitySettings{
		MinBrightness:                        row.MinBrightness,
		MaxBrightness:                        row.MaxBrightness,
		BlurThresholdFactor:                  row.BlurThresholdFactor,
		BlurThresholdMin:                     row.BlurThresholdMin,
		BlurThresholdMax:                     row.BlurThresholdMax,
		DuplicateHammingThreshold:            row.DuplicateHammingThreshold,
		DuplicateBacktrackFrames:             row.DuplicateBacktrackFrames,
		FallbackBlurRelaxFactor:              row.FallbackBlurRelaxFactor,
		FallbackHammingThreshold:             row.FallbackHammingThreshold,
		MinKeepBase:                          row.MinKeepBase,
		MinKeepRatio:                         row.MinKeepRatio,
		QualityAnalysisWorkers:               row.QualityAnalysisWorkers,
		UploadConcurrency:                    row.UploadConcurrency,
		GIFProfile:                           row.GIFProfile,
		WebPProfile:                          row.WebPProfile,
		LiveProfile:                          row.LiveProfile,
		JPGProfile:                           row.JPGProfile,
		PNGProfile:                           row.PNGProfile,
		GIFDefaultFPS:                        row.GIFDefaultFPS,
		GIFDefaultMaxColors:                  row.GIFDefaultMaxColors,
		GIFDitherMode:                        row.GIFDitherMode,
		GIFTargetSizeKB:                      row.GIFTargetSizeKB,
		GIFLoopTuneEnabled:                   row.GIFLoopTuneEnabled,
		GIFLoopTuneMinEnableSec:              row.GIFLoopTuneMinEnableSec,
		GIFLoopTuneMinImprovement:            row.GIFLoopTuneMinImprovement,
		GIFLoopTuneMotionTarget:              row.GIFLoopTuneMotionTarget,
		GIFLoopTunePreferDuration:            row.GIFLoopTunePreferDuration,
		GIFCandidateMaxOutputs:               row.GIFCandidateMaxOutputs,
		GIFCandidateConfidenceThreshold:      row.GIFCandidateConfidenceThreshold,
		GIFCandidateDedupIOUThreshold:        row.GIFCandidateDedupIOUThreshold,
		WebPTargetSizeKB:                     row.WebPTargetSizeKB,
		JPGTargetSizeKB:                      row.JPGTargetSizeKB,
		PNGTargetSizeKB:                      row.PNGTargetSizeKB,
		StillMinBlurScore:                    row.StillMinBlurScore,
		StillMinExposureScore:                row.StillMinExposureScore,
		StillMinWidth:                        row.StillMinWidth,
		StillMinHeight:                       row.StillMinHeight,
		LiveCoverPortraitWeight:              row.LiveCoverPortraitWeight,
		LiveCoverSceneMinSamples:             row.LiveCoverSceneMinSamples,
		LiveCoverGuardMinTotal:               row.LiveCoverGuardMinTotal,
		LiveCoverGuardScoreFloor:             row.LiveCoverGuardScoreFloor,
		HighlightFeedbackEnabled:             row.HighlightFeedbackEnabled,
		HighlightFeedbackRollout:             row.HighlightFeedbackRollout,
		HighlightFeedbackMinJobs:             row.HighlightFeedbackMinJobs,
		HighlightFeedbackMinScore:            row.HighlightFeedbackMinScore,
		HighlightFeedbackBoost:               row.HighlightFeedbackBoost,
		HighlightWeightPosition:              row.HighlightWeightPosition,
		HighlightWeightDuration:              row.HighlightWeightDuration,
		HighlightWeightReason:                row.HighlightWeightReason,
		HighlightNegativeGuardEnabled:        row.HighlightNegativeGuardEnabled,
		HighlightNegativeGuardThreshold:      row.HighlightNegativeGuardThreshold,
		HighlightNegativeGuardMinWeight:      row.HighlightNegativeGuardMinWeight,
		HighlightNegativePenaltyScale:        row.HighlightNegativePenaltyScale,
		HighlightNegativePenaltyWeight:       row.HighlightNegativePenaltyWeight,
		AIDirectorOperatorInstruction:        row.AIDirectorOperatorInstruction,
		AIDirectorOperatorInstructionVersion: row.AIDirectorOperatorInstructionVersion,
		AIDirectorOperatorEnabled:            row.AIDirectorOperatorEnabled,
	})
}

func (p *Processor) HandleProcessVideoJob(ctx context.Context, t *asynq.Task) error {
	var payload ProcessVideoJobPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("decode payload: %w", err)
	}
	if payload.JobID == 0 {
		return fmt.Errorf("%w: invalid job id", asynq.SkipRetry)
	}
	if p.db == nil {
		return fmt.Errorf("%w: db not initialized", asynq.SkipRetry)
	}
	if p.qiniu == nil {
		p.markVideoJobFailed(payload.JobID, "qiniu not configured")
		p.syncJobCost(payload.JobID)
		p.syncJobPointSettlement(payload.JobID, models.VideoJobStatusFailed)
		return fmt.Errorf("%w: qiniu not configured", asynq.SkipRetry)
	}

	if err := p.process(ctx, payload.JobID); err != nil {
		return p.handleJobError(ctx, payload.JobID, err)
	}
	return nil
}

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
	if err := p.downloadObjectByKey(ctx, job.SourceVideoKey, sourcePath); err != nil {
		return fmt.Errorf("download source video: %w", err)
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
		suggestion, err := suggestHighlightWindow(ctx, sourcePath, meta, qualitySettings)
		if err != nil {
			metrics["highlight_v1"] = map[string]interface{}{
				"version": "v1",
				"enabled": true,
				"error":   err.Error(),
			}
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "highlight scorer failed; fallback to default interval", map[string]interface{}{
				"error": err.Error(),
			})
		} else if suggestion.Selected != nil {
			var directorDirective *gifAIDirectiveProfile
			briefingStarted := markGIFSubStageRunning(gifSubStageBriefing, map[string]interface{}{
				"entry_stage": models.VideoJobStageAnalyzing,
			})
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "gif sub-stage briefing started", map[string]interface{}{
				"sub_stage": gifSubStageBriefing,
				"status":    "running",
			})
			directorDirective, directorSnapshot, directorErr := p.requestAIGIFPromptDirective(ctx, job, meta, suggestion, qualitySettings)
			if directorErr != nil {
				metrics["highlight_ai_director_v1"] = map[string]interface{}{
					"enabled":        directorSnapshot["enabled"],
					"provider":       directorSnapshot["provider"],
					"model":          directorSnapshot["model"],
					"prompt_version": directorSnapshot["prompt_version"],
					"applied":        false,
					"error":          directorErr.Error(),
				}
				markGIFSubStageDone(gifSubStageBriefing, briefingStarted, "degraded", map[string]interface{}{
					"error":    directorErr.Error(),
					"fallback": true,
				})
				p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "ai director unavailable; fallback to default planner context", map[string]interface{}{
					"error":     directorErr.Error(),
					"sub_stage": gifSubStageBriefing,
				})
			} else {
				metrics["highlight_ai_director_v1"] = directorSnapshot
				markGIFSubStageDone(gifSubStageBriefing, briefingStarted, "done", map[string]interface{}{
					"applied": true,
				})
				p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "ai director prompt pack generated", map[string]interface{}{
					"business_goal":  directorSnapshot["business_goal"],
					"clip_count_min": directorSnapshot["clip_count_min"],
					"clip_count_max": directorSnapshot["clip_count_max"],
					"sub_stage":      gifSubStageBriefing,
				})
			}

			planningStarted := markGIFSubStageRunning(gifSubStagePlanning, map[string]interface{}{
				"entry_stage": models.VideoJobStageAnalyzing,
			})
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "gif sub-stage planning started", map[string]interface{}{
				"sub_stage": gifSubStagePlanning,
				"status":    "running",
			})
			plannerSuggestion, plannerSnapshot, plannerErr := p.requestAIGIFPlannerSuggestion(ctx, job, meta, suggestion, directorDirective, qualitySettings)
			if plannerErr != nil {
				metrics["highlight_ai_planner_v1"] = map[string]interface{}{
					"enabled":        plannerSnapshot["enabled"],
					"provider":       plannerSnapshot["provider"],
					"model":          plannerSnapshot["model"],
					"prompt_version": plannerSnapshot["prompt_version"],
					"applied":        false,
					"error":          plannerErr.Error(),
				}
				markGIFSubStageDone(gifSubStagePlanning, planningStarted, "degraded", map[string]interface{}{
					"error": plannerErr.Error(),
				})
				p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "warn", "ai planner unavailable; fallback to local highlight", map[string]interface{}{
					"error":     plannerErr.Error(),
					"sub_stage": gifSubStagePlanning,
				})
			} else {
				suggestion = plannerSuggestion
				metrics["highlight_ai_planner_v1"] = plannerSnapshot
				markGIFSubStageDone(gifSubStagePlanning, planningStarted, "done", map[string]interface{}{
					"applied": true,
				})
				p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "ai planner suggestion applied", map[string]interface{}{
					"selected_start_sec": suggestion.Selected.StartSec,
					"selected_end_sec":   suggestion.Selected.EndSec,
					"selected_score":     suggestion.Selected.Score,
					"selected_count":     len(suggestion.Candidates),
					"sub_stage":          gifSubStagePlanning,
				})
			}

			feedbackMetrics := map[string]interface{}{
				"enabled": qualitySettings.HighlightFeedbackEnabled,
				"group":   "off",
			}
			if qualitySettings.HighlightFeedbackEnabled {
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

			highlightPlan = &suggestion
			metrics["highlight_v1"] = suggestion
			p.appendJobEvent(job.ID, models.VideoJobStageAnalyzing, "info", "highlight scorer selected clip window", map[string]interface{}{
				"start_sec": suggestion.Selected.StartSec,
				"end_sec":   suggestion.Selected.EndSec,
				"score":     suggestion.Selected.Score,
			})
		}

		if highlightPlan != nil {
			options.StartSec = highlightPlan.Selected.StartSec
			options.EndSec = highlightPlan.Selected.EndSec
			optionsPayload["start_sec"] = options.StartSec
			optionsPayload["end_sec"] = options.EndSec
			optionsPayload["highlight_selected"] = highlightPlan.Selected
			p.updateVideoJob(job.ID, map[string]interface{}{
				"options": mustJSON(optionsPayload),
			})
		}

		if suggestion.Selected != nil && p.shouldUseCloudHighlightFallback(suggestion) {
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
			highlightCandidates = append(highlightCandidates, highlightPlan.Candidates...)
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
		"min_brightness":                qualitySettings.MinBrightness,
		"max_brightness":                qualitySettings.MaxBrightness,
		"blur_threshold_factor":         qualitySettings.BlurThresholdFactor,
		"duplicate_hamming_threshold":   qualitySettings.DuplicateHammingThreshold,
		"gif_default_fps":               qualitySettings.GIFDefaultFPS,
		"gif_default_max_colors":        qualitySettings.GIFDefaultMaxColors,
		"gif_default_dither_mode":       qualitySettings.GIFDitherMode,
		"gif_target_size_kb":            qualitySettings.GIFTargetSizeKB,
		"gif_loop_tune_enabled":         qualitySettings.GIFLoopTuneEnabled,
		"gif_loop_tune_min_enable":      qualitySettings.GIFLoopTuneMinEnableSec,
		"gif_loop_tune_min_improve":     qualitySettings.GIFLoopTuneMinImprovement,
		"gif_loop_tune_motion_target":   qualitySettings.GIFLoopTuneMotionTarget,
		"gif_loop_tune_prefer_sec":      qualitySettings.GIFLoopTunePreferDuration,
		"gif_candidate_max_outputs":     qualitySettings.GIFCandidateMaxOutputs,
		"gif_candidate_conf_threshold":  qualitySettings.GIFCandidateConfidenceThreshold,
		"gif_candidate_dedup_iou":       qualitySettings.GIFCandidateDedupIOUThreshold,
		"gif_long_video_threshold_sec":  gifLongVideoThreshold,
		"gif_ultra_video_threshold_sec": gifUltraVideoThreshold,
		"gif_segment_timeout_min_sec":   int(gifRenderTimeoutMin / time.Second),
		"gif_segment_timeout_max_sec":   int(gifRenderTimeoutMax / time.Second),
		"webp_target_size_kb":           qualitySettings.WebPTargetSizeKB,
		"jpg_target_size_kb":            qualitySettings.JPGTargetSizeKB,
		"png_target_size_kb":            qualitySettings.PNGTargetSizeKB,
		"duplicate_backtrack_frames":    qualitySettings.DuplicateBacktrackFrames,
		"fallback_blur_relax_factor":    qualitySettings.FallbackBlurRelaxFactor,
		"fallback_hamming_threshold":    qualitySettings.FallbackHammingThreshold,
		"quality_min_keep_base":         qualitySettings.MinKeepBase,
		"quality_min_keep_ratio":        qualitySettings.MinKeepRatio,
		"still_min_blur_score":          qualitySettings.StillMinBlurScore,
		"still_min_exposure_score":      qualitySettings.StillMinExposureScore,
		"still_min_width":               qualitySettings.StillMinWidth,
		"still_min_height":              qualitySettings.StillMinHeight,
		"quality_analysis_workers":      qualitySettings.QualityAnalysisWorkers,
		"upload_concurrency":            qualitySettings.UploadConcurrency,
		"gif_profile":                   qualitySettings.GIFProfile,
		"webp_profile":                  qualitySettings.WebPProfile,
		"live_profile":                  qualitySettings.LiveProfile,
		"jpg_profile":                   qualitySettings.JPGProfile,
		"png_profile":                   qualitySettings.PNGProfile,
		"still_clarity_enhance":         shouldApplyStillClarityEnhancement(meta, extractOptions, qualitySettings),
		"quality_candidate_budget":      candidateBudget,
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

	p.updateVideoJob(job.ID, map[string]interface{}{
		"stage":    models.VideoJobStageUploading,
		"progress": 70,
	})

	resultCollectionID, totalFrames, uploadedKeys, generatedFormats, packageOutcome, err := p.persistJobResults(ctx, job, framePaths, sourcePath, meta, options, highlightCandidates, qualitySettings)
	if err != nil {
		deleteQiniuKeysByPrefix(p.qiniu, uploadedKeys)
		return err
	}

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
	if containsString(generatedFormats, "gif") {
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
	} else {
		markGIFSubStageSkipped(gifSubStageReviewing, "gif_not_generated")
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

func (p *Processor) acquireVideoJobRun(jobID uint64, startedAt time.Time) (bool, error) {
	if p == nil || p.db == nil || jobID == 0 {
		return false, errors.New("invalid processor or job id")
	}
	updates := map[string]interface{}{
		"status":        models.VideoJobStatusRunning,
		"stage":         models.VideoJobStagePreprocessing,
		"progress":      5,
		"started_at":    startedAt,
		"error_message": "",
	}
	result := p.db.Model(&models.VideoJob{}).
		Where("id = ? AND status = ?", jobID, models.VideoJobStatusQueued).
		Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected > 0 {
		_ = SyncPublicVideoImageJobUpdates(p.db, jobID, updates)
	}
	return result.RowsAffected > 0, nil
}

func (p *Processor) recoverCompletedJobFromExistingResult(job *models.VideoJob) (bool, error) {
	if p == nil || p.db == nil || job == nil {
		return false, errors.New("invalid processor or job")
	}
	if job.ResultCollectionID == nil || *job.ResultCollectionID == 0 {
		return false, nil
	}

	var collection models.Collection
	if err := p.db.Select("id").Where("id = ?", *job.ResultCollectionID).First(&collection).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	generatedFormats := make([]string, 0, 4)
	if err := p.db.Model(&models.Emoji{}).
		Where("collection_id = ? AND status = ?", collection.ID, "active").
		Distinct("format").
		Pluck("format", &generatedFormats).Error; err != nil {
		return false, err
	}
	generatedFormats = normalizeFormatSlice(generatedFormats)
	if len(generatedFormats) == 0 {
		return false, nil
	}

	metrics := parseJSONMap(job.Metrics)
	metrics["result_collection_id"] = collection.ID
	metrics["output_formats_requested"] = normalizeOutputFormats(job.OutputFormats)
	metrics["output_formats"] = generatedFormats

	finishedAt := time.Now()
	updates := map[string]interface{}{
		"status":        models.VideoJobStatusDone,
		"stage":         models.VideoJobStageDone,
		"progress":      100,
		"error_message": "",
		"finished_at":   finishedAt,
		"metrics":       mustJSON(metrics),
	}
	result := p.db.Model(&models.VideoJob{}).
		Where("id = ? AND status <> ?", job.ID, models.VideoJobStatusCancelled).
		Updates(updates)
	if result.Error != nil {
		return false, result.Error
	}
	if result.RowsAffected == 0 {
		return false, nil
	}
	_ = SyncPublicVideoImageJobUpdates(p.db, job.ID, updates)

	p.appendJobEvent(job.ID, models.VideoJobStageDone, "warn", "job recovered from existing result collection", map[string]interface{}{
		"collection_id": collection.ID,
		"formats":       generatedFormats,
	})
	p.syncJobCost(job.ID)
	p.syncJobPointSettlement(job.ID, models.VideoJobStatusDone)
	p.syncGIFBaseline(job.ID)
	p.cleanupSourceVideo(job.ID, "done")
	return true, nil
}

func (p *Processor) persistJobResults(
	ctx context.Context,
	job models.VideoJob,
	framePaths []string,
	sourcePath string,
	meta videoProbeMeta,
	options jobOptions,
	highlightCandidates []highlightCandidate,
	qualitySettings QualitySettings,
) (uint64, int, []string, []string, packageBundleOutcome, error) {
	formats := normalizeOutputFormats(job.OutputFormats)
	stillFormats, animatedFormats := splitVideoOutputFormats(formats)
	if len(stillFormats) == 0 && len(animatedFormats) == 0 {
		stillFormats = []string{"jpg"}
	}
	if len(stillFormats) > 0 && len(framePaths) == 0 {
		return 0, 0, nil, nil, packageBundleOutcome{}, permanentError{err: errors.New("frame paths is empty")}
	}

	prefix, err := p.resolveCollectionPrefix(job)
	if err != nil {
		return 0, 0, nil, nil, packageBundleOutcome{}, err
	}

	uploader := qiniustorage.NewFormUploader(p.qiniu.Cfg)
	uploadedKeys := make([]string, 0, len(framePaths)*(len(stillFormats)+1)+16)
	generatedFormatSet := map[string]struct{}{}

	tx := p.db.Begin()
	if tx.Error != nil {
		return 0, 0, uploadedKeys, nil, packageBundleOutcome{}, tx.Error
	}

	collection := models.Collection{
		Title:       fallbackTitle(job.Title),
		Slug:        ensureUniqueSlug(tx, slugify(fallbackTitle(job.Title))),
		Description: fmt.Sprintf("由视频任务 #%d 自动生成", job.ID),
		OwnerID:     job.UserID,
		CategoryID:  job.CategoryID,
		Source:      "user_video_mvp",
		QiniuPrefix: prefix,
		FileCount:   0,
		Visibility:  "private",
		Status:      "active",
	}
	code, err := ensureUniqueDownloadCode(tx)
	if err != nil {
		_ = tx.Rollback()
		return 0, 0, uploadedKeys, nil, packageBundleOutcome{}, err
	}
	collection.DownloadCode = code

	if err := tx.Create(&collection).Error; err != nil {
		_ = tx.Rollback()
		return 0, 0, uploadedKeys, nil, packageBundleOutcome{}, err
	}

	displayOrder := 1
	staticCount := 0
	coverKey := ""

	if len(stillFormats) > 0 {
		stillCreated, stillCover, err := p.persistStillFrameOutputs(
			tx,
			job.ID,
			collection,
			prefix,
			framePaths,
			stillFormats,
			qualitySettings,
			displayOrder,
			uploader,
			&uploadedKeys,
			generatedFormatSet,
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, 0, uploadedKeys, nil, packageBundleOutcome{}, err
		}
		staticCount += stillCreated
		displayOrder += stillCreated
		if coverKey == "" {
			coverKey = stillCover
		}
	}

	if len(animatedFormats) > 0 {
		windows := resolveOutputClipWindows(meta, options, highlightCandidates, qualitySettings.GIFCandidateMaxOutputs)
		animatedCreated, animatedCover, err := p.persistAnimatedOutputs(
			ctx,
			tx,
			job.ID,
			collection,
			prefix,
			sourcePath,
			meta,
			options,
			windows,
			animatedFormats,
			qualitySettings,
			displayOrder,
			uploader,
			&uploadedKeys,
			coverKey,
			generatedFormatSet,
		)
		if err != nil {
			_ = tx.Rollback()
			return 0, 0, uploadedKeys, nil, packageBundleOutcome{}, err
		}
		displayOrder += animatedCreated
		if coverKey == "" {
			coverKey = animatedCover
		}
	}

	packageOutcome := p.persistCollectionOutputZipWithRetry(
		ctx,
		tx,
		job,
		collection,
		prefix,
		uploader,
		&uploadedKeys,
		generatedFormatSet,
	)
	if packageOutcome.Key != "" {
		now := time.Now()
		collection.LatestZipKey = packageOutcome.Key
		collection.LatestZipName = packageOutcome.Name
		collection.LatestZipSize = packageOutcome.SizeBytes
		collection.LatestZipAt = &now
	}

	if displayOrder <= 1 {
		_ = tx.Rollback()
		return 0, 0, uploadedKeys, nil, packageOutcome, permanentError{err: errors.New("no output generated; please select at least one supported format")}
	}

	if coverKey != "" {
		collection.CoverURL = coverKey
	}
	collection.FileCount = displayOrder - 1
	if err := tx.Save(&collection).Error; err != nil {
		_ = tx.Rollback()
		return 0, 0, uploadedKeys, nil, packageOutcome, err
	}

	if err := tx.Model(&models.VideoJob{}).
		Where("id = ?", job.ID).
		Update("result_collection_id", collection.ID).Error; err != nil {
		_ = tx.Rollback()
		return 0, 0, uploadedKeys, nil, packageOutcome, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, 0, uploadedKeys, nil, packageOutcome, err
	}

	generatedFormats := make([]string, 0, len(generatedFormatSet))
	for format := range generatedFormatSet {
		generatedFormats = append(generatedFormats, format)
	}
	sort.Strings(generatedFormats)

	return collection.ID, staticCount, uploadedKeys, generatedFormats, packageOutcome, nil
}

func (p *Processor) persistStillFrameOutputs(
	tx *gorm.DB,
	jobID uint64,
	collection models.Collection,
	prefix string,
	framePaths []string,
	formats []string,
	qualitySettings QualitySettings,
	startOrder int,
	uploader *qiniustorage.FormUploader,
	uploadedKeys *[]string,
	generatedFormatSet map[string]struct{},
) (int, string, error) {
	if len(formats) == 0 {
		return 0, "", nil
	}
	tasks := make([]stillFrameTask, 0, len(formats)*len(framePaths))
	order := startOrder
	for _, format := range formats {
		for index, framePath := range framePaths {
			tasks = append(tasks, stillFrameTask{
				Format:    format,
				FramePath: framePath,
				FrameIdx:  index + 1,
				Order:     order,
				Key:       buildVideoImageOutputObjectKey(prefix, format, fmt.Sprintf("%04d.%s", order, format)),
			})
			order++
		}
	}

	results := p.processStillFrameTasks(tasks, qualitySettings, uploader)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Task.Order < results[j].Task.Order
	})

	var firstErr error
	for _, result := range results {
		if result.Err != nil {
			if firstErr == nil {
				firstErr = result.Err
			}
			continue
		}
		*uploadedKeys = append(*uploadedKeys, result.Task.Key)
	}
	if firstErr != nil {
		return 0, "", firstErr
	}

	created := 0
	coverKey := ""
	for _, result := range results {
		generatedFormatSet[result.Task.Format] = struct{}{}
		title := fmt.Sprintf("%s-%02d", collection.Title, result.Task.FrameIdx)
		if len(formats) > 1 {
			title = fmt.Sprintf("%s-%s-%02d", collection.Title, strings.ToUpper(result.Task.Format), result.Task.FrameIdx)
		}

		emoji := models.Emoji{
			CollectionID: collection.ID,
			Title:        title,
			FileURL:      result.Task.Key,
			ThumbURL:     result.Task.Key,
			Format:       result.Task.Format,
			Width:        result.Width,
			Height:       result.Height,
			SizeBytes:    result.SizeBytes,
			DisplayOrder: result.Task.Order,
			Status:       "active",
		}
		if err := upsertEmojiByCollectionFile(tx, &emoji); err != nil {
			return created, coverKey, err
		}

		artifact := models.VideoJobArtifact{
			JobID:     jobID,
			Type:      "frame",
			QiniuKey:  result.Task.Key,
			MimeType:  mimeTypeByFormat(result.Task.Format),
			SizeBytes: result.SizeBytes,
			Width:     result.Width,
			Height:    result.Height,
			Metadata: mustJSON(map[string]interface{}{
				"index":  result.Task.FrameIdx,
				"format": result.Task.Format,
			}),
		}
		if err := upsertVideoJobArtifactByJobKey(tx, &artifact); err != nil {
			return created, coverKey, err
		}

		if coverKey == "" {
			coverKey = result.Task.Key
		}
		created++
	}
	return created, coverKey, nil
}

type stillFrameTask struct {
	Format    string
	FramePath string
	FrameIdx  int
	Order     int
	Key       string
}

type stillFrameTaskResult struct {
	Task      stillFrameTask
	SizeBytes int64
	Width     int
	Height    int
	Err       error
}

func (p *Processor) processStillFrameTasks(tasks []stillFrameTask, qualitySettings QualitySettings, uploader *qiniustorage.FormUploader) []stillFrameTaskResult {
	results := make([]stillFrameTaskResult, len(tasks))
	if len(tasks) == 0 {
		return results
	}

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	workers := qualitySettings.UploadConcurrency
	if workers < 1 {
		workers = 1
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}

	jobs := make(chan int)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				task := tasks[idx]
				targetPath, err := prepareStillFrameTarget(task.FramePath, task.Format, qualitySettings)
				if err != nil {
					results[idx] = stillFrameTaskResult{
						Task: task,
						Err:  fmt.Errorf("prepare %s frame %d: %w", task.Format, task.FrameIdx, err),
					}
					continue
				}

				if err := uploadFileToQiniu(uploader, p.qiniu, task.Key, targetPath); err != nil {
					results[idx] = stillFrameTaskResult{
						Task: task,
						Err:  fmt.Errorf("upload %s frame %d: %w", task.Format, task.FrameIdx, err),
					}
					continue
				}

				sizeBytes, width, height := readImageInfo(targetPath)
				results[idx] = stillFrameTaskResult{
					Task:      task,
					SizeBytes: sizeBytes,
					Width:     width,
					Height:    height,
				}
			}
		}()
	}

	for idx := range tasks {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()
	return results
}

func prepareStillFrameTarget(framePath, format string, qualitySettings QualitySettings) (string, error) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	switch format {
	case "jpg":
		if qualitySettings.JPGProfile == QualityProfileClarity {
			return framePath, nil
		}
		convertedPath := filepath.Join(
			filepath.Dir(framePath),
			fmt.Sprintf("%s_size.jpg", strings.TrimSuffix(filepath.Base(framePath), filepath.Ext(framePath))),
		)
		if err := convertImageToJPG(framePath, convertedPath, qualitySettings.JPGProfile, qualitySettings.JPGTargetSizeKB); err != nil {
			return "", err
		}
		return convertedPath, nil
	case "png":
		convertedPath := filepath.Join(
			filepath.Dir(framePath),
			fmt.Sprintf("%s.png", strings.TrimSuffix(filepath.Base(framePath), filepath.Ext(framePath))),
		)
		if err := convertImageToPNG(framePath, convertedPath, qualitySettings.PNGProfile, qualitySettings.PNGTargetSizeKB); err != nil {
			return "", err
		}
		return convertedPath, nil
	default:
		return "", fmt.Errorf("unsupported still format: %s", format)
	}
}

func (p *Processor) persistAnimatedOutputs(
	ctx context.Context,
	tx *gorm.DB,
	jobID uint64,
	collection models.Collection,
	prefix string,
	sourcePath string,
	meta videoProbeMeta,
	options jobOptions,
	windows []highlightCandidate,
	formats []string,
	qualitySettings QualitySettings,
	startOrder int,
	uploader *qiniustorage.FormUploader,
	uploadedKeys *[]string,
	fallbackCover string,
	generatedFormatSet map[string]struct{},
) (int, string, error) {
	if len(windows) == 0 || len(formats) == 0 {
		return 0, fallbackCover, nil
	}
	outputDir := filepath.Join(filepath.Dir(sourcePath), "animated")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return 0, fallbackCover, fmt.Errorf("create animated output dir: %w", err)
	}

	order := startOrder
	tasks := make([]animatedTask, 0, len(windows)*len(formats))
	for windowIndex, window := range windows {
		for _, format := range formats {
			tasks = append(tasks, animatedTask{
				WindowIndex: windowIndex + 1,
				Window:      window,
				Format:      format,
				Order:       order,
			})
			order++
		}
	}

	results := p.processAnimatedTasks(ctx, sourcePath, outputDir, prefix, meta, options, qualitySettings, uploader, tasks)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Task.Order < results[j].Task.Order
	})

	unsupportedFormatReasons := map[string]string{}
	var firstErr error
	for _, result := range results {
		if len(result.UploadedKeys) > 0 {
			*uploadedKeys = append(*uploadedKeys, result.UploadedKeys...)
		}
		if result.Err != nil && firstErr == nil {
			firstErr = result.Err
		}
	}
	if firstErr != nil {
		return 0, fallbackCover, firstErr
	}

	coverKey := fallbackCover
	created := 0
	for _, result := range results {
		if result.UnsupportedReason != "" {
			if _, exists := unsupportedFormatReasons[result.Task.Format]; !exists {
				unsupportedFormatReasons[result.Task.Format] = result.UnsupportedReason
			}
			continue
		}
		if result.Err != nil {
			continue
		}
		generatedFormatSet[result.Task.Format] = struct{}{}
		if coverKey == "" {
			coverKey = result.ThumbKey
		}

		emoji := models.Emoji{
			CollectionID: collection.ID,
			Title:        buildAnimatedEmojiTitle(collection.Title, result.Task.WindowIndex, result.Task.Format),
			FileURL:      result.FileKey,
			ThumbURL:     result.ThumbKey,
			Format:       result.Task.Format,
			Width:        result.Width,
			Height:       result.Height,
			SizeBytes:    result.SizeBytes,
			DisplayOrder: result.Task.Order,
			Status:       "active",
		}
		if err := upsertEmojiByCollectionFile(tx, &emoji); err != nil {
			return created, coverKey, err
		}

		for _, payload := range result.Artifacts {
			artifact := models.VideoJobArtifact{
				JobID:      jobID,
				Type:       payload.Type,
				QiniuKey:   payload.Key,
				MimeType:   payload.MimeType,
				SizeBytes:  payload.SizeBytes,
				Width:      payload.Width,
				Height:     payload.Height,
				DurationMs: payload.DurationMs,
				Metadata:   mustJSON(payload.Metadata),
			}
			if err := upsertVideoJobArtifactByJobKey(tx, &artifact); err != nil {
				return created, coverKey, err
			}
		}
		created++
	}

	for format, reason := range unsupportedFormatReasons {
		p.appendJobEvent(jobID, models.VideoJobStageRendering, "warn", "skip unsupported output format", map[string]interface{}{
			"format": format,
			"reason": reason,
		})
	}
	return created, coverKey, nil
}

const (
	packageZipStatusSkipped = "skipped"
	packageZipStatusPending = "pending"
	packageZipStatusReady   = "ready"
	packageZipStatusFailed  = "failed"

	defaultPackageZipMaxAttempts = 3
)

type packageBundleOutcome struct {
	Status     string
	Key        string
	Name       string
	SizeBytes  int64
	Attempts   int
	RetryCount int
	Error      string
}

func (p *Processor) persistCollectionOutputZipWithRetry(
	ctx context.Context,
	tx *gorm.DB,
	job models.VideoJob,
	collection models.Collection,
	prefix string,
	uploader *qiniustorage.FormUploader,
	uploadedKeys *[]string,
	generatedFormatSet map[string]struct{},
) packageBundleOutcome {
	outcome := packageBundleOutcome{
		Status: packageZipStatusSkipped,
	}
	if p == nil || tx == nil || p.qiniu == nil || uploader == nil || job.ID == 0 || collection.ID == 0 {
		return outcome
	}

	outcome.Status = packageZipStatusPending
	maxAttempts := defaultPackageZipMaxAttempts
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		outcome.Attempts = attempt
		packageKey, packageName, packageSize, packageErr := p.persistCollectionOutputZip(
			ctx,
			tx,
			job,
			collection,
			prefix,
			uploader,
			uploadedKeys,
			generatedFormatSet,
		)
		if packageErr == nil {
			if packageKey == "" {
				outcome.Status = packageZipStatusSkipped
				outcome.RetryCount = max(0, attempt-1)
				return outcome
			}
			outcome.Status = packageZipStatusReady
			outcome.Key = packageKey
			outcome.Name = packageName
			outcome.SizeBytes = packageSize
			outcome.RetryCount = max(0, attempt-1)
			return outcome
		}

		lastErr = packageErr
		outcome.RetryCount = max(0, attempt-1)
		p.appendJobEvent(job.ID, models.VideoJobStageUploading, "warn", "zip package attempt failed", map[string]interface{}{
			"attempt":      attempt,
			"max_attempts": maxAttempts,
			"error":        packageErr.Error(),
		})
		if attempt >= maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			lastErr = ctx.Err()
			attempt = maxAttempts
		case <-time.After(time.Duration(attempt) * time.Second):
		}
	}

	outcome.Status = packageZipStatusFailed
	if lastErr != nil {
		outcome.Error = lastErr.Error()
	}
	outcome.RetryCount = max(0, outcome.Attempts-1)
	p.appendJobEvent(job.ID, models.VideoJobStageUploading, "warn", "zip package generation exhausted retries", map[string]interface{}{
		"attempts": outcome.Attempts,
		"error":    outcome.Error,
	})
	return outcome
}

func (p *Processor) persistCollectionOutputZip(
	ctx context.Context,
	tx *gorm.DB,
	job models.VideoJob,
	collection models.Collection,
	prefix string,
	uploader *qiniustorage.FormUploader,
	uploadedKeys *[]string,
	generatedFormatSet map[string]struct{},
) (string, string, int64, error) {
	if p == nil || tx == nil || p.qiniu == nil || uploader == nil || job.ID == 0 || collection.ID == 0 {
		return "", "", 0, nil
	}

	var emojis []models.Emoji
	if err := tx.Where("collection_id = ? AND status = ?", collection.ID, "active").
		Order("display_order ASC, id ASC").
		Find(&emojis).Error; err != nil {
		return "", "", 0, err
	}
	if len(emojis) == 0 {
		return "", "", 0, nil
	}

	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("video-job-%d-zip-*", job.ID))
	if err != nil {
		return "", "", 0, err
	}
	defer os.RemoveAll(tmpDir)

	entries := make([]zipEntrySource, 0, len(emojis))
	for idx, item := range emojis {
		key := strings.TrimLeft(strings.TrimSpace(item.FileURL), "/")
		if key == "" {
			continue
		}
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(key)), ".")
		if ext == "" {
			ext = strings.TrimSpace(strings.ToLower(item.Format))
		}
		if ext == "" {
			ext = "bin"
		}

		entryBase := sanitizeZipEntryComponent(item.Title)
		if entryBase == "" {
			entryBase = fmt.Sprintf("item_%03d", idx+1)
		}
		entryName := fmt.Sprintf("%03d_%s.%s", idx+1, entryBase, ext)
		localFile := filepath.Join(tmpDir, fmt.Sprintf("%03d.%s", idx+1, ext))

		if err := p.downloadObjectByKey(ctx, key, localFile); err != nil {
			return "", "", 0, err
		}
		entries = append(entries, zipEntrySource{
			Name: entryName,
			Path: localFile,
		})
	}
	if len(entries) == 0 {
		return "", "", 0, nil
	}

	zipPath := filepath.Join(tmpDir, fmt.Sprintf("%d_outputs.zip", job.ID))
	if err := createZipArchive(zipPath, entries); err != nil {
		return "", "", 0, err
	}
	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return "", "", 0, err
	}

	packageFormat := resolvePackageFormatFromGeneratedSet(generatedFormatSet)
	packageName := fmt.Sprintf("%d_%s_v1.zip", job.ID, packageFormat)
	packageKey := buildVideoImagePackageObjectKey(prefix, packageName)
	if err := uploadFileToQiniu(uploader, p.qiniu, packageKey, zipPath); err != nil {
		return "", "", 0, err
	}
	if uploadedKeys != nil {
		*uploadedKeys = append(*uploadedKeys, packageKey)
	}

	artifact := models.VideoJobArtifact{
		JobID:     job.ID,
		Type:      "package",
		QiniuKey:  packageKey,
		MimeType:  "application/zip",
		SizeBytes: zipInfo.Size(),
		Metadata: mustJSON(map[string]interface{}{
			"format":      "zip",
			"source":      "auto_bundle",
			"file_count":  len(entries),
			"bundle_type": packageFormat,
		}),
	}
	if err := upsertVideoJobArtifactByJobKey(tx, &artifact); err != nil {
		return "", "", 0, err
	}

	uploadedAt := time.Now()
	zipRecord := models.CollectionZip{
		CollectionID: collection.ID,
		ZipKey:       packageKey,
		ZipName:      packageName,
		SizeBytes:    zipInfo.Size(),
		UploadedAt:   &uploadedAt,
	}
	if err := tx.Where("collection_id = ? AND zip_key = ?", collection.ID, packageKey).
		Assign(models.CollectionZip{
			ZipName:    packageName,
			SizeBytes:  zipInfo.Size(),
			UploadedAt: &uploadedAt,
		}).
		FirstOrCreate(&zipRecord).Error; err != nil {
		return "", "", 0, err
	}

	return packageKey, packageName, zipInfo.Size(), nil
}

func resolvePackageFormatFromGeneratedSet(generatedFormatSet map[string]struct{}) string {
	if len(generatedFormatSet) == 1 {
		for format := range generatedFormatSet {
			clean := strings.TrimSpace(strings.ToLower(format))
			if clean != "" {
				return clean
			}
		}
	}
	return "mixed"
}

func sanitizeZipEntryComponent(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "\\", "-")
	value = strings.ReplaceAll(value, ":", "-")
	value = strings.ReplaceAll(value, "*", "-")
	value = strings.ReplaceAll(value, "?", "-")
	value = strings.ReplaceAll(value, "\"", "-")
	value = strings.ReplaceAll(value, "<", "-")
	value = strings.ReplaceAll(value, ">", "-")
	value = strings.ReplaceAll(value, "|", "-")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > 64 {
		value = strings.TrimSpace(value[:64])
	}
	value = strings.Trim(value, ". ")
	if value == "" {
		return ""
	}
	return value
}

type animatedTask struct {
	WindowIndex int
	Window      highlightCandidate
	Format      string
	Order       int
}

type animatedArtifactPayload struct {
	Type       string
	Key        string
	MimeType   string
	SizeBytes  int64
	Width      int
	Height     int
	DurationMs int
	Metadata   map[string]interface{}
}

type animatedTaskResult struct {
	Task              animatedTask
	FileKey           string
	ThumbKey          string
	Width             int
	Height            int
	SizeBytes         int64
	UploadedKeys      []string
	Artifacts         []animatedArtifactPayload
	UnsupportedReason string
	Err               error
}

type qiniuUploadTask struct {
	Key   string
	Path  string
	Label string
}

type animatedAdaptiveProfile struct {
	MotionScore        float64
	Level              string
	DurationSec        float64
	FPS                int
	Width              int
	MaxColors          int
	StabilityTier      string
	LongVideoDownshift bool
}

type gifLoopSampleFrame struct {
	TimestampSec float64
	Hash         uint64
	QualityScore float64
}

type gifLoopTuningResult struct {
	Applied        bool
	BaseStartSec   float64
	BaseEndSec     float64
	TunedStartSec  float64
	TunedEndSec    float64
	EffectiveSec   float64
	DurationSec    float64
	Score          float64
	BaseScore      float64
	BestScore      float64
	Improvement    float64
	MinImprovement float64
	LoopClosure    float64
	BaseLoop       float64
	BestLoop       float64
	MotionMean     float64
	BaseMotion     float64
	BestMotion     float64
	QualityMean    float64
	SampleFrames   int
	Candidates     int
	FallbackToBase bool
	FallbackReason string
	DecisionReason string
}

func (p *Processor) processAnimatedTasks(
	ctx context.Context,
	sourcePath string,
	outputDir string,
	prefix string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	uploader *qiniustorage.FormUploader,
	tasks []animatedTask,
) []animatedTaskResult {
	results := make([]animatedTaskResult, len(tasks))
	if len(tasks) == 0 {
		return results
	}

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	workers := qualitySettings.UploadConcurrency
	if workers < 1 {
		workers = 1
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}
	if shouldLimitGIFRenderConcurrency(meta, tasks) && workers > 1 {
		workers = 1
	}

	sem := make(chan struct{}, workers)
	var group errgroup.Group
	for idx := range tasks {
		idx := idx
		group.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = p.processAnimatedTask(ctx, sourcePath, outputDir, prefix, meta, options, qualitySettings, uploader, tasks[idx])
			return nil
		})
	}
	_ = group.Wait()
	return results
}

func shouldLimitGIFRenderConcurrency(meta videoProbeMeta, tasks []animatedTask) bool {
	if len(tasks) <= 1 {
		return false
	}
	gifTaskCount := 0
	for _, task := range tasks {
		if strings.EqualFold(strings.TrimSpace(task.Format), "gif") {
			gifTaskCount++
		}
	}
	if gifTaskCount <= 1 {
		return false
	}
	longSide := meta.Width
	if meta.Height > longSide {
		longSide = meta.Height
	}
	// 高分辨率或中长视频场景下，多窗口并发 GIF 渲染会明显放大超时风险，串行更稳。
	return longSide >= 1400 || meta.DurationSec >= 45
}

func (p *Processor) processAnimatedTask(
	ctx context.Context,
	sourcePath string,
	outputDir string,
	prefix string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	uploader *qiniustorage.FormUploader,
	task animatedTask,
) animatedTaskResult {
	result := animatedTaskResult{Task: task}
	window := task.Window
	adaptiveOptions, adaptiveProfile := tuneAnimatedOptionsForWindow(meta, options, qualitySettings, task.Format, window)
	if adaptiveProfile.DurationSec > 0 && (task.Format == "gif" || task.Format == "webp" || task.Format == "mp4") {
		window = clampWindowDuration(window, adaptiveProfile.DurationSec, meta.DurationSec)
	}
	baseAnimatedWindow := window
	var gifLoopTune *gifLoopTuningResult
	if task.Format == "gif" {
		tunedWindow, tune, tuneErr := optimizeGIFLoopWindow(ctx, sourcePath, meta, adaptiveOptions, qualitySettings, window)
		if tuneErr == nil && tune.SampleFrames > 0 {
			gifLoopTune = &tune
			if tune.Applied {
				window = tunedWindow
			}
		}
	}
	if task.Format == "live" {
		window = clampWindowDuration(window, 3.0, meta.DurationSec)
		uploaded := make([]string, 0, 3)
		liveOut, err := renderLiveOutputPackage(ctx, sourcePath, outputDir, meta, adaptiveOptions, qualitySettings, window, task.WindowIndex)
		if err != nil {
			var unsupported unsupportedOutputFormatError
			if errors.As(err, &unsupported) {
				reason := strings.TrimSpace(unsupported.Reason)
				if reason == "" {
					reason = "format is not supported by ffmpeg runtime"
				}
				result.UnsupportedReason = reason
				return result
			}
			result.Err = fmt.Errorf("render live clip %d failed: %w", task.WindowIndex, err)
			return result
		}

		liveVideoKey := buildVideoImageOutputObjectKey(prefix, "live", fmt.Sprintf("clip_%02d_video.mov", task.WindowIndex))
		coverFileKey := buildVideoImageOutputObjectKey(prefix, "live", fmt.Sprintf("clip_%02d_cover.jpg", task.WindowIndex))
		packageKey := buildVideoImagePackageObjectKey(prefix, fmt.Sprintf("clip_%02d_live.zip", task.WindowIndex))

		uploadTasks := []qiniuUploadTask{
			{
				Key:   liveVideoKey,
				Path:  liveOut.VideoPath,
				Label: fmt.Sprintf("live video clip %d", task.WindowIndex),
			},
			{
				Key:   coverFileKey,
				Path:  liveOut.CoverPath,
				Label: fmt.Sprintf("live cover clip %d", task.WindowIndex),
			},
			{
				Key:   packageKey,
				Path:  liveOut.PackagePath,
				Label: fmt.Sprintf("live package clip %d", task.WindowIndex),
			},
		}
		maxUploadWorkers := qualitySettings.UploadConcurrency
		if maxUploadWorkers > 3 {
			maxUploadWorkers = 3
		}
		uploaded, err = uploadQiniuTasksConcurrently(ctx, uploader, p.qiniu, uploadTasks, maxUploadWorkers)
		if err != nil {
			result.Err = err
			result.UploadedKeys = uploaded
			return result
		}

		result.FileKey = packageKey
		result.ThumbKey = coverFileKey
		result.Width = liveOut.Width
		result.Height = liveOut.Height
		result.SizeBytes = liveOut.PackageSizeBytes
		result.UploadedKeys = uploaded
		result.Artifacts = []animatedArtifactPayload{
			{
				Type:       "live_video",
				Key:        liveVideoKey,
				MimeType:   "video/quicktime",
				SizeBytes:  liveOut.VideoSizeBytes,
				Width:      liveOut.Width,
				Height:     liveOut.Height,
				DurationMs: liveOut.DurationMs,
				Metadata: map[string]interface{}{
					"window_index": task.WindowIndex,
					"start_sec":    window.StartSec,
					"end_sec":      window.EndSec,
					"score":        window.Score,
					"reason":       strings.TrimSpace(window.Reason),
					"format":       "live",
					"motion_score": adaptiveProfile.MotionScore,
					"motion_level": adaptiveProfile.Level,
				},
			},
			{
				Type:      "live_cover",
				Key:       coverFileKey,
				MimeType:  "image/jpeg",
				SizeBytes: liveOut.CoverSizeBytes,
				Width:     liveOut.Width,
				Height:    liveOut.Height,
				Metadata: map[string]interface{}{
					"window_index":          task.WindowIndex,
					"format":                "live",
					"cover_score":           roundTo(liveOut.CoverScore, 3),
					"cover_ts_sec":          roundTo(liveOut.CoverTimestamp, 3),
					"cover_quality":         roundTo(liveOut.CoverQuality, 3),
					"cover_stability":       roundTo(liveOut.CoverStability, 3),
					"cover_temporal":        roundTo(liveOut.CoverTemporal, 3),
					"cover_portrait":        roundTo(liveOut.CoverPortrait, 3),
					"cover_exposure":        roundTo(liveOut.CoverExposure, 3),
					"cover_face":            roundTo(liveOut.CoverFace, 3),
					"cover_portrait_weight": roundTo(qualitySettings.LiveCoverPortraitWeight, 3),
				},
			},
			{
				Type:       "live_package",
				Key:        packageKey,
				MimeType:   mimeTypeByFormat("live"),
				SizeBytes:  liveOut.PackageSizeBytes,
				Width:      liveOut.Width,
				Height:     liveOut.Height,
				DurationMs: liveOut.DurationMs,
				Metadata: map[string]interface{}{
					"window_index":          task.WindowIndex,
					"start_sec":             window.StartSec,
					"end_sec":               window.EndSec,
					"score":                 window.Score,
					"reason":                strings.TrimSpace(window.Reason),
					"format":                "live",
					"entries":               []string{"photo.jpg", "video.mov"},
					"cover_score":           roundTo(liveOut.CoverScore, 3),
					"cover_ts_sec":          roundTo(liveOut.CoverTimestamp, 3),
					"cover_quality":         roundTo(liveOut.CoverQuality, 3),
					"cover_stability":       roundTo(liveOut.CoverStability, 3),
					"cover_temporal":        roundTo(liveOut.CoverTemporal, 3),
					"cover_portrait":        roundTo(liveOut.CoverPortrait, 3),
					"cover_exposure":        roundTo(liveOut.CoverExposure, 3),
					"cover_face":            roundTo(liveOut.CoverFace, 3),
					"cover_portrait_weight": roundTo(qualitySettings.LiveCoverPortraitWeight, 3),
					"motion_score":          adaptiveProfile.MotionScore,
					"motion_level":          adaptiveProfile.Level,
				},
			},
		}
		for idx := range result.Artifacts {
			appendWindowBindingMetadata(result.Artifacts[idx].Metadata, window)
		}
		return result
	}

	filePath := filepath.Join(outputDir, fmt.Sprintf("clip_%02d_%s.%s", task.WindowIndex, task.Format, task.Format))
	renderErr := renderClipOutput(ctx, sourcePath, filePath, meta, adaptiveOptions, qualitySettings, window, task.Format)
	if renderErr != nil && task.Format == "gif" && gifLoopTune != nil && gifLoopTune.Applied {
		retryWindow := baseAnimatedWindow
		retryErr := renderClipOutput(ctx, sourcePath, filePath, meta, adaptiveOptions, qualitySettings, retryWindow, task.Format)
		if retryErr == nil {
			window = retryWindow
			gifLoopTune.FallbackToBase = true
			gifLoopTune.FallbackReason = "tuned_window_render_failed"
			gifLoopTune.EffectiveSec = roundTo(retryWindow.EndSec-retryWindow.StartSec, 3)
			renderErr = nil
		} else {
			renderErr = fmt.Errorf("render failed with tuned window (%v), fallback also failed: %w", renderErr, retryErr)
		}
	}
	if renderErr != nil {
		var unsupported unsupportedOutputFormatError
		if errors.As(renderErr, &unsupported) {
			reason := strings.TrimSpace(unsupported.Reason)
			if reason == "" {
				reason = "format is not supported by ffmpeg runtime"
			}
			result.UnsupportedReason = reason
			return result
		}
		result.Err = fmt.Errorf("render %s clip %d failed: %w", task.Format, task.WindowIndex, renderErr)
		return result
	}

	fileKey := buildVideoImageOutputObjectKey(prefix, task.Format, fmt.Sprintf("clip_%02d.%s", task.WindowIndex, task.Format))
	if err := uploadFileToQiniu(uploader, p.qiniu, fileKey, filePath); err != nil {
		result.Err = fmt.Errorf("upload %s clip %d failed: %w", task.Format, task.WindowIndex, err)
		return result
	}

	sizeBytes, width, height, durationMs := readMediaOutputInfo(filePath)
	thumbKey := fileKey
	uploads := []string{fileKey}
	clipMetadata := map[string]interface{}{
		"window_index": task.WindowIndex,
		"start_sec":    window.StartSec,
		"end_sec":      window.EndSec,
		"score":        window.Score,
		"reason":       strings.TrimSpace(window.Reason),
		"format":       task.Format,
		"motion_score": adaptiveProfile.MotionScore,
		"motion_level": adaptiveProfile.Level,
		"adaptive_fps": adaptiveProfile.FPS,
		"adaptive_w":   adaptiveProfile.Width,
		"adaptive_sec": roundTo(adaptiveProfile.DurationSec, 3),
	}
	appendWindowBindingMetadata(clipMetadata, window)
	if strings.TrimSpace(adaptiveProfile.StabilityTier) != "" {
		clipMetadata["adaptive_stability_tier"] = adaptiveProfile.StabilityTier
	}
	if adaptiveProfile.LongVideoDownshift {
		clipMetadata["adaptive_long_video_downshift"] = true
	}
	if gifLoopTune != nil {
		effectiveApplied := gifLoopTune.Applied && !gifLoopTune.FallbackToBase
		effectiveDuration := window.EndSec - window.StartSec
		if effectiveDuration < 0 {
			effectiveDuration = 0
		}
		if gifLoopTune.EffectiveSec > 0 {
			effectiveDuration = gifLoopTune.EffectiveSec
		}
		clipMetadata["gif_loop_tune"] = map[string]interface{}{
			"applied":           gifLoopTune.Applied,
			"effective_applied": effectiveApplied,
			"fallback_to_base":  gifLoopTune.FallbackToBase,
			"fallback_reason":   gifLoopTune.FallbackReason,
			"decision_reason":   gifLoopTune.DecisionReason,
			"base_start_sec":    roundTo(gifLoopTune.BaseStartSec, 3),
			"base_end_sec":      roundTo(gifLoopTune.BaseEndSec, 3),
			"tuned_start_sec":   roundTo(gifLoopTune.TunedStartSec, 3),
			"tuned_end_sec":     roundTo(gifLoopTune.TunedEndSec, 3),
			"base_score":        roundTo(gifLoopTune.BaseScore, 3),
			"best_score":        roundTo(gifLoopTune.BestScore, 3),
			"score_improvement": roundTo(gifLoopTune.Improvement, 4),
			"min_improvement":   roundTo(gifLoopTune.MinImprovement, 4),
			"duration_sec":      roundTo(gifLoopTune.DurationSec, 3),
			"effective_sec":     roundTo(effectiveDuration, 3),
			"score":             roundTo(gifLoopTune.Score, 3),
			"loop_closure":      roundTo(gifLoopTune.LoopClosure, 3),
			"base_loop":         roundTo(gifLoopTune.BaseLoop, 3),
			"best_loop":         roundTo(gifLoopTune.BestLoop, 3),
			"motion_mean":       roundTo(gifLoopTune.MotionMean, 3),
			"base_motion":       roundTo(gifLoopTune.BaseMotion, 3),
			"best_motion":       roundTo(gifLoopTune.BestMotion, 3),
			"quality_mean":      roundTo(gifLoopTune.QualityMean, 3),
			"sample_frames":     gifLoopTune.SampleFrames,
			"candidate_windows": gifLoopTune.Candidates,
		}
	}
	artifacts := []animatedArtifactPayload{
		{
			Type:       "clip",
			Key:        fileKey,
			MimeType:   mimeTypeByFormat(task.Format),
			SizeBytes:  sizeBytes,
			Width:      width,
			Height:     height,
			DurationMs: durationMs,
			Metadata:   clipMetadata,
		},
	}

	if task.Format == "mp4" {
		posterPath := filepath.Join(outputDir, fmt.Sprintf("clip_%02d_poster.jpg", task.WindowIndex))
		if err := extractPosterFrame(ctx, filePath, posterPath); err == nil {
			posterKey := buildVideoImageOutputObjectKey(prefix, task.Format, fmt.Sprintf("clip_%02d_poster.jpg", task.WindowIndex))
			if err := uploadFileToQiniu(uploader, p.qiniu, posterKey, posterPath); err == nil {
				thumbKey = posterKey
				uploads = append(uploads, posterKey)
				posterSize, posterW, posterH := readImageInfo(posterPath)
				artifacts = append(artifacts, animatedArtifactPayload{
					Type:      "poster",
					Key:       posterKey,
					MimeType:  "image/jpeg",
					SizeBytes: posterSize,
					Width:     posterW,
					Height:    posterH,
					Metadata: map[string]interface{}{
						"window_index": task.WindowIndex,
						"format":       task.Format,
						"motion_level": adaptiveProfile.Level,
					},
				})
			}
		}
	}
	for idx := range artifacts {
		appendWindowBindingMetadata(artifacts[idx].Metadata, window)
	}

	result.FileKey = fileKey
	result.ThumbKey = thumbKey
	result.Width = width
	result.Height = height
	result.SizeBytes = sizeBytes
	result.UploadedKeys = uploads
	result.Artifacts = artifacts
	return result
}

func uploadQiniuTasksConcurrently(
	ctx context.Context,
	uploader *qiniustorage.FormUploader,
	q *storage.QiniuClient,
	tasks []qiniuUploadTask,
	maxConcurrency int,
) ([]string, error) {
	if len(tasks) == 0 {
		return nil, nil
	}
	if maxConcurrency < 1 {
		maxConcurrency = 1
	}
	if maxConcurrency > len(tasks) {
		maxConcurrency = len(tasks)
	}

	g, gctx := errgroup.WithContext(ctx)
	sem := make(chan struct{}, maxConcurrency)
	successMap := make(map[string]struct{}, len(tasks))
	var mu sync.Mutex
	for _, task := range tasks {
		task := task
		g.Go(func() error {
			select {
			case sem <- struct{}{}:
			case <-gctx.Done():
				return gctx.Err()
			}
			defer func() { <-sem }()

			if err := uploadFileToQiniu(uploader, q, task.Key, task.Path); err != nil {
				return fmt.Errorf("upload %s failed: %w", task.Label, err)
			}
			mu.Lock()
			successMap[task.Key] = struct{}{}
			mu.Unlock()
			return nil
		})
	}
	err := g.Wait()
	uploaded := make([]string, 0, len(successMap))
	for _, task := range tasks {
		if _, ok := successMap[task.Key]; ok {
			uploaded = append(uploaded, task.Key)
		}
	}
	if err != nil {
		return uploaded, err
	}
	return uploaded, nil
}

func buildAnimatedEmojiTitle(collectionTitle string, windowIndex int, format string) string {
	if strings.EqualFold(format, "live") {
		return fmt.Sprintf("%s-Clip%02d-LIVE", collectionTitle, windowIndex)
	}
	return fmt.Sprintf("%s-Clip%02d-%s", collectionTitle, windowIndex, strings.ToUpper(format))
}

func buildVideoImageOutputObjectKey(prefix string, format string, fileName string) string {
	base := strings.Trim(strings.TrimSpace(prefix), "/")
	format = strings.Trim(strings.ToLower(strings.TrimSpace(format)), "/")
	fileName = strings.Trim(strings.TrimSpace(fileName), "/")
	if format == "" {
		format = "unknown"
	}
	if fileName == "" {
		fileName = "output.bin"
	}
	return path.Join(base, "outputs", format, fileName)
}

func buildVideoImagePackageObjectKey(prefix string, fileName string) string {
	base := strings.Trim(strings.TrimSpace(prefix), "/")
	fileName = strings.Trim(strings.TrimSpace(fileName), "/")
	if fileName == "" {
		fileName = "package.zip"
	}
	return path.Join(base, "package", fileName)
}

func upsertEmojiByCollectionFile(tx *gorm.DB, emoji *models.Emoji) error {
	if tx == nil || emoji == nil {
		return errors.New("invalid emoji upsert input")
	}

	emoji.FileURL = strings.TrimSpace(emoji.FileURL)
	if emoji.CollectionID == 0 || emoji.FileURL == "" {
		return tx.Create(emoji).Error
	}

	var existing models.Emoji
	err := tx.Where("collection_id = ? AND file_url = ?", emoji.CollectionID, emoji.FileURL).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(emoji).Error
		}
		return err
	}

	updates := map[string]interface{}{
		"title":         emoji.Title,
		"thumb_url":     emoji.ThumbURL,
		"format":        emoji.Format,
		"width":         emoji.Width,
		"height":        emoji.Height,
		"size_bytes":    emoji.SizeBytes,
		"display_order": emoji.DisplayOrder,
		"status":        emoji.Status,
	}
	if err := tx.Model(&models.Emoji{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
		return err
	}
	emoji.ID = existing.ID
	return nil
}

func upsertVideoJobArtifactByJobKey(tx *gorm.DB, artifact *models.VideoJobArtifact) error {
	if tx == nil || artifact == nil {
		return errors.New("invalid artifact upsert input")
	}

	artifact.QiniuKey = strings.TrimSpace(artifact.QiniuKey)
	if artifact.JobID == 0 || artifact.QiniuKey == "" {
		return tx.Create(artifact).Error
	}

	var existing models.VideoJobArtifact
	err := tx.Where("job_id = ? AND qiniu_key = ?", artifact.JobID, artifact.QiniuKey).First(&existing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(artifact).Error; err != nil {
				return err
			}
			return UpsertPublicVideoImageOutputByArtifact(tx, *artifact)
		}
		return err
	}

	updates := map[string]interface{}{
		"type":        artifact.Type,
		"mime_type":   artifact.MimeType,
		"size_bytes":  artifact.SizeBytes,
		"width":       artifact.Width,
		"height":      artifact.Height,
		"duration_ms": artifact.DurationMs,
		"metadata":    artifact.Metadata,
	}
	if err := tx.Model(&models.VideoJobArtifact{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
		return err
	}
	artifact.ID = existing.ID
	return UpsertPublicVideoImageOutputByArtifact(tx, *artifact)
}

func splitVideoOutputFormats(formats []string) ([]string, []string) {
	staticFormats := make([]string, 0, 2)
	animatedFormats := make([]string, 0, 3)
	for _, format := range formats {
		switch format {
		case "jpg", "png":
			if !containsString(staticFormats, format) {
				staticFormats = append(staticFormats, format)
			}
		case "gif", "webp", "mp4", "live":
			if !containsString(animatedFormats, format) {
				animatedFormats = append(animatedFormats, format)
			}
		}
	}
	return staticFormats, animatedFormats
}

func applyAnimatedProfileDefaults(options jobOptions, formats []string, qualitySettings QualitySettings) jobOptions {
	qualitySettings = NormalizeQualitySettings(qualitySettings)

	if options.FPS <= 0 {
		switch {
		case containsString(formats, "gif"):
			if qualitySettings.GIFProfile == QualityProfileSize {
				options.FPS = 10
			} else {
				options.FPS = 14
			}
		case containsString(formats, "webp"):
			if qualitySettings.WebPProfile == QualityProfileSize {
				options.FPS = 10
			} else {
				options.FPS = 12
			}
		case containsString(formats, "live"), containsString(formats, "mp4"):
			if qualitySettings.LiveProfile == QualityProfileSize {
				options.FPS = 10
			} else {
				options.FPS = 12
			}
		}
	}

	if options.MaxColors <= 0 && containsString(formats, "gif") {
		if qualitySettings.GIFProfile == QualityProfileSize {
			options.MaxColors = 96
		} else {
			options.MaxColors = 192
		}
	}

	if options.Width <= 0 {
		switch {
		case containsString(formats, "gif") && qualitySettings.GIFProfile == QualityProfileSize:
			options.Width = 720
		case containsString(formats, "webp") && qualitySettings.WebPProfile == QualityProfileSize:
			options.Width = 720
		case containsString(formats, "live") && qualitySettings.LiveProfile == QualityProfileSize:
			options.Width = 1080
		}
	}

	return options
}

func applyStillProfileDefaults(options jobOptions, formats []string, qualitySettings QualitySettings) jobOptions {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	options.RequestedJPG = containsString(formats, "jpg")
	options.RequestedPNG = containsString(formats, "png")
	if options.Width > 0 {
		return options
	}

	jpgSizeMode := options.RequestedJPG && qualitySettings.JPGProfile == QualityProfileSize
	pngSizeMode := options.RequestedPNG && qualitySettings.PNGProfile == QualityProfileSize
	jpgClarityMode := options.RequestedJPG && qualitySettings.JPGProfile == QualityProfileClarity
	pngClarityMode := options.RequestedPNG && qualitySettings.PNGProfile == QualityProfileClarity
	if (jpgSizeMode || pngSizeMode) && !(jpgClarityMode || pngClarityMode) {
		options.Width = 1280
	}
	return options
}

func resolveOutputClipWindows(meta videoProbeMeta, options jobOptions, candidates []highlightCandidate, maxOutputs int) []highlightCandidate {
	if maxOutputs <= 0 {
		maxOutputs = defaultHighlightTopN
	}
	if maxOutputs > 6 {
		maxOutputs = 6
	}

	windows := make([]highlightCandidate, 0, maxOutputs)
	for _, candidate := range candidates {
		start, end := clampHighlightWindow(candidate.StartSec, candidate.EndSec, meta.DurationSec)
		if end <= start {
			continue
		}
		windows = append(windows, highlightCandidate{
			StartSec:     start,
			EndSec:       end,
			Score:        candidate.Score,
			Reason:       candidate.Reason,
			ProposalRank: candidate.ProposalRank,
			ProposalID:   candidate.ProposalID,
			CandidateID:  candidate.CandidateID,
		})
		if len(windows) >= maxOutputs {
			return windows
		}
	}

	startSec, durationSec := resolveClipWindow(meta, options)
	if durationSec > 0 {
		return []highlightCandidate{{
			StartSec: startSec,
			EndSec:   startSec + durationSec,
			Score:    1,
			Reason:   "single_window",
		}}
	}
	if meta.DurationSec > 0 {
		if startSec > 0 && startSec < meta.DurationSec {
			return []highlightCandidate{{
				StartSec: startSec,
				EndSec:   meta.DurationSec,
				Score:    1,
				Reason:   "single_window",
			}}
		}
		defaultWindow := chooseHighlightDuration(meta.DurationSec)
		end := defaultWindow
		if end > meta.DurationSec {
			end = meta.DurationSec
		}
		return []highlightCandidate{{
			StartSec: 0,
			EndSec:   end,
			Score:    1,
			Reason:   "single_window",
		}}
	}
	return nil
}

func appendWindowBindingMetadata(meta map[string]interface{}, window highlightCandidate) {
	if len(meta) == 0 {
		return
	}
	if window.ProposalRank > 0 {
		meta["proposal_rank"] = window.ProposalRank
	}
	if window.ProposalID != nil && *window.ProposalID > 0 {
		meta["proposal_id"] = *window.ProposalID
	}
	if window.CandidateID != nil && *window.CandidateID > 0 {
		meta["candidate_id"] = *window.CandidateID
	}
}

func renderClipOutput(
	ctx context.Context,
	sourcePath string,
	outputPath string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	window highlightCandidate,
	format string,
) error {
	startSec := window.StartSec
	durationSec := window.EndSec - window.StartSec
	if durationSec <= 0 {
		return errors.New("invalid clip window")
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
	}
	if startSec > 0 {
		args = append(args, "-ss", formatFFmpegNumber(startSec))
	}
	args = append(args, "-i", sourcePath, "-t", formatFFmpegNumber(durationSec))

	filters := buildAnimatedFilters(meta, options, format)
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}

	switch format {
	case "gif":
		return renderGIFOutput(ctx, sourcePath, outputPath, meta, options, qualitySettings, window)
	case "webp":
		return renderWebPOutput(ctx, sourcePath, outputPath, meta, options, qualitySettings, window)
	case "mp4":
		supported, err := supportsFFmpegEncoder("libx264")
		if err != nil {
			return fmt.Errorf("check libx264 encoder support: %w", err)
		}
		if !supported {
			return unsupportedOutputFormatError{
				Format: "mp4",
				Reason: "ffmpeg missing libx264 encoder",
			}
		}
		args = append(args, "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", outputPath)
	default:
		return fmt.Errorf("unsupported animated output format: %s", format)
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg render failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func renderGIFOutput(
	ctx context.Context,
	sourcePath string,
	outputPath string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	window highlightCandidate,
) error {
	startSec := window.StartSec
	durationSec := window.EndSec - window.StartSec
	if durationSec <= 0 {
		return errors.New("invalid gif clip window")
	}

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	maxColors := options.MaxColors
	if maxColors <= 0 {
		maxColors = qualitySettings.GIFDefaultMaxColors
		if qualitySettings.GIFProfile == QualityProfileSize && maxColors > 96 {
			maxColors = 96
		}
		if qualitySettings.GIFProfile == QualityProfileClarity && maxColors < 160 {
			maxColors = 160
		}
	}
	if maxColors < 16 {
		maxColors = 16
	}
	if maxColors > 256 {
		maxColors = 256
	}

	gifOptions := options
	if gifOptions.FPS <= 0 {
		gifOptions.FPS = qualitySettings.GIFDefaultFPS
		if qualitySettings.GIFProfile == QualityProfileSize && gifOptions.FPS > 10 {
			gifOptions.FPS = 10
		}
		if qualitySettings.GIFProfile == QualityProfileClarity && gifOptions.FPS < 12 {
			gifOptions.FPS = 12
		}
	}
	ditherMode := qualitySettings.GIFDitherMode
	targetBytes := int64(qualitySettings.GIFTargetSizeKB) * 1024
	if targetBytes < 0 {
		targetBytes = 0
	}
	segmentTimeout := chooseGIFSegmentRenderTimeout(meta, gifOptions, window, maxColors)
	timeoutFallbackApplied := false
	emergencyFallbackApplied := false

	for attempt := 0; attempt < 6; attempt++ {
		args := []string{
			"-hide_banner",
			"-loglevel", "error",
			"-y",
		}
		if startSec > 0 {
			args = append(args, "-ss", formatFFmpegNumber(startSec))
		}
		args = append(args, "-i", sourcePath, "-t", formatFFmpegNumber(durationSec))
		baseFilters := buildAnimatedFilters(meta, gifOptions, "gif")
		var complex string
		if len(baseFilters) > 0 {
			complex = fmt.Sprintf(
				"[0:v]%s,split[v0][v1];[v0]palettegen=stats_mode=diff:max_colors=%d[p];[v1][p]paletteuse=dither=%s:diff_mode=rectangle[v]",
				strings.Join(baseFilters, ","),
				maxColors,
				ditherMode,
			)
		} else {
			complex = fmt.Sprintf(
				"[0:v]split[v0][v1];[v0]palettegen=stats_mode=diff:max_colors=%d[p];[v1][p]paletteuse=dither=%s:diff_mode=rectangle[v]",
				maxColors,
				ditherMode,
			)
		}
		args = append(args,
			"-filter_complex", complex,
			"-map", "[v]",
			"-an",
			"-loop", "0",
			outputPath,
		)
		out, err, timedOut := runFFmpegWithTimeout(ctx, segmentTimeout, args)
		if err != nil {
			if timedOut {
				if !timeoutFallbackApplied {
					nextOptions, nextColors, nextDither, changed := applyGIFTimeoutFallbackProfile(gifOptions, maxColors, ditherMode, meta.DurationSec)
					if changed {
						gifOptions = nextOptions
						maxColors = nextColors
						ditherMode = nextDither
						timeoutFallbackApplied = true
						segmentTimeout = chooseGIFSegmentRenderTimeout(meta, gifOptions, highlightCandidate{
							StartSec: startSec,
							EndSec:   startSec + durationSec,
						}, maxColors)
						continue
					}
				}
				if !emergencyFallbackApplied {
					nextOptions, nextColors, nextDither, nextDuration, changed := applyGIFEmergencyFallbackProfile(gifOptions, maxColors, ditherMode, durationSec)
					if changed {
						gifOptions = nextOptions
						maxColors = nextColors
						ditherMode = nextDither
						durationSec = nextDuration
						emergencyFallbackApplied = true
						segmentTimeout = chooseGIFSegmentRenderTimeout(meta, gifOptions, highlightCandidate{
							StartSec: startSec,
							EndSec:   startSec + durationSec,
						}, maxColors)
						continue
					}
				}
				return permanentError{err: fmt.Errorf(
					"ffmpeg gif render timeout after %s (attempt=%d): %s",
					segmentTimeout.String(),
					attempt+1,
					strings.TrimSpace(string(out)),
				)}
			}
			return fmt.Errorf("ffmpeg gif render failed: %w: %s", err, strings.TrimSpace(string(out)))
		}
		if targetBytes <= 0 {
			break
		}
		info, statErr := os.Stat(outputPath)
		if statErr == nil && info.Size() <= targetBytes {
			break
		}
		changed := false
		if maxColors > 96 {
			maxColors -= 32
			if maxColors < 96 {
				maxColors = 96
			}
			changed = true
		} else if gifOptions.FPS > 8 {
			gifOptions.FPS -= 2
			if gifOptions.FPS < 8 {
				gifOptions.FPS = 8
			}
			changed = true
		} else if gifOptions.Width > 0 && gifOptions.Width > 480 {
			nextWidth := int(math.Round(float64(gifOptions.Width) * 0.85))
			if nextWidth%2 == 1 {
				nextWidth--
			}
			if nextWidth < 360 {
				nextWidth = 360
			}
			if nextWidth < gifOptions.Width {
				gifOptions.Width = nextWidth
				changed = true
			}
		} else if maxColors > 48 {
			maxColors -= 16
			if maxColors < 48 {
				maxColors = 48
			}
			changed = true
		} else if ditherMode != "none" {
			ditherMode = "none"
			changed = true
		}
		if !changed {
			break
		}
	}
	return nil
}

func chooseGIFSegmentRenderTimeout(
	meta videoProbeMeta,
	options jobOptions,
	window highlightCandidate,
	maxColors int,
) time.Duration {
	durationSec := window.EndSec - window.StartSec
	if durationSec <= 0 {
		durationSec = 2.4
	}
	timeoutSec := 30.0 + durationSec*8.0

	if meta.DurationSec >= gifLongVideoThreshold {
		timeoutSec += 12
	}
	if meta.DurationSec >= gifUltraVideoThreshold {
		timeoutSec += 8
	}

	targetWidth := options.Width
	if targetWidth <= 0 {
		targetWidth = meta.Width
	}
	switch {
	case targetWidth >= 1080:
		timeoutSec += 22
	case targetWidth >= 960:
		timeoutSec += 14
	case targetWidth >= 720:
		timeoutSec += 8
	}

	targetFPS := options.FPS
	if targetFPS <= 0 {
		targetFPS = 12
	}
	if targetFPS >= 16 {
		timeoutSec += 7
	}
	if targetFPS >= 20 {
		timeoutSec += 4
	}

	switch {
	case maxColors >= 192:
		timeoutSec += 6
	case maxColors >= 128:
		timeoutSec += 3
	}

	timeout := time.Duration(math.Round(timeoutSec)) * time.Second
	if timeout < gifRenderTimeoutMin {
		return gifRenderTimeoutMin
	}
	if timeout > gifRenderTimeoutMax {
		return gifRenderTimeoutMax
	}
	return timeout
}

func applyGIFTimeoutFallbackProfile(
	options jobOptions,
	maxColors int,
	ditherMode string,
	sourceDurationSec float64,
) (jobOptions, int, string, bool) {
	next := options
	changed := false

	fpsCap := 10
	widthCap := 720
	colorsCap := 96
	minWidth := 360
	if sourceDurationSec >= gifUltraVideoThreshold {
		fpsCap = 8
		widthCap = 640
		colorsCap = 64
	}

	if next.FPS <= 0 || next.FPS > fpsCap {
		next.FPS = fpsCap
		changed = true
	}
	if next.Width <= 0 || next.Width > widthCap {
		next.Width = widthCap
		changed = true
	}
	if next.Width > 0 && next.Width%2 != 0 {
		next.Width--
		changed = true
	}
	if next.Width > 0 && next.Width < minWidth {
		next.Width = minWidth
		changed = true
	}

	if maxColors <= 0 || maxColors > colorsCap {
		maxColors = colorsCap
		changed = true
	}

	mode := strings.ToLower(strings.TrimSpace(ditherMode))
	if mode != "none" {
		ditherMode = "none"
		changed = true
	}

	return next, maxColors, ditherMode, changed
}

func applyGIFEmergencyFallbackProfile(
	options jobOptions,
	maxColors int,
	ditherMode string,
	durationSec float64,
) (jobOptions, int, string, float64, bool) {
	next := options
	changed := false

	if next.FPS <= 0 || next.FPS > 8 {
		next.FPS = 8
		changed = true
	}
	if next.Width <= 0 || next.Width > 540 {
		next.Width = 540
		changed = true
	}
	if next.Width > 0 && next.Width%2 != 0 {
		next.Width--
		changed = true
	}
	if next.Width > 0 && next.Width < 320 {
		next.Width = 320
		changed = true
	}
	if maxColors <= 0 || maxColors > 64 {
		maxColors = 64
		changed = true
	}
	mode := strings.ToLower(strings.TrimSpace(ditherMode))
	if mode != "none" {
		ditherMode = "none"
		changed = true
	}

	nextDuration := durationSec
	if nextDuration > 2.0 {
		nextDuration = math.Max(1.4, nextDuration*0.75)
		changed = true
	}
	return next, maxColors, ditherMode, nextDuration, changed
}

func runFFmpegWithTimeout(ctx context.Context, timeout time.Duration, args []string) ([]byte, error, bool) {
	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return out, nil, false
	}

	timedOut := false
	if timeout > 0 && runCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		timedOut = true
	}
	return out, err, timedOut
}

func optimizeGIFLoopWindow(
	ctx context.Context,
	sourcePath string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	window highlightCandidate,
) (highlightCandidate, gifLoopTuningResult, error) {
	result := gifLoopTuningResult{
		BaseStartSec:  window.StartSec,
		BaseEndSec:    window.EndSec,
		TunedStartSec: window.StartSec,
		TunedEndSec:   window.EndSec,
		DurationSec:   window.EndSec - window.StartSec,
		EffectiveSec:  window.EndSec - window.StartSec,
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	if !qualitySettings.GIFLoopTuneEnabled {
		result.DecisionReason = "feature_disabled"
		return window, result, nil
	}
	durationSec := window.EndSec - window.StartSec
	if durationSec < qualitySettings.GIFLoopTuneMinEnableSec {
		result.DecisionReason = "duration_below_min_enable"
		result.MinImprovement = roundTo(qualitySettings.GIFLoopTuneMinImprovement, 4)
		return window, result, nil
	}

	sampleFPS := 6.0
	if options.FPS > 0 {
		sampleFPS = clampFloat(float64(options.FPS)*0.6, 4, 10)
	}
	maxFrames := int(math.Round(durationSec * sampleFPS))
	if maxFrames < 16 {
		maxFrames = 16
	}
	if maxFrames > 72 {
		maxFrames = 72
	}

	sampleDir, err := os.MkdirTemp("", "gif-loop-tune-*")
	if err != nil {
		return window, result, err
	}
	defer os.RemoveAll(sampleDir)

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
	}
	if window.StartSec > 0 {
		args = append(args, "-ss", formatFFmpegNumber(window.StartSec))
	}
	args = append(args,
		"-i", sourcePath,
		"-t", formatFFmpegNumber(durationSec),
		"-vf", fmt.Sprintf("fps=%s,scale=160:-1:flags=lanczos", formatFFmpegNumber(sampleFPS)),
		"-frames:v", strconv.Itoa(maxFrames),
		filepath.Join(sampleDir, "frame_%03d.jpg"),
	)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return window, result, fmt.Errorf("ffmpeg gif loop sample failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	paths, err := collectFramePaths(sampleDir, maxFrames)
	if err != nil {
		return window, result, err
	}
	if len(paths) < 6 {
		result.SampleFrames = len(paths)
		result.DecisionReason = "insufficient_sample_frames"
		return window, result, nil
	}

	samples := analyzeFrameQualityBatch(paths, minInt(qualitySettings.QualityAnalysisWorkers, 6))
	if len(samples) < 6 {
		result.SampleFrames = len(samples)
		result.DecisionReason = "insufficient_quality_samples"
		return window, result, nil
	}
	if len(samples) != len(paths) {
		// analyzeFrameQualityBatch may drop unreadable frames; keep timeline deterministic by truncating.
		if len(samples) < 6 {
			result.SampleFrames = len(samples)
			result.DecisionReason = "insufficient_quality_samples"
			return window, result, nil
		}
	}

	blurScores := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if sample.BlurScore > 0 {
			blurScores = append(blurScores, sample.BlurScore)
		}
	}
	blurThreshold := chooseBlurThreshold(blurScores, qualitySettings)
	step := durationSec / float64(maxIntValue(1, len(samples)-1))
	loopSamples := make([]gifLoopSampleFrame, 0, len(samples))
	for idx := range samples {
		samples[idx].Exposure = roundTo(computeExposureScore(samples[idx].Brightness, qualitySettings), 3)
		samples[idx].QualityScore = roundTo(computeFrameQualityScore(samples[idx], blurThreshold), 3)
		ts := window.StartSec + float64(idx)*step
		if ts > window.EndSec {
			ts = window.EndSec
		}
		loopSamples = append(loopSamples, gifLoopSampleFrame{
			TimestampSec: ts,
			Hash:         samples[idx].Hash,
			QualityScore: clampZeroOne(samples[idx].QualityScore),
		})
	}

	tunedWindow, tunedResult := selectBestGIFLoopWindowFromSamples(window, loopSamples, qualitySettings)
	tunedResult.SampleFrames = len(loopSamples)
	if tunedResult.Candidates == 0 {
		if strings.TrimSpace(tunedResult.DecisionReason) == "" {
			tunedResult.DecisionReason = "no_candidate_window"
		}
		return window, tunedResult, nil
	}
	if tunedResult.Applied {
		tunedWindow = clampWindowDuration(tunedWindow, tunedWindow.EndSec-tunedWindow.StartSec, meta.DurationSec)
		tunedResult.EffectiveSec = roundTo(tunedWindow.EndSec-tunedWindow.StartSec, 3)
		if strings.TrimSpace(tunedResult.DecisionReason) == "" {
			tunedResult.DecisionReason = "applied"
		}
	} else if strings.TrimSpace(tunedResult.DecisionReason) == "" {
		tunedResult.DecisionReason = "not_applied"
	}
	return tunedWindow, tunedResult, nil
}

func selectBestGIFLoopWindowFromSamples(window highlightCandidate, samples []gifLoopSampleFrame, qualitySettings QualitySettings) (highlightCandidate, gifLoopTuningResult) {
	result := gifLoopTuningResult{
		BaseStartSec:  window.StartSec,
		BaseEndSec:    window.EndSec,
		TunedStartSec: window.StartSec,
		TunedEndSec:   window.EndSec,
		DurationSec:   window.EndSec - window.StartSec,
		EffectiveSec:  window.EndSec - window.StartSec,
		SampleFrames:  len(samples),
	}
	if len(samples) < 6 {
		result.DecisionReason = "insufficient_samples"
		return window, result
	}
	baseDuration := window.EndSec - window.StartSec
	if baseDuration <= 0 {
		result.DecisionReason = "invalid_base_duration"
		return window, result
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	minDuration := math.Min(1.6, baseDuration*0.75)
	if minDuration < qualitySettings.GIFLoopTuneMinEnableSec {
		minDuration = qualitySettings.GIFLoopTuneMinEnableSec
	}
	maxDuration := baseDuration
	if maxDuration > 3.2 {
		maxDuration = 3.2
	}
	motionTarget := qualitySettings.GIFLoopTuneMotionTarget
	preferDuration := qualitySettings.GIFLoopTunePreferDuration
	durationSpan := clampFloat(preferDuration*0.67, 0.6, 2.4)
	minImprovement := qualitySettings.GIFLoopTuneMinImprovement

	evaluateRange := func(startIdx, endIdx int) (float64, float64, float64, float64) {
		if startIdx < 0 || endIdx >= len(samples) || endIdx <= startIdx {
			return -1, 0, 0, 0
		}
		duration := samples[endIdx].TimestampSec - samples[startIdx].TimestampSec
		if duration < minDuration || duration > maxDuration {
			return -1, 0, 0, duration
		}
		loopClosure := 1 - float64(hammingDistance64(samples[startIdx].Hash, samples[endIdx].Hash))/64.0
		loopClosure = clampZeroOne(loopClosure)
		motionMean := meanHashMotion(samples[startIdx : endIdx+1])
		motionScore := 1 - math.Abs(motionMean-motionTarget)/motionTarget
		motionScore = clampZeroOne(motionScore)
		qualityMean := clampZeroOne((samples[startIdx].QualityScore + samples[endIdx].QualityScore) / 2)
		durationScore := 1 - math.Abs(duration-preferDuration)/durationSpan
		durationScore = clampZeroOne(durationScore)
		totalScore := loopClosure*0.52 + motionScore*0.22 + qualityMean*0.16 + durationScore*0.10
		return totalScore, loopClosure, motionMean, duration
	}

	baseScore, baseLoop, baseMotion, baseDurationEval := evaluateRange(0, len(samples)-1)
	if baseScore < 0 {
		baseScore = 0
		baseLoop = 0
		baseMotion = 0
		baseDurationEval = baseDuration
	}
	result.BaseScore = roundTo(baseScore, 3)
	result.BaseLoop = roundTo(baseLoop, 3)
	result.BaseMotion = roundTo(baseMotion, 3)
	adaptiveMinImprovement := minImprovement
	if minImprovement <= 0.05 {
		if baseLoop < 0.6 {
			adaptiveMinImprovement = math.Min(adaptiveMinImprovement, 0.025)
		}
		if baseLoop < 0.5 {
			adaptiveMinImprovement = math.Min(adaptiveMinImprovement, 0.015)
		}
	}
	result.MinImprovement = roundTo(adaptiveMinImprovement, 4)
	result.Score = roundTo(baseScore, 3)
	result.LoopClosure = roundTo(baseLoop, 3)
	result.MotionMean = roundTo(baseMotion, 3)
	result.QualityMean = roundTo((samples[0].QualityScore+samples[len(samples)-1].QualityScore)/2, 3)
	result.DurationSec = roundTo(baseDurationEval, 3)

	bestScore := baseScore
	bestStart := 0
	bestEnd := len(samples) - 1
	bestLoop := baseLoop
	bestMotion := baseMotion
	bestDuration := baseDurationEval
	candidates := 0

	for i := 0; i < len(samples)-2; i++ {
		for j := i + 2; j < len(samples); j++ {
			score, loopClosure, motionMean, duration := evaluateRange(i, j)
			if score < 0 {
				continue
			}
			candidates++
			if score > bestScore {
				bestScore = score
				bestStart = i
				bestEnd = j
				bestLoop = loopClosure
				bestMotion = motionMean
				bestDuration = duration
			}
		}
	}
	result.Candidates = candidates
	if candidates == 0 {
		result.BestScore = result.BaseScore
		result.BestLoop = result.BaseLoop
		result.BestMotion = result.BaseMotion
		result.Improvement = 0
		result.DecisionReason = "no_candidate_window"
		return window, result
	}

	improvement := bestScore - baseScore
	loopGain := bestLoop - baseLoop
	result.BestScore = roundTo(bestScore, 3)
	result.BestLoop = roundTo(bestLoop, 3)
	result.BestMotion = roundTo(bestMotion, 3)
	result.Improvement = roundTo(improvement, 4)

	softApply := shouldApplyGIFLoopTuneBySoftGate(
		improvement,
		adaptiveMinImprovement,
		baseScore,
		bestScore,
		baseLoop,
		bestLoop,
		baseMotion,
		bestMotion,
		motionTarget,
	)
	if improvement < adaptiveMinImprovement && !softApply {
		switch {
		case baseLoop >= 0.82 && loopGain <= 0.05:
			result.DecisionReason = "already_loop_stable"
		case loopGain >= 0.08 && improvement > 0:
			result.DecisionReason = "loop_gain_but_score_small"
		default:
			result.DecisionReason = "improvement_below_threshold"
		}
		return window, result
	}
	tunedStart := samples[bestStart].TimestampSec
	tunedEnd := samples[bestEnd].TimestampSec
	if tunedEnd <= tunedStart {
		result.DecisionReason = "invalid_tuned_window"
		return window, result
	}
	tunedWindow := window
	tunedWindow.StartSec = tunedStart
	tunedWindow.EndSec = tunedEnd
	tunedWindow.Score = window.Score + improvement*0.08

	result.Applied = true
	result.TunedStartSec = tunedStart
	result.TunedEndSec = tunedEnd
	result.DurationSec = roundTo(bestDuration, 3)
	result.EffectiveSec = result.DurationSec
	result.Score = roundTo(bestScore, 3)
	result.LoopClosure = roundTo(bestLoop, 3)
	result.MotionMean = roundTo(bestMotion, 3)
	result.QualityMean = roundTo((samples[bestStart].QualityScore+samples[bestEnd].QualityScore)/2, 3)
	if softApply {
		result.DecisionReason = "soft_loop_closure_gain"
	} else {
		result.DecisionReason = "score_improvement_gate_pass"
	}
	return tunedWindow, result
}

func shouldApplyGIFLoopTuneBySoftGate(
	improvement float64,
	minImprovement float64,
	baseScore float64,
	bestScore float64,
	baseLoop float64,
	bestLoop float64,
	baseMotion float64,
	bestMotion float64,
	motionTarget float64,
) bool {
	// Respect explicitly strict thresholds from ops (e.g. manual roll-back / conservative mode).
	if minImprovement > 0.05 {
		return false
	}
	// Soft gate is only for near-threshold improvements and should not accept score regressions.
	if improvement < 0 {
		return false
	}
	if bestScore+1e-6 < baseScore-0.003 {
		return false
	}

	loopGain := bestLoop - baseLoop
	requiredLoopGain := 0.06
	if baseLoop < 0.6 {
		requiredLoopGain = 0.045
	}
	if baseLoop < 0.5 {
		requiredLoopGain = 0.03
	}
	if loopGain < requiredLoopGain {
		return false
	}

	softMinImprovement := clampFloat(minImprovement*0.35, 0.004, 0.02)
	if improvement < softMinImprovement {
		// Permit very small score gains only when loop closure gain is clearly significant.
		if !(loopGain >= 0.1 && improvement >= -0.001) {
			return false
		}
	}

	baseMotionDelta := math.Abs(baseMotion - motionTarget)
	bestMotionDelta := math.Abs(bestMotion - motionTarget)
	if bestMotionDelta-baseMotionDelta > 0.1 {
		return false
	}
	return true
}

func meanHashMotion(samples []gifLoopSampleFrame) float64 {
	if len(samples) <= 1 {
		return 0
	}
	total := 0.0
	count := 0
	for idx := 1; idx < len(samples); idx++ {
		total += float64(hammingDistance64(samples[idx-1].Hash, samples[idx].Hash)) / 64.0
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func renderWebPOutput(
	ctx context.Context,
	sourcePath string,
	outputPath string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	window highlightCandidate,
) error {
	startSec := window.StartSec
	durationSec := window.EndSec - window.StartSec
	if durationSec <= 0 {
		return errors.New("invalid webp clip window")
	}

	encoder, err := resolveWebPEncoder()
	if err != nil {
		return err
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	webpOptions := options
	if webpOptions.FPS <= 0 {
		webpOptions.FPS = 12
	}
	if qualitySettings.WebPProfile == QualityProfileSize && webpOptions.FPS > 10 {
		webpOptions.FPS = 10
	}

	webpQ := 78
	webpCompression := 5
	if qualitySettings.WebPProfile == QualityProfileSize {
		webpQ = 62
		webpCompression = 6
	}
	targetBytes := int64(qualitySettings.WebPTargetSizeKB) * 1024
	if targetBytes < 0 {
		targetBytes = 0
	}
	for attempt := 0; attempt < 6; attempt++ {
		args := []string{
			"-hide_banner",
			"-loglevel", "error",
			"-y",
		}
		if startSec > 0 {
			args = append(args, "-ss", formatFFmpegNumber(startSec))
		}
		args = append(args, "-i", sourcePath, "-t", formatFFmpegNumber(durationSec))
		filters := buildAnimatedFilters(meta, webpOptions, "webp")
		if len(filters) > 0 {
			args = append(args, "-vf", strings.Join(filters, ","))
		}
		args = append(args,
			"-an",
			"-c:v", encoder,
			"-q:v", strconv.Itoa(webpQ),
			"-compression_level", strconv.Itoa(webpCompression),
			"-loop", "0",
			outputPath,
		)
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		out, runErr := cmd.CombinedOutput()
		if runErr != nil {
			return fmt.Errorf("ffmpeg webp render failed: %w: %s", runErr, strings.TrimSpace(string(out)))
		}
		if targetBytes <= 0 {
			break
		}
		info, statErr := os.Stat(outputPath)
		if statErr == nil && info.Size() <= targetBytes {
			break
		}
		changed := false
		if webpQ > 42 {
			webpQ -= 8
			if webpQ < 42 {
				webpQ = 42
			}
			changed = true
		} else if webpOptions.FPS > 8 {
			webpOptions.FPS -= 2
			if webpOptions.FPS < 8 {
				webpOptions.FPS = 8
			}
			changed = true
		} else if webpOptions.Width > 0 && webpOptions.Width > 480 {
			nextWidth := int(math.Round(float64(webpOptions.Width) * 0.85))
			if nextWidth%2 == 1 {
				nextWidth--
			}
			if nextWidth < 360 {
				nextWidth = 360
			}
			if nextWidth < webpOptions.Width {
				webpOptions.Width = nextWidth
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return nil
}

type liveOutputBundle struct {
	VideoPath        string
	CoverPath        string
	PackagePath      string
	Width            int
	Height           int
	DurationMs       int
	VideoSizeBytes   int64
	CoverSizeBytes   int64
	PackageSizeBytes int64
	CoverScore       float64
	CoverTimestamp   float64
	CoverQuality     float64
	CoverStability   float64
	CoverTemporal    float64
	CoverPortrait    float64
	CoverExposure    float64
	CoverFace        float64
}

type zipEntrySource struct {
	Name string
	Path string
}

type liveCoverSelection struct {
	TimestampSec   float64
	FinalScore     float64
	QualityScore   float64
	StabilityScore float64
	TemporalScore  float64
	PortraitScore  float64
	ExposureScore  float64
	FaceScore      float64
}

func renderLiveOutputPackage(
	ctx context.Context,
	sourcePath string,
	outputDir string,
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	window highlightCandidate,
	clipIndex int,
) (liveOutputBundle, error) {
	supported, err := supportsFFmpegEncoder("libx264")
	if err != nil {
		return liveOutputBundle{}, unsupportedOutputFormatError{
			Format: "live",
			Reason: fmt.Sprintf("check libx264 encoder support failed: %v", err),
		}
	}
	if !supported {
		return liveOutputBundle{}, unsupportedOutputFormatError{
			Format: "live",
			Reason: "ffmpeg missing libx264 encoder required by live package",
		}
	}

	window = clampWindowDuration(window, 3.0, meta.DurationSec)
	profiledOptions := options
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	if qualitySettings.LiveProfile == QualityProfileSize {
		if profiledOptions.Width <= 0 {
			profiledOptions.Width = 1080
		}
		if profiledOptions.FPS <= 0 {
			profiledOptions.FPS = 10
		}
	}

	videoPath := filepath.Join(outputDir, fmt.Sprintf("clip_%02d_live.mov", clipIndex))
	if err := renderClipOutput(ctx, sourcePath, videoPath, meta, profiledOptions, qualitySettings, window, "mp4"); err != nil {
		var unsupported unsupportedOutputFormatError
		if errors.As(err, &unsupported) {
			return liveOutputBundle{}, unsupportedOutputFormatError{
				Format: "live",
				Reason: unsupported.Reason,
			}
		}
		return liveOutputBundle{}, err
	}

	coverPath := filepath.Join(outputDir, fmt.Sprintf("clip_%02d_live_cover.jpg", clipIndex))
	coverSelection, err := extractBestPosterFrame(ctx, videoPath, coverPath, qualitySettings)
	if err != nil {
		// Fallback to the first frame for robustness.
		if fallbackErr := extractPosterFrame(ctx, videoPath, coverPath); fallbackErr != nil {
			return liveOutputBundle{}, err
		}
		coverSelection = liveCoverSelection{
			TimestampSec:   0,
			FinalScore:     0,
			QualityScore:   0,
			StabilityScore: 0,
			TemporalScore:  0,
			PortraitScore:  0,
			ExposureScore:  0,
			FaceScore:      0,
		}
	}

	packagePath := filepath.Join(outputDir, fmt.Sprintf("clip_%02d_live.zip", clipIndex))
	if err := createZipArchive(packagePath, []zipEntrySource{
		{Name: "photo.jpg", Path: coverPath},
		{Name: "video.mov", Path: videoPath},
	}); err != nil {
		return liveOutputBundle{}, err
	}

	videoSize, width, height, durationMs := readMediaOutputInfo(videoPath)
	coverSize, coverW, coverH := readImageInfo(coverPath)
	if coverW > 0 && coverH > 0 {
		width = coverW
		height = coverH
	}

	packageInfo, err := os.Stat(packagePath)
	if err != nil {
		return liveOutputBundle{}, err
	}

	return liveOutputBundle{
		VideoPath:        videoPath,
		CoverPath:        coverPath,
		PackagePath:      packagePath,
		Width:            width,
		Height:           height,
		DurationMs:       durationMs,
		VideoSizeBytes:   videoSize,
		CoverSizeBytes:   coverSize,
		PackageSizeBytes: packageInfo.Size(),
		CoverScore:       coverSelection.FinalScore,
		CoverTimestamp:   coverSelection.TimestampSec,
		CoverQuality:     coverSelection.QualityScore,
		CoverStability:   coverSelection.StabilityScore,
		CoverTemporal:    coverSelection.TemporalScore,
		CoverPortrait:    coverSelection.PortraitScore,
		CoverExposure:    coverSelection.ExposureScore,
		CoverFace:        coverSelection.FaceScore,
	}, nil
}

func clampWindowDuration(window highlightCandidate, maxDurationSec, maxEndSec float64) highlightCandidate {
	if maxDurationSec <= 0 {
		return window
	}
	start := window.StartSec
	end := window.EndSec
	if end <= start {
		end = start + maxDurationSec
	}
	if end-start > maxDurationSec {
		end = start + maxDurationSec
	}
	if maxEndSec > 0 && end > maxEndSec {
		end = maxEndSec
		if end <= start {
			start = math.Max(0, maxEndSec-maxDurationSec)
			end = maxEndSec
		}
	}
	window.StartSec = roundTo(start, 3)
	window.EndSec = roundTo(end, 3)
	return window
}

func createZipArchive(outputPath string, entries []zipEntrySource) error {
	if strings.TrimSpace(outputPath) == "" {
		return errors.New("zip output path is empty")
	}
	if len(entries) == 0 {
		return errors.New("zip entries is empty")
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()

	zipWriter := zip.NewWriter(out)

	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		sourcePath := strings.TrimSpace(entry.Path)
		if name == "" || sourcePath == "" {
			return errors.New("zip entry is invalid")
		}

		in, err := os.Open(sourcePath)
		if err != nil {
			return err
		}

		header := &zip.FileHeader{
			Name:   name,
			Method: zip.Deflate,
		}
		header.SetMode(0o644)
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			_ = in.Close()
			return err
		}
		if _, err := io.Copy(writer, in); err != nil {
			_ = in.Close()
			return err
		}
		if err := in.Close(); err != nil {
			return err
		}
	}
	return zipWriter.Close()
}

func buildAnimatedFilters(meta videoProbeMeta, options jobOptions, format string) []string {
	filters := make([]string, 0, 4)
	if options.Speed > 0 && math.Abs(options.Speed-1.0) > 0.001 {
		filters = append(filters, fmt.Sprintf("setpts=PTS/%.4f", options.Speed))
	}
	if cropFilter, ok := buildCropFilter(meta, options); ok {
		filters = append(filters, cropFilter)
	}
	switch format {
	case "gif", "webp":
		fps := options.FPS
		if fps <= 0 {
			fps = 12
		}
		filters = append(filters, fmt.Sprintf("fps=%d", fps))
	case "mp4":
		if options.FPS > 0 {
			filters = append(filters, fmt.Sprintf("fps=%d", options.FPS))
		}
	}
	if options.Width > 0 {
		filters = append(filters, fmt.Sprintf("scale=%d:-2", options.Width))
	}
	return filters
}

func tuneAnimatedOptionsForWindow(
	meta videoProbeMeta,
	options jobOptions,
	qualitySettings QualitySettings,
	format string,
	window highlightCandidate,
) (jobOptions, animatedAdaptiveProfile) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	profile := animatedAdaptiveProfile{
		MotionScore: estimateWindowMotionScore(window),
		Level:       "medium",
		DurationSec: window.EndSec - window.StartSec,
	}
	if profile.MotionScore < 0.30 {
		profile.Level = "low"
	} else if profile.MotionScore > 0.64 {
		profile.Level = "high"
	}

	tuned := options
	formatProfile := QualityProfileClarity
	switch format {
	case "gif":
		formatProfile = qualitySettings.GIFProfile
	case "webp":
		formatProfile = qualitySettings.WebPProfile
	case "live":
		formatProfile = qualitySettings.LiveProfile
	case "mp4":
		formatProfile = qualitySettings.LiveProfile
	}

	targetFPS := tuned.FPS
	if targetFPS <= 0 {
		switch format {
		case "gif":
			targetFPS = qualitySettings.GIFDefaultFPS
		case "webp", "mp4", "live":
			targetFPS = 12
		}
	}
	switch profile.Level {
	case "low":
		targetFPS -= 2
	case "high":
		targetFPS += 2
	}

	sourceFPSCap := 0
	if meta.FPS > 0 {
		sourceFPSCap = int(math.Round(meta.FPS))
		if sourceFPSCap < 1 {
			sourceFPSCap = 1
		}
	}
	switch format {
	case "gif", "webp":
		minFPS := 6
		if sourceFPSCap > 0 && sourceFPSCap < minFPS {
			minFPS = sourceFPSCap
		}
		if minFPS < 2 {
			minFPS = 2
		}
		if targetFPS < 6 {
			targetFPS = minFPS
		}
		if targetFPS > 18 {
			targetFPS = 18
		}
	case "mp4", "live":
		minFPS := 8
		if sourceFPSCap > 0 && sourceFPSCap < minFPS {
			minFPS = sourceFPSCap
		}
		if minFPS < 4 {
			minFPS = 4
		}
		if targetFPS < 8 {
			targetFPS = minFPS
		}
		if targetFPS > 24 {
			targetFPS = 24
		}
	}
	if sourceFPSCap > 0 && targetFPS > sourceFPSCap {
		targetFPS = sourceFPSCap
	}
	if targetFPS > 0 {
		tuned.FPS = targetFPS
		profile.FPS = targetFPS
	}

	if tuned.Width <= 0 {
		switch format {
		case "gif", "webp":
			if formatProfile == QualityProfileSize {
				switch profile.Level {
				case "low":
					tuned.Width = 640
				case "high":
					tuned.Width = 768
				default:
					tuned.Width = 720
				}
			} else {
				switch profile.Level {
				case "low":
					tuned.Width = 720
				case "high":
					tuned.Width = 1080
				default:
					tuned.Width = 960
				}
			}
		case "mp4":
			if formatProfile == QualityProfileSize {
				tuned.Width = 960
			}
		}
	}
	if tuned.Width > 0 && meta.Width > 0 && tuned.Width > meta.Width {
		tuned.Width = meta.Width
	}
	if tuned.Width > 0 && (strings.EqualFold(format, "mp4") || strings.EqualFold(format, "live")) {
		if tuned.Width%2 != 0 {
			tuned.Width--
		}
		if tuned.Width < 2 {
			tuned.Width = 2
		}
	}
	profile.Width = tuned.Width

	if format == "gif" {
		targetColors := tuned.MaxColors
		if targetColors <= 0 {
			if formatProfile == QualityProfileSize {
				switch profile.Level {
				case "low":
					targetColors = 72
				case "high":
					targetColors = 128
				default:
					targetColors = 96
				}
			} else {
				switch profile.Level {
				case "low":
					targetColors = 128
				case "high":
					targetColors = 224
				default:
					targetColors = 176
				}
			}
		}
		if targetColors < 16 {
			targetColors = 16
		}
		if targetColors > 256 {
			targetColors = 256
		}
		tuned.MaxColors = targetColors
		profile.MaxColors = targetColors
	}

	if format == "gif" || format == "webp" || format == "mp4" {
		switch profile.Level {
		case "low":
			profile.DurationSec = 2.0
		case "high":
			profile.DurationSec = 2.8
		default:
			profile.DurationSec = 2.4
		}
		if formatProfile == QualityProfileSize && profile.DurationSec > 2.4 {
			profile.DurationSec = 2.4
		}
		if windowDuration := window.EndSec - window.StartSec; windowDuration > 0 && profile.DurationSec > windowDuration {
			profile.DurationSec = windowDuration
		}
	}
	if format == "live" {
		profile.DurationSec = 3.0
	}

	applyLongVideoStabilityCaps(meta, format, &tuned, &profile)
	if strings.EqualFold(format, "gif") {
		if tuned.FPS > 0 {
			profile.FPS = tuned.FPS
		}
		if tuned.Width > 0 {
			profile.Width = tuned.Width
		}
		if tuned.MaxColors > 0 {
			profile.MaxColors = tuned.MaxColors
		}
	}

	return tuned, profile
}

func applyLongVideoStabilityCaps(
	meta videoProbeMeta,
	format string,
	tuned *jobOptions,
	profile *animatedAdaptiveProfile,
) {
	if tuned == nil || profile == nil || !strings.EqualFold(format, "gif") {
		return
	}

	sourceDuration := meta.DurationSec
	if sourceDuration < gifLongVideoThreshold {
		longSide := meta.Width
		if meta.Height > longSide {
			longSide = meta.Height
		}
		// 高分辨率视频即使不属于超长时长，也容易触发 GIF palette 渲染超时，提前降档保稳。
		if !(sourceDuration >= 45 && longSide >= 1800) {
			return
		}
	}

	fpsCap := 10
	widthCap := 720
	colorCap := 128
	durationCap := 2.3
	tier := "long"
	longSide := meta.Width
	if meta.Height > longSide {
		longSide = meta.Height
	}
	if sourceDuration < gifLongVideoThreshold && longSide >= 1800 {
		fpsCap = 9
		widthCap = 640
		colorCap = 96
		durationCap = 2.1
		tier = "high_res_stability"
	}
	if sourceDuration >= gifUltraVideoThreshold {
		fpsCap = 8
		widthCap = 640
		colorCap = 96
		durationCap = 2.2
		tier = "ultra_long"
	}

	changed := false
	if tuned.FPS <= 0 || tuned.FPS > fpsCap {
		tuned.FPS = fpsCap
		changed = true
	}
	if tuned.Width <= 0 || tuned.Width > widthCap {
		tuned.Width = widthCap
		changed = true
	}
	if tuned.MaxColors <= 0 || tuned.MaxColors > colorCap {
		tuned.MaxColors = colorCap
		changed = true
	}
	if profile.DurationSec > 0 && profile.DurationSec > durationCap {
		profile.DurationSec = durationCap
		changed = true
	}

	profile.StabilityTier = tier
	profile.LongVideoDownshift = changed
}

func estimateWindowMotionScore(window highlightCandidate) float64 {
	score := math.Max(window.SceneScore, window.Score)
	reason := strings.ToLower(strings.TrimSpace(window.Reason))
	if (reason == "single_window" || reason == "fallback_uniform") && window.SceneScore <= 0 {
		score = 0.45
	}
	if score <= 0 {
		score = 0.45
	}
	if score > 1 {
		score = 1
	}
	return roundTo(score, 3)
}

func extractPosterFrame(ctx context.Context, clipPath, outputPath string) error {
	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-i", clipPath,
		"-frames:v", "1",
		outputPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("extract poster failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

type liveCoverCandidate struct {
	TimestampSec float64
	Path         string
	Sample       frameQualitySample
	QualityScore float64
	PortraitHint float64
	ExposureHint float64
	FaceHint     float64
}

type liveCoverScoringWeights struct {
	Quality   float64
	Stability float64
	Temporal  float64
	Portrait  float64
	Exposure  float64
	Face      float64
}

func extractBestPosterFrame(ctx context.Context, clipPath, outputPath string, qualitySettings QualitySettings) (liveCoverSelection, error) {
	_, _, _, durationMs := readMediaOutputInfo(clipPath)
	durationSec := float64(durationMs) / 1000.0
	if durationSec <= 0 {
		durationSec = 3.0
	}

	timestamps := buildPosterCandidateTimestamps(durationSec)
	if len(timestamps) == 0 {
		timestamps = []float64{0}
	}

	tmpDir, err := os.MkdirTemp("", "live-cover-*")
	if err != nil {
		return liveCoverSelection{}, err
	}
	defer os.RemoveAll(tmpDir)

	qualitySettings = NormalizeQualitySettings(qualitySettings)
	weights := resolveLiveCoverScoringWeights(qualitySettings)
	candidates := make([]liveCoverCandidate, 0, len(timestamps))
	for idx, ts := range timestamps {
		candidatePath := filepath.Join(tmpDir, fmt.Sprintf("cover_%02d.jpg", idx))
		cmd := exec.CommandContext(
			ctx,
			"ffmpeg",
			"-hide_banner",
			"-loglevel", "error",
			"-y",
			"-ss", formatFFmpegNumber(ts),
			"-i", clipPath,
			"-frames:v", "1",
			"-q:v", "2",
			candidatePath,
		)
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			_ = out
			continue
		}
		sample, ok := analyzeFrameQuality(candidatePath)
		if !ok {
			continue
		}
		sample.Exposure = computeExposureScore(sample.Brightness, qualitySettings)
		qualityScore := computeFrameQualityScore(sample, maxFloat(qualitySettings.BlurThresholdMin, 8))
		portraitHint := estimatePortraitHintScore(candidatePath)
		faceHint := estimateFaceQualityHintScore(candidatePath, qualitySettings)
		candidates = append(candidates, liveCoverCandidate{
			TimestampSec: ts,
			Path:         candidatePath,
			Sample:       sample,
			QualityScore: qualityScore,
			PortraitHint: portraitHint,
			FaceHint:     faceHint,
		})
	}
	if len(candidates) == 0 {
		return liveCoverSelection{}, errors.New("no candidate cover extracted")
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].TimestampSec < candidates[j].TimestampSec
	})
	medianBrightness := computeCandidateMedianBrightness(candidates)
	for idx := range candidates {
		candidates[idx].ExposureHint = computePosterExposureConsistency(
			candidates[idx].Sample,
			medianBrightness,
			qualitySettings,
		)
	}

	best := liveCoverSelection{FinalScore: -1}
	bestPath := ""
	for idx, candidate := range candidates {
		stability := computePosterStabilityScore(candidates, idx)
		temporal := computePosterTemporalScore(candidate.TimestampSec, durationSec)
		finalScore := candidate.QualityScore*weights.Quality +
			stability*weights.Stability +
			temporal*weights.Temporal +
			candidate.PortraitHint*weights.Portrait +
			candidate.ExposureHint*weights.Exposure +
			candidate.FaceHint*weights.Face
		if finalScore > best.FinalScore {
			best = liveCoverSelection{
				TimestampSec:   candidate.TimestampSec,
				FinalScore:     finalScore,
				QualityScore:   candidate.QualityScore,
				StabilityScore: stability,
				TemporalScore:  temporal,
				PortraitScore:  candidate.PortraitHint,
				ExposureScore:  candidate.ExposureHint,
				FaceScore:      candidate.FaceHint,
			}
			bestPath = candidate.Path
		}
	}
	if err := copyFile(bestPath, outputPath); err != nil {
		return liveCoverSelection{}, err
	}
	best.TimestampSec = roundTo(best.TimestampSec, 3)
	best.FinalScore = roundTo(best.FinalScore, 3)
	best.QualityScore = roundTo(best.QualityScore, 3)
	best.StabilityScore = roundTo(best.StabilityScore, 3)
	best.TemporalScore = roundTo(best.TemporalScore, 3)
	best.PortraitScore = roundTo(best.PortraitScore, 3)
	best.ExposureScore = roundTo(best.ExposureScore, 3)
	best.FaceScore = roundTo(best.FaceScore, 3)
	return best, nil
}

func resolveLiveCoverScoringWeights(qualitySettings QualitySettings) liveCoverScoringWeights {
	const (
		stabilityWeight = 0.16
		temporalWeight  = 0.10
		exposureWeight  = 0.08
		faceWeight      = 0.06
	)
	portraitWeight := clampFloat(qualitySettings.LiveCoverPortraitWeight, 0, 0.25)
	qualityWeight := 1.0 - stabilityWeight - temporalWeight - portraitWeight - exposureWeight - faceWeight
	if qualityWeight < 0.35 {
		qualityWeight = 0.35
	}
	return liveCoverScoringWeights{
		Quality:   qualityWeight,
		Stability: stabilityWeight,
		Temporal:  temporalWeight,
		Portrait:  portraitWeight,
		Exposure:  exposureWeight,
		Face:      faceWeight,
	}
}

func computeCandidateMedianBrightness(candidates []liveCoverCandidate) float64 {
	if len(candidates) == 0 {
		return 0
	}
	values := make([]float64, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Sample.Brightness <= 0 {
			continue
		}
		values = append(values, candidate.Sample.Brightness)
	}
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

func computePosterExposureConsistency(sample frameQualitySample, medianBrightness float64, qualitySettings QualitySettings) float64 {
	exposure := sample.Exposure
	if exposure <= 0 {
		exposure = computeExposureScore(sample.Brightness, qualitySettings)
	}
	tolerance := (qualitySettings.MaxBrightness - qualitySettings.MinBrightness) * 0.22
	if tolerance < 18 {
		tolerance = 18
	}
	if tolerance > 64 {
		tolerance = 64
	}
	consistency := 1.0
	if medianBrightness > 0 {
		distance := math.Abs(sample.Brightness - medianBrightness)
		consistency = 1 - clampZeroOne(distance/tolerance)
	}
	score := exposure*0.6 + consistency*0.4
	if sample.Brightness < qualitySettings.MinBrightness || sample.Brightness > qualitySettings.MaxBrightness {
		score *= 0.65
	}
	return clampZeroOne(score)
}

func estimateFaceQualityHintScore(filePath string, qualitySettings QualitySettings) float64 {
	f, err := os.Open(filePath)
	if err != nil {
		return 0.45
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return 0.45
	}
	return estimateFaceQualityHintFromImage(prepareQualityAnalysisImage(img, 256), qualitySettings)
}

func estimateFaceQualityHintFromImage(img image.Image, qualitySettings QualitySettings) float64 {
	if img == nil {
		return 0.45
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width < 16 || height < 16 {
		return 0.45
	}

	mask := make([]bool, width*height)
	centerMinX := width / 6
	centerMaxX := width * 5 / 6
	centerMinY := height / 6
	centerMaxY := height * 5 / 6
	if centerMaxX <= centerMinX || centerMaxY <= centerMinY {
		return 0.45
	}

	if rgba, ok := img.(*image.RGBA); ok {
		for y := centerMinY; y < centerMaxY; y++ {
			row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+width*4]
			for x := centerMinX; x < centerMaxX; x++ {
				idx := x * 4
				r := row[idx]
				g := row[idx+1]
				b := row[idx+2]
				yy, cb, cr := color.RGBToYCbCr(r, g, b)
				if yy >= 40 && yy <= 235 && cb >= 77 && cb <= 127 && cr >= 133 && cr <= 173 {
					mask[y*width+x] = true
				}
			}
		}
	} else {
		for y := centerMinY; y < centerMaxY; y++ {
			srcY := bounds.Min.Y + y
			for x := centerMinX; x < centerMaxX; x++ {
				srcX := bounds.Min.X + x
				r16, g16, b16, _ := img.At(srcX, srcY).RGBA()
				yy, cb, cr := color.RGBToYCbCr(uint8(r16/257), uint8(g16/257), uint8(b16/257))
				if yy >= 40 && yy <= 235 && cb >= 77 && cb <= 127 && cr >= 133 && cr <= 173 {
					mask[y*width+x] = true
				}
			}
		}
	}

	largestArea, minX, minY, maxX, maxY := largestMaskComponent(mask, width, height)
	if largestArea <= 0 {
		return 0.45
	}

	componentRatio := float64(largestArea) / float64(width*height)
	areaScore := 1.0
	if componentRatio < 0.02 {
		areaScore = clampZeroOne(componentRatio / 0.02)
	} else if componentRatio > 0.30 {
		areaScore = clampZeroOne(1 - (componentRatio-0.30)/0.30)
	}

	boxW := maxX - minX + 1
	boxH := maxY - minY + 1
	if boxW <= 0 || boxH <= 0 {
		return 0.45
	}
	aspect := float64(boxW) / float64(boxH)
	aspectScore := 1.0
	if aspect < 0.75 {
		aspectScore = clampZeroOne((aspect - 0.45) / 0.30)
	} else if aspect > 1.55 {
		aspectScore = clampZeroOne((2.20 - aspect) / 0.65)
	}

	padX := int(math.Round(float64(boxW) * 0.18))
	padY := int(math.Round(float64(boxH) * 0.18))
	cropRect := image.Rect(minX-padX, minY-padY, maxX+padX+1, maxY+padY+1).Intersect(image.Rect(0, 0, width, height))
	if cropRect.Dx() < 8 || cropRect.Dy() < 8 {
		return 0.45
	}
	faceImg := cropImageToRGBA(img, cropRect)
	faceBlur := laplacianVariance(faceImg)
	faceSharpScore := clampZeroOne(faceBlur / 95.0)
	faceBrightness := imageBrightness(faceImg)
	faceExposure := computeExposureScore(faceBrightness, qualitySettings)

	score := areaScore*0.22 + aspectScore*0.18 + faceSharpScore*0.38 + faceExposure*0.22
	return clampZeroOne(score)
}

func largestMaskComponent(mask []bool, width, height int) (area, minX, minY, maxX, maxY int) {
	if width <= 0 || height <= 0 || len(mask) != width*height {
		return 0, 0, 0, 0, 0
	}
	visited := make([]bool, len(mask))
	bestArea := 0
	bestMinX, bestMinY := 0, 0
	bestMaxX, bestMaxY := 0, 0
	stack := make([]int, 0, 256)

	directions := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for idx := 0; idx < len(mask); idx++ {
		if !mask[idx] || visited[idx] {
			continue
		}
		visited[idx] = true
		stack = append(stack[:0], idx)
		currentArea := 0
		currentMinX, currentMinY := width, height
		currentMaxX, currentMaxY := 0, 0
		for len(stack) > 0 {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x := last % width
			y := last / width
			currentArea++
			if x < currentMinX {
				currentMinX = x
			}
			if y < currentMinY {
				currentMinY = y
			}
			if x > currentMaxX {
				currentMaxX = x
			}
			if y > currentMaxY {
				currentMaxY = y
			}
			for _, d := range directions {
				nx := x + d[0]
				ny := y + d[1]
				if nx < 0 || nx >= width || ny < 0 || ny >= height {
					continue
				}
				nextIdx := ny*width + nx
				if !mask[nextIdx] || visited[nextIdx] {
					continue
				}
				visited[nextIdx] = true
				stack = append(stack, nextIdx)
			}
		}
		if currentArea > bestArea {
			bestArea = currentArea
			bestMinX = currentMinX
			bestMinY = currentMinY
			bestMaxX = currentMaxX
			bestMaxY = currentMaxY
		}
	}
	return bestArea, bestMinX, bestMinY, bestMaxX, bestMaxY
}

func cropImageToRGBA(src image.Image, rect image.Rectangle) *image.RGBA {
	rect = rect.Intersect(src.Bounds())
	if rect.Empty() {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	xdraw.Draw(out, out.Bounds(), src, rect.Min, xdraw.Src)
	return out
}

func buildPosterCandidateTimestamps(durationSec float64) []float64 {
	if durationSec <= 0 {
		return []float64{0}
	}
	anchors := []float64{0.05, 0.22, 0.38, 0.55, 0.72, 0.88}
	out := make([]float64, 0, len(anchors))
	seen := map[int]struct{}{}
	for _, ratio := range anchors {
		ts := durationSec * ratio
		if ts < 0 {
			ts = 0
		}
		if ts > durationSec {
			ts = durationSec
		}
		// Keep unique timestamps at millisecond granularity.
		key := int(math.Round(ts * 1000))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ts)
	}
	return out
}

func computePosterStabilityScore(candidates []liveCoverCandidate, idx int) float64 {
	if len(candidates) == 0 || idx < 0 || idx >= len(candidates) {
		return 0
	}
	neighborSimilarities := make([]float64, 0, 2)
	baseHash := candidates[idx].Sample.Hash
	if idx > 0 {
		diff := float64(hammingDistance64(baseHash, candidates[idx-1].Sample.Hash))
		neighborSimilarities = append(neighborSimilarities, 1-clampZeroOne(diff/22.0))
	}
	if idx+1 < len(candidates) {
		diff := float64(hammingDistance64(baseHash, candidates[idx+1].Sample.Hash))
		neighborSimilarities = append(neighborSimilarities, 1-clampZeroOne(diff/22.0))
	}
	if len(neighborSimilarities) == 0 {
		return 0.6
	}
	sum := 0.0
	for _, score := range neighborSimilarities {
		sum += score
	}
	return clampZeroOne(sum / float64(len(neighborSimilarities)))
}

func computePosterTemporalScore(ts, durationSec float64) float64 {
	if durationSec <= 0 {
		return 0.5
	}
	t := clampZeroOne(ts / durationSec)
	distance := math.Abs(t-0.5) / 0.5
	score := 1 - distance
	if score < 0.2 {
		score = 0.2
	}
	return clampZeroOne(score)
}

func copyFile(srcPath, dstPath string) error {
	in, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func readMediaOutputInfo(filePath string) (int64, int, int, int) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, 0, 0, 0
	}
	size := info.Size()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	meta, err := probeVideo(ctx, filePath)
	if err != nil {
		return size, 0, 0, 0
	}
	durationMs := int(math.Round(meta.DurationSec * 1000))
	return size, meta.Width, meta.Height, durationMs
}

func convertImageToPNG(inputPath, outputPath, profile string, targetSizeKB int) error {
	in, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer in.Close()

	img, _, err := image.Decode(in)
	if err != nil {
		return err
	}

	normalizedProfile := normalizeQualityProfile(profile, QualityProfileClarity)
	if normalizedProfile != QualityProfileSize {
		data, err := encodePNGBytes(img, imagepng.DefaultCompression)
		if err != nil {
			return err
		}
		return writeBytesToFile(outputPath, data)
	}

	budgetBytes := int64(targetSizeKB) * 1024
	widthAttempts := []int{1280, 1152, 1024, 960, 848, 768, 640, 560}
	var best []byte
	lastWidth := 0
	for _, maxWidth := range widthAttempts {
		candidate := fitImageToMaxWidth(img, maxWidth)
		currentWidth := candidate.Bounds().Dx()
		if currentWidth == lastWidth && len(best) > 0 {
			continue
		}
		lastWidth = currentWidth
		encoded, err := encodePNGBytes(candidate, imagepng.BestCompression)
		if err != nil {
			return err
		}
		best = encoded
		if budgetBytes <= 0 || int64(len(encoded)) <= budgetBytes {
			break
		}
	}
	return writeBytesToFile(outputPath, best)
}

func convertImageToJPG(inputPath, outputPath, profile string, targetSizeKB int) error {
	in, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer in.Close()

	img, _, err := image.Decode(in)
	if err != nil {
		return err
	}

	normalizedProfile := normalizeQualityProfile(profile, QualityProfileClarity)
	if normalizedProfile != QualityProfileSize {
		data, err := encodeJPGBytes(img, 92)
		if err != nil {
			return err
		}
		return writeBytesToFile(outputPath, data)
	}

	budgetBytes := int64(targetSizeKB) * 1024
	attempts := []struct {
		maxWidth int
		quality  int
	}{
		{maxWidth: 1280, quality: 82},
		{maxWidth: 1280, quality: 76},
		{maxWidth: 1152, quality: 74},
		{maxWidth: 1024, quality: 72},
		{maxWidth: 960, quality: 68},
		{maxWidth: 848, quality: 64},
		{maxWidth: 768, quality: 60},
		{maxWidth: 640, quality: 56},
	}

	var best []byte
	lastSignatureWidth := 0
	lastSignatureQuality := 0
	for _, attempt := range attempts {
		candidate := fitImageToMaxWidth(img, attempt.maxWidth)
		currentWidth := candidate.Bounds().Dx()
		if currentWidth == lastSignatureWidth && attempt.quality == lastSignatureQuality && len(best) > 0 {
			continue
		}
		lastSignatureWidth = currentWidth
		lastSignatureQuality = attempt.quality
		encoded, err := encodeJPGBytes(candidate, attempt.quality)
		if err != nil {
			return err
		}
		best = encoded
		if budgetBytes <= 0 || int64(len(encoded)) <= budgetBytes {
			break
		}
	}
	return writeBytesToFile(outputPath, best)
}

func fitImageToMaxWidth(img image.Image, targetMaxWidth int) image.Image {
	if img == nil {
		return img
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 || width <= targetMaxWidth {
		return img
	}

	targetWidth := targetMaxWidth
	targetHeight := int(math.Round(float64(height) * (float64(targetWidth) / float64(width))))
	if targetHeight < 1 {
		targetHeight = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), img, bounds, xdraw.Over, nil)
	return dst
}

func encodePNGBytes(img image.Image, compression imagepng.CompressionLevel) ([]byte, error) {
	var buf bytes.Buffer
	encoder := imagepng.Encoder{CompressionLevel: compression}
	if err := encoder.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeJPGBytes(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := imagejpeg.Encode(&buf, img, &imagejpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeBytesToFile(outputPath string, data []byte) error {
	out, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := out.Write(data); err != nil {
		return err
	}
	return out.Sync()
}

func mimeTypeByFormat(format string) string {
	switch format {
	case "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "mp4":
		return "video/mp4"
	case "live":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func normalizeFormatSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		format := strings.ToLower(strings.TrimSpace(item))
		if format == "" {
			continue
		}
		if format == "jpeg" {
			format = "jpg"
		}
		if _, ok := seen[format]; ok {
			continue
		}
		seen[format] = struct{}{}
		out = append(out, format)
	}
	sort.Strings(out)
	return out
}

func resolveWebPEncoder() (string, error) {
	candidates := []string{"libwebp_anim", "libwebp"}
	for _, encoder := range candidates {
		supported, err := supportsFFmpegEncoder(encoder)
		if err != nil {
			return "", err
		}
		if supported {
			return encoder, nil
		}
	}
	return "", unsupportedOutputFormatError{
		Format: "webp",
		Reason: "ffmpeg missing libwebp/libwebp_anim encoder",
	}
}

func supportsFFmpegEncoder(encoderName string) (bool, error) {
	encoderName = strings.TrimSpace(encoderName)
	if encoderName == "" {
		return false, nil
	}
	ffmpegEncodersOnce.Do(loadFFmpegEncoders)
	if ffmpegEncodersErr != nil {
		return false, ffmpegEncodersErr
	}
	_, ok := ffmpegEncoderSet[encoderName]
	return ok, nil
}

func loadFFmpegEncoders() {
	out, err := exec.Command("ffmpeg", "-hide_banner", "-encoders").CombinedOutput()
	if err != nil {
		ffmpegEncodersErr = fmt.Errorf("run ffmpeg -encoders failed: %w: %s", err, strings.TrimSpace(string(out)))
		return
	}

	encoders := map[string]struct{}{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Encoders:") || strings.HasPrefix(line, "------") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		encoders[fields[1]] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		ffmpegEncodersErr = fmt.Errorf("parse ffmpeg encoders failed: %w", err)
		return
	}
	ffmpegEncoderSet = encoders
}

func (p *Processor) resolveCollectionPrefix(job models.VideoJob) (string, error) {
	if job.ID == 0 || job.UserID == 0 {
		return "", errors.New("invalid job for collection prefix")
	}
	layout := NewVideoImageStorageLayout(p.cfg.Env)
	return layout.JobPrefix(job.UserID, job.ID), nil
}

func (p *Processor) buildObjectReadURL(key string) (string, error) {
	cleanKey := strings.TrimLeft(strings.TrimSpace(key), "/")
	if cleanKey == "" {
		return "", errors.New("empty source video key")
	}
	if p.qiniu.Private {
		signed, err := p.qiniu.SignedURL(cleanKey, 3600)
		if err != nil {
			return "", err
		}
		signed = strings.TrimSpace(signed)
		if strings.HasPrefix(signed, "http://") || strings.HasPrefix(signed, "https://") {
			return signed, nil
		}
		return "", errors.New("qiniu signed url unavailable")
	}
	publicURL := strings.TrimSpace(p.qiniu.PublicURL(cleanKey))
	if strings.HasPrefix(publicURL, "http://") || strings.HasPrefix(publicURL, "https://") {
		return publicURL, nil
	}
	return "", errors.New("qiniu public url unavailable")
}

func (p *Processor) downloadObjectByKey(ctx context.Context, key, outPath string) error {
	cleanKey := strings.TrimLeft(strings.TrimSpace(key), "/")
	if cleanKey == "" {
		return errors.New("empty object key")
	}

	var sdkErr error
	if p.qiniu != nil {
		sdkErr = p.downloadObjectByBucketManager(ctx, cleanKey, outPath)
		if sdkErr == nil {
			return nil
		}
	}

	sourceURL, err := p.buildObjectReadURL(cleanKey)
	if err != nil {
		if sdkErr != nil {
			return fmt.Errorf("download object failed (sdk=%v, url=%w)", sdkErr, err)
		}
		return err
	}
	if err := p.downloadObject(ctx, sourceURL, outPath); err != nil {
		if sdkErr != nil {
			return fmt.Errorf("download object failed (sdk=%v, http=%w)", sdkErr, err)
		}
		return err
	}
	return nil
}

func (p *Processor) downloadObjectByBucketManager(ctx context.Context, key, outPath string) error {
	if p == nil || p.qiniu == nil {
		return errors.New("qiniu not configured")
	}
	output, err := p.qiniu.BucketManager().Get(p.qiniu.Bucket, key, nil)
	if err != nil {
		return err
	}
	defer output.Close()

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := copyWithContext(ctx, f, output.Body); err != nil {
		return err
	}
	return nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 256*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if errors.Is(er, io.EOF) {
				return written, nil
			}
			return written, er
		}
	}
}

func (p *Processor) downloadObject(ctx context.Context, objectURL, outPath string) error {
	tryURLs := []string{objectURL}
	if fallback, ok := httpFallbackURL(objectURL); ok {
		tryURLs = append(tryURLs, fallback)
	}

	var lastErr error
	for idx, candidate := range tryURLs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, candidate, nil)
		if err != nil {
			return err
		}
		resp, err := p.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if idx == 0 && len(tryURLs) > 1 && isTLSError(err) {
				continue
			}
			return err
		}

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
			_ = resp.Body.Close()
			return lastErr
		}

		f, err := os.Create(outPath)
		if err != nil {
			_ = resp.Body.Close()
			return err
		}

		_, copyErr := io.Copy(f, resp.Body)
		closeErr := resp.Body.Close()
		_ = f.Close()
		if copyErr != nil {
			lastErr = copyErr
			return copyErr
		}
		if closeErr != nil {
			lastErr = closeErr
			return closeErr
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("download source object failed")
}

func httpFallbackURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", false
	}
	parsed.Scheme = "http"
	return parsed.String(), true
}

func isTLSError(err error) bool {
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "tls") || strings.Contains(msg, "x509")
}

func (p *Processor) updateVideoJob(jobID uint64, updates map[string]interface{}) {
	if jobID == 0 || len(updates) == 0 {
		return
	}
	query := p.db.Model(&models.VideoJob{}).Where("id = ?", jobID)
	if shouldGuardCancelledStatus(updates) {
		query = query.Where("status <> ?", models.VideoJobStatusCancelled)
	}
	result := query.Updates(updates)
	if result.Error != nil || result.RowsAffected == 0 {
		return
	}
	_ = SyncPublicVideoImageJobUpdates(p.db, jobID, updates)
}

func shouldGuardCancelledStatus(updates map[string]interface{}) bool {
	if len(updates) == 0 {
		return false
	}
	rawStatus, ok := updates["status"]
	if !ok {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(rawStatus)))
	return status != models.VideoJobStatusCancelled
}

func (p *Processor) appendJobEvent(jobID uint64, stage, level, message string, metadata map[string]interface{}) {
	if jobID == 0 {
		return
	}
	event := models.VideoJobEvent{
		JobID:    jobID,
		Stage:    strings.TrimSpace(stage),
		Level:    strings.TrimSpace(level),
		Message:  strings.TrimSpace(message),
		Metadata: mustJSON(metadata),
	}
	_ = p.db.Create(&event).Error
	_ = CreatePublicVideoImageEvent(p.db, event)
}

func (p *Processor) syncJobCost(jobID uint64) {
	if p == nil || p.db == nil || jobID == 0 {
		return
	}
	if err := UpsertJobCost(p.db, jobID); err != nil {
		p.appendJobEvent(jobID, models.VideoJobStageIndexing, "warn", "cost snapshot update failed", map[string]interface{}{
			"error": err.Error(),
		})
	}
}

func (p *Processor) recordVideoJobAIUsage(input videoJobAIUsageInput) {
	if p == nil || p.db == nil || input.JobID == 0 || input.UserID == 0 {
		return
	}
	if err := RecordVideoJobAIUsage(p.db, input); err != nil {
		p.appendJobEvent(input.JobID, models.VideoJobStageAnalyzing, "warn", "ai usage snapshot insert failed", map[string]interface{}{
			"stage":    strings.TrimSpace(input.Stage),
			"error":    err.Error(),
			"model":    strings.TrimSpace(input.Model),
			"provider": strings.TrimSpace(input.Provider),
		})
	}
}

func (p *Processor) syncJobPointSettlement(jobID uint64, status string) {
	if p == nil || p.db == nil || jobID == 0 {
		return
	}
	if err := SettleReservedPointsForJob(p.db, jobID, strings.ToLower(strings.TrimSpace(status))); err != nil {
		p.appendJobEvent(jobID, models.VideoJobStageIndexing, "warn", "compute points settlement failed", map[string]interface{}{
			"status": strings.TrimSpace(status),
			"error":  err.Error(),
		})
	}
}

func (p *Processor) syncGIFBaseline(jobID uint64) {
	if p == nil || p.db == nil || jobID == 0 {
		return
	}
	if err := SyncGIFBaselineByJobID(p.db, jobID); err != nil {
		p.appendJobEvent(jobID, models.VideoJobStageIndexing, "warn", "gif baseline snapshot update failed", map[string]interface{}{
			"error": err.Error(),
		})
	}
}

func (p *Processor) markVideoJobFailed(jobID uint64, errMsg string) {
	if p.isJobCancelled(jobID) {
		return
	}
	finishedAt := time.Now()
	p.updateVideoJob(jobID, map[string]interface{}{
		"status":        models.VideoJobStatusFailed,
		"stage":         models.VideoJobStageFailed,
		"error_message": errMsg,
		"finished_at":   finishedAt,
	})
	p.appendJobEvent(jobID, models.VideoJobStageFailed, "error", errMsg, nil)
	p.syncGIFBaseline(jobID)
}

func (p *Processor) markVideoJobRetrying(jobID uint64, errMsg string) {
	if p.isJobCancelled(jobID) {
		return
	}
	p.updateVideoJob(jobID, map[string]interface{}{
		"status":        models.VideoJobStatusQueued,
		"stage":         models.VideoJobStageRetrying,
		"error_message": errMsg,
	})
	p.appendJobEvent(jobID, models.VideoJobStageRetrying, "warn", errMsg, nil)
}

func (p *Processor) handleJobError(ctx context.Context, jobID uint64, err error) error {
	var perr permanentError
	if errors.As(err, &perr) {
		p.markVideoJobFailed(jobID, perr.Error())
		p.syncJobCost(jobID)
		p.syncJobPointSettlement(jobID, models.VideoJobStatusFailed)
		p.cleanupSourceVideo(jobID, "failed")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, perr)
	}

	retryCount, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	if retryCount >= maxRetry {
		p.markVideoJobFailed(jobID, err.Error())
		p.syncJobCost(jobID)
		p.syncJobPointSettlement(jobID, models.VideoJobStatusFailed)
		p.cleanupSourceVideo(jobID, "failed")
		return fmt.Errorf("%w: %v", asynq.SkipRetry, err)
	}

	p.markVideoJobRetrying(jobID, err.Error())
	return err
}

func (p *Processor) cleanupSourceVideo(jobID uint64, reason string) {
	if p == nil || p.db == nil || p.qiniu == nil || jobID == 0 {
		return
	}

	var job models.VideoJob
	if err := p.db.Select("id", "source_video_key", "metrics").Where("id = ?", jobID).First(&job).Error; err != nil {
		return
	}

	key := strings.TrimLeft(strings.TrimSpace(job.SourceVideoKey), "/")
	if key == "" {
		return
	}

	metrics := parseJSONMap(job.Metrics)
	if sourceVideoDeleted(metrics) {
		return
	}

	if err := deleteQiniuKey(p.qiniu, key); err != nil {
		p.appendJobEvent(jobID, models.VideoJobStageUploading, "warn", "source video cleanup failed", map[string]interface{}{
			"source_video_key": key,
			"reason":           strings.TrimSpace(reason),
			"error":            err.Error(),
		})
		return
	}

	metrics["source_video_deleted"] = true
	metrics["source_video_deleted_at"] = time.Now().Format(time.RFC3339)
	metrics["source_video_cleanup_reason"] = strings.TrimSpace(reason)
	p.updateVideoJob(jobID, map[string]interface{}{
		"metrics": mustJSON(metrics),
	})
	p.appendJobEvent(jobID, models.VideoJobStageUploading, "info", "source video cleaned", map[string]interface{}{
		"source_video_key": key,
		"reason":           strings.TrimSpace(reason),
	})
}

func (p *Processor) isJobCancelled(jobID uint64) bool {
	if jobID == 0 {
		return false
	}
	var status string
	if err := p.db.Model(&models.VideoJob{}).Select("status").Where("id = ?", jobID).Scan(&status).Error; err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(status), models.VideoJobStatusCancelled)
}

type permanentError struct {
	err error
}

func (e permanentError) Error() string {
	if e.err == nil {
		return "permanent error"
	}
	return e.err.Error()
}

func chooseFrameInterval(durationSec float64, preferred float64, maxStatic int) float64 {
	if preferred > 0 {
		return clampFloat(preferred, 0.2, 8.0)
	}
	if durationSec <= 0 {
		return 0.8
	}
	if maxStatic <= 0 {
		maxStatic = 24
	}
	interval := durationSec / float64(maxStatic)
	return clampFloat(interval, 0.2, 8.0)
}

func clampFloat(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func (p *Processor) loadCloudHighlightFallbackConfig() cloudHighlightFallbackConfig {
	cfg := cloudHighlightFallbackConfig{
		Enabled:   false,
		URL:       strings.TrimSpace(os.Getenv("VIDEO_HIGHLIGHT_CLOUD_FALLBACK_URL")),
		Token:     strings.TrimSpace(os.Getenv("VIDEO_HIGHLIGHT_CLOUD_FALLBACK_TOKEN")),
		MinScore:  0.34,
		Timeout:   8 * time.Second,
		TopN:      0,
		Threshold: 0.08,
	}
	if p != nil {
		if cfg.URL == "" {
			cfg.URL = strings.TrimSpace(p.cfg.LLMEndpoint)
		}
		if cfg.Token == "" {
			cfg.Token = strings.TrimSpace(p.cfg.LLMAPIKey)
		}
	}
	if raw := strings.TrimSpace(os.Getenv("VIDEO_HIGHLIGHT_CLOUD_FALLBACK_MIN_SCORE")); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			cfg.MinScore = clampFloat(v, 0.05, 0.95)
		}
	}
	if raw := strings.TrimSpace(os.Getenv("VIDEO_HIGHLIGHT_CLOUD_FALLBACK_TIMEOUT_SEC")); raw != "" {
		if sec, err := strconv.Atoi(raw); err == nil && sec > 0 {
			if sec > 60 {
				sec = 60
			}
			cfg.Timeout = time.Duration(sec) * time.Second
		}
	}
	if raw := strings.TrimSpace(os.Getenv("VIDEO_HIGHLIGHT_CLOUD_FALLBACK_TOP_N")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			if n > 6 {
				n = 6
			}
			cfg.TopN = n
		}
	}
	if raw := strings.TrimSpace(os.Getenv("VIDEO_HIGHLIGHT_CLOUD_FALLBACK_SCENE_THRESHOLD")); raw != "" {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			cfg.Threshold = clampFloat(v, 0.03, 0.30)
		}
	}
	if raw := strings.TrimSpace(os.Getenv("VIDEO_HIGHLIGHT_CLOUD_FALLBACK_ENABLED")); raw != "" {
		if enabled, err := strconv.ParseBool(raw); err == nil {
			cfg.Enabled = enabled
		}
	}
	if !cfg.Enabled {
		return cfg
	}
	if strings.TrimSpace(cfg.URL) == "" {
		cfg.Enabled = false
		return cfg
	}
	return cfg
}

func (p *Processor) shouldUseCloudHighlightFallback(local highlightSuggestion) bool {
	cfg := p.loadCloudHighlightFallbackConfig()
	if !cfg.Enabled {
		return false
	}
	if local.Selected == nil {
		return true
	}
	if local.Selected.Score < cfg.MinScore {
		return true
	}
	return len(local.Candidates) < 2
}

func detectCloudFallbackProvider(url string, p *Processor) string {
	if p != nil {
		if provider := strings.ToLower(strings.TrimSpace(p.cfg.LLMProvider)); provider != "" {
			return provider
		}
	}
	host := strings.ToLower(strings.TrimSpace(url))
	switch {
	case strings.Contains(host, "dashscope.aliyuncs.com"), strings.Contains(host, "aliyuncs.com"):
		return "qwen"
	case strings.Contains(host, "deepseek.com"):
		return "deepseek"
	case strings.Contains(host, "volces.com"), strings.Contains(host, "volc"):
		return "volcengine"
	case strings.Contains(host, "openai.com"):
		return "openai"
	default:
		return "cloud_fallback"
	}
}

func detectCloudFallbackModel(p *Processor) string {
	if p == nil {
		return ""
	}
	model := strings.TrimSpace(p.cfg.LLMModel)
	if model != "" {
		return strings.ToLower(model)
	}
	return ""
}

func normalizeCloudHighlightUsage(in cloudHighlightUsage) cloudHighlightUsage {
	out := in
	if out.InputTokens <= 0 {
		out.InputTokens = out.PromptTokens
	}
	if out.OutputTokens <= 0 {
		out.OutputTokens = out.CompletionTokens
	}
	if out.InputTokens < 0 {
		out.InputTokens = 0
	}
	if out.OutputTokens < 0 {
		out.OutputTokens = 0
	}
	if out.CachedInputTokens < 0 {
		out.CachedInputTokens = 0
	}
	if out.CachedInputTokens > out.InputTokens {
		out.CachedInputTokens = out.InputTokens
	}
	if out.ImageTokens < 0 {
		out.ImageTokens = 0
	}
	if out.VideoTokens < 0 {
		out.VideoTokens = 0
	}
	if out.AudioSeconds < 0 {
		out.AudioSeconds = 0
	}
	return out
}

func extractCloudHighlightUsage(parsed cloudHighlightResponse, body []byte) *cloudHighlightUsage {
	if parsed.Usage != nil {
		usage := normalizeCloudHighlightUsage(*parsed.Usage)
		return &usage
	}
	if len(body) == 0 {
		return nil
	}
	var raw cloudHighlightGenericResponse
	if err := json.Unmarshal(body, &raw); err != nil || raw.Usage == nil {
		return nil
	}
	usage := normalizeCloudHighlightUsage(*raw.Usage)
	return &usage
}

func (p *Processor) requestCloudHighlightFallback(
	ctx context.Context,
	jobID uint64,
	userID uint64,
	sourcePath string,
	meta videoProbeMeta,
	local highlightSuggestion,
	qualitySettings QualitySettings,
) (highlightSuggestion, error) {
	suggestion := highlightSuggestion{
		Version:  "cloud_v1",
		Strategy: "cloud_fallback_service",
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	cfg := p.loadCloudHighlightFallbackConfig()
	startedAt := time.Now()
	usageInput := videoJobAIUsageInput{
		JobID:    jobID,
		UserID:   userID,
		Stage:    "planner_fallback",
		Provider: detectCloudFallbackProvider(cfg.URL, p),
		Model:    detectCloudFallbackModel(p),
		Endpoint: cfg.URL,
		Metadata: map[string]interface{}{
			"top_n":            qualitySettings.GIFCandidateMaxOutputs,
			"scene_threshold":  cfg.Threshold,
			"local_candidates": len(local.Candidates),
		},
	}
	finishUsage := func(status, errMessage string, usage *cloudHighlightUsage, extra map[string]interface{}) {
		if usageInput.Metadata == nil {
			usageInput.Metadata = map[string]interface{}{}
		}
		for key, value := range extra {
			usageInput.Metadata[key] = value
		}
		usageInput.RequestDurationMs = clampDurationMillis(startedAt)
		usageInput.RequestStatus = strings.TrimSpace(status)
		usageInput.RequestError = strings.TrimSpace(errMessage)
		if usage != nil {
			normalizedUsage := normalizeCloudHighlightUsage(*usage)
			usageInput.InputTokens = normalizedUsage.InputTokens
			usageInput.OutputTokens = normalizedUsage.OutputTokens
			usageInput.CachedInputTokens = normalizedUsage.CachedInputTokens
			usageInput.ImageTokens = normalizedUsage.ImageTokens
			usageInput.VideoTokens = normalizedUsage.VideoTokens
			usageInput.AudioSeconds = normalizedUsage.AudioSeconds
			usageInput.Metadata["usage"] = map[string]interface{}{
				"input_tokens":        normalizedUsage.InputTokens,
				"output_tokens":       normalizedUsage.OutputTokens,
				"cached_input_tokens": normalizedUsage.CachedInputTokens,
				"image_tokens":        normalizedUsage.ImageTokens,
				"video_tokens":        normalizedUsage.VideoTokens,
				"audio_seconds":       roundTo(normalizedUsage.AudioSeconds, 3),
			}
		}
		p.recordVideoJobAIUsage(usageInput)
	}

	if !cfg.Enabled {
		finishUsage("disabled", "cloud highlight fallback disabled", nil, nil)
		return suggestion, errors.New("cloud highlight fallback disabled")
	}

	topN := qualitySettings.GIFCandidateMaxOutputs
	if cfg.TopN > 0 {
		topN = cfg.TopN
	}
	if topN <= 0 {
		topN = defaultHighlightTopN
	}
	if topN > 6 {
		topN = 6
	}

	scenePoints, err := detectScenePoints(ctx, sourcePath, cfg.Threshold)
	if err != nil {
		finishUsage("error", "collect scene points: "+err.Error(), nil, nil)
		return suggestion, fmt.Errorf("collect scene points: %w", err)
	}
	targetDuration := 2.6
	if local.Selected != nil {
		duration := local.Selected.EndSec - local.Selected.StartSec
		if duration > 0 {
			targetDuration = clampFloat(duration, 1.2, 4.0)
		}
	}

	reqPayload := cloudHighlightRequest{
		DurationSec:     roundTo(meta.DurationSec, 3),
		Width:           meta.Width,
		Height:          meta.Height,
		FPS:             roundTo(meta.FPS, 3),
		TargetDuration:  roundTo(targetDuration, 3),
		TopN:            topN,
		SceneThreshold:  cfg.Threshold,
		ScenePoints:     scenePoints,
		LocalCandidates: local.Candidates,
	}
	rawReq, _ := json.Marshal(reqPayload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(rawReq))
	if err != nil {
		finishUsage("error", err.Error(), nil, nil)
		return suggestion, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	client := p.httpClient
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	} else {
		client = &http.Client{
			Transport: client.Transport,
			Timeout:   cfg.Timeout,
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		finishUsage("error", err.Error(), nil, map[string]interface{}{"transport_error": true})
		return suggestion, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		finishUsage("error", fmt.Sprintf("http %d", resp.StatusCode), nil, map[string]interface{}{
			"http_status": resp.StatusCode,
			"response":    strings.TrimSpace(string(body)),
		})
		return suggestion, fmt.Errorf("cloud highlight http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed cloudHighlightResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		finishUsage("error", "decode response: "+err.Error(), nil, map[string]interface{}{
			"http_status": resp.StatusCode,
			"response":    strings.TrimSpace(string(body)),
		})
		return suggestion, fmt.Errorf("decode cloud highlight response: %w", err)
	}
	usage := extractCloudHighlightUsage(parsed, body)

	candidates := make([]highlightCandidate, 0, len(parsed.Candidates))
	for _, item := range parsed.Candidates {
		start, end := clampHighlightWindow(item.StartSec, item.EndSec, meta.DurationSec)
		if end <= start {
			continue
		}
		score := item.Score
		if score <= 0 {
			score = 0.4
		}
		if score > 1 {
			score = 1
		}
		reason := strings.TrimSpace(item.Reason)
		if reason == "" {
			reason = "cloud_fallback"
		}
		candidates = append(candidates, highlightCandidate{
			StartSec:   start,
			EndSec:     end,
			Score:      score,
			SceneScore: item.SceneScore,
			Reason:     reason,
		})
	}
	if len(candidates) == 0 {
		finishUsage("error", "cloud highlight response has no valid candidates", usage, map[string]interface{}{
			"http_status":      resp.StatusCode,
			"candidate_count":  0,
			"selected_present": parsed.Selected != nil,
		})
		return suggestion, errors.New("cloud highlight response has no valid candidates")
	}

	selected := pickNonOverlapCandidates(candidates, topN, qualitySettings.GIFCandidateDedupIOUThreshold)
	selected = applyGIFCandidateConfidenceThreshold(selected, candidates, qualitySettings.GIFCandidateConfidenceThreshold)
	if len(selected) == 0 {
		selected = candidates
	}
	if len(selected) > topN {
		selected = selected[:topN]
	}
	suggestion.Candidates = selected
	suggestion.All = candidates
	suggestion.Selected = &selected[0]
	finishUsage("ok", "", usage, map[string]interface{}{
		"http_status":      resp.StatusCode,
		"candidate_count":  len(candidates),
		"selected_count":   len(selected),
		"selected_present": suggestion.Selected != nil,
	})
	return suggestion, nil
}

func (p *Processor) persistGIFHighlightCandidates(
	ctx context.Context,
	sourcePath string,
	meta videoProbeMeta,
	jobID uint64,
	suggestion highlightSuggestion,
	qualitySettings QualitySettings,
) error {
	if p == nil || p.db == nil || jobID == 0 {
		return nil
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	pool := append([]highlightCandidate{}, suggestion.All...)
	if len(pool) == 0 {
		pool = append(pool, suggestion.Candidates...)
	}
	if len(pool) == 0 {
		return nil
	}

	selectedRanks := make(map[string]int, len(suggestion.Candidates))
	for idx, candidate := range suggestion.Candidates {
		key := highlightCandidateWindowKey(candidate)
		if key == "" {
			continue
		}
		selectedRanks[key] = idx + 1
	}

	topScore := 0.0
	minScore := 0.0
	if len(suggestion.Candidates) > 0 {
		topScore = suggestion.Candidates[0].Score
		minScore = suggestion.Candidates[0].Score
		for _, candidate := range suggestion.Candidates {
			if candidate.Score > topScore {
				topScore = candidate.Score
			}
			if candidate.Score < minScore {
				minScore = candidate.Score
			}
		}
	}
	scoreSpread := topScore - minScore
	if scoreSpread < 0 {
		scoreSpread = 0
	}

	merged := make(map[string]models.VideoJobGIFCandidate, len(pool))
	for _, candidate := range pool {
		startMs, endMs, ok := normalizeHighlightCandidateWindowMs(candidate)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%d-%d", startMs, endMs)
		rank, selected := selectedRanks[key]
		featureSnapshot := p.buildGIFCandidateFeatureSnapshot(
			ctx,
			sourcePath,
			meta,
			candidate,
			qualitySettings,
			topScore,
			scoreSpread,
			selected,
			suggestion.Strategy,
			suggestion.Version,
		)
		rejectReason := ""
		if !selected {
			rejectReason = inferGIFCandidateRejectReason(
				candidate,
				suggestion.Candidates,
				qualitySettings.GIFCandidateDedupIOUThreshold,
				qualitySettings.GIFCandidateConfidenceThreshold,
				topScore,
				scoreSpread,
				featureSnapshot,
				qualitySettings,
			)
		}

		row := models.VideoJobGIFCandidate{
			JobID:           jobID,
			StartMs:         startMs,
			EndMs:           endMs,
			DurationMs:      endMs - startMs,
			BaseScore:       roundTo(candidate.Score, 4),
			ConfidenceScore: roundTo(estimateGIFCandidateConfidence(candidate.Score, topScore, scoreSpread, selected), 4),
			FinalRank:       rank,
			IsSelected:      selected,
			RejectReason:    rejectReason,
			FeatureJSON:     mustJSON(featureSnapshot),
		}

		if existing, exists := merged[key]; exists {
			replace := false
			if row.IsSelected && !existing.IsSelected {
				replace = true
			} else if row.IsSelected == existing.IsSelected {
				if row.FinalRank > 0 && (existing.FinalRank == 0 || row.FinalRank < existing.FinalRank) {
					replace = true
				} else if row.BaseScore > existing.BaseScore {
					replace = true
				}
			}
			if replace {
				merged[key] = row
			}
			continue
		}
		merged[key] = row
	}

	if len(merged) == 0 {
		return nil
	}

	rows := make([]models.VideoJobGIFCandidate, 0, len(merged))
	for _, row := range merged {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].IsSelected != rows[j].IsSelected {
			return rows[i].IsSelected
		}
		if rows[i].FinalRank != rows[j].FinalRank {
			if rows[i].FinalRank == 0 {
				return false
			}
			if rows[j].FinalRank == 0 {
				return true
			}
			return rows[i].FinalRank < rows[j].FinalRank
		}
		if rows[i].BaseScore != rows[j].BaseScore {
			return rows[i].BaseScore > rows[j].BaseScore
		}
		return rows[i].StartMs < rows[j].StartMs
	})

	return p.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("job_id = ?", jobID).Delete(&models.VideoJobGIFCandidate{}).Error; err != nil {
			return err
		}
		return tx.CreateInBatches(rows, 100).Error
	})
}

func (p *Processor) attachGIFCandidateBindings(jobID uint64, in []highlightCandidate) []highlightCandidate {
	if p == nil || p.db == nil || jobID == 0 || len(in) == 0 {
		return in
	}
	type candidateRow struct {
		ID      uint64 `gorm:"column:id"`
		StartMs int    `gorm:"column:start_ms"`
		EndMs   int    `gorm:"column:end_ms"`
	}
	var rows []candidateRow
	if err := p.db.Model(&models.VideoJobGIFCandidate{}).
		Select("id", "start_ms", "end_ms").
		Where("job_id = ?", jobID).
		Order("is_selected DESC, final_rank ASC, id ASC").
		Limit(200).
		Find(&rows).Error; err != nil {
		return in
	}
	if len(rows) == 0 {
		return in
	}
	exactByWindow := map[string]uint64{}
	for _, row := range rows {
		if row.ID == 0 || row.EndMs <= row.StartMs {
			continue
		}
		key := fmt.Sprintf("%d-%d", row.StartMs, row.EndMs)
		if _, exists := exactByWindow[key]; exists {
			continue
		}
		exactByWindow[key] = row.ID
	}

	out := make([]highlightCandidate, 0, len(in))
	for _, item := range in {
		startMs, endMs, ok := normalizeHighlightCandidateWindowMs(item)
		if !ok {
			out = append(out, item)
			continue
		}
		key := fmt.Sprintf("%d-%d", startMs, endMs)
		if matchedID, exists := exactByWindow[key]; exists && matchedID > 0 {
			id := matchedID
			item.CandidateID = &id
			out = append(out, item)
			continue
		}
		bestID := uint64(0)
		bestIOU := 0.0
		for _, row := range rows {
			if row.ID == 0 || row.EndMs <= row.StartMs {
				continue
			}
			iou := windowIOUMs(startMs, endMs, row.StartMs, row.EndMs)
			if iou > bestIOU {
				bestIOU = iou
				bestID = row.ID
			}
		}
		if bestID > 0 && bestIOU >= 0.9 {
			bestID := bestID
			item.CandidateID = &bestID
		}
		out = append(out, item)
	}
	return out
}

func (p *Processor) persistGIFRerankLogs(
	jobID uint64,
	userID uint64,
	before []highlightCandidate,
	after []highlightCandidate,
	profile highlightFeedbackProfile,
) {
	if p == nil || p.db == nil || jobID == 0 || userID == 0 || len(after) == 0 {
		return
	}
	type rankMeta struct {
		Rank  int
		Score float64
	}
	beforeMap := make(map[string]rankMeta, len(before))
	for idx, candidate := range before {
		key := highlightCandidateWindowKey(candidate)
		if key == "" {
			continue
		}
		beforeMap[key] = rankMeta{
			Rank:  idx + 1,
			Score: candidate.Score,
		}
	}
	rows := make([]models.VideoJobGIFRerankLog, 0, len(after))
	for idx, candidate := range after {
		startMs, endMs, ok := normalizeHighlightCandidateWindowMs(candidate)
		if !ok {
			continue
		}
		key := highlightCandidateWindowKey(candidate)
		beforeRank := 0
		beforeScore := candidate.Score
		if prev, exists := beforeMap[key]; exists {
			beforeRank = prev.Rank
			beforeScore = prev.Score
		}
		afterRank := idx + 1
		if beforeRank == afterRank && math.Abs(beforeScore-candidate.Score) < 1e-6 {
			continue
		}
		rows = append(rows, models.VideoJobGIFRerankLog{
			JobID:       jobID,
			UserID:      userID,
			StartMs:     startMs,
			EndMs:       endMs,
			BeforeRank:  beforeRank,
			AfterRank:   afterRank,
			BeforeScore: roundTo(beforeScore, 4),
			AfterScore:  roundTo(candidate.Score, 4),
			ScoreDelta:  roundTo(candidate.Score-beforeScore, 4),
			Reason:      strings.ToLower(strings.TrimSpace(candidate.Reason)),
			Metadata: mustJSON(map[string]interface{}{
				"profile_engaged_jobs":       profile.EngagedJobs,
				"profile_weighted_signals":   roundTo(profile.WeightedSignals, 3),
				"profile_avg_signal_weight":  roundTo(profile.AverageSignalWeight, 3),
				"profile_public_positive":    roundTo(profile.PublicPositiveSignals, 3),
				"profile_public_negative":    roundTo(profile.PublicNegativeSignals, 3),
				"profile_preferred_center":   roundTo(profile.PreferredCenter, 4),
				"profile_preferred_duration": roundTo(profile.PreferredDuration, 4),
				"profile_reason_preference":  profile.ReasonPreference,
				"profile_reason_negative":    profile.ReasonNegativeGuard,
			}),
		})
	}
	if len(rows) == 0 {
		return
	}
	_ = p.db.CreateInBatches(rows, 100).Error
}

func (p *Processor) buildGIFCandidateFeatureSnapshot(
	ctx context.Context,
	sourcePath string,
	meta videoProbeMeta,
	candidate highlightCandidate,
	qualitySettings QualitySettings,
	topScore float64,
	scoreSpread float64,
	selected bool,
	strategy string,
	version string,
) map[string]interface{} {
	durationSec := candidate.EndSec - candidate.StartSec
	if durationSec < 0 {
		durationSec = 0
	}
	confidence := estimateGIFCandidateConfidence(candidate.Score, topScore, scoreSpread, selected)
	feature := map[string]interface{}{
		"scene_score":            roundTo(candidate.SceneScore, 4),
		"reason":                 strings.TrimSpace(candidate.Reason),
		"proposal_rank":          candidate.ProposalRank,
		"window_sec":             roundTo(durationSec, 3),
		"window_center_sec":      roundTo((candidate.StartSec+candidate.EndSec)/2, 3),
		"window_position_ratio":  roundTo(candidateWindowPositionRatio(candidate, meta.DurationSec), 4),
		"strategy":               strings.TrimSpace(suggestionStrategyOrDefault(strategy)),
		"version":                strings.TrimSpace(suggestionVersionOrDefault(version)),
		"base_score":             roundTo(candidate.Score, 4),
		"confidence_score":       roundTo(confidence, 4),
		"confidence_threshold":   roundTo(qualitySettings.GIFCandidateConfidenceThreshold, 4),
		"dedup_iou_threshold":    roundTo(qualitySettings.GIFCandidateDedupIOUThreshold, 4),
		"estimated_size_kb":      roundTo(estimateGIFCandidateSizeKB(meta, candidate, qualitySettings), 2),
		"gif_profile":            strings.TrimSpace(strings.ToLower(qualitySettings.GIFProfile)),
		"gif_default_fps":        qualitySettings.GIFDefaultFPS,
		"gif_default_max_colors": qualitySettings.GIFDefaultMaxColors,
	}
	if candidate.ProposalID != nil && *candidate.ProposalID > 0 {
		feature["proposal_id"] = *candidate.ProposalID
	}
	if sampled := p.sampleGIFCandidateFrameQuality(ctx, sourcePath, candidate, qualitySettings); len(sampled) > 0 {
		for key, value := range sampled {
			feature[key] = value
		}
	}
	return feature
}

func (p *Processor) sampleGIFCandidateFrameQuality(
	ctx context.Context,
	sourcePath string,
	candidate highlightCandidate,
	qualitySettings QualitySettings,
) map[string]interface{} {
	if p == nil || strings.TrimSpace(sourcePath) == "" {
		return nil
	}
	timestamps := buildCandidateSampleTimestamps(candidate.StartSec, candidate.EndSec)
	if len(timestamps) == 0 {
		return nil
	}
	tmpDir, err := os.MkdirTemp("", "gif-candidate-sample-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(tmpDir)

	samples := make([]frameQualitySample, 0, len(timestamps))
	for idx, ts := range timestamps {
		target := filepath.Join(tmpDir, fmt.Sprintf("sample_%02d.jpg", idx))
		if err := extractFrameAtTimestamp(ctx, sourcePath, ts, target); err != nil {
			continue
		}
		sample, ok := analyzeFrameQuality(target)
		if !ok {
			continue
		}
		sample.Index = len(samples)
		samples = append(samples, sample)
	}
	if len(samples) == 0 {
		return map[string]interface{}{
			"sample_count": 0,
		}
	}

	sceneCutThreshold := chooseSceneCutThreshold(samples)
	assignSceneAndMotionScores(samples, sceneCutThreshold)
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	blurThreshold := chooseBlurThreshold(extractSampleBlurScores(samples), qualitySettings)

	brightnessSum := 0.0
	blurSum := 0.0
	subjectSum := 0.0
	exposureSum := 0.0
	motionSum := 0.0
	qualitySum := 0.0
	sceneMax := 1
	for idx := range samples {
		samples[idx].Exposure = roundTo(computeExposureScore(samples[idx].Brightness, qualitySettings), 4)
		samples[idx].QualityScore = roundTo(computeFrameQualityScore(samples[idx], blurThreshold), 4)
		brightnessSum += samples[idx].Brightness
		blurSum += samples[idx].BlurScore
		subjectSum += samples[idx].SubjectScore
		exposureSum += samples[idx].Exposure
		motionSum += samples[idx].MotionScore
		qualitySum += samples[idx].QualityScore
		if samples[idx].SceneID > sceneMax {
			sceneMax = samples[idx].SceneID
		}
	}
	count := float64(len(samples))
	blurMean := blurSum / count
	blurNorm := clampZeroOne(blurMean / maxFloat(qualitySettings.BlurThresholdMin, 12))

	return map[string]interface{}{
		"sample_count":        len(samples),
		"brightness_mean":     roundTo(brightnessSum/count, 3),
		"blur_mean":           roundTo(blurMean, 3),
		"blur_norm":           roundTo(blurNorm, 4),
		"subject_mean":        roundTo(subjectSum/count, 4),
		"exposure_mean":       roundTo(exposureSum/count, 4),
		"motion_mean":         roundTo(motionSum/count, 4),
		"quality_mean":        roundTo(qualitySum/count, 4),
		"scene_count":         sceneMax,
		"scene_cut_threshold": roundTo(sceneCutThreshold, 3),
		"blur_threshold":      roundTo(blurThreshold, 3),
	}
}

func buildCandidateSampleTimestamps(startSec, endSec float64) []float64 {
	if endSec <= startSec {
		return nil
	}
	duration := endSec - startSec
	if duration <= 0 {
		return nil
	}
	lead := startSec + duration*0.12
	mid := startSec + duration*0.5
	tail := endSec - duration*0.12
	seq := []float64{lead, mid, tail}
	out := make([]float64, 0, len(seq))
	for _, ts := range seq {
		if ts <= startSec {
			ts = startSec + duration*0.05
		}
		if ts >= endSec {
			ts = endSec - duration*0.05
		}
		if ts <= startSec || ts >= endSec {
			continue
		}
		out = append(out, ts)
	}
	return out
}

func extractSampleBlurScores(samples []frameQualitySample) []float64 {
	out := make([]float64, 0, len(samples))
	for _, item := range samples {
		if item.BlurScore > 0 {
			out = append(out, item.BlurScore)
		}
	}
	return out
}

func estimateGIFCandidateSizeKB(meta videoProbeMeta, candidate highlightCandidate, qualitySettings QualitySettings) float64 {
	width := meta.Width
	height := meta.Height
	if width <= 0 || height <= 0 {
		width = 480
		height = 270
	}
	durationSec := candidate.EndSec - candidate.StartSec
	if durationSec <= 0 {
		durationSec = 2.4
	}
	fps := qualitySettings.GIFDefaultFPS
	if fps <= 0 {
		fps = 12
	}
	maxColors := qualitySettings.GIFDefaultMaxColors
	if maxColors <= 0 {
		maxColors = 128
	}
	pixels := float64(width * height)
	colorFactor := 0.6 + clampZeroOne(float64(maxColors)/256.0)*0.8
	frameFactor := float64(fps) / 12.0
	rawBytes := pixels * durationSec * frameFactor * colorFactor * 0.065
	return math.Max(32, rawBytes/1024.0)
}

func candidateWindowPositionRatio(candidate highlightCandidate, totalDurationSec float64) float64 {
	if totalDurationSec <= 0 {
		return 0
	}
	center := (candidate.StartSec + candidate.EndSec) / 2
	if center < 0 {
		center = 0
	}
	return clampZeroOne(center / totalDurationSec)
}

func suggestionStrategyOrDefault(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "scene_score"
	}
	return value
}

func suggestionVersionOrDefault(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "v1"
	}
	return value
}

func highlightCandidateWindowKey(candidate highlightCandidate) string {
	startMs, endMs, ok := normalizeHighlightCandidateWindowMs(candidate)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d-%d", startMs, endMs)
}

func normalizeHighlightCandidateWindowMs(candidate highlightCandidate) (int, int, bool) {
	if candidate.EndSec <= candidate.StartSec {
		return 0, 0, false
	}
	startMs := int(math.Round(candidate.StartSec * 1000))
	endMs := int(math.Round(candidate.EndSec * 1000))
	if startMs < 0 {
		startMs = 0
	}
	if endMs <= startMs {
		return 0, 0, false
	}
	return startMs, endMs, true
}

func inferGIFCandidateRejectReason(
	candidate highlightCandidate,
	selected []highlightCandidate,
	dedupIOUThreshold float64,
	confidenceThreshold float64,
	topScore float64,
	scoreSpread float64,
	feature map[string]interface{},
	qualitySettings QualitySettings,
) string {
	if dedupIOUThreshold <= 0 {
		dedupIOUThreshold = 0.45
	}
	for _, picked := range selected {
		if windowIoU(candidate.StartSec, candidate.EndSec, picked.StartSec, picked.EndSec) > dedupIOUThreshold {
			return GIFCandidateRejectReasonDuplicate
		}
	}
	if confidenceThreshold > 0 {
		confidence := estimateGIFCandidateConfidence(candidate.Score, topScore, scoreSpread, false)
		if confidence < confidenceThreshold {
			return GIFCandidateRejectReasonLowConfidence
		}
	}
	if isGIFCandidateBlurLow(feature, qualitySettings) {
		return GIFCandidateRejectReasonBlurLow
	}
	if isGIFCandidateSizeBudgetExceeded(feature, qualitySettings) {
		return GIFCandidateRejectReasonSizeBudgetExceeded
	}
	if isGIFCandidateLoopPoor(feature, qualitySettings) {
		return GIFCandidateRejectReasonLoopPoor
	}
	return GIFCandidateRejectReasonLowEmotion
}

func isGIFCandidateBlurLow(feature map[string]interface{}, qualitySettings QualitySettings) bool {
	if len(feature) == 0 {
		return false
	}
	blurMean := floatFromAny(feature["blur_mean"])
	if blurMean <= 0 {
		return false
	}
	blurFloor := qualitySettings.StillMinBlurScore
	if blurFloor <= 0 {
		blurFloor = DefaultQualitySettings().StillMinBlurScore
	}
	blurThreshold := floatFromAny(feature["blur_threshold"])
	if blurThreshold > 0 {
		blurFloor = maxFloat(blurFloor, blurThreshold*0.82)
	}
	return blurMean < blurFloor
}

func isGIFCandidateSizeBudgetExceeded(feature map[string]interface{}, qualitySettings QualitySettings) bool {
	if len(feature) == 0 {
		return false
	}
	estimatedSizeKB := floatFromAny(feature["estimated_size_kb"])
	if estimatedSizeKB <= 0 {
		return false
	}
	targetSizeKB := float64(qualitySettings.GIFTargetSizeKB)
	if targetSizeKB <= 0 {
		targetSizeKB = float64(DefaultQualitySettings().GIFTargetSizeKB)
	}
	return estimatedSizeKB > targetSizeKB*1.18
}

func isGIFCandidateLoopPoor(feature map[string]interface{}, qualitySettings QualitySettings) bool {
	if len(feature) == 0 {
		return false
	}
	motionMean := floatFromAny(feature["motion_mean"])
	sceneCount := floatFromAny(feature["scene_count"])
	qualityMean := floatFromAny(feature["quality_mean"])
	loopMotionTarget := qualitySettings.GIFLoopTuneMotionTarget
	if loopMotionTarget <= 0 {
		loopMotionTarget = DefaultQualitySettings().GIFLoopTuneMotionTarget
	}
	if motionMean > maxFloat(loopMotionTarget*2.2, 0.48) && sceneCount >= 2 {
		return true
	}
	if qualityMean > 0 && qualityMean < 0.34 && sceneCount >= 2 {
		return true
	}
	return false
}

func applyGIFCandidateConfidenceThreshold(
	selected []highlightCandidate,
	reference []highlightCandidate,
	confidenceThreshold float64,
) []highlightCandidate {
	if len(selected) == 0 || confidenceThreshold <= 0 {
		return selected
	}
	pool := reference
	if len(pool) == 0 {
		pool = selected
	}
	topScore := pool[0].Score
	minScore := pool[0].Score
	for _, candidate := range pool {
		if candidate.Score > topScore {
			topScore = candidate.Score
		}
		if candidate.Score < minScore {
			minScore = candidate.Score
		}
	}
	scoreSpread := topScore - minScore
	if scoreSpread < 0 {
		scoreSpread = 0
	}

	filtered := make([]highlightCandidate, 0, len(selected))
	for idx, candidate := range selected {
		confidence := estimateGIFCandidateConfidence(candidate.Score, topScore, scoreSpread, idx == 0)
		if confidence >= confidenceThreshold || len(filtered) == 0 {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func estimateGIFCandidateConfidence(score, topScore, spread float64, selected bool) float64 {
	base := clampZeroOne(score)
	if topScore > 0 {
		base = clampZeroOne(base - math.Max(0, topScore-score)*0.35)
	}
	if spread > 0 && spread < 0.06 {
		base *= 0.85
	}
	if selected {
		base = clampZeroOne(base + 0.05)
	}
	return base
}

type videoProbeMeta struct {
	DurationSec float64
	Width       int
	Height      int
	FPS         float64
}

type ffprobeJSON struct {
	Streams []struct {
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
		Duration   string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

func probeVideo(ctx context.Context, sourcePath string) (videoProbeMeta, error) {
	args := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate,duration",
		"-show_entries", "format=duration",
		"-of", "json",
		sourcePath,
	}
	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return videoProbeMeta{}, fmt.Errorf("ffprobe failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	var result ffprobeJSON
	if err := json.Unmarshal(out, &result); err != nil {
		return videoProbeMeta{}, err
	}
	meta := videoProbeMeta{}
	if len(result.Streams) > 0 {
		meta.Width = result.Streams[0].Width
		meta.Height = result.Streams[0].Height
		meta.FPS = parseFPS(result.Streams[0].RFrameRate)
		meta.DurationSec = parseFloat(result.Streams[0].Duration)
	}
	if meta.DurationSec <= 0 {
		meta.DurationSec = parseFloat(result.Format.Duration)
	}
	return meta, nil
}

func suggestHighlightWindow(ctx context.Context, sourcePath string, meta videoProbeMeta, qualitySettings QualitySettings) (highlightSuggestion, error) {
	suggestion := highlightSuggestion{
		Version:  "v1",
		Strategy: "scene_score",
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	if meta.DurationSec <= 0 {
		return suggestion, errors.New("video duration unavailable for highlight scoring")
	}

	targetDuration := chooseHighlightDuration(meta.DurationSec)
	scenePoints, err := detectScenePoints(ctx, sourcePath, 0.10)
	if err != nil {
		return suggestion, err
	}

	candidates := make([]highlightCandidate, 0, len(scenePoints))
	for _, point := range scenePoints {
		start := point.PtsSec - targetDuration*0.35
		end := start + targetDuration
		start, end = clampHighlightWindow(start, end, meta.DurationSec)
		if end-start < 0.8 {
			continue
		}

		mid := (start + end) / 2
		centerBias := 1 - math.Min(1, math.Abs(mid-meta.DurationSec/2)/(meta.DurationSec/2))
		score := point.Score*0.85 + centerBias*0.15
		candidates = append(candidates, highlightCandidate{
			StartSec:   start,
			EndSec:     end,
			Score:      roundTo(score, 4),
			SceneScore: roundTo(point.Score, 4),
			Reason:     "scene_change_peak",
		})
	}

	if len(candidates) == 0 {
		suggestion.Strategy = "fallback_uniform"
		candidates = buildFallbackHighlightCandidates(meta.DurationSec, targetDuration)
	}
	if len(candidates) == 0 {
		return suggestion, errors.New("no highlight candidates generated")
	}

	selected := pickNonOverlapCandidates(candidates, qualitySettings.GIFCandidateMaxOutputs, qualitySettings.GIFCandidateDedupIOUThreshold)
	selected = applyGIFCandidateConfidenceThreshold(selected, candidates, qualitySettings.GIFCandidateConfidenceThreshold)
	if len(selected) == 0 {
		selected = candidates
	}
	if len(selected) > qualitySettings.GIFCandidateMaxOutputs {
		selected = selected[:qualitySettings.GIFCandidateMaxOutputs]
	}
	suggestion.Candidates = selected
	suggestion.All = candidates
	suggestion.Selected = &selected[0]
	return suggestion, nil
}

func (p *Processor) loadUserHighlightFeedbackProfile(
	userID uint64,
	limit int,
	qualitySettings QualitySettings,
) (highlightFeedbackProfile, error) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	profile := highlightFeedbackProfile{
		ReasonPreference:    map[string]float64{},
		ReasonNegativeGuard: map[string]float64{},
		ScenePreference:     map[string]float64{},
	}
	if p == nil || p.db == nil || userID == 0 {
		return profile, nil
	}
	if limit <= 0 {
		limit = 80
	}
	if limit > 200 {
		limit = 200
	}

	var jobs []models.VideoJob
	if err := p.db.
		Select("id", "metrics").
		Where("user_id = ? AND status = ?", userID, models.VideoJobStatusDone).
		Order("finished_at DESC NULLS LAST, id DESC").
		Limit(limit).
		Find(&jobs).Error; err != nil {
		return profile, err
	}
	if len(jobs) == 0 {
		return profile, nil
	}

	jobIDs := make([]uint64, 0, len(jobs))
	for _, job := range jobs {
		jobIDs = append(jobIDs, job.ID)
	}

	allowLegacyFallback := p.cfg.EnableLegacyFeedbackFallback
	publicSignalSummaryByJob, err := p.loadUserPublicFeedbackSignalSummary(userID, jobIDs)
	if err != nil {
		if !allowLegacyFallback {
			// 严格链路下，公共反馈读取失败时直接降级为“无画像”，不消费 legacy metrics。
			return profile, nil
		}
		publicSignalSummaryByJob = nil
	}
	if !allowLegacyFallback && len(publicSignalSummaryByJob) == 0 {
		return profile, nil
	}

	totalWeight := 0.0
	centerWeighted := 0.0
	durationWeighted := 0.0
	reasonWeights := map[string]float64{}
	reasonPositiveWeights := map[string]float64{}
	reasonNegativeWeights := map[string]float64{}
	sceneWeights := map[string]float64{}

	for _, job := range jobs {
		metrics := parseJSONMap(job.Metrics)
		highlight := mapFromAny(metrics["highlight_v1"])
		selected := mapFromAny(highlight["selected"])
		if len(selected) == 0 {
			continue
		}
		selectedReason := normalizeReason(selected["reason"])

		durationSec := floatFromAny(metrics["duration_sec"])
		startSec := floatFromAny(selected["start_sec"])
		endSec := floatFromAny(selected["end_sec"])
		if durationSec <= 0 && endSec > 0 {
			durationSec = endSec
		}

		publicSummary, hasPublicFeedback := publicSignalSummaryByJob[job.ID]
		if hasPublicFeedback {
			profile.PublicPositiveSignals += publicSummary.PositiveWeight
			profile.PublicNegativeSignals += publicSummary.NegativeWeight

			usedOutputDetails := false
			jobPositiveWeight := 0.0
			for _, detail := range publicSummary.Details {
				usedOutputDetails = true
				prefWeight := detail.Weight
				if prefWeight < 0 {
					prefWeight = 0
				}

				signalStartSec := detail.StartSec
				signalEndSec := detail.EndSec
				if signalEndSec <= signalStartSec {
					signalStartSec = startSec
					signalEndSec = endSec
				}
				if prefWeight > 0 && durationSec > 0 && signalEndSec > signalStartSec {
					mid := clampZeroOne(((signalStartSec + signalEndSec) / 2) / durationSec)
					windowDuration := clampZeroOne((signalEndSec - signalStartSec) / durationSec)
					centerWeighted += mid * prefWeight
					durationWeighted += windowDuration * prefWeight
					totalWeight += prefWeight
					profile.WeightedSignals += prefWeight
					jobPositiveWeight += prefWeight
				}

				reason := normalizeReason(detail.Reason)
				if reason == "" {
					reason = selectedReason
				}
				if reason != "" {
					if prefWeight > 0 && durationSec > 0 && signalEndSec > signalStartSec {
						reasonWeights[reason] += prefWeight
						reasonPositiveWeights[reason] += prefWeight
					}
					if detail.Weight < 0 {
						reasonNegativeWeights[reason] += -detail.Weight
					}
				}

				sceneTag := strings.TrimSpace(strings.ToLower(detail.SceneTag))
				if sceneTag != "" && prefWeight > 0 && durationSec > 0 && signalEndSec > signalStartSec {
					sceneWeights[sceneTag] += prefWeight
				}
			}
			if selectedReason != "" {
				if publicSummary.PositiveWeight > 0 && !usedOutputDetails {
					reasonPositiveWeights[selectedReason] += publicSummary.PositiveWeight
				}
				if publicSummary.NegativeWeight > 0 {
					reasonNegativeWeights[selectedReason] += publicSummary.NegativeWeight
				}
			}
			if !usedOutputDetails {
				signalWeight := publicSummary.PositiveWeight
				if signalWeight > 0 && durationSec > 0 && endSec > startSec {
					mid := clampZeroOne(((startSec + endSec) / 2) / durationSec)
					windowDuration := clampZeroOne((endSec - startSec) / durationSec)
					centerWeighted += mid * signalWeight
					durationWeighted += windowDuration * signalWeight
					totalWeight += signalWeight
					profile.WeightedSignals += signalWeight

					if selectedReason != "" {
						reasonWeights[selectedReason] += signalWeight
						reasonPositiveWeights[selectedReason] += signalWeight
					}

					tags := stringSliceFromAny(metrics["scene_tags_v1"])
					for _, tag := range tags {
						sceneWeights[tag] += signalWeight
					}
					jobPositiveWeight += signalWeight
				}
			}
			if jobPositiveWeight > 0 || publicSummary.NegativeWeight > 0 {
				profile.EngagedJobs++
			}
			continue
		}

		if !allowLegacyFallback {
			continue
		}

		feedback := mapFromAny(metrics["feedback_v1"])
		if len(feedback) == 0 {
			feedback = map[string]interface{}{}
		}
		totalSignals := floatFromAny(feedback["total_signals"])
		if totalSignals <= 0 {
			totalSignals = 0
		}
		if durationSec <= 0 || endSec <= startSec {
			continue
		}

		mid := clampZeroOne(((startSec + endSec) / 2) / durationSec)
		windowDuration := clampZeroOne((endSec - startSec) / durationSec)

		favoriteCount := floatFromAny(feedback["favorite_count"])
		signalWeight := totalSignals + favoriteCount*0.8
		if signalWeight <= 0 {
			continue
		}

		centerWeighted += mid * signalWeight
		durationWeighted += windowDuration * signalWeight
		totalWeight += signalWeight
		profile.WeightedSignals += signalWeight
		profile.EngagedJobs++

		if selectedReason != "" {
			reasonWeights[selectedReason] += signalWeight
			reasonPositiveWeights[selectedReason] += signalWeight
		}

		tags := stringSliceFromAny(metrics["scene_tags_v1"])
		for _, tag := range tags {
			sceneWeights[tag] += signalWeight
		}
	}

	if profile.EngagedJobs == 0 || totalWeight <= 0 {
		return profile, nil
	}

	profile.PreferredCenter = centerWeighted / totalWeight
	profile.PreferredDuration = durationWeighted / totalWeight
	profile.AverageSignalWeight = profile.WeightedSignals / float64(profile.EngagedJobs)

	for reason, value := range reasonWeights {
		if value <= 0 {
			continue
		}
		profile.ReasonPreference[reason] = value / totalWeight
	}
	if qualitySettings.HighlightNegativeGuardEnabled {
		for reason, negWeight := range reasonNegativeWeights {
			if negWeight <= 0 {
				continue
			}
			posWeight := reasonPositiveWeights[reason]
			total := posWeight + negWeight
			if total <= 0 {
				continue
			}
			dominance := negWeight / total
			threshold := qualitySettings.HighlightNegativeGuardThreshold
			if dominance <= threshold {
				continue
			}
			confidence := math.Min(1, negWeight/qualitySettings.HighlightNegativeGuardMinWeight)
			rangeWidth := 1 - threshold
			if rangeWidth <= 0 {
				rangeWidth = 0.01
			}
			guard := clampZeroOne((dominance-threshold)/rangeWidth) * confidence
			if guard <= 0 {
				continue
			}
			profile.ReasonNegativeGuard[reason] = roundTo(guard, 4)
		}
	}
	for tag, value := range sceneWeights {
		if value <= 0 {
			continue
		}
		profile.ScenePreference[tag] = value / totalWeight
	}
	return profile, nil
}

func legacySignalWeightByFeedbackAction(action string) float64 {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "download":
		return 1.0
	case "favorite", "like", "top_pick":
		// legacy feedback_v1 通过 total_signals + favorite_count*0.8 计分，favorite 类动作约等价 1.8
		return 1.8
	default:
		return 0
	}
}

func (p *Processor) loadUserPublicFeedbackSignalSummary(
	userID uint64,
	jobIDs []uint64,
) (map[uint64]publicFeedbackSignalSummary, error) {
	out := map[uint64]publicFeedbackSignalSummary{}
	if p == nil || p.db == nil || userID == 0 || len(jobIDs) == 0 {
		return out, nil
	}

	type row struct {
		JobID            uint64         `gorm:"column:job_id"`
		OutputID         *uint64        `gorm:"column:output_id"`
		Action           string         `gorm:"column:action"`
		Weight           float64        `gorm:"column:weight"`
		SceneTag         string         `gorm:"column:scene_tag"`
		FeedbackMetadata datatypes.JSON `gorm:"column:feedback_metadata"`
		OutputMetadata   datatypes.JSON `gorm:"column:output_metadata"`
	}

	candidateWindowsByJob, err := p.loadGIFCandidateFeedbackWindows(jobIDs)
	if err != nil {
		// 候选窗口映射异常不阻塞主链路，后续回退到 output metadata / legacy 逻辑。
		candidateWindowsByJob = map[uint64][]gifCandidateFeedbackWindow{}
	}

	var rows []row
	if err := p.db.Table("public.video_image_feedback AS f").
		Select(
			"f.job_id",
			"f.output_id",
			"LOWER(COALESCE(f.action, '')) AS action",
			"COALESCE(f.weight, 0) AS weight",
			"LOWER(COALESCE(NULLIF(TRIM(f.scene_tag), ''), '')) AS scene_tag",
			"f.metadata AS feedback_metadata",
			"o.metadata AS output_metadata",
		).
		Joins("LEFT JOIN public.video_image_outputs o ON o.id = f.output_id AND o.job_id = f.job_id").
		Where("f.user_id = ? AND f.job_id IN ?", userID, jobIDs).
		Order("f.created_at DESC, f.id DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, item := range rows {
		if item.OutputID == nil || *item.OutputID == 0 {
			// 反馈画像严格收敛到 output_id 级，不再消费 job 级历史回退信号。
			continue
		}
		action := strings.ToLower(strings.TrimSpace(item.Action))
		entry := out[item.JobID]
		entry.TotalWeight += item.Weight
		entry.TotalCount++
		entry.DeltaWeight += item.Weight - legacySignalWeightByFeedbackAction(action)
		if item.Weight > 0 {
			entry.PositiveWeight += item.Weight
		} else if item.Weight < 0 {
			entry.NegativeWeight += -item.Weight
		}

		startSec, endSec, reason := parseFeedbackOutputWindowContext(item.OutputMetadata)
		if reason == "" {
			startMeta, endMeta, reasonMeta := parseFeedbackOutputWindowContext(item.FeedbackMetadata)
			if startSec <= 0 && endSec <= 0 {
				startSec = startMeta
				endSec = endMeta
			}
			if reasonMeta != "" {
				reason = reasonMeta
			}
		}
		if reason == "" {
			reason = matchGIFCandidateReasonByWindow(startSec, endSec, candidateWindowsByJob[item.JobID])
		}
		sceneTag := strings.TrimSpace(strings.ToLower(item.SceneTag))
		if sceneTag == "" {
			sceneTag = strings.TrimSpace(strings.ToLower(parseFeedbackSceneTagFromMetadata(item.FeedbackMetadata)))
		}
		entry.Details = append(entry.Details, publicFeedbackSignalDetail{
			OutputID: *item.OutputID,
			Action:   action,
			Weight:   item.Weight,
			SceneTag: sceneTag,
			StartSec: startSec,
			EndSec:   endSec,
			Reason:   reason,
		})
		out[item.JobID] = entry
	}
	return out, nil
}

type gifCandidateFeedbackWindow struct {
	StartSec float64
	EndSec   float64
	Reason   string
	Rank     int
	Selected bool
}

func (p *Processor) loadGIFCandidateFeedbackWindows(jobIDs []uint64) (map[uint64][]gifCandidateFeedbackWindow, error) {
	out := map[uint64][]gifCandidateFeedbackWindow{}
	if p == nil || p.db == nil || len(jobIDs) == 0 {
		return out, nil
	}

	type row struct {
		JobID       uint64         `gorm:"column:job_id"`
		StartMs     int            `gorm:"column:start_ms"`
		EndMs       int            `gorm:"column:end_ms"`
		FinalRank   int            `gorm:"column:final_rank"`
		IsSelected  bool           `gorm:"column:is_selected"`
		FeatureJSON datatypes.JSON `gorm:"column:feature_json"`
	}

	var rows []row
	if err := p.db.Model(&models.VideoJobGIFCandidate{}).
		Select("job_id", "start_ms", "end_ms", "final_rank", "is_selected", "feature_json").
		Where("job_id IN ?", jobIDs).
		Order("job_id ASC, is_selected DESC, final_rank ASC, id ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, item := range rows {
		if item.EndMs <= item.StartMs {
			continue
		}
		reason := parseGIFCandidateReason(item.FeatureJSON)
		if reason == "" {
			if item.IsSelected {
				reason = "selected_candidate"
			} else {
				reason = "candidate"
			}
		}
		out[item.JobID] = append(out[item.JobID], gifCandidateFeedbackWindow{
			StartSec: float64(item.StartMs) / 1000.0,
			EndSec:   float64(item.EndMs) / 1000.0,
			Reason:   reason,
			Rank:     item.FinalRank,
			Selected: item.IsSelected,
		})
	}
	return out, nil
}

func parseGIFCandidateReason(feature datatypes.JSON) string {
	if len(feature) == 0 || string(feature) == "null" {
		return ""
	}
	payload := parseJSONMap(feature)
	return normalizeReason(payload["reason"])
}

func parseFeedbackOutputWindowContext(raw datatypes.JSON) (float64, float64, string) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, 0, ""
	}
	payload := parseJSONMap(raw)

	startSec := floatFromAny(payload["start_sec"])
	if startSec <= 0 {
		startSec = floatFromAny(payload["output_start_sec"])
	}
	endSec := floatFromAny(payload["end_sec"])
	if endSec <= 0 {
		endSec = floatFromAny(payload["output_end_sec"])
	}

	reason := normalizeReason(payload["reason"])
	if reason == "" {
		reason = normalizeReason(payload["output_reason"])
	}
	return startSec, endSec, reason
}

func parseFeedbackSceneTagFromMetadata(raw datatypes.JSON) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	payload := parseJSONMap(raw)
	return stringFromAny(payload["scene_tag"])
}

func matchGIFCandidateReasonByWindow(startSec, endSec float64, windows []gifCandidateFeedbackWindow) string {
	if len(windows) == 0 || endSec <= startSec {
		return ""
	}
	bestReason := ""
	bestIOU := 0.0
	for _, candidate := range windows {
		if candidate.EndSec <= candidate.StartSec {
			continue
		}
		if math.Abs(candidate.StartSec-startSec) <= 0.08 && math.Abs(candidate.EndSec-endSec) <= 0.08 {
			return candidate.Reason
		}
		iou := windowIoU(startSec, endSec, candidate.StartSec, candidate.EndSec)
		if iou <= bestIOU {
			continue
		}
		bestIOU = iou
		bestReason = candidate.Reason
	}
	if bestIOU >= 0.35 {
		return bestReason
	}
	return ""
}

func shouldApplyFeedbackRerank(jobID uint64, settings QualitySettings) bool {
	if !settings.HighlightFeedbackEnabled {
		return false
	}
	rollout := settings.HighlightFeedbackRollout
	if rollout <= 0 {
		return false
	}
	if rollout >= 100 {
		return true
	}
	bucket := int(jobID % 100)
	return bucket < rollout
}

func applyHighlightFeedbackProfile(suggestion highlightSuggestion, durationSec float64, profile highlightFeedbackProfile, qualitySettings QualitySettings) (highlightSuggestion, bool) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	if len(suggestion.Candidates) == 0 || durationSec <= 0 {
		return suggestion, false
	}
	if profile.EngagedJobs < qualitySettings.HighlightFeedbackMinJobs || profile.WeightedSignals < qualitySettings.HighlightFeedbackMinScore {
		return suggestion, false
	}

	baseWeight := qualitySettings.HighlightWeightPosition + qualitySettings.HighlightWeightDuration + qualitySettings.HighlightWeightReason
	if baseWeight <= 0 {
		return suggestion, false
	}

	strength := math.Min(1, profile.WeightedSignals/36) * qualitySettings.HighlightFeedbackBoost
	if strength < 0.15 {
		return suggestion, false
	}
	if profile.PreferredDuration <= 0 {
		profile.PreferredDuration = 0.16
	}

	ranked := append([]highlightCandidate{}, suggestion.Candidates...)
	for idx := range ranked {
		candidate := ranked[idx]
		position := clampZeroOne(((candidate.StartSec + candidate.EndSec) / 2) / durationSec)
		durationRatio := clampZeroOne((candidate.EndSec - candidate.StartSec) / durationSec)
		positionMatch := 1 - math.Abs(position-profile.PreferredCenter)
		if positionMatch < 0 {
			positionMatch = 0
		}

		durationMatch := 1 - math.Abs(durationRatio-profile.PreferredDuration)/0.35
		if durationMatch < 0 {
			durationMatch = 0
		}
		if durationMatch > 1 {
			durationMatch = 1
		}

		reasonWeight := profile.ReasonPreference[normalizeReason(candidate.Reason)]
		if reasonWeight > 1 {
			reasonWeight = 1
		}
		negativeGuard := 0.0
		if qualitySettings.HighlightNegativeGuardEnabled {
			negativeGuard = profile.ReasonNegativeGuard[normalizeReason(candidate.Reason)]
			if negativeGuard > 1 {
				negativeGuard = 1
			}
		}

		boost := (positionMatch*qualitySettings.HighlightWeightPosition +
			durationMatch*qualitySettings.HighlightWeightDuration +
			reasonWeight*qualitySettings.HighlightWeightReason) * strength
		guardPenalty := negativeGuard * qualitySettings.HighlightWeightReason * strength * qualitySettings.HighlightNegativePenaltyWeight
		nextScore := candidate.Score + boost - guardPenalty
		if negativeGuard > 0 {
			guardScale := 1 - negativeGuard*qualitySettings.HighlightNegativePenaltyScale
			if guardScale < 0.2 {
				guardScale = 0.2
			}
			nextScore *= guardScale
		}
		if nextScore < 0 {
			nextScore = 0
		}
		candidate.Score = roundTo(math.Min(1.5, nextScore), 4)
		ranked[idx] = candidate
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			return ranked[i].StartSec < ranked[j].StartSec
		}
		return ranked[i].Score > ranked[j].Score
	})
	selected := pickNonOverlapCandidates(ranked, qualitySettings.GIFCandidateMaxOutputs, qualitySettings.GIFCandidateDedupIOUThreshold)
	selected = applyGIFCandidateConfidenceThreshold(selected, ranked, qualitySettings.GIFCandidateConfidenceThreshold)
	if len(selected) == 0 {
		selected = ranked
	}
	if len(selected) > qualitySettings.GIFCandidateMaxOutputs {
		selected = selected[:qualitySettings.GIFCandidateMaxOutputs]
	}
	if len(selected) == 0 {
		return suggestion, false
	}

	suggestion.Candidates = selected
	if len(suggestion.All) == 0 {
		suggestion.All = ranked
	}
	suggestion.Selected = &selected[0]
	suggestion.Strategy = strings.TrimSpace(suggestion.Strategy + "+feedback_rerank")
	return suggestion, true
}

func inferSceneTags(title, sourceKey string, formats []string) []string {
	text := strings.ToLower(strings.TrimSpace(title + " " + sourceKey))
	if text == "" && len(formats) == 0 {
		return nil
	}

	tagSet := map[string]struct{}{}
	addTag := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		tagSet[tag] = struct{}{}
	}

	containsAny := func(keywords ...string) bool {
		for _, kw := range keywords {
			kw = strings.TrimSpace(strings.ToLower(kw))
			if kw == "" {
				continue
			}
			if strings.Contains(text, kw) {
				return true
			}
		}
		return false
	}

	if containsAny("猫", "狗", "pet", "puppy", "kitten", "宠物", "dog", "cat") {
		addTag("pet")
	}
	if containsAny("探店", "店", "餐厅", "咖啡", "美食", "vlog", "store", "restaurant", "food") {
		addTag("explore")
	}
	if containsAny("搞笑", "笑", "funny", "meme", "reaction", "整活", "沙雕") {
		addTag("funny")
	}
	if containsAny("教程", "讲解", "教学", "lesson", "how to", "guide", "review") {
		addTag("knowledge")
	}
	if containsAny("纪录片", "采访", "新闻", "doc", "interview", "news") {
		addTag("documentary")
	}

	for _, format := range formats {
		switch strings.ToLower(strings.TrimSpace(format)) {
		case "gif", "webp":
			addTag("social")
		case "live":
			addTag("live_creator")
		case "png", "jpg":
			addTag("design")
		}
	}

	if len(tagSet) == 0 {
		addTag("general")
	}

	out := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func detectScenePoints(ctx context.Context, sourcePath string, threshold float64) ([]scenePoint, error) {
	if threshold <= 0 {
		threshold = 0.10
	}
	filter := fmt.Sprintf("select=gt(scene\\,%s),metadata=print:file=-", formatFFmpegNumber(threshold))
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", sourcePath,
		"-vf", filter,
		"-an",
		"-f", "null",
		"-",
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	points := parseSceneMetadataOutput(out)
	if err != nil && len(points) == 0 {
		return nil, fmt.Errorf("scene scoring failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return points, nil
}

func parseSceneMetadataOutput(raw []byte) []scenePoint {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	points := make([]scenePoint, 0)
	pendingPts := -1.0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "frame:") {
			idx := strings.Index(line, "pts_time:")
			if idx >= 0 {
				pendingPts = parseLooseFloat(line[idx+len("pts_time:"):])
			}
			continue
		}
		if strings.HasPrefix(line, "lavfi.scene_score=") {
			score := parseLooseFloat(strings.TrimPrefix(line, "lavfi.scene_score="))
			if pendingPts >= 0 && score > 0 {
				points = append(points, scenePoint{
					PtsSec: pendingPts,
					Score:  score,
				})
			}
			pendingPts = -1
		}
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].Score == points[j].Score {
			return points[i].PtsSec < points[j].PtsSec
		}
		return points[i].Score > points[j].Score
	})
	return points
}

func buildFallbackHighlightCandidates(durationSec, targetDuration float64) []highlightCandidate {
	if durationSec <= 0 {
		return nil
	}
	if targetDuration <= 0 {
		targetDuration = chooseHighlightDuration(durationSec)
	}

	anchors := []float64{durationSec * 0.50, durationSec * 0.25, durationSec * 0.75}
	candidates := make([]highlightCandidate, 0, len(anchors))
	for idx, anchor := range anchors {
		start := anchor - targetDuration/2
		end := start + targetDuration
		start, end = clampHighlightWindow(start, end, durationSec)
		if end-start < 0.8 {
			continue
		}
		score := 0.45 - float64(idx)*0.05
		candidates = append(candidates, highlightCandidate{
			StartSec: start,
			EndSec:   end,
			Score:    roundTo(score, 4),
			Reason:   "fallback_uniform",
		})
	}
	return candidates
}

func chooseHighlightDuration(durationSec float64) float64 {
	if durationSec <= 0 {
		return 3
	}
	if durationSec < 2 {
		return durationSec
	}
	if durationSec <= 8 {
		return math.Max(1.6, durationSec*0.45)
	}
	if durationSec <= 30 {
		return 3.2
	}
	return 4.0
}

func clampHighlightWindow(startSec, endSec, durationSec float64) (float64, float64) {
	if durationSec <= 0 {
		return 0, 0
	}
	if startSec < 0 {
		startSec = 0
	}
	if endSec <= startSec {
		endSec = startSec + 1.2
	}
	if endSec > durationSec {
		overflow := endSec - durationSec
		endSec = durationSec
		startSec -= overflow
		if startSec < 0 {
			startSec = 0
		}
	}
	if endSec <= startSec {
		endSec = durationSec
	}
	return roundTo(startSec, 3), roundTo(endSec, 3)
}

func pickNonOverlapCandidates(candidates []highlightCandidate, topN int, iouThreshold float64) []highlightCandidate {
	if len(candidates) == 0 || topN <= 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].StartSec == candidates[j].StartSec {
				return candidates[i].EndSec < candidates[j].EndSec
			}
			return candidates[i].StartSec < candidates[j].StartSec
		}
		return candidates[i].Score > candidates[j].Score
	})

	out := make([]highlightCandidate, 0, topN)
	for _, cand := range candidates {
		overlap := false
		for _, picked := range out {
			if windowIoU(cand.StartSec, cand.EndSec, picked.StartSec, picked.EndSec) > iouThreshold {
				overlap = true
				break
			}
		}
		if overlap {
			continue
		}
		out = append(out, cand)
		if len(out) >= topN {
			break
		}
	}
	return out
}

func windowIoU(startA, endA, startB, endB float64) float64 {
	interStart := math.Max(startA, startB)
	interEnd := math.Min(endA, endB)
	if interEnd <= interStart {
		return 0
	}
	intersection := interEnd - interStart
	union := (endA - startA) + (endB - startB) - intersection
	if union <= 0 {
		return 0
	}
	return intersection / union
}

func roundTo(v float64, digits int) float64 {
	if digits <= 0 {
		return math.Round(v)
	}
	base := math.Pow(10, float64(digits))
	return math.Round(v*base) / base
}

func roundFloatSlice(in []float64, digits int) []float64 {
	if len(in) == 0 {
		return nil
	}
	out := make([]float64, 0, len(in))
	for _, item := range in {
		out = append(out, roundTo(item, digits))
	}
	return out
}

func averageFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	count := 0
	for _, item := range values {
		if item <= 0 {
			continue
		}
		total += item
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func extractFrames(
	ctx context.Context,
	sourcePath,
	frameDir string,
	meta videoProbeMeta,
	options jobOptions,
	intervalSec float64,
	qualitySettings QualitySettings,
) error {
	args := buildExtractFrameArgs(sourcePath, frameDir, meta, options, intervalSec, qualitySettings)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg extract failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func extractFramesByHighlightCandidates(
	ctx context.Context,
	sourcePath string,
	frameDir string,
	meta videoProbeMeta,
	baseOptions jobOptions,
	candidates []highlightCandidate,
	frameBudget int,
	qualitySettings QualitySettings,
) ([]string, []float64, error) {
	if frameBudget <= 0 {
		frameBudget = baseOptions.MaxStatic
	}
	if frameBudget <= 0 {
		frameBudget = 24
	}
	selected := selectHighlightCandidatesForExtraction(candidates, frameBudget, qualitySettings.GIFCandidateMaxOutputs)
	if len(selected) == 0 {
		return nil, nil, nil
	}
	budgets := allocateFrameBudgets(frameBudget, len(selected))
	if len(budgets) != len(selected) {
		return nil, nil, errors.New("invalid frame budget allocation")
	}

	limit := frameBudget

	allFrames := make([]string, 0, limit)
	intervals := make([]float64, 0, len(selected))

	for idx, candidate := range selected {
		windowDir := filepath.Join(frameDir, fmt.Sprintf("segment_%02d", idx+1))
		if err := os.MkdirAll(windowDir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("create segment dir: %w", err)
		}

		options := baseOptions
		options.StartSec = candidate.StartSec
		options.EndSec = candidate.EndSec
		durationSec := candidate.EndSec - candidate.StartSec
		intervalSec := chooseFrameInterval(durationSec, baseOptions.FrameIntervalSec, budgets[idx])
		intervals = append(intervals, intervalSec)

		if err := extractFrames(ctx, sourcePath, windowDir, meta, options, intervalSec, qualitySettings); err != nil {
			return nil, nil, fmt.Errorf("extract candidate %d: %w", idx+1, err)
		}

		frames, err := collectFramePaths(windowDir, budgets[idx])
		if err != nil {
			return nil, nil, fmt.Errorf("collect candidate %d: %w", idx+1, err)
		}
		allFrames = append(allFrames, frames...)
		if len(allFrames) >= limit {
			allFrames = allFrames[:limit]
			break
		}
	}

	return allFrames, intervals, nil
}

func selectHighlightCandidatesForExtraction(candidates []highlightCandidate, maxStatic int, maxOutputs int) []highlightCandidate {
	if len(candidates) == 0 {
		return nil
	}
	maxWindows := maxOutputs
	if maxWindows <= 0 {
		maxWindows = defaultHighlightTopN
	}
	if maxWindows > 6 {
		maxWindows = 6
	}
	if maxWindows > len(candidates) {
		maxWindows = len(candidates)
	}
	if maxStatic > 0 && maxStatic < maxWindows {
		maxWindows = maxStatic
	}
	out := make([]highlightCandidate, 0, maxWindows)
	for _, candidate := range candidates {
		if candidate.EndSec <= candidate.StartSec {
			continue
		}
		out = append(out, candidate)
		if len(out) >= maxWindows {
			break
		}
	}
	return out
}

func allocateFrameBudgets(totalFrames int, windows int) []int {
	if windows <= 0 {
		return nil
	}
	if totalFrames <= 0 {
		totalFrames = 24
	}
	if totalFrames < windows {
		windows = totalFrames
	}
	base := totalFrames / windows
	extra := totalFrames % windows
	budgets := make([]int, windows)
	for i := 0; i < windows; i++ {
		share := base
		if i < extra {
			share++
		}
		if share <= 0 {
			share = 1
		}
		budgets[i] = share
	}
	return budgets
}

func buildExtractFrameArgs(
	sourcePath,
	frameDir string,
	meta videoProbeMeta,
	options jobOptions,
	intervalSec float64,
	qualitySettings QualitySettings,
) []string {
	startSec, durationSec := resolveClipWindow(meta, options)
	filters := buildFrameFilters(meta, options, intervalSec, qualitySettings)
	outputPattern := filepath.Join(frameDir, "frame_%04d.jpg")
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	jpegQ, extractExt := chooseExtractFrameQualityAndExt(options, qualitySettings)
	if extractExt == "png" {
		outputPattern = filepath.Join(frameDir, "frame_%04d.png")
	}

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
	}
	if startSec > 0 {
		args = append(args, "-ss", formatFFmpegNumber(startSec))
	}
	args = append(args, "-i", sourcePath)
	if durationSec > 0 {
		args = append(args, "-t", formatFFmpegNumber(durationSec))
	}
	if len(filters) > 0 {
		args = append(args, "-vf", strings.Join(filters, ","))
	}
	if extractExt == "png" {
		args = append(args, "-f", "image2", "-vcodec", "png", outputPattern)
	} else {
		args = append(args, "-q:v", jpegQ, outputPattern)
	}
	return args
}

func chooseExtractFrameQualityAndExt(options jobOptions, qualitySettings QualitySettings) (jpegQ string, ext string) {
	jpegQ = "3"
	ext = "jpg"

	wantsJPG := options.RequestedJPG
	wantsPNG := options.RequestedPNG
	if !wantsJPG && !wantsPNG {
		wantsJPG = true
	}

	// In PNG-only clarity mode we keep extraction lossless to avoid JPEG artifacts
	// before PNG post-processing.
	if wantsPNG && !wantsJPG && qualitySettings.PNGProfile == QualityProfileClarity {
		return jpegQ, "png"
	}

	allRequestedSize := true
	if wantsJPG && qualitySettings.JPGProfile != QualityProfileSize {
		allRequestedSize = false
	}
	if wantsPNG && qualitySettings.PNGProfile != QualityProfileSize {
		allRequestedSize = false
	}
	anyRequestedClarity := (wantsJPG && qualitySettings.JPGProfile == QualityProfileClarity) ||
		(wantsPNG && qualitySettings.PNGProfile == QualityProfileClarity)
	switch {
	case allRequestedSize:
		jpegQ = "5"
	case anyRequestedClarity:
		jpegQ = "2"
	}
	return jpegQ, ext
}

func buildFrameFilters(meta videoProbeMeta, options jobOptions, intervalSec float64, qualitySettings QualitySettings) []string {
	filters := make([]string, 0, 4)
	if options.Speed > 0 && math.Abs(options.Speed-1.0) > 0.001 {
		filters = append(filters, fmt.Sprintf("setpts=PTS/%.4f", options.Speed))
	}
	if cropFilter, ok := buildCropFilter(meta, options); ok {
		filters = append(filters, cropFilter)
	}
	if options.FPS > 0 {
		filters = append(filters, fmt.Sprintf("fps=%d", options.FPS))
	} else {
		if intervalSec <= 0 {
			intervalSec = 1.5
		}
		filters = append(filters, fmt.Sprintf("fps=1/%s", formatFFmpegNumber(intervalSec)))
	}
	if options.Width > 0 {
		filters = append(filters, fmt.Sprintf("scale=%d:-2", options.Width))
	}
	if shouldApplyStillClarityEnhancement(meta, options, qualitySettings) {
		// Subtle enhancement chain for photo-like still outputs.
		filters = append(filters, "eq=contrast=1.02:saturation=1.03")
		filters = append(filters, "unsharp=5:5:0.35:5:5:0.0")
	}
	return filters
}

func shouldApplyStillClarityEnhancement(meta videoProbeMeta, options jobOptions, qualitySettings QualitySettings) bool {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	wantsJPG := options.RequestedJPG
	wantsPNG := options.RequestedPNG
	if !wantsJPG && !wantsPNG {
		// Backward compatibility for direct callers/tests that bypass
		// applyStillProfileDefaults and do not set requested still formats.
		wantsJPG = true
		wantsPNG = true
	}

	clarityEnabled := (wantsJPG && qualitySettings.JPGProfile == QualityProfileClarity) ||
		(wantsPNG && qualitySettings.PNGProfile == QualityProfileClarity)
	if !clarityEnabled {
		return false
	}

	targetWidth := meta.Width
	if options.CropW > 0 {
		targetWidth = options.CropW
	}
	if options.Width > 0 {
		targetWidth = options.Width
	}
	if targetWidth > 0 && targetWidth < 720 {
		return false
	}
	return true
}

func buildCropFilter(meta videoProbeMeta, options jobOptions) (string, bool) {
	if options.CropW <= 0 || options.CropH <= 0 {
		return "", false
	}
	x := options.CropX
	y := options.CropY
	w := options.CropW
	h := options.CropH
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if meta.Width > 0 {
		if x >= meta.Width {
			x = 0
		}
		maxW := meta.Width - x
		if maxW <= 0 {
			return "", false
		}
		if w > maxW {
			w = maxW
		}
	}
	if meta.Height > 0 {
		if y >= meta.Height {
			y = 0
		}
		maxH := meta.Height - y
		if maxH <= 0 {
			return "", false
		}
		if h > maxH {
			h = maxH
		}
	}
	if w <= 0 || h <= 0 {
		return "", false
	}
	return fmt.Sprintf("crop=%d:%d:%d:%d", w, h, x, y), true
}

type cropDetectRect struct {
	W int
	H int
	X int
	Y int
}

type cropDetectCandidate struct {
	Rect  cropDetectRect
	Count int
}

func detectAutoLetterboxCrop(ctx context.Context, sourcePath string, meta videoProbeMeta) (autoCropSuggestion, bool, error) {
	if meta.Width <= 0 || meta.Height <= 0 || meta.DurationSec <= 0 {
		return autoCropSuggestion{}, false, nil
	}

	sampleDuration := math.Min(6.0, math.Max(2.5, meta.DurationSec*0.18))
	if sampleDuration > meta.DurationSec {
		sampleDuration = meta.DurationSec
	}
	if sampleDuration < 1.2 {
		return autoCropSuggestion{}, false, nil
	}
	sampleStart := 0.0
	if meta.DurationSec > sampleDuration+1.5 {
		sampleStart = math.Min(2.0, meta.DurationSec*0.2)
	}

	cropCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	args := []string{
		"-hide_banner",
		"-loglevel", "info",
		"-nostats",
	}
	if sampleStart > 0 {
		args = append(args, "-ss", formatFFmpegNumber(sampleStart))
	}
	args = append(args, "-i", sourcePath)
	if sampleDuration > 0 {
		args = append(args, "-t", formatFFmpegNumber(sampleDuration))
	}
	args = append(args,
		"-vf", "cropdetect=24:16:0",
		"-an",
		"-f", "null",
		"-",
	)

	cmd := exec.CommandContext(cropCtx, "ffmpeg", args...)
	rawOutput, err := cmd.CombinedOutput()
	if err != nil {
		return autoCropSuggestion{}, false, fmt.Errorf("ffmpeg cropdetect failed: %w: %s", err, strings.TrimSpace(string(rawOutput)))
	}

	candidate, totalMatches, ok := chooseCropDetectCandidate(string(rawOutput), meta)
	if !ok || totalMatches <= 0 {
		return autoCropSuggestion{}, false, nil
	}

	removedW := meta.Width - candidate.Rect.W
	removedH := meta.Height - candidate.Rect.H
	if removedW < 0 || removedH < 0 {
		return autoCropSuggestion{}, false, nil
	}

	removedAreaRate := 1 - float64(candidate.Rect.W*candidate.Rect.H)/float64(meta.Width*meta.Height)
	if removedAreaRate < 0.06 || removedAreaRate > 0.45 {
		return autoCropSuggestion{}, false, nil
	}

	left := candidate.Rect.X
	right := meta.Width - (candidate.Rect.X + candidate.Rect.W)
	top := candidate.Rect.Y
	bottom := meta.Height - (candidate.Rect.Y + candidate.Rect.H)
	if left < 0 || right < 0 || top < 0 || bottom < 0 {
		return autoCropSuggestion{}, false, nil
	}

	maxLRDiff := int(math.Max(6, float64(meta.Width)*0.08))
	maxTBDiff := int(math.Max(6, float64(meta.Height)*0.08))
	if absInt(left-right) > maxLRDiff || absInt(top-bottom) > maxTBDiff {
		return autoCropSuggestion{}, false, nil
	}

	minRemovedW := int(math.Max(4, float64(meta.Width)*0.04))
	minRemovedH := int(math.Max(4, float64(meta.Height)*0.04))
	if removedW < minRemovedW && removedH < minRemovedH {
		return autoCropSuggestion{}, false, nil
	}

	confidence := float64(candidate.Count) / float64(totalMatches)
	suggestion := autoCropSuggestion{
		CropX:           candidate.Rect.X,
		CropY:           candidate.Rect.Y,
		CropW:           candidate.Rect.W,
		CropH:           candidate.Rect.H,
		MatchCount:      candidate.Count,
		TotalMatches:    totalMatches,
		Confidence:      roundTo(confidence, 3),
		SampleStartSec:  roundTo(sampleStart, 3),
		SampleDuration:  roundTo(sampleDuration, 3),
		RemovedAreaRate: roundTo(removedAreaRate, 4),
	}
	return suggestion, true, nil
}

func chooseCropDetectCandidate(output string, meta videoProbeMeta) (cropDetectCandidate, int, bool) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	counts := map[cropDetectRect]int{}
	totalMatches := 0

	for scanner.Scan() {
		rect, ok := parseCropDetectRectLine(scanner.Text())
		if !ok {
			continue
		}
		if rect.W <= 0 || rect.H <= 0 || rect.X < 0 || rect.Y < 0 {
			continue
		}
		if rect.W > meta.Width || rect.H > meta.Height {
			continue
		}
		if rect.X+rect.W > meta.Width || rect.Y+rect.H > meta.Height {
			continue
		}
		if rect.W == meta.Width && rect.H == meta.Height {
			continue
		}
		counts[rect]++
		totalMatches++
	}
	if totalMatches == 0 || len(counts) == 0 {
		return cropDetectCandidate{}, totalMatches, false
	}

	best := cropDetectCandidate{}
	for rect, count := range counts {
		if count > best.Count {
			best = cropDetectCandidate{Rect: rect, Count: count}
			continue
		}
		if count == best.Count {
			currentArea := rect.W * rect.H
			bestArea := best.Rect.W * best.Rect.H
			if currentArea > bestArea {
				best = cropDetectCandidate{Rect: rect, Count: count}
			}
		}
	}
	if best.Count <= 0 {
		return cropDetectCandidate{}, totalMatches, false
	}
	return best, totalMatches, true
}

func parseCropDetectRectLine(line string) (cropDetectRect, bool) {
	idx := strings.LastIndex(line, "crop=")
	if idx < 0 {
		return cropDetectRect{}, false
	}
	token := strings.TrimSpace(line[idx+len("crop="):])
	var rect cropDetectRect
	if _, err := fmt.Sscanf(token, "%d:%d:%d:%d", &rect.W, &rect.H, &rect.X, &rect.Y); err != nil {
		return cropDetectRect{}, false
	}
	return rect, true
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func resolveClipWindow(meta videoProbeMeta, options jobOptions) (startSec float64, durationSec float64) {
	startSec = options.StartSec
	endSec := options.EndSec
	if startSec < 0 {
		startSec = 0
	}
	if meta.DurationSec > 0 {
		if startSec > meta.DurationSec {
			startSec = 0
		}
		if endSec > meta.DurationSec {
			endSec = meta.DurationSec
		}
	}
	if endSec > 0 && endSec > startSec {
		durationSec = endSec - startSec
	}
	return startSec, durationSec
}

func effectiveSampleDuration(meta videoProbeMeta, options jobOptions) float64 {
	startSec, durationSec := resolveClipWindow(meta, options)
	if durationSec > 0 {
		return durationSec
	}
	if meta.DurationSec > 0 && startSec > 0 && startSec < meta.DurationSec {
		return meta.DurationSec - startSec
	}
	return meta.DurationSec
}

func formatFFmpegNumber(v float64) string {
	if v <= 0 {
		return "0"
	}
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}

func collectFramePaths(frameDir string, maxStatic int) ([]string, error) {
	paths := make([]string, 0, 64)
	for _, pattern := range []string{"frame_*.jpg", "frame_*.png"} {
		matches, err := filepath.Glob(filepath.Join(frameDir, pattern))
		if err != nil {
			return nil, err
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)
	if maxStatic <= 0 {
		maxStatic = 24
	}
	return trimFramePathsEvenly(paths, maxStatic), nil
}

func trimFramePathsEvenly(paths []string, limit int) []string {
	if limit <= 0 || len(paths) <= limit {
		return paths
	}
	if limit == 1 {
		return []string{paths[0]}
	}
	out := make([]string, 0, limit)
	n := len(paths)
	for i := 0; i < limit; i++ {
		idx := i * (n - 1) / (limit - 1)
		if idx < 0 {
			idx = 0
		}
		if idx >= n {
			idx = n - 1
		}
		out = append(out, paths[idx])
	}
	return out
}

func qualitySelectionCandidateBudget(maxStatic int) int {
	if maxStatic <= 0 {
		maxStatic = 24
	}
	budget := maxStatic * 2
	if budget < maxStatic+8 {
		budget = maxStatic + 8
	}
	if budget < 24 {
		budget = 24
	}
	if budget > 160 {
		budget = 160
	}
	return budget
}

func optimizeFramePathsForQuality(paths []string, maxStatic int, qualitySettings QualitySettings) ([]string, frameQualityReport) {
	report := frameQualityReport{
		TotalFrames:     len(paths),
		SelectorVersion: "v1_scene_ranker",
	}
	if len(paths) == 0 {
		return nil, report
	}
	if maxStatic <= 0 {
		maxStatic = 24
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	samples := analyzeFrameQualityBatch(paths, qualitySettings.QualityAnalysisWorkers)
	if len(samples) == 0 {
		fallback := paths
		if len(fallback) > maxStatic {
			fallback = fallback[:maxStatic]
		}
		report.KeptFrames = len(fallback)
		report.FallbackApplied = true
		report.KeptSample = pickFramePathSample(fallback, 6)
		return fallback, report
	}

	blurScores := make([]float64, 0, len(samples))
	for _, sample := range samples {
		if sample.BlurScore > 0 {
			blurScores = append(blurScores, sample.BlurScore)
		}
	}

	blurThreshold := chooseBlurThreshold(blurScores, qualitySettings)
	report.BlurThreshold = roundTo(blurThreshold, 2)

	sceneCutThreshold := chooseSceneCutThreshold(samples)
	report.SceneCutThreshold = roundTo(sceneCutThreshold, 2)
	assignSceneAndMotionScores(samples, sceneCutThreshold)

	sceneIDs := map[int]struct{}{}
	for idx := range samples {
		samples[idx].Exposure = roundTo(computeExposureScore(samples[idx].Brightness, qualitySettings), 3)
		samples[idx].QualityScore = roundTo(computeFrameQualityScore(samples[idx], blurThreshold), 3)
		sceneIDs[samples[idx].SceneID] = struct{}{}
	}
	report.SceneCount = len(sceneIDs)

	selected := make([]frameQualitySample, 0, len(samples))
	rejected := make([]frameQualitySample, 0, len(samples))
	for _, sample := range samples {
		if qualitySettings.StillMinWidth > 0 && sample.Width > 0 && sample.Width < qualitySettings.StillMinWidth {
			report.RejectedResolution++
			rejected = append(rejected, sample)
			continue
		}
		if qualitySettings.StillMinHeight > 0 && sample.Height > 0 && sample.Height < qualitySettings.StillMinHeight {
			report.RejectedResolution++
			rejected = append(rejected, sample)
			continue
		}
		if sample.Brightness < qualitySettings.MinBrightness || sample.Brightness > qualitySettings.MaxBrightness {
			report.RejectedBrightness++
			rejected = append(rejected, sample)
			continue
		}
		if sample.Exposure < qualitySettings.StillMinExposureScore {
			report.RejectedExposure++
			rejected = append(rejected, sample)
			continue
		}
		if sample.BlurScore < qualitySettings.StillMinBlurScore {
			report.RejectedStillBlurGate++
			rejected = append(rejected, sample)
			continue
		}
		if sample.BlurScore < blurThreshold {
			report.RejectedBlur++
			rejected = append(rejected, sample)
			continue
		}
		if hasNearDuplicate(selected, sample.Hash, qualitySettings.DuplicateHammingThreshold, qualitySettings.DuplicateBacktrackFrames) {
			report.RejectedNearDuplicate++
			rejected = append(rejected, sample)
			continue
		}
		selected = append(selected, sample)
	}

	if len(selected) > 0 {
		selected = rankFrameCandidatesByScene(selected, maxStatic, qualitySettings)
	}

	minKeep := qualitySettings.MinKeepBase
	if limit := int(math.Round(float64(len(samples)) * qualitySettings.MinKeepRatio)); limit > minKeep {
		minKeep = limit
	}
	if minKeep > maxStatic {
		minKeep = maxStatic
	}
	if minKeep <= 0 {
		minKeep = 1
	}

	if len(selected) < minKeep {
		report.FallbackApplied = true
		sort.SliceStable(rejected, func(i, j int) bool {
			if rejected[i].QualityScore == rejected[j].QualityScore {
				return rejected[i].Index < rejected[j].Index
			}
			return rejected[i].QualityScore > rejected[j].QualityScore
		})
		for _, sample := range rejected {
			if !passesStillHardQualityGate(sample, qualitySettings) {
				continue
			}
			if sample.Brightness < qualitySettings.MinBrightness || sample.Brightness > qualitySettings.MaxBrightness {
				continue
			}
			if sample.BlurScore < blurThreshold*qualitySettings.FallbackBlurRelaxFactor {
				continue
			}
			if hasNearDuplicate(selected, sample.Hash, qualitySettings.FallbackHammingThreshold, qualitySettings.DuplicateBacktrackFrames) {
				continue
			}
			selected = append(selected, sample)
			if len(selected) >= minKeep || len(selected) >= maxStatic {
				break
			}
		}
		selected = rankFrameCandidatesByScene(selected, maxStatic, qualitySettings)
	}

	if len(selected) == 0 {
		report.FallbackApplied = true
		fallbackCandidates := append([]frameQualitySample{}, samples...)
		sort.SliceStable(fallbackCandidates, func(i, j int) bool {
			if fallbackCandidates[i].QualityScore == fallbackCandidates[j].QualityScore {
				return fallbackCandidates[i].Index < fallbackCandidates[j].Index
			}
			return fallbackCandidates[i].QualityScore > fallbackCandidates[j].QualityScore
		})
		hardFiltered := make([]string, 0, maxStatic)
		for _, sample := range fallbackCandidates {
			if sample.Brightness < qualitySettings.MinBrightness || sample.Brightness > qualitySettings.MaxBrightness {
				continue
			}
			if !passesStillHardQualityGate(sample, qualitySettings) {
				continue
			}
			hardFiltered = append(hardFiltered, sample.Path)
			if len(hardFiltered) >= maxStatic {
				break
			}
		}
		if len(hardFiltered) > 0 {
			report.KeptFrames = len(hardFiltered)
			report.KeptSample = pickFramePathSample(hardFiltered, 6)
			return hardFiltered, report
		}
		fallback := paths
		if len(fallback) > maxStatic {
			fallback = fallback[:maxStatic]
		}
		report.KeptFrames = len(fallback)
		report.KeptSample = pickFramePathSample(fallback, 6)
		return fallback, report
	}

	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].QualityScore == selected[j].QualityScore {
			return selected[i].Index < selected[j].Index
		}
		return selected[i].QualityScore > selected[j].QualityScore
	})

	out := make([]string, 0, len(selected))
	totalScore := 0.0
	for _, sample := range selected {
		out = append(out, sample.Path)
		totalScore += sample.QualityScore
		if len(out) >= maxStatic {
			break
		}
	}

	report.KeptFrames = len(out)
	if report.KeptFrames > 0 {
		report.AvgKeptScore = roundTo(totalScore/float64(report.KeptFrames), 3)
	}
	report.KeptSample = pickFramePathSample(out, 6)
	return out, report
}

func chooseSceneCutThreshold(samples []frameQualitySample) float64 {
	if len(samples) <= 1 {
		return 12
	}
	diffs := make([]float64, 0, len(samples)-1)
	for idx := 1; idx < len(samples); idx++ {
		diff := float64(hammingDistance64(samples[idx-1].Hash, samples[idx].Hash))
		if diff <= 0 {
			continue
		}
		diffs = append(diffs, diff)
	}
	if len(diffs) == 0 {
		return 12
	}
	sort.Float64s(diffs)
	median := diffs[len(diffs)/2]
	p85 := diffs[int(math.Round(float64(len(diffs)-1)*0.85))]
	threshold := math.Max(median*1.45, p85*0.92)
	if threshold < 8 {
		threshold = 8
	}
	if threshold > 28 {
		threshold = 28
	}
	return threshold
}

func assignSceneAndMotionScores(samples []frameQualitySample, sceneCutThreshold float64) {
	if len(samples) == 0 {
		return
	}
	if sceneCutThreshold <= 0 {
		sceneCutThreshold = 12
	}
	sceneID := 1
	for idx := range samples {
		if idx > 0 {
			diff := float64(hammingDistance64(samples[idx-1].Hash, samples[idx].Hash))
			if diff >= sceneCutThreshold {
				sceneID++
			}
			motion := diff / sceneCutThreshold
			if motion > 1 {
				motion = 1
			}
			samples[idx].MotionScore = roundTo(motion, 3)
		} else {
			samples[idx].MotionScore = 0
		}
		samples[idx].SceneID = sceneID
	}
}

func computeExposureScore(brightness float64, qualitySettings QualitySettings) float64 {
	minB := qualitySettings.MinBrightness
	maxB := qualitySettings.MaxBrightness
	if maxB <= minB {
		minB = 16
		maxB = 244
	}
	if brightness < minB || brightness > maxB {
		return 0
	}
	center := (minB + maxB) / 2
	half := (maxB - minB) / 2
	if half <= 0 {
		return 0
	}
	distance := math.Abs(brightness-center) / half
	score := 1 - distance
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

func computeFrameQualityScore(sample frameQualitySample, blurThreshold float64) float64 {
	if blurThreshold <= 0 {
		blurThreshold = 1
	}
	blurScore := sample.BlurScore / (blurThreshold * 2.4)
	if blurScore > 1 {
		blurScore = 1
	}
	if blurScore < 0 {
		blurScore = 0
	}
	subject := sample.SubjectScore
	if subject < 0 {
		subject = 0
	}
	if subject > 1 {
		subject = 1
	}
	motion := sample.MotionScore
	if motion < 0 {
		motion = 0
	}
	if motion > 1 {
		motion = 1
	}
	exposure := sample.Exposure
	if exposure < 0 {
		exposure = 0
	}
	if exposure > 1 {
		exposure = 1
	}
	// Weight visual clarity first, then exposure/subject, with a small boost for motion peaks.
	return blurScore*0.44 + exposure*0.24 + subject*0.20 + motion*0.12
}

func passesStillHardQualityGate(sample frameQualitySample, qualitySettings QualitySettings) bool {
	if qualitySettings.StillMinWidth > 0 && sample.Width > 0 && sample.Width < qualitySettings.StillMinWidth {
		return false
	}
	if qualitySettings.StillMinHeight > 0 && sample.Height > 0 && sample.Height < qualitySettings.StillMinHeight {
		return false
	}
	if sample.Exposure < qualitySettings.StillMinExposureScore {
		return false
	}
	if sample.BlurScore < qualitySettings.StillMinBlurScore {
		return false
	}
	return true
}

func rankFrameCandidatesByScene(in []frameQualitySample, maxStatic int, qualitySettings QualitySettings) []frameQualitySample {
	if len(in) == 0 {
		return nil
	}
	if maxStatic <= 0 {
		maxStatic = len(in)
	}

	grouped := map[int][]frameQualitySample{}
	sceneIDs := make([]int, 0, len(in))
	seenScene := map[int]struct{}{}
	for _, item := range in {
		sceneID := item.SceneID
		if sceneID <= 0 {
			sceneID = 1
		}
		grouped[sceneID] = append(grouped[sceneID], item)
		if _, ok := seenScene[sceneID]; !ok {
			seenScene[sceneID] = struct{}{}
			sceneIDs = append(sceneIDs, sceneID)
		}
	}
	sort.Ints(sceneIDs)
	for _, sceneID := range sceneIDs {
		items := grouped[sceneID]
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].QualityScore == items[j].QualityScore {
				return items[i].Index < items[j].Index
			}
			return items[i].QualityScore > items[j].QualityScore
		})
		grouped[sceneID] = items
	}

	out := make([]frameQualitySample, 0, minInt(maxStatic, len(in)))
	seenPath := map[string]struct{}{}
	addSample := func(item frameQualitySample) bool {
		key := strings.TrimSpace(item.Path)
		if key == "" {
			return false
		}
		if _, ok := seenPath[key]; ok {
			return false
		}
		if hasNearDuplicate(out, item.Hash, qualitySettings.DuplicateHammingThreshold, qualitySettings.DuplicateBacktrackFrames) {
			return false
		}
		seenPath[key] = struct{}{}
		out = append(out, item)
		return true
	}

	// First pass: keep at least one best candidate per detected scene.
	for _, sceneID := range sceneIDs {
		for _, item := range grouped[sceneID] {
			if addSample(item) {
				break
			}
		}
		if len(out) >= maxStatic {
			return out[:maxStatic]
		}
	}

	// Second pass: fill the rest with global best candidates.
	global := append([]frameQualitySample{}, in...)
	sort.SliceStable(global, func(i, j int) bool {
		if global[i].QualityScore == global[j].QualityScore {
			return global[i].Index < global[j].Index
		}
		return global[i].QualityScore > global[j].QualityScore
	})
	for _, item := range global {
		if len(out) >= maxStatic {
			break
		}
		_ = addSample(item)
	}
	return out
}

func analyzeFrameQualityBatch(paths []string, workers int) []frameQualitySample {
	if len(paths) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(paths) {
		workers = len(paths)
	}

	type frameTask struct {
		Index int
		Path  string
	}

	results := make([]*frameQualitySample, len(paths))
	jobs := make(chan frameTask)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range jobs {
				sample, ok := analyzeFrameQuality(task.Path)
				if !ok {
					continue
				}
				sample.Index = task.Index
				copySample := sample
				results[task.Index] = &copySample
			}
		}()
	}

	for idx, filePath := range paths {
		jobs <- frameTask{
			Index: idx,
			Path:  filePath,
		}
	}
	close(jobs)
	wg.Wait()

	out := make([]frameQualitySample, 0, len(paths))
	for _, sample := range results {
		if sample == nil {
			continue
		}
		out = append(out, *sample)
	}
	return out
}

func analyzeFrameQuality(filePath string) (frameQualitySample, bool) {
	f, err := os.Open(filePath)
	if err != nil {
		return frameQualitySample{}, false
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return frameQualitySample{}, false
	}
	analysisImage := prepareQualityAnalysisImage(img, 256)
	bounds := img.Bounds()

	brightness := imageBrightness(analysisImage)
	blurScore := laplacianVariance(analysisImage)
	subjectScore := estimateCenterSaliencyScore(analysisImage)
	hash := imageDHash(analysisImage)

	return frameQualitySample{
		Path:         filePath,
		Width:        bounds.Dx(),
		Height:       bounds.Dy(),
		Brightness:   roundTo(brightness, 2),
		BlurScore:    roundTo(blurScore, 2),
		SubjectScore: roundTo(subjectScore, 3),
		Hash:         hash,
	}, true
}

func prepareQualityAnalysisImage(src image.Image, maxEdge int) image.Image {
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return src
	}
	if maxEdge <= 0 {
		maxEdge = 256
	}

	targetW := width
	targetH := height
	if width > maxEdge || height > maxEdge {
		scale := math.Min(float64(maxEdge)/float64(width), float64(maxEdge)/float64(height))
		targetW = int(math.Round(float64(width) * scale))
		targetH = int(math.Round(float64(height) * scale))
		if targetW < 1 {
			targetW = 1
		}
		if targetH < 1 {
			targetH = 1
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)
	return dst
}

func chooseBlurThreshold(values []float64, qualitySettings QualitySettings) float64 {
	if len(values) == 0 {
		return qualitySettings.BlurThresholdMin
	}
	scores := make([]float64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			scores = append(scores, value)
		}
	}
	if len(scores) == 0 {
		return qualitySettings.BlurThresholdMin
	}
	sort.Float64s(scores)
	median := scores[len(scores)/2]
	threshold := median * qualitySettings.BlurThresholdFactor
	if threshold < qualitySettings.BlurThresholdMin {
		threshold = qualitySettings.BlurThresholdMin
	}
	if threshold > qualitySettings.BlurThresholdMax {
		threshold = qualitySettings.BlurThresholdMax
	}
	return threshold
}

func hasNearDuplicate(selected []frameQualitySample, hash uint64, maxDistance int, backtrack int) bool {
	if len(selected) == 0 {
		return false
	}
	if maxDistance < 0 {
		maxDistance = 0
	}
	if backtrack <= 0 {
		backtrack = 1
	}
	start := len(selected) - backtrack
	if start < 0 {
		start = 0
	}
	for idx := len(selected) - 1; idx >= start; idx-- {
		if hammingDistance64(selected[idx].Hash, hash) <= maxDistance {
			return true
		}
	}
	return false
}

func hammingDistance64(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

func imageBrightness(img image.Image) float64 {
	bounds := img.Bounds()
	if bounds.Empty() {
		return 0
	}

	if rgba, ok := img.(*image.RGBA); ok {
		width := bounds.Dx()
		height := bounds.Dy()
		if width <= 0 || height <= 0 {
			return 0
		}
		sum := 0.0
		count := 0
		for y := 0; y < height; y++ {
			row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+width*4]
			for x := 0; x < width; x++ {
				idx := x * 4
				luma := 0.299*float64(row[idx]) + 0.587*float64(row[idx+1]) + 0.114*float64(row[idx+2])
				sum += luma
				count++
			}
		}
		if count == 0 {
			return 0
		}
		return sum / float64(count)
	}

	sum := 0.0
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// RGBA values are 16-bit in [0, 65535], convert to 8-bit luminance.
			luma := 0.299*float64(r)/257.0 + 0.587*float64(g)/257.0 + 0.114*float64(b)/257.0
			sum += luma
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

func laplacianVariance(img image.Image) float64 {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width < 3 || height < 3 {
		return 0
	}

	gray := make([]float64, width*height)

	if rgba, ok := img.(*image.RGBA); ok {
		for y := 0; y < height; y++ {
			row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+width*4]
			for x := 0; x < width; x++ {
				idx := y*width + x
				pixIdx := x * 4
				gray[idx] = 0.299*float64(row[pixIdx]) + 0.587*float64(row[pixIdx+1]) + 0.114*float64(row[pixIdx+2])
			}
		}
	} else {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				idx := (y-bounds.Min.Y)*width + (x - bounds.Min.X)
				r, g, b, _ := img.At(x, y).RGBA()
				gray[idx] = 0.299*float64(r)/257.0 + 0.587*float64(g)/257.0 + 0.114*float64(b)/257.0
			}
		}
	}

	laplacians := make([]float64, 0, (width-2)*(height-2))
	for y := 1; y < height-1; y++ {
		row := y * width
		for x := 1; x < width-1; x++ {
			center := gray[row+x]
			top := gray[row-width+x]
			bottom := gray[row+width+x]
			left := gray[row+x-1]
			right := gray[row+x+1]
			lap := top + bottom + left + right - 4*center
			laplacians = append(laplacians, lap)
		}
	}
	if len(laplacians) == 0 {
		return 0
	}

	mean := 0.0
	for _, value := range laplacians {
		mean += value
	}
	mean /= float64(len(laplacians))

	variance := 0.0
	for _, value := range laplacians {
		diff := value - mean
		variance += diff * diff
	}
	return variance / float64(len(laplacians))
}

func imageDHash(img image.Image) uint64 {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return 0
	}

	sample := [8][9]float64{}
	if rgba, ok := img.(*image.RGBA); ok {
		for y := 0; y < 8; y++ {
			srcY := y * (height - 1) / 7
			row := rgba.Pix[srcY*rgba.Stride : srcY*rgba.Stride+width*4]
			for x := 0; x < 9; x++ {
				srcX := x * (width - 1) / 8
				idx := srcX * 4
				sample[y][x] = 0.299*float64(row[idx]) + 0.587*float64(row[idx+1]) + 0.114*float64(row[idx+2])
			}
		}
	} else {
		for y := 0; y < 8; y++ {
			srcY := bounds.Min.Y + y*(height-1)/7
			for x := 0; x < 9; x++ {
				srcX := bounds.Min.X + x*(width-1)/8
				r, g, b, _ := img.At(srcX, srcY).RGBA()
				sample[y][x] = 0.299*float64(r)/257.0 + 0.587*float64(g)/257.0 + 0.114*float64(b)/257.0
			}
		}
	}

	var hash uint64
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			hash <<= 1
			if sample[y][x] > sample[y][x+1] {
				hash |= 1
			}
		}
	}
	return hash
}

func estimateCenterSaliencyScore(img image.Image) float64 {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width < 3 || height < 3 {
		return 0
	}

	gray := make([]float64, width*height)
	if rgba, ok := img.(*image.RGBA); ok {
		for y := 0; y < height; y++ {
			row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+width*4]
			for x := 0; x < width; x++ {
				idx := y*width + x
				pixIdx := x * 4
				gray[idx] = 0.299*float64(row[pixIdx]) + 0.587*float64(row[pixIdx+1]) + 0.114*float64(row[pixIdx+2])
			}
		}
	} else {
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				idx := (y-bounds.Min.Y)*width + (x - bounds.Min.X)
				r, g, b, _ := img.At(x, y).RGBA()
				gray[idx] = 0.299*float64(r)/257.0 + 0.587*float64(g)/257.0 + 0.114*float64(b)/257.0
			}
		}
	}

	centerMinX := width / 4
	centerMaxX := width * 3 / 4
	centerMinY := height / 4
	centerMaxY := height * 3 / 4
	if centerMaxX <= centerMinX || centerMaxY <= centerMinY {
		return 0
	}

	totalEnergy := 0.0
	centerEnergy := 0.0
	totalCount := 0
	centerCount := 0
	for y := 1; y < height-1; y++ {
		row := y * width
		for x := 1; x < width-1; x++ {
			gx := gray[row+x+1] - gray[row+x-1]
			gy := gray[row+width+x] - gray[row-width+x]
			energy := math.Abs(gx) + math.Abs(gy)
			totalEnergy += energy
			totalCount++
			if x >= centerMinX && x < centerMaxX && y >= centerMinY && y < centerMaxY {
				centerEnergy += energy
				centerCount++
			}
		}
	}
	if totalEnergy <= 0 || totalCount <= 0 || centerCount <= 0 {
		return 0
	}
	energyRatio := centerEnergy / totalEnergy
	areaRatio := float64(centerCount) / float64(totalCount)
	if areaRatio <= 0 {
		return 0
	}
	// Normalize by expected center area share; values >1 mean center has denser visual information.
	densityRatio := energyRatio / areaRatio
	score := (densityRatio - 0.75) / 1.35
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score
}

func estimatePortraitHintScore(filePath string) float64 {
	f, err := os.Open(filePath)
	if err != nil {
		return 0
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return 0
	}
	return estimatePortraitHintFromImage(prepareQualityAnalysisImage(img, 256))
}

func estimatePortraitHintFromImage(img image.Image) float64 {
	if img == nil {
		return 0
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width < 8 || height < 8 {
		return 0
	}

	centerMinX := width / 4
	centerMaxX := width * 3 / 4
	centerMinY := height / 4
	centerMaxY := height * 3 / 4
	if centerMaxX <= centerMinX || centerMaxY <= centerMinY {
		return 0
	}

	totalSkin := 0
	totalCount := 0
	centerSkin := 0
	centerCount := 0
	step := 2
	if rgba, ok := img.(*image.RGBA); ok {
		for y := 0; y < height; y += step {
			row := rgba.Pix[y*rgba.Stride : y*rgba.Stride+width*4]
			inCenterY := y >= centerMinY && y < centerMaxY
			for x := 0; x < width; x += step {
				idx := x * 4
				r := row[idx]
				g := row[idx+1]
				b := row[idx+2]
				_, cb, cr := color.RGBToYCbCr(r, g, b)
				isSkin := cb >= 77 && cb <= 127 && cr >= 133 && cr <= 173
				totalCount++
				if isSkin {
					totalSkin++
				}
				if inCenterY && x >= centerMinX && x < centerMaxX {
					centerCount++
					if isSkin {
						centerSkin++
					}
				}
			}
		}
	} else {
		for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
			yy := y - bounds.Min.Y
			inCenterY := yy >= centerMinY && yy < centerMaxY
			for x := bounds.Min.X; x < bounds.Max.X; x += step {
				xx := x - bounds.Min.X
				r16, g16, b16, _ := img.At(x, y).RGBA()
				_, cb, cr := color.RGBToYCbCr(uint8(r16/257), uint8(g16/257), uint8(b16/257))
				isSkin := cb >= 77 && cb <= 127 && cr >= 133 && cr <= 173
				totalCount++
				if isSkin {
					totalSkin++
				}
				if inCenterY && xx >= centerMinX && xx < centerMaxX {
					centerCount++
					if isSkin {
						centerSkin++
					}
				}
			}
		}
	}

	if totalCount == 0 || centerCount == 0 {
		return 0
	}
	centerSkinRatio := float64(centerSkin) / float64(centerCount)
	if centerSkinRatio < 0.02 {
		return 0
	}
	globalSkinRatio := float64(totalSkin) / float64(totalCount)
	centerScore := clampZeroOne((centerSkinRatio - 0.02) / 0.20)
	concentration := centerSkinRatio / (globalSkinRatio + 0.01)
	concentrationScore := clampZeroOne((concentration - 1.0) / 2.0)
	saliencyScore := estimateCenterSaliencyScore(img)
	return clampZeroOne(centerScore*0.50 + concentrationScore*0.25 + saliencyScore*0.25)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxIntValue(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func pickFramePathSample(paths []string, limit int) []string {
	if len(paths) == 0 || limit <= 0 {
		return nil
	}
	if len(paths) <= limit {
		return append([]string{}, paths...)
	}
	if limit == 1 {
		return []string{paths[0]}
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		idx := int(math.Round(float64(i) * float64(len(paths)-1) / float64(limit-1)))
		out = append(out, paths[idx])
	}
	return out
}

func readImageInfo(filePath string) (int64, int, int) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, 0, 0
	}
	size := info.Size()
	f, err := os.Open(filePath)
	if err != nil {
		return size, 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return size, 0, 0
	}
	return size, cfg.Width, cfg.Height
}

func uploadFileToQiniu(uploader *qiniustorage.FormUploader, q *storage.QiniuClient, key, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	putPolicy := qiniustorage.PutPolicy{Scope: q.Bucket + ":" + key}
	upToken := putPolicy.UploadToken(q.Mac)
	var ret qiniustorage.PutRet
	return uploader.Put(context.Background(), &ret, upToken, key, f, info.Size(), &qiniustorage.PutExtra{})
}

func deleteQiniuKeysByPrefix(q *storage.QiniuClient, keys []string) {
	if q == nil || len(keys) == 0 {
		return
	}
	ops := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimLeft(strings.TrimSpace(key), "/")
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ops = append(ops, qiniustorage.URIDelete(q.Bucket, key))
	}
	if len(ops) == 0 {
		return
	}
	_, _ = q.BucketManager().Batch(ops)
}

func deleteQiniuKey(q *storage.QiniuClient, key string) error {
	if q == nil {
		return nil
	}
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return nil
	}
	if err := q.BucketManager().Delete(q.Bucket, key); err != nil && !isQiniuNotFoundErr(err) {
		return err
	}
	return nil
}

func isQiniuNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "612")
}

func slugify(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return fmt.Sprintf("video-job-%d", time.Now().Unix())
	}
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '_' || r == ' ' {
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fmt.Sprintf("video-job-%d", time.Now().Unix())
	}
	return out
}

func ensureUniqueSlug(db *gorm.DB, slug string) string {
	base := strings.TrimSpace(slug)
	if base == "" {
		base = fmt.Sprintf("video-job-%d", time.Now().Unix())
	}
	candidate := base
	for i := 0; i < 100; i++ {
		var count int64
		_ = db.Model(&models.Collection{}).Where("slug = ?", candidate).Count(&count).Error
		if count == 0 {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, i+1)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

func ensureUniqueDownloadCode(db *gorm.DB) (string, error) {
	for i := 0; i < 10; i++ {
		code, err := randomDownloadCode(downloadCodeLength)
		if err != nil {
			return "", err
		}
		var count int64
		if err := db.Model(&models.Collection{}).Where("download_code = ?", code).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", errors.New("failed to generate unique download code")
}

func randomDownloadCode(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("invalid code length")
	}
	max := big.NewInt(int64(len(downloadCodeAlphabet)))
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = downloadCodeAlphabet[n.Int64()]
	}
	return string(out), nil
}

func parseFPS(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if strings.Contains(raw, "/") {
		parts := strings.SplitN(raw, "/", 2)
		num := parseFloat(parts[0])
		den := parseFloat(parts[1])
		if den <= 0 {
			return 0
		}
		return num / den
	}
	return parseFloat(raw)
}

func parseFloat(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func parseLooseFloat(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if idx := strings.IndexAny(raw, " \t\r\n"); idx >= 0 {
		raw = raw[:idx]
	}
	return parseFloat(raw)
}

func mapFromAny(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case map[string]interface{}:
		return value
	default:
		return map[string]interface{}{}
	}
}

func stringSliceFromAny(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok {
		if values, ok2 := raw.([]string); ok2 {
			out := make([]string, 0, len(values))
			for _, value := range values {
				value = strings.TrimSpace(strings.ToLower(value))
				if value == "" {
					continue
				}
				out = append(out, value)
			}
			return out
		}
		return nil
	}

	out := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(strings.ToLower(stringFromAny(item)))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func stringFromAny(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func floatFromAny(raw interface{}) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case int32:
		return float64(value)
	case uint64:
		return float64(value)
	case json.Number:
		f, _ := value.Float64()
		return f
	case string:
		return parseFloat(value)
	default:
		return 0
	}
}

func normalizeReason(raw interface{}) string {
	return strings.ToLower(strings.TrimSpace(stringFromAny(raw)))
}

func clampZeroOne(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

type jobOptions struct {
	AutoHighlight    bool    `json:"auto_highlight"`
	MaxStatic        int     `json:"max_static"`
	FrameIntervalSec float64 `json:"frame_interval_sec"`
	StartSec         float64 `json:"start_sec"`
	EndSec           float64 `json:"end_sec"`
	CropX            int     `json:"crop_x"`
	CropY            int     `json:"crop_y"`
	CropW            int     `json:"crop_w"`
	CropH            int     `json:"crop_h"`
	Speed            float64 `json:"speed"`
	FPS              int     `json:"fps"`
	Width            int     `json:"width"`
	MaxColors        int     `json:"max_colors"`
	RequestedJPG     bool    `json:"-"`
	RequestedPNG     bool    `json:"-"`
}

func parseJobOptions(raw datatypes.JSON) jobOptions {
	out := jobOptions{
		AutoHighlight:    true,
		MaxStatic:        24,
		FrameIntervalSec: 0,
		Speed:            1,
	}
	if len(raw) == 0 || string(raw) == "null" {
		return out
	}
	var payload struct {
		AutoHighlight    *bool   `json:"auto_highlight"`
		MaxStatic        int     `json:"max_static"`
		FrameIntervalSec float64 `json:"frame_interval_sec"`
		StartSec         float64 `json:"start_sec"`
		EndSec           float64 `json:"end_sec"`
		CropX            int     `json:"crop_x"`
		CropY            int     `json:"crop_y"`
		CropW            int     `json:"crop_w"`
		CropH            int     `json:"crop_h"`
		Speed            float64 `json:"speed"`
		FPS              int     `json:"fps"`
		Width            int     `json:"width"`
		MaxColors        int     `json:"max_colors"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return out
	}
	if payload.AutoHighlight != nil {
		out.AutoHighlight = *payload.AutoHighlight
	}
	out.MaxStatic = payload.MaxStatic
	out.FrameIntervalSec = payload.FrameIntervalSec
	out.StartSec = payload.StartSec
	out.EndSec = payload.EndSec
	out.CropX = payload.CropX
	out.CropY = payload.CropY
	out.CropW = payload.CropW
	out.CropH = payload.CropH
	out.Speed = payload.Speed
	out.FPS = payload.FPS
	out.Width = payload.Width
	out.MaxColors = payload.MaxColors
	if out.MaxStatic <= 0 {
		out.MaxStatic = 24
	}
	if out.MaxStatic > 80 {
		out.MaxStatic = 80
	}
	if out.FrameIntervalSec < 0 {
		out.FrameIntervalSec = 0
	}
	if out.StartSec < 0 {
		out.StartSec = 0
	}
	if out.EndSec < 0 {
		out.EndSec = 0
	}
	if out.EndSec > 0 && out.EndSec <= out.StartSec {
		out.EndSec = 0
	}

	if out.CropX < 0 {
		out.CropX = 0
	}
	if out.CropY < 0 {
		out.CropY = 0
	}
	if out.CropW < 0 {
		out.CropW = 0
	}
	if out.CropH < 0 {
		out.CropH = 0
	}

	if out.Speed <= 0 {
		out.Speed = 1
	}
	if out.Speed < 0.5 {
		out.Speed = 0.5
	}
	if out.Speed > 2.0 {
		out.Speed = 2.0
	}

	if out.FPS < 0 {
		out.FPS = 0
	}
	if out.FPS > 30 {
		out.FPS = 30
	}
	if out.FPS > 0 && out.FPS < 4 {
		out.FPS = 4
	}

	if out.Width < 0 {
		out.Width = 0
	}
	if out.Width > 1280 {
		out.Width = 1280
	}
	if out.Width > 0 && out.Width < 120 {
		out.Width = 120
	}

	if out.MaxColors < 0 {
		out.MaxColors = 0
	}
	if out.MaxColors > 256 {
		out.MaxColors = 256
	}
	if out.MaxColors > 0 && out.MaxColors < 16 {
		out.MaxColors = 16
	}
	return out
}

func applyQualityProfileOverridesFromOptions(
	settings QualitySettings,
	options map[string]interface{},
	requestedFormats []string,
) (QualitySettings, map[string]string) {
	settings = NormalizeQualitySettings(settings)
	if len(options) == 0 {
		return settings, nil
	}
	raw, ok := options["quality_profile_overrides"]
	if !ok || raw == nil {
		return settings, nil
	}
	overrides, ok := raw.(map[string]interface{})
	if !ok || len(overrides) == 0 {
		return settings, nil
	}

	requested := map[string]struct{}{}
	for _, format := range requestedFormats {
		format = strings.ToLower(strings.TrimSpace(format))
		if format == "" {
			continue
		}
		requested[format] = struct{}{}
	}

	normalizeProfile := func(value interface{}) (string, bool) {
		profile := strings.ToLower(strings.TrimSpace(stringFromAny(value)))
		switch profile {
		case QualityProfileClarity, QualityProfileSize:
			return profile, true
		default:
			return "", false
		}
	}

	applied := map[string]string{}
	setIfRequested := func(format string) (string, bool) {
		if _, ok := requested[format]; !ok {
			return "", false
		}
		value, exists := overrides[format]
		if !exists {
			return "", false
		}
		return normalizeProfile(value)
	}

	if profile, ok := setIfRequested("gif"); ok {
		settings.GIFProfile = profile
		applied["gif"] = profile
	}
	if profile, ok := setIfRequested("webp"); ok {
		settings.WebPProfile = profile
		applied["webp"] = profile
	}
	liveProfile := ""
	if profile, ok := setIfRequested("live"); ok {
		liveProfile = profile
		applied["live"] = profile
	}
	if profile, ok := setIfRequested("mp4"); ok {
		if liveProfile == "" {
			liveProfile = profile
		}
		applied["mp4"] = profile
	}
	if liveProfile != "" {
		settings.LiveProfile = liveProfile
	}
	if profile, ok := setIfRequested("jpg"); ok {
		settings.JPGProfile = profile
		applied["jpg"] = profile
	}
	if profile, ok := setIfRequested("png"); ok {
		settings.PNGProfile = profile
		applied["png"] = profile
	}
	if len(applied) == 0 {
		return settings, nil
	}
	return NormalizeQualitySettings(settings), applied
}

func jobOptionsMetrics(options jobOptions, intervalSec float64) map[string]interface{} {
	out := map[string]interface{}{
		"auto_highlight":     options.AutoHighlight,
		"max_static":         options.MaxStatic,
		"frame_interval_sec": intervalSec,
	}
	if options.StartSec > 0 {
		out["start_sec"] = options.StartSec
	}
	if options.EndSec > 0 {
		out["end_sec"] = options.EndSec
	}
	if options.CropW > 0 && options.CropH > 0 {
		out["crop"] = map[string]interface{}{
			"x": options.CropX,
			"y": options.CropY,
			"w": options.CropW,
			"h": options.CropH,
		}
	}
	if options.Speed > 0 && math.Abs(options.Speed-1.0) > 0.001 {
		out["speed"] = options.Speed
	}
	if options.FPS > 0 {
		out["fps"] = options.FPS
	}
	if options.Width > 0 {
		out["width"] = options.Width
	}
	if options.MaxColors > 0 {
		out["max_colors"] = options.MaxColors
	}
	return out
}

func mustJSON(v interface{}) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("{}"))
	}
	b, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	if len(b) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(b)
}

func parseJSONMap(raw datatypes.JSON) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func sourceVideoDeleted(metrics map[string]interface{}) bool {
	if len(metrics) == 0 {
		return false
	}
	raw, ok := metrics["source_video_deleted"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func fallbackTitle(title string) string {
	title = strings.TrimSpace(title)
	if title != "" {
		return title
	}
	return fmt.Sprintf("视频表情包-%s", time.Now().Format("20060102150405"))
}

func normalizeOutputFormats(raw string) []string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(raw)), ",")
	if len(parts) == 0 {
		return []string{"jpg", "gif"}
	}
	allow := map[string]struct{}{
		"jpg":  {},
		"jpeg": {},
		"png":  {},
		"gif":  {},
		"webp": {},
		"mp4":  {},
		"live": {},
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if p == "jpeg" {
			p = "jpg"
		}
		if _, ok := allow[p]; !ok {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		result = append(result, p)
	}
	if len(result) == 0 {
		return []string{"jpg", "gif"}
	}
	return result
}

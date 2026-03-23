package videojobs

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
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
	xdraw "golang.org/x/image/draw"
	"gorm.io/gorm"
)

const (
	defaultVideoJobTimeout    = 20 * time.Minute
	defaultHighlightTopN      = 3
	gifRenderSelectionVersion = "v1"
	gifSubStageBriefing       = "briefing"
	gifSubStagePlanning       = "planning"
	gifSubStageScoring        = "scoring"
	gifSubStageReviewing      = "reviewing"
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
		GIFGifsicleEnabled:                   row.GIFGifsicleEnabled,
		GIFGifsicleLevel:                     row.GIFGifsicleLevel,
		GIFGifsicleSkipBelowKB:               row.GIFGifsicleSkipBelowKB,
		GIFGifsicleMinGainRatio:              row.GIFGifsicleMinGainRatio,
		GIFLoopTuneEnabled:                   row.GIFLoopTuneEnabled,
		GIFLoopTuneMinEnableSec:              row.GIFLoopTuneMinEnableSec,
		GIFLoopTuneMinImprovement:            row.GIFLoopTuneMinImprovement,
		GIFLoopTuneMotionTarget:              row.GIFLoopTuneMotionTarget,
		GIFLoopTunePreferDuration:            row.GIFLoopTunePreferDuration,
		GIFCandidateMaxOutputs:               row.GIFCandidateMaxOutputs,
		GIFCandidateLongVideoMaxOutputs:      row.GIFCandidateLongVideoMaxOutputs,
		GIFCandidateUltraVideoMaxOutputs:     row.GIFCandidateUltraVideoMaxOutputs,
		GIFCandidateConfidenceThreshold:      row.GIFCandidateConfidenceThreshold,
		GIFCandidateDedupIOUThreshold:        row.GIFCandidateDedupIOUThreshold,
		GIFRenderBudgetNormalMultiplier:      row.GIFRenderBudgetNormalMultiplier,
		GIFRenderBudgetLongMultiplier:        row.GIFRenderBudgetLongMultiplier,
		GIFRenderBudgetUltraMultiplier:       row.GIFRenderBudgetUltraMultiplier,
		GIFPipelineShortVideoMaxSec:          row.GIFPipelineShortVideoMaxSec,
		GIFPipelineLongVideoMinSec:           row.GIFPipelineLongVideoMinSec,
		GIFPipelineShortVideoMode:            row.GIFPipelineShortVideoMode,
		GIFPipelineDefaultMode:               row.GIFPipelineDefaultMode,
		GIFPipelineLongVideoMode:             row.GIFPipelineLongVideoMode,
		GIFPipelineHighPriorityEnabled:       row.GIFPipelineHighPriorityEnabled,
		GIFPipelineHighPriorityMode:          row.GIFPipelineHighPriorityMode,
		GIFDurationTierMediumSec:             row.GIFDurationTierMediumSec,
		GIFDurationTierLongSec:               row.GIFDurationTierLongSec,
		GIFDurationTierUltraSec:              row.GIFDurationTierUltraSec,
		GIFSegmentTimeoutMinSec:              row.GIFSegmentTimeoutMinSec,
		GIFSegmentTimeoutMaxSec:              row.GIFSegmentTimeoutMaxSec,
		GIFSegmentTimeoutFallbackCapSec:      row.GIFSegmentTimeoutFallbackCapSec,
		GIFSegmentTimeoutEmergencyCapSec:     row.GIFSegmentTimeoutEmergencyCapSec,
		GIFSegmentTimeoutLastResortCapSec:    row.GIFSegmentTimeoutLastResortCapSec,
		GIFRenderRetryMaxAttempts:            row.GIFRenderRetryMaxAttempts,
		GIFRenderRetryPrimaryColorsFloor:     row.GIFRenderRetryPrimaryColorsFloor,
		GIFRenderRetryPrimaryColorsStep:      row.GIFRenderRetryPrimaryColorsStep,
		GIFRenderRetryFPSFloor:               row.GIFRenderRetryFPSFloor,
		GIFRenderRetryFPSStep:                row.GIFRenderRetryFPSStep,
		GIFRenderRetryWidthTrigger:           row.GIFRenderRetryWidthTrigger,
		GIFRenderRetryWidthScale:             row.GIFRenderRetryWidthScale,
		GIFRenderRetryWidthFloor:             row.GIFRenderRetryWidthFloor,
		GIFRenderRetrySecondaryColorsFloor:   row.GIFRenderRetrySecondaryColorsFloor,
		GIFRenderRetrySecondaryColorsStep:    row.GIFRenderRetrySecondaryColorsStep,
		GIFRenderInitialSizeFPSCap:           row.GIFRenderInitialSizeFPSCap,
		GIFRenderInitialClarityFPSFloor:      row.GIFRenderInitialClarityFPSFloor,
		GIFRenderInitialSizeColorsCap:        row.GIFRenderInitialSizeColorsCap,
		GIFRenderInitialClarityColorsFloor:   row.GIFRenderInitialClarityColorsFloor,
		GIFMotionLowScoreThreshold:           row.GIFMotionLowScoreThreshold,
		GIFMotionHighScoreThreshold:          row.GIFMotionHighScoreThreshold,
		GIFMotionLowFPSDelta:                 row.GIFMotionLowFPSDelta,
		GIFMotionHighFPSDelta:                row.GIFMotionHighFPSDelta,
		GIFAdaptiveFPSMin:                    row.GIFAdaptiveFPSMin,
		GIFAdaptiveFPSMax:                    row.GIFAdaptiveFPSMax,
		GIFWidthSizeLow:                      row.GIFWidthSizeLow,
		GIFWidthSizeMedium:                   row.GIFWidthSizeMedium,
		GIFWidthSizeHigh:                     row.GIFWidthSizeHigh,
		GIFWidthClarityLow:                   row.GIFWidthClarityLow,
		GIFWidthClarityMedium:                row.GIFWidthClarityMedium,
		GIFWidthClarityHigh:                  row.GIFWidthClarityHigh,
		GIFColorsSizeLow:                     row.GIFColorsSizeLow,
		GIFColorsSizeMedium:                  row.GIFColorsSizeMedium,
		GIFColorsSizeHigh:                    row.GIFColorsSizeHigh,
		GIFColorsClarityLow:                  row.GIFColorsClarityLow,
		GIFColorsClarityMedium:               row.GIFColorsClarityMedium,
		GIFColorsClarityHigh:                 row.GIFColorsClarityHigh,
		GIFDurationLowSec:                    row.GIFDurationLowSec,
		GIFDurationMediumSec:                 row.GIFDurationMediumSec,
		GIFDurationHighSec:                   row.GIFDurationHighSec,
		GIFDurationSizeProfileMaxSec:         row.GIFDurationSizeProfileMaxSec,
		GIFDownshiftHighResLongSideThreshold: row.GIFDownshiftHighResLongSideThreshold,
		GIFDownshiftEarlyDurationSec:         row.GIFDownshiftEarlyDurationSec,
		GIFDownshiftEarlyLongSideThreshold:   row.GIFDownshiftEarlyLongSideThreshold,
		GIFDownshiftMediumFPSCap:             row.GIFDownshiftMediumFPSCap,
		GIFDownshiftMediumWidthCap:           row.GIFDownshiftMediumWidthCap,
		GIFDownshiftMediumColorsCap:          row.GIFDownshiftMediumColorsCap,
		GIFDownshiftMediumDurationCapSec:     row.GIFDownshiftMediumDurationCapSec,
		GIFDownshiftLongFPSCap:               row.GIFDownshiftLongFPSCap,
		GIFDownshiftLongWidthCap:             row.GIFDownshiftLongWidthCap,
		GIFDownshiftLongColorsCap:            row.GIFDownshiftLongColorsCap,
		GIFDownshiftLongDurationCapSec:       row.GIFDownshiftLongDurationCapSec,
		GIFDownshiftUltraFPSCap:              row.GIFDownshiftUltraFPSCap,
		GIFDownshiftUltraWidthCap:            row.GIFDownshiftUltraWidthCap,
		GIFDownshiftUltraColorsCap:           row.GIFDownshiftUltraColorsCap,
		GIFDownshiftUltraDurationCapSec:      row.GIFDownshiftUltraDurationCapSec,
		GIFDownshiftHighResFPSCap:            row.GIFDownshiftHighResFPSCap,
		GIFDownshiftHighResWidthCap:          row.GIFDownshiftHighResWidthCap,
		GIFDownshiftHighResColorsCap:         row.GIFDownshiftHighResColorsCap,
		GIFDownshiftHighResDurationCapSec:    row.GIFDownshiftHighResDurationCapSec,
		GIFTimeoutFallbackFPSCap:             row.GIFTimeoutFallbackFPSCap,
		GIFTimeoutFallbackWidthCap:           row.GIFTimeoutFallbackWidthCap,
		GIFTimeoutFallbackColorsCap:          row.GIFTimeoutFallbackColorsCap,
		GIFTimeoutFallbackMinWidth:           row.GIFTimeoutFallbackMinWidth,
		GIFTimeoutFallbackUltraFPSCap:        row.GIFTimeoutFallbackUltraFPSCap,
		GIFTimeoutFallbackUltraWidthCap:      row.GIFTimeoutFallbackUltraWidthCap,
		GIFTimeoutFallbackUltraColorsCap:     row.GIFTimeoutFallbackUltraColorsCap,
		GIFTimeoutEmergencyFPSCap:            row.GIFTimeoutEmergencyFPSCap,
		GIFTimeoutEmergencyWidthCap:          row.GIFTimeoutEmergencyWidthCap,
		GIFTimeoutEmergencyColorsCap:         row.GIFTimeoutEmergencyColorsCap,
		GIFTimeoutEmergencyMinWidth:          row.GIFTimeoutEmergencyMinWidth,
		GIFTimeoutEmergencyDurationTrigger:   row.GIFTimeoutEmergencyDurationTrigger,
		GIFTimeoutEmergencyDurationScale:     row.GIFTimeoutEmergencyDurationScale,
		GIFTimeoutEmergencyDurationMinSec:    row.GIFTimeoutEmergencyDurationMinSec,
		GIFTimeoutLastResortFPSCap:           row.GIFTimeoutLastResortFPSCap,
		GIFTimeoutLastResortWidthCap:         row.GIFTimeoutLastResortWidthCap,
		GIFTimeoutLastResortColorsCap:        row.GIFTimeoutLastResortColorsCap,
		GIFTimeoutLastResortMinWidth:         row.GIFTimeoutLastResortMinWidth,
		GIFTimeoutLastResortDurationMinSec:   row.GIFTimeoutLastResortDurationMinSec,
		GIFTimeoutLastResortDurationMaxSec:   row.GIFTimeoutLastResortDurationMaxSec,
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
		AIDirectorInputMode:                  row.AIDirectorInputMode,
		AIDirectorOperatorInstruction:        row.AIDirectorOperatorInstruction,
		AIDirectorOperatorInstructionVersion: row.AIDirectorOperatorInstructionVersion,
		AIDirectorOperatorEnabled:            row.AIDirectorOperatorEnabled,
		AIDirectorConstraintOverrideEnabled:  row.AIDirectorConstraintOverrideEnabled,
		AIDirectorCountExpandRatio:           row.AIDirectorCountExpandRatio,
		AIDirectorDurationExpandRatio:        row.AIDirectorDurationExpandRatio,
		AIDirectorCountAbsoluteCap:           row.AIDirectorCountAbsoluteCap,
		AIDirectorDurationAbsoluteCapSec:     row.AIDirectorDurationAbsoluteCapSec,
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

	generatedFormats := make([]string, 0, 4)
	collectionID := *job.ResultCollectionID
	assetDomain := strings.ToLower(strings.TrimSpace(job.AssetDomain))
	if assetDomain == "" {
		assetDomain = models.VideoJobAssetDomainVideo
	}
	if assetDomain == models.VideoJobAssetDomainVideo {
		var collection models.VideoAssetCollection
		if err := p.db.Select("id").Where("id = ?", collectionID).First(&collection).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		if err := p.db.Model(&models.VideoAssetEmoji{}).
			Where("collection_id = ? AND status = ?", collection.ID, "active").
			Distinct("format").
			Pluck("format", &generatedFormats).Error; err != nil {
			return false, err
		}
	} else {
		var collection models.Collection
		if err := p.db.Select("id").Where("id = ?", collectionID).First(&collection).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		if err := p.db.Model(&models.Emoji{}).
			Where("collection_id = ? AND status = ?", collection.ID, "active").
			Distinct("format").
			Pluck("format", &generatedFormats).Error; err != nil {
			return false, err
		}
	}
	generatedFormats = normalizeFormatSlice(generatedFormats)
	if len(generatedFormats) == 0 {
		return false, nil
	}

	metrics := parseJSONMap(job.Metrics)
	metrics["result_collection_id"] = collectionID
	metrics["asset_domain"] = assetDomain
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
		"collection_id": collectionID,
		"asset_domain":  assetDomain,
		"formats":       generatedFormats,
	})
	p.syncJobCost(job.ID)
	p.syncJobPointSettlement(job.ID, models.VideoJobStatusDone)
	p.syncGIFBaseline(job.ID)
	p.cleanupSourceVideo(job.ID, "done")
	return true, nil
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

	sourceURL, err := p.buildObjectReadURL(cleanKey)
	if err != nil {
		return err
	}
	if err := p.downloadObject(ctx, sourceURL, outPath); err != nil {
		return fmt.Errorf("download object failed (http=%w)", err)
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
	defer func() {
		if r := recover(); r != nil {
			p.appendJobEvent(jobID, models.VideoJobStageUploading, "warn", "source video cleanup panic recovered", map[string]interface{}{
				"reason": strings.TrimSpace(reason),
				"panic":  fmt.Sprintf("%v", r),
			})
		}
	}()

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
			if n > maxGIFCandidateOutputs {
				n = maxGIFCandidateOutputs
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
	if maxWindows > maxGIFCandidateOutputs {
		maxWindows = maxGIFCandidateOutputs
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

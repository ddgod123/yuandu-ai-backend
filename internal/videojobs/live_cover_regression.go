package videojobs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type LiveCoverRegressionResult struct {
	DurationSec        float64 `json:"duration_sec"`
	SelectedTimestamp  float64 `json:"selected_timestamp_sec"`
	SelectedScore      float64 `json:"selected_score"`
	SelectedQuality    float64 `json:"selected_quality"`
	SelectedStability  float64 `json:"selected_stability"`
	SelectedTemporal   float64 `json:"selected_temporal"`
	SelectedPortrait   float64 `json:"selected_portrait"`
	SelectedExposure   float64 `json:"selected_exposure"`
	SelectedFace       float64 `json:"selected_face"`
	FirstFrameScore    float64 `json:"first_frame_score"`
	FirstFrameQuality  float64 `json:"first_frame_quality"`
	FirstFrameTemporal float64 `json:"first_frame_temporal"`
	FirstFramePortrait float64 `json:"first_frame_portrait"`
	FirstFrameExposure float64 `json:"first_frame_exposure"`
	FirstFrameFace     float64 `json:"first_frame_face"`
	ScoreDelta         float64 `json:"score_delta"`
}

func AnalyzeLiveCoverSelection(ctx context.Context, clipPath string, qualitySettings QualitySettings) (LiveCoverRegressionResult, error) {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	_, _, _, durationMs := readMediaOutputInfo(clipPath)
	durationSec := float64(durationMs) / 1000.0
	if durationSec <= 0 {
		durationSec = 3
	}
	weights := resolveLiveCoverScoringWeights(qualitySettings)

	tmpDir, err := os.MkdirTemp("", "live-cover-reg-*")
	if err != nil {
		return LiveCoverRegressionResult{}, err
	}
	defer os.RemoveAll(tmpDir)

	selectedPath := filepath.Join(tmpDir, "selected.jpg")
	selected, err := extractBestPosterFrame(ctx, clipPath, selectedPath, qualitySettings)
	if err != nil {
		return LiveCoverRegressionResult{}, err
	}

	firstPath := filepath.Join(tmpDir, "first.jpg")
	if err := extractFrameAtTimestamp(ctx, clipPath, 0, firstPath); err != nil {
		return LiveCoverRegressionResult{}, err
	}
	firstSample, ok := analyzeFrameQuality(firstPath)
	if !ok {
		return LiveCoverRegressionResult{}, errors.New("analyze first frame failed")
	}
	firstSample.Exposure = computeExposureScore(firstSample.Brightness, qualitySettings)
	firstQuality := computeFrameQualityScore(firstSample, maxFloat(qualitySettings.BlurThresholdMin, 8))
	firstTemporal := computePosterTemporalScore(0, durationSec)
	firstPortrait := estimatePortraitHintScore(firstPath)
	firstExposure := firstSample.Exposure
	firstFace := estimateFaceQualityHintScore(firstPath, qualitySettings)
	firstScore := firstQuality*weights.Quality +
		0.6*weights.Stability +
		firstTemporal*weights.Temporal +
		firstPortrait*weights.Portrait +
		firstExposure*weights.Exposure +
		firstFace*weights.Face

	return LiveCoverRegressionResult{
		DurationSec:        roundTo(durationSec, 3),
		SelectedTimestamp:  selected.TimestampSec,
		SelectedScore:      selected.FinalScore,
		SelectedQuality:    selected.QualityScore,
		SelectedStability:  selected.StabilityScore,
		SelectedTemporal:   selected.TemporalScore,
		SelectedPortrait:   selected.PortraitScore,
		SelectedExposure:   selected.ExposureScore,
		SelectedFace:       selected.FaceScore,
		FirstFrameScore:    roundTo(firstScore, 3),
		FirstFrameQuality:  roundTo(firstQuality, 3),
		FirstFrameTemporal: roundTo(firstTemporal, 3),
		FirstFramePortrait: roundTo(firstPortrait, 3),
		FirstFrameExposure: roundTo(firstExposure, 3),
		FirstFrameFace:     roundTo(firstFace, 3),
		ScoreDelta:         roundTo(selected.FinalScore-firstScore, 3),
	}, nil
}

func extractFrameAtTimestamp(ctx context.Context, clipPath string, timestampSec float64, outputPath string) error {
	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-ss", formatFFmpegNumber(timestampSec),
		"-i", clipPath,
		"-frames:v", "1",
		"-q:v", "2",
		outputPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("extract frame failed: %w: %s", err, string(out))
	}
	return nil
}

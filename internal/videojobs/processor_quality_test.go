package videojobs

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOptimizeFramePathsForQuality(t *testing.T) {
	tmpDir := t.TempDir()

	sharpPath := filepath.Join(tmpDir, "frame_0001.jpg")
	dupPath := filepath.Join(tmpDir, "frame_0002.jpg")
	brightPath := filepath.Join(tmpDir, "frame_0003.jpg")
	darkPath := filepath.Join(tmpDir, "frame_0004.jpg")
	flatPath := filepath.Join(tmpDir, "frame_0005.jpg")

	if err := writeJPEG(sharpPath, buildCheckerImage(128, 96)); err != nil {
		t.Fatalf("write sharp image: %v", err)
	}
	if err := writeJPEG(dupPath, buildCheckerImage(128, 96)); err != nil {
		t.Fatalf("write dup image: %v", err)
	}
	if err := writeJPEG(brightPath, buildSolidImage(128, 96, color.Gray{Y: 255})); err != nil {
		t.Fatalf("write bright image: %v", err)
	}
	if err := writeJPEG(darkPath, buildSolidImage(128, 96, color.Gray{Y: 0})); err != nil {
		t.Fatalf("write dark image: %v", err)
	}
	if err := writeJPEG(flatPath, buildSolidImage(128, 96, color.Gray{Y: 127})); err != nil {
		t.Fatalf("write flat image: %v", err)
	}

	selected, report := optimizeFramePathsForQuality([]string{
		sharpPath,
		dupPath,
		brightPath,
		darkPath,
		flatPath,
	}, 10, DefaultQualitySettings())

	if len(selected) == 0 {
		t.Fatalf("expected at least one frame kept")
	}
	if len(selected) > 2 {
		t.Fatalf("expected compact result set, got %d (%+v)", len(selected), selected)
	}
	if selected[0] != sharpPath {
		t.Fatalf("expected sharp frame kept at first position, got %s", selected[0])
	}
	if report.RejectedNearDuplicate == 0 {
		t.Fatalf("expected duplicate rejection, report=%+v", report)
	}
	if report.RejectedBrightness == 0 {
		t.Fatalf("expected brightness rejection, report=%+v", report)
	}
	if report.RejectedBlur == 0 && report.RejectedStillBlurGate == 0 {
		t.Fatalf("expected blur-related rejection, report=%+v", report)
	}
}

func TestPickFramePathSample(t *testing.T) {
	items := []string{"a", "b", "c", "d", "e", "f", "g"}
	sample := pickFramePathSample(items, 3)
	if len(sample) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(sample))
	}
	if sample[0] != "a" || sample[2] != "g" {
		t.Fatalf("expected first/last retained, got %+v", sample)
	}
}

func TestTrimFramePathsEvenly(t *testing.T) {
	paths := []string{
		"frame_0001.jpg",
		"frame_0002.jpg",
		"frame_0003.jpg",
		"frame_0004.jpg",
		"frame_0005.jpg",
		"frame_0006.jpg",
		"frame_0007.jpg",
		"frame_0008.jpg",
		"frame_0009.jpg",
		"frame_0010.jpg",
	}
	out := trimFramePathsEvenly(paths, 4)
	if len(out) != 4 {
		t.Fatalf("expected 4 paths, got %d", len(out))
	}
	if out[0] != "frame_0001.jpg" {
		t.Fatalf("expected first frame retained, got %s", out[0])
	}
	if out[len(out)-1] != "frame_0010.jpg" {
		t.Fatalf("expected last frame retained, got %s", out[len(out)-1])
	}
	// Even sampling should include middle timeline, not only earliest frames.
	if out[1] == "frame_0002.jpg" && out[2] == "frame_0003.jpg" {
		t.Fatalf("expected timeline spread, got %+v", out)
	}
}

func TestQualitySelectionCandidateBudget(t *testing.T) {
	if got := qualitySelectionCandidateBudget(24); got != 48 {
		t.Fatalf("expected candidate budget 48 for maxStatic=24, got %d", got)
	}
	if got := qualitySelectionCandidateBudget(1); got != 24 {
		t.Fatalf("expected min candidate budget 24, got %d", got)
	}
	if got := qualitySelectionCandidateBudget(120); got != 160 {
		t.Fatalf("expected capped budget 160, got %d", got)
	}
}

func TestSelectBestGIFLoopWindowFromSamples(t *testing.T) {
	window := highlightCandidate{
		StartSec: 10.0,
		EndSec:   13.0,
		Score:    0.7,
	}
	samples := []gifLoopSampleFrame{
		{TimestampSec: 10.0, Hash: 0x0000000000000000, QualityScore: 0.35},
		{TimestampSec: 10.5, Hash: 0x0F0F0F0F0F0F0F0F, QualityScore: 0.92},
		{TimestampSec: 11.0, Hash: 0xF0F0F0F0F0F0F0F0, QualityScore: 0.70},
		{TimestampSec: 11.5, Hash: 0xAAAAAAAAAAAAAAAA, QualityScore: 0.68},
		{TimestampSec: 12.0, Hash: 0x5555555555555555, QualityScore: 0.72},
		{TimestampSec: 12.5, Hash: 0x0F0F0F0F0F0F0F0F, QualityScore: 0.90},
		{TimestampSec: 13.0, Hash: 0xFFFFFFFFFFFFFFFF, QualityScore: 0.40},
	}
	tuned, result := selectBestGIFLoopWindowFromSamples(window, samples, DefaultQualitySettings())
	if !result.Applied {
		t.Fatalf("expected gif loop tuning applied, result=%+v", result)
	}
	if tuned.StartSec >= tuned.EndSec {
		t.Fatalf("expected valid tuned window, got start=%.3f end=%.3f", tuned.StartSec, tuned.EndSec)
	}
	if tuned.StartSec <= window.StartSec {
		t.Fatalf("expected tuned start moved forward, got %.3f", tuned.StartSec)
	}
	if tuned.EndSec >= window.EndSec {
		t.Fatalf("expected tuned end moved earlier, got %.3f", tuned.EndSec)
	}
	if result.Score <= 0 {
		t.Fatalf("expected positive score, got %.3f", result.Score)
	}
}

func TestSelectBestGIFLoopWindowFromSamples_NoApplyForShortWindow(t *testing.T) {
	window := highlightCandidate{
		StartSec: 0,
		EndSec:   1.1,
		Score:    0.6,
	}
	samples := []gifLoopSampleFrame{
		{TimestampSec: 0.0, Hash: 0x10, QualityScore: 0.6},
		{TimestampSec: 0.3, Hash: 0x11, QualityScore: 0.7},
		{TimestampSec: 0.6, Hash: 0x12, QualityScore: 0.7},
		{TimestampSec: 0.9, Hash: 0x13, QualityScore: 0.6},
	}
	tuned, result := selectBestGIFLoopWindowFromSamples(window, samples, DefaultQualitySettings())
	if result.Applied {
		t.Fatalf("expected no apply for short window, result=%+v", result)
	}
	if tuned.StartSec != window.StartSec || tuned.EndSec != window.EndSec {
		t.Fatalf("expected unchanged window, tuned=%+v", tuned)
	}
}

func TestSelectBestGIFLoopWindowFromSamples_RespectsMinImprovement(t *testing.T) {
	window := highlightCandidate{
		StartSec: 0.0,
		EndSec:   3.0,
		Score:    0.7,
	}
	samples := []gifLoopSampleFrame{
		{TimestampSec: 0.0, Hash: 0x0F0F0F0F0F0F0F0F, QualityScore: 0.92},
		{TimestampSec: 0.5, Hash: 0x0000000000000000, QualityScore: 0.73},
		{TimestampSec: 1.0, Hash: 0xFFFFFFFFFFFFFFFF, QualityScore: 0.75},
		{TimestampSec: 1.5, Hash: 0xAAAAAAAAAAAAAAAA, QualityScore: 0.74},
		{TimestampSec: 2.0, Hash: 0x5555555555555555, QualityScore: 0.73},
		{TimestampSec: 2.5, Hash: 0x3333333333333333, QualityScore: 0.74},
		{TimestampSec: 3.0, Hash: 0x0F0F0F0F0F0F0F0F, QualityScore: 0.90},
	}
	settings := DefaultQualitySettings()
	settings.GIFLoopTuneMinImprovement = 0.1
	tuned, result := selectBestGIFLoopWindowFromSamples(window, samples, settings)
	if result.Applied {
		t.Fatalf("expected tuning not applied when min improvement is high, result=%+v", result)
	}
	if tuned.StartSec != window.StartSec || tuned.EndSec != window.EndSec {
		t.Fatalf("expected unchanged window, tuned=%+v", tuned)
	}
}

func TestRankFrameCandidatesByScene(t *testing.T) {
	settings := DefaultQualitySettings()
	samples := []frameQualitySample{
		{Path: "scene1-best.jpg", Index: 0, QualityScore: 0.95, SceneID: 1, Hash: 0x0001},
		{Path: "scene1-dup.jpg", Index: 1, QualityScore: 0.90, SceneID: 1, Hash: 0x0002},
		{Path: "scene2-best.jpg", Index: 2, QualityScore: 0.88, SceneID: 2, Hash: 0xFF00},
		{Path: "scene3-best.jpg", Index: 3, QualityScore: 0.86, SceneID: 3, Hash: 0x00FF},
	}

	ranked := rankFrameCandidatesByScene(samples, 3, settings)
	if len(ranked) != 3 {
		t.Fatalf("expected 3 ranked frames, got %d", len(ranked))
	}
	hasScene2 := false
	hasScene3 := false
	for _, item := range ranked {
		if item.SceneID == 2 {
			hasScene2 = true
		}
		if item.SceneID == 3 {
			hasScene3 = true
		}
	}
	if !hasScene2 || !hasScene3 {
		t.Fatalf("expected scene diversity in ranked output, got %+v", ranked)
	}
}

func TestTuneAnimatedOptionsForWindow(t *testing.T) {
	settings := DefaultQualitySettings()

	lowWindow := highlightCandidate{
		StartSec: 0,
		EndSec:   3,
		Score:    1,
		Reason:   "single_window",
	}
	_, lowProfile := tuneAnimatedOptionsForWindow(videoProbeMeta{}, jobOptions{}, settings, "gif", lowWindow)
	if lowProfile.Level != "medium" {
		t.Fatalf("single_window should fall back to medium motion, got %s", lowProfile.Level)
	}

	highWindow := highlightCandidate{
		StartSec:   1,
		EndSec:     4,
		Score:      0.82,
		SceneScore: 0.76,
		Reason:     "scene_change_peak",
	}
	opts, highProfile := tuneAnimatedOptionsForWindow(videoProbeMeta{}, jobOptions{}, settings, "gif", highWindow)
	if highProfile.Level != "high" {
		t.Fatalf("expected high motion profile, got %s", highProfile.Level)
	}
	if opts.FPS < 14 {
		t.Fatalf("expected adaptive fps boost for high motion, got %d", opts.FPS)
	}
	if opts.MaxColors < 176 {
		t.Fatalf("expected adaptive color boost for high motion gif, got %d", opts.MaxColors)
	}
	if highProfile.DurationSec < 2.6 {
		t.Fatalf("expected longer duration for high motion clip, got %.2f", highProfile.DurationSec)
	}
}

func TestTuneAnimatedOptionsForWindow_CapsToSourceMeta(t *testing.T) {
	settings := DefaultQualitySettings()

	highWindow := highlightCandidate{
		StartSec:   1,
		EndSec:     4,
		Score:      0.9,
		SceneScore: 0.82,
		Reason:     "scene_change_peak",
	}
	meta := videoProbeMeta{
		Width: 640,
		FPS:   8,
	}

	opts, profile := tuneAnimatedOptionsForWindow(meta, jobOptions{}, settings, "gif", highWindow)
	if opts.FPS > 8 {
		t.Fatalf("expected fps capped by source fps=8, got %d", opts.FPS)
	}
	if profile.FPS != opts.FPS {
		t.Fatalf("expected profile fps sync with options, profile=%d opts=%d", profile.FPS, opts.FPS)
	}
	if opts.Width > 640 {
		t.Fatalf("expected width capped by source width=640, got %d", opts.Width)
	}
}

func TestTuneAnimatedOptionsForWindow_LongVideoDownshiftForGIF(t *testing.T) {
	settings := DefaultQualitySettings()
	meta := videoProbeMeta{
		DurationSec: 265,
		Width:       1280,
		FPS:         30,
	}
	window := highlightCandidate{
		StartSec:   10,
		EndSec:     13,
		Score:      0.88,
		SceneScore: 0.84,
		Reason:     "scene_change_peak",
	}

	opts, profile := tuneAnimatedOptionsForWindow(meta, jobOptions{}, settings, "gif", window)
	if opts.FPS > 8 {
		t.Fatalf("expected long-video fps downshift to <=8, got %d", opts.FPS)
	}
	if opts.Width > 640 {
		t.Fatalf("expected long-video width downshift to <=640, got %d", opts.Width)
	}
	if opts.MaxColors > 96 {
		t.Fatalf("expected long-video max colors downshift to <=96, got %d", opts.MaxColors)
	}
	if profile.DurationSec > 2.2 {
		t.Fatalf("expected long-video duration cap <=2.2s, got %.2f", profile.DurationSec)
	}
	if !profile.LongVideoDownshift {
		t.Fatalf("expected profile.LongVideoDownshift=true")
	}
	if profile.StabilityTier != "ultra_long" {
		t.Fatalf("expected stability tier ultra_long, got %s", profile.StabilityTier)
	}
}

func TestApplyGIFTimeoutFallbackProfile(t *testing.T) {
	opts := jobOptions{
		FPS:   16,
		Width: 1081,
	}
	next, colors, dither, changed := applyGIFTimeoutFallbackProfile(opts, 192, "sierra2_4a", 250)
	if !changed {
		t.Fatalf("expected timeout fallback to change render params")
	}
	if next.FPS != 8 {
		t.Fatalf("expected fallback fps=8, got %d", next.FPS)
	}
	if next.Width != 640 {
		t.Fatalf("expected fallback width=640, got %d", next.Width)
	}
	if colors != 64 {
		t.Fatalf("expected fallback max colors=64, got %d", colors)
	}
	if dither != "none" {
		t.Fatalf("expected fallback dither=none, got %s", dither)
	}
}

func TestChooseGIFSegmentRenderTimeout(t *testing.T) {
	short := chooseGIFSegmentRenderTimeout(
		videoProbeMeta{DurationSec: 20, Width: 720, FPS: 30},
		jobOptions{FPS: 10, Width: 720},
		highlightCandidate{StartSec: 0, EndSec: 2.4},
		96,
	)
	long := chooseGIFSegmentRenderTimeout(
		videoProbeMeta{DurationSec: 260, Width: 1280, FPS: 30},
		jobOptions{FPS: 12, Width: 1080},
		highlightCandidate{StartSec: 0, EndSec: 2.4},
		192,
	)
	if short < gifRenderTimeoutMin {
		t.Fatalf("expected short timeout >= min, got %s", short)
	}
	if long <= short {
		t.Fatalf("expected long timeout > short timeout, long=%s short=%s", long, short)
	}
	if long > gifRenderTimeoutMax {
		t.Fatalf("expected long timeout <= max, got %s", long)
	}
	if long < 60*time.Second {
		t.Fatalf("expected long timeout at least 60s, got %s", long)
	}
}

func TestTuneAnimatedOptionsForWindow_EnsuresEvenWidthForLive(t *testing.T) {
	settings := DefaultQualitySettings()
	meta := videoProbeMeta{
		Width: 721,
		FPS:   12,
	}
	opts, _ := tuneAnimatedOptionsForWindow(meta, jobOptions{Width: 721}, settings, "live", highlightCandidate{
		StartSec: 0,
		EndSec:   3,
		Score:    0.5,
	})
	if opts.Width%2 != 0 {
		t.Fatalf("expected live width to be even, got %d", opts.Width)
	}
}

func TestApplyHighlightFeedbackProfile(t *testing.T) {
	suggestion := highlightSuggestion{
		Version:  "v1",
		Strategy: "scene_score",
		Candidates: []highlightCandidate{
			{StartSec: 2.0, EndSec: 5.0, Score: 0.62, Reason: "fallback_uniform"},
			{StartSec: 7.5, EndSec: 10.5, Score: 0.58, Reason: "scene_change_peak"},
		},
	}
	suggestion.Selected = &suggestion.Candidates[0]

	profile := highlightFeedbackProfile{
		EngagedJobs:       6,
		WeightedSignals:   42,
		PreferredCenter:   0.75,
		PreferredDuration: 0.30,
		ReasonPreference: map[string]float64{
			"scene_change_peak": 0.8,
		},
	}

	settings := DefaultQualitySettings()
	reranked, applied := applyHighlightFeedbackProfile(suggestion, 12.0, profile, settings)
	if !applied {
		t.Fatalf("expected feedback rerank to apply")
	}
	if reranked.Selected == nil {
		t.Fatalf("expected selected candidate after rerank")
	}
	if reranked.Selected.StartSec != 7.5 || reranked.Selected.EndSec != 10.5 {
		t.Fatalf("expected second candidate to be selected after rerank, got %+v", reranked.Selected)
	}
	if reranked.Strategy != "scene_score+feedback_rerank" {
		t.Fatalf("unexpected strategy: %s", reranked.Strategy)
	}
}

func TestApplyHighlightFeedbackProfile_DislikeGuard(t *testing.T) {
	suggestion := highlightSuggestion{
		Version:  "v1",
		Strategy: "scene_score",
		Candidates: []highlightCandidate{
			{StartSec: 2.0, EndSec: 5.0, Score: 0.62, Reason: "fallback_uniform"},
			{StartSec: 7.5, EndSec: 10.5, Score: 0.72, Reason: "scene_change_peak"},
		},
	}
	suggestion.Selected = &suggestion.Candidates[1]

	profile := highlightFeedbackProfile{
		EngagedJobs:       8,
		WeightedSignals:   45,
		PreferredCenter:   0.65,
		PreferredDuration: 0.28,
		ReasonPreference: map[string]float64{
			"scene_change_peak": 0.7,
		},
		ReasonNegativeGuard: map[string]float64{
			"scene_change_peak": 1.0,
		},
	}

	settings := DefaultQualitySettings()
	reranked, applied := applyHighlightFeedbackProfile(suggestion, 12.0, profile, settings)
	if !applied {
		t.Fatalf("expected feedback rerank to apply")
	}
	if reranked.Selected == nil {
		t.Fatalf("expected selected candidate after rerank")
	}
	if reranked.Selected.Reason != "fallback_uniform" {
		t.Fatalf("expected dislike guard to avoid scene_change_peak, got %+v", reranked.Selected)
	}
}

func TestShouldApplyFeedbackRerank(t *testing.T) {
	settings := DefaultQualitySettings()
	settings.HighlightFeedbackRollout = 0
	if shouldApplyFeedbackRerank(123, settings) {
		t.Fatalf("expected rollout 0 to disable rerank")
	}

	settings.HighlightFeedbackRollout = 100
	if !shouldApplyFeedbackRerank(123, settings) {
		t.Fatalf("expected rollout 100 to always enable rerank")
	}
}

func TestApplyQualityProfileOverridesFromOptions(t *testing.T) {
	settings := DefaultQualitySettings()
	options := map[string]interface{}{
		"quality_profile_overrides": map[string]interface{}{
			"gif":  "size",
			"webp": "size",
			"jpg":  "clarity",
			"mp4":  "size",
		},
	}
	updated, applied := applyQualityProfileOverridesFromOptions(settings, options, []string{"gif", "webp", "jpg", "mp4"})
	if len(applied) == 0 {
		t.Fatalf("expected applied overrides")
	}
	if updated.GIFProfile != QualityProfileSize {
		t.Fatalf("expected gif profile size, got %s", updated.GIFProfile)
	}
	if updated.WebPProfile != QualityProfileSize {
		t.Fatalf("expected webp profile size, got %s", updated.WebPProfile)
	}
	if updated.JPGProfile != QualityProfileClarity {
		t.Fatalf("expected jpg profile clarity, got %s", updated.JPGProfile)
	}
	if updated.LiveProfile != QualityProfileSize {
		t.Fatalf("expected live profile size from mp4 override, got %s", updated.LiveProfile)
	}
}

func TestInferSceneTags(t *testing.T) {
	tags := inferSceneTags("探店搞笑狗狗Vlog", "uploads/reel.mp4", []string{"live", "gif", "png"})
	if len(tags) == 0 {
		t.Fatalf("expected tags, got empty")
	}

	expected := map[string]bool{
		"explore":      false,
		"funny":        false,
		"pet":          false,
		"social":       false,
		"live_creator": false,
		"design":       false,
	}
	for _, tag := range tags {
		if _, ok := expected[tag]; ok {
			expected[tag] = true
		}
	}
	for key, ok := range expected {
		if !ok {
			t.Fatalf("expected tag %q in %+v", key, tags)
		}
	}
}

func TestParseCropDetectRectLine(t *testing.T) {
	line := "[Parsed_cropdetect_0 @ 0x7f8f690a7000] x1:0 x2:1919 y1:140 y2:939 w:1920 h:800 x:0 y:140 pts:5120 t:0.200000 crop=1920:800:0:140"
	rect, ok := parseCropDetectRectLine(line)
	if !ok {
		t.Fatalf("expected parse success")
	}
	if rect.W != 1920 || rect.H != 800 || rect.X != 0 || rect.Y != 140 {
		t.Fatalf("unexpected rect: %+v", rect)
	}
}

func TestChooseCropDetectCandidate(t *testing.T) {
	meta := videoProbeMeta{
		Width:       1920,
		Height:      1080,
		DurationSec: 30,
	}
	output := `
[Parsed_cropdetect_0 @ 0x0] crop=1920:800:0:140
[Parsed_cropdetect_0 @ 0x0] crop=1920:800:0:140
[Parsed_cropdetect_0 @ 0x0] crop=1918:800:1:140
[Parsed_cropdetect_0 @ 0x0] crop=1920:800:0:140
`

	candidate, total, ok := chooseCropDetectCandidate(output, meta)
	if !ok {
		t.Fatalf("expected candidate")
	}
	if total != 4 {
		t.Fatalf("expected total matches 4, got %d", total)
	}
	if candidate.Count != 3 {
		t.Fatalf("expected best count 3, got %d", candidate.Count)
	}
	if candidate.Rect.W != 1920 || candidate.Rect.H != 800 || candidate.Rect.X != 0 || candidate.Rect.Y != 140 {
		t.Fatalf("unexpected candidate rect: %+v", candidate.Rect)
	}
}

func TestComputePosterTemporalScore(t *testing.T) {
	center := computePosterTemporalScore(1.5, 3.0)
	edge := computePosterTemporalScore(0.0, 3.0)
	if center <= edge {
		t.Fatalf("expected center score > edge score, got center=%.3f edge=%.3f", center, edge)
	}
	if center < 0.9 {
		t.Fatalf("expected center score close to 1, got %.3f", center)
	}
	if edge < 0.2 || edge > 0.3 {
		t.Fatalf("expected edge baseline around 0.2, got %.3f", edge)
	}
}

func TestComputePosterStabilityScore(t *testing.T) {
	stable := []liveCoverCandidate{
		{Sample: frameQualitySample{Hash: 0x0f0f0f0f0f0f0f0f}},
		{Sample: frameQualitySample{Hash: 0x0f0f0f0f0f0f0f0f}},
		{Sample: frameQualitySample{Hash: 0x0f0f0f0f0f0f0f0f}},
	}
	stableScore := computePosterStabilityScore(stable, 1)
	if stableScore < 0.9 {
		t.Fatalf("expected high stability score, got %.3f", stableScore)
	}

	unstable := []liveCoverCandidate{
		{Sample: frameQualitySample{Hash: 0x0000000000000000}},
		{Sample: frameQualitySample{Hash: 0xffffffffffffffff}},
		{Sample: frameQualitySample{Hash: 0x0000000000000000}},
	}
	unstableScore := computePosterStabilityScore(unstable, 1)
	if unstableScore > 0.2 {
		t.Fatalf("expected low stability score, got %.3f", unstableScore)
	}
	if stableScore <= unstableScore {
		t.Fatalf("expected stable score > unstable score, got stable=%.3f unstable=%.3f", stableScore, unstableScore)
	}
}

func TestEstimatePortraitHintFromImage(t *testing.T) {
	portraitLike := image.NewRGBA(image.Rect(0, 0, 120, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			portraitLike.SetRGBA(x, y, color.RGBA{R: 35, G: 35, B: 40, A: 255})
		}
	}
	for y := 30; y < 90; y++ {
		for x := 30; x < 90; x++ {
			portraitLike.SetRGBA(x, y, color.RGBA{R: 220, G: 168, B: 138, A: 255})
		}
	}

	landscapeLike := image.NewRGBA(image.Rect(0, 0, 120, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			landscapeLike.SetRGBA(x, y, color.RGBA{R: 70, G: 120, B: 170, A: 255})
		}
	}

	portraitScore := estimatePortraitHintFromImage(portraitLike)
	landscapeScore := estimatePortraitHintFromImage(landscapeLike)
	if portraitScore <= landscapeScore {
		t.Fatalf("expected portrait-like frame score > landscape score, got portrait=%.3f landscape=%.3f", portraitScore, landscapeScore)
	}
	if portraitScore < 0.25 {
		t.Fatalf("expected portrait-like frame to have non-trivial score, got %.3f", portraitScore)
	}
	if landscapeScore > 0.2 {
		t.Fatalf("expected non-portrait frame score to stay low, got %.3f", landscapeScore)
	}
}

func TestResolveLiveCoverScoringWeights(t *testing.T) {
	weights := resolveLiveCoverScoringWeights(QualitySettings{LiveCoverPortraitWeight: 0.08})
	if weights.Quality < 0.51 || weights.Quality > 0.53 {
		t.Fatalf("unexpected quality weight: %.3f", weights.Quality)
	}
	if weights.Portrait != 0.08 {
		t.Fatalf("unexpected portrait weight: %.3f", weights.Portrait)
	}
	if weights.Exposure != 0.08 {
		t.Fatalf("unexpected exposure weight: %.3f", weights.Exposure)
	}
	if weights.Face != 0.06 {
		t.Fatalf("unexpected face weight: %.3f", weights.Face)
	}
	total := weights.Quality + weights.Stability + weights.Temporal + weights.Portrait + weights.Exposure + weights.Face
	if total < 0.99 || total > 1.01 {
		t.Fatalf("expected total weight ~=1, got %.3f", total)
	}
}

func TestComputePosterExposureConsistency(t *testing.T) {
	settings := DefaultQualitySettings()
	median := 128.0

	near := computePosterExposureConsistency(frameQualitySample{
		Brightness: 130,
		Exposure:   0.95,
	}, median, settings)
	far := computePosterExposureConsistency(frameQualitySample{
		Brightness: 230,
		Exposure:   0.30,
	}, median, settings)
	if near <= far {
		t.Fatalf("expected near exposure score > far score, got near=%.3f far=%.3f", near, far)
	}
	if near < 0.8 {
		t.Fatalf("expected near exposure score high, got %.3f", near)
	}
	if far > 0.6 {
		t.Fatalf("expected far exposure score low, got %.3f", far)
	}
}

func TestEstimateFaceQualityHintFromImage(t *testing.T) {
	settings := DefaultQualitySettings()

	faceLike := image.NewRGBA(image.Rect(0, 0, 160, 160))
	for y := 0; y < 160; y++ {
		for x := 0; x < 160; x++ {
			faceLike.SetRGBA(x, y, color.RGBA{R: 30, G: 35, B: 42, A: 255})
		}
	}
	for y := 45; y < 130; y++ {
		for x := 50; x < 120; x++ {
			if ((x/4)+(y/4))%2 == 0 {
				faceLike.SetRGBA(x, y, color.RGBA{R: 218, G: 168, B: 138, A: 255})
			} else {
				faceLike.SetRGBA(x, y, color.RGBA{R: 198, G: 150, B: 122, A: 255})
			}
		}
	}

	nonFace := image.NewRGBA(image.Rect(0, 0, 160, 160))
	for y := 0; y < 160; y++ {
		for x := 0; x < 160; x++ {
			nonFace.SetRGBA(x, y, color.RGBA{R: 72, G: 120, B: 180, A: 255})
		}
	}

	faceScore := estimateFaceQualityHintFromImage(faceLike, settings)
	nonFaceScore := estimateFaceQualityHintFromImage(nonFace, settings)
	if faceScore <= nonFaceScore {
		t.Fatalf("expected face-like score > non-face score, got face=%.3f non-face=%.3f", faceScore, nonFaceScore)
	}
	if faceScore < 0.55 {
		t.Fatalf("expected face-like score >=0.55, got %.3f", faceScore)
	}
	if nonFaceScore > 0.5 {
		t.Fatalf("expected non-face score <=0.5, got %.3f", nonFaceScore)
	}
}

func writeJPEG(filePath string, img image.Image) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: 95})
}

func buildSolidImage(width, height int, c color.Gray) image.Image {
	img := image.NewGray(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetGray(x, y, c)
		}
	}
	return img
}

func buildCheckerImage(width, height int) image.Image {
	img := image.NewGray(image.Rect(0, 0, width, height))
	cell := 8
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if ((x/cell)+(y/cell))%2 == 0 {
				img.SetGray(x, y, color.Gray{Y: 40})
			} else {
				img.SetGray(x, y, color.Gray{Y: 220})
			}
		}
	}
	return img
}

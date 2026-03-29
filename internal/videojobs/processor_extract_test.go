package videojobs

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildExtractFrameArgs_Default(t *testing.T) {
	meta := videoProbeMeta{DurationSec: 12.3, Width: 640, Height: 360}
	opts := jobOptions{MaxStatic: 24, Speed: 1}

	args := buildExtractFrameArgs("/tmp/source.mp4", "/tmp/frames", meta, opts, 1.5, DefaultQualitySettings())
	joined := strings.Join(args, " ")

	if strings.Contains(joined, " -ss ") {
		t.Fatalf("unexpected -ss in args: %v", args)
	}
	if strings.Contains(joined, " -t ") {
		t.Fatalf("unexpected -t in args: %v", args)
	}
	if !containsArgPair(args, "-vf", "fps=1/1.5") {
		t.Fatalf("expected default fps filter, got: %v", args)
	}
	if !strings.HasSuffix(args[len(args)-1], "frame_%04d.jpg") {
		t.Fatalf("unexpected output pattern: %v", args[len(args)-1])
	}
}

func TestBuildExtractFrameArgs_WithEditOptions(t *testing.T) {
	meta := videoProbeMeta{DurationSec: 13.34, Width: 640, Height: 360}
	opts := jobOptions{
		StartSec: 2,
		EndSec:   7.5,
		Speed:    1.25,
		FPS:      12,
		Width:    320,
		CropX:    120,
		CropY:    40,
		CropW:    480,
		CropH:    480,
	}

	args := buildExtractFrameArgs("/tmp/source.mp4", "/tmp/frames", meta, opts, 1.5, DefaultQualitySettings())
	if !containsArgPair(args, "-ss", "2") {
		t.Fatalf("expected -ss 2, got: %v", args)
	}
	if !containsArgPair(args, "-t", "5.5") {
		t.Fatalf("expected -t 5.5, got: %v", args)
	}

	vf := argValue(args, "-vf")
	expectedVF := "setpts=PTS/1.2500,crop=480:320:120:40,fps=12,scale=320:-2"
	if vf != expectedVF {
		t.Fatalf("unexpected vf filter\nexpected: %s\nactual:   %s", expectedVF, vf)
	}
}

func TestBuildFrameFilters_AppliesStillEnhancementWhenClarityAndHighRes(t *testing.T) {
	meta := videoProbeMeta{Width: 1920, Height: 1080}
	opts := jobOptions{RequestedJPG: true}
	filters := buildFrameFilters(meta, opts, 1.5, DefaultQualitySettings())
	joined := strings.Join(filters, ",")
	if !strings.Contains(joined, "eq=contrast=1.02:saturation=1.03") {
		t.Fatalf("expected eq enhancement in filters, got %s", joined)
	}
	if !strings.Contains(joined, "unsharp=5:5:0.35:5:5:0.0") {
		t.Fatalf("expected unsharp enhancement in filters, got %s", joined)
	}
}

func TestBuildFrameFilters_SkipsStillEnhancementForSizeProfile(t *testing.T) {
	meta := videoProbeMeta{Width: 1920, Height: 1080}
	opts := jobOptions{RequestedJPG: true}
	settings := DefaultQualitySettings()
	settings.JPGProfile = QualityProfileSize
	settings.PNGProfile = QualityProfileSize
	filters := buildFrameFilters(meta, opts, 1.5, settings)
	joined := strings.Join(filters, ",")
	if strings.Contains(joined, "unsharp=") || strings.Contains(joined, "eq=contrast=") {
		t.Fatalf("expected no enhancement for size profile, got %s", joined)
	}
}

func TestBuildFrameFilters_UsesRequestedStillFormatsForClarityEnhancement(t *testing.T) {
	meta := videoProbeMeta{Width: 1920, Height: 1080}
	settings := DefaultQualitySettings()
	settings.JPGProfile = QualityProfileClarity
	settings.PNGProfile = QualityProfileSize

	jpgFilters := buildFrameFilters(meta, jobOptions{RequestedJPG: true}, 1.5, settings)
	if !strings.Contains(strings.Join(jpgFilters, ","), "unsharp=") {
		t.Fatalf("expected enhancement for requested JPG clarity profile")
	}

	pngFilters := buildFrameFilters(meta, jobOptions{RequestedPNG: true}, 1.5, settings)
	if strings.Contains(strings.Join(pngFilters, ","), "unsharp=") {
		t.Fatalf("expected no enhancement for requested PNG size profile")
	}
}

func TestBuildFrameFilters_AppliesRiskStrategyFilters(t *testing.T) {
	meta := videoProbeMeta{Width: 1280, Height: 720}
	opts := jobOptions{
		RequestedPNG:   true,
		RiskLowLight:   true,
		RiskFastMotion: true,
		AIAvoidDark:    true,
	}
	filters := buildFrameFilters(meta, opts, 1.0, DefaultQualitySettings())
	joined := strings.Join(filters, ",")
	if !strings.Contains(joined, "hqdn3d=") {
		t.Fatalf("expected low_light denoise filter, got %s", joined)
	}
	if !strings.Contains(joined, "unsharp=") {
		t.Fatalf("expected fast_motion sharpen filter, got %s", joined)
	}
}

func TestApplyStillProfileDefaults_DoesNotForceGlobalScaleWhenMixedProfiles(t *testing.T) {
	settings := DefaultQualitySettings()
	settings.JPGProfile = QualityProfileClarity
	settings.PNGProfile = QualityProfileSize

	opts := applyStillProfileDefaults(jobOptions{}, []string{"jpg", "png"}, settings)
	if opts.Width != 0 {
		t.Fatalf("expected width unchanged for mixed still profiles, got %d", opts.Width)
	}
}

func TestApplyStillProfileDefaults_AppliesGlobalScaleWhenAllSize(t *testing.T) {
	settings := DefaultQualitySettings()
	settings.JPGProfile = QualityProfileSize
	settings.PNGProfile = QualityProfileSize

	opts := applyStillProfileDefaults(jobOptions{}, []string{"jpg", "png"}, settings)
	if opts.Width != 1280 {
		t.Fatalf("expected width=1280 for all-size still profiles, got %d", opts.Width)
	}
}

func TestBuildExtractFrameArgs_UsesRequestedStillProfilesForJpegQuality(t *testing.T) {
	meta := videoProbeMeta{DurationSec: 8, Width: 1920, Height: 1080}
	settings := DefaultQualitySettings()
	settings.JPGProfile = QualityProfileClarity
	settings.PNGProfile = QualityProfileSize

	jpgArgs := buildExtractFrameArgs("/tmp/source.mp4", "/tmp/frames", meta, jobOptions{RequestedJPG: true}, 1.5, settings)
	if !containsArgPair(jpgArgs, "-q:v", "2") {
		t.Fatalf("expected jpeg quality q=2 for requested JPG clarity, got %v", jpgArgs)
	}

	pngArgs := buildExtractFrameArgs("/tmp/source.mp4", "/tmp/frames", meta, jobOptions{RequestedPNG: true}, 1.5, settings)
	if !containsArgPair(pngArgs, "-q:v", "5") {
		t.Fatalf("expected jpeg quality q=5 for requested PNG size, got %v", pngArgs)
	}
}

func TestBuildExtractFrameArgs_UsesLosslessPNGForPNGClarityOnly(t *testing.T) {
	meta := videoProbeMeta{DurationSec: 8, Width: 1920, Height: 1080}
	settings := DefaultQualitySettings()
	settings.JPGProfile = QualityProfileSize
	settings.PNGProfile = QualityProfileClarity

	args := buildExtractFrameArgs("/tmp/source.mp4", "/tmp/frames", meta, jobOptions{RequestedPNG: true}, 1.5, settings)
	if containsArgPair(args, "-q:v", "2") || containsArgPair(args, "-q:v", "3") || containsArgPair(args, "-q:v", "5") {
		t.Fatalf("expected no jpeg quality args for PNG clarity extract, got %v", args)
	}
	if !containsArgPair(args, "-vcodec", "png") {
		t.Fatalf("expected png codec for PNG clarity extract, got %v", args)
	}
	if !strings.HasSuffix(args[len(args)-1], "frame_%04d.png") {
		t.Fatalf("unexpected output pattern: %v", args[len(args)-1])
	}
}

func TestCollectFramePaths_SupportsPNGFrames(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"frame_0001.png",
		"frame_0002.png",
		"frame_0003.jpg",
		"ignore.txt",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write file %s: %v", name, err)
		}
	}

	got, err := collectFramePaths(dir, 10)
	if err != nil {
		t.Fatalf("collectFramePaths error: %v", err)
	}
	wantSuffixes := []string{"frame_0001.png", "frame_0002.png", "frame_0003.jpg"}
	if len(got) != len(wantSuffixes) {
		t.Fatalf("unexpected count: got=%d paths=%v", len(got), got)
	}
	for idx, suffix := range wantSuffixes {
		if !strings.HasSuffix(got[idx], suffix) {
			t.Fatalf("unexpected order at %d: got=%s want suffix=%s", idx, got[idx], suffix)
		}
	}
}

func TestBuildCropFilter_ClampsOutOfRange(t *testing.T) {
	meta := videoProbeMeta{Width: 640, Height: 360}
	opts := jobOptions{CropX: 700, CropY: 10, CropW: 700, CropH: 500}

	got, ok := buildCropFilter(meta, opts)
	if !ok {
		t.Fatalf("expected crop filter")
	}
	want := "crop=640:350:0:10"
	if got != want {
		t.Fatalf("unexpected crop filter, want=%s got=%s", want, got)
	}
}

func TestResolveClipWindow(t *testing.T) {
	meta := videoProbeMeta{DurationSec: 8}
	start, dur := resolveClipWindow(meta, jobOptions{StartSec: 99, EndSec: 3})
	if !reflect.DeepEqual([]float64{start, dur}, []float64{0, 3}) {
		t.Fatalf("unexpected clip window, start=%v dur=%v", start, dur)
	}
}

func TestEffectiveSampleDuration_PrefersClipDuration(t *testing.T) {
	meta := videoProbeMeta{DurationSec: 12}
	got := effectiveSampleDuration(meta, jobOptions{StartSec: 2, EndSec: 6.5})
	if got != 4.5 {
		t.Fatalf("unexpected effective duration: %v", got)
	}
}

func TestChooseFrameInterval_AllowsShortClipDensity(t *testing.T) {
	got := chooseFrameInterval(3.2, 0, 24)
	if got >= 0.3 {
		t.Fatalf("expected dense interval for short clip, got %v", got)
	}
}

func containsArgPair(args []string, key, val string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key && args[i+1] == val {
			return true
		}
	}
	return false
}

func argValue(args []string, key string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"emoji/internal/videojobs"
)

type videoResult struct {
	File   string                            `json:"file"`
	Result videojobs.GIFLoopRegressionResult `json:"result"`
	Error  string                            `json:"error,omitempty"`
}

type regressionReport struct {
	GeneratedAt         string        `json:"generated_at"`
	InputDir            string        `json:"input_dir"`
	Files               int           `json:"files"`
	Succeeded           int           `json:"succeeded"`
	Failed              int           `json:"failed"`
	TunedApplied        int           `json:"tuned_applied"`
	EffectiveApplied    int           `json:"effective_applied"`
	FallbackToBase      int           `json:"fallback_to_base"`
	RenderFailed        int           `json:"render_failed"`
	AvgBaseLoopClosure  float64       `json:"avg_base_loop_closure"`
	AvgFinalLoopClosure float64       `json:"avg_final_loop_closure"`
	AvgLoopClosureDelta float64       `json:"avg_loop_closure_delta"`
	AvgBaseScore        float64       `json:"avg_base_score"`
	AvgFinalScore       float64       `json:"avg_final_score"`
	AvgScoreDelta       float64       `json:"avg_score_delta"`
	AvgBaseRenderSec    float64       `json:"avg_base_render_sec"`
	AvgFinalRenderSec   float64       `json:"avg_final_render_sec"`
	AvgFinalSizeBytes   float64       `json:"avg_final_size_bytes"`
	ElapsedSec          float64       `json:"elapsed_sec"`
	Results             []videoResult `json:"results"`
}

func main() {
	inputDir := flag.String("input", "", "input directory containing test videos")
	outJSON := flag.String("out-json", "", "optional output report JSON path")
	timeoutSec := flag.Int("timeout-sec", 120, "timeout per video in seconds")
	preferWindowSec := flag.Float64("prefer-window-sec", 3.0, "preferred gif clip duration in seconds")
	maxFiles := flag.Int("max-files", 0, "max files to run, 0 means all")
	useHighlight := flag.Bool("use-highlight", true, "use highlight scorer to choose base window")
	render := flag.Bool("render", true, "render base/final gif and collect render metrics")
	gifProfile := flag.String("gif-profile", "clarity", "gif quality profile: clarity | size")
	flag.Parse()

	if strings.TrimSpace(*inputDir) == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --input is required")
		os.Exit(1)
	}

	files, err := scanVideoFiles(*inputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: scan input failed: %v\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "ERROR: no video files found")
		os.Exit(1)
	}
	if *maxFiles > 0 && len(files) > *maxFiles {
		files = files[:*maxFiles]
	}

	settings := videojobs.DefaultQualitySettings()
	switch strings.ToLower(strings.TrimSpace(*gifProfile)) {
	case videojobs.QualityProfileSize:
		settings.GIFProfile = videojobs.QualityProfileSize
	default:
		settings.GIFProfile = videojobs.QualityProfileClarity
	}
	settings = videojobs.NormalizeQualitySettings(settings)

	opts := videojobs.GIFLoopRegressionOptions{
		PreferWindowSec: *preferWindowSec,
		UseHighlight:    *useHighlight,
		RenderOutputs:   *render,
	}

	started := time.Now()
	results := make([]videoResult, 0, len(files))
	success := 0
	failed := 0
	tunedApplied := 0
	effectiveApplied := 0
	fallbackUsed := 0
	renderFailed := 0
	var sumBaseLoop, sumFinalLoop, sumDeltaLoop float64
	var sumBaseScore, sumFinalScore, sumDeltaScore float64
	var sumBaseRenderSec, sumFinalRenderSec, sumFinalSize float64

	for _, filePath := range files {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSec)*time.Second)
		res, analyzeErr := videojobs.AnalyzeGIFLoopRegression(ctx, filePath, settings, opts)
		cancel()

		item := videoResult{File: filePath}
		if analyzeErr != nil {
			item.Error = analyzeErr.Error()
			results = append(results, item)
			failed++
			continue
		}
		item.Result = res
		results = append(results, item)
		success++

		if res.TuneApplied {
			tunedApplied++
		}
		if res.EffectiveApplied {
			effectiveApplied++
		}
		if res.FallbackToBase {
			fallbackUsed++
		}
		if opts.RenderOutputs && !res.FinalRender.Success {
			renderFailed++
		}

		sumBaseLoop += res.BaseScore.LoopClosure
		sumFinalLoop += res.FinalScore.LoopClosure
		sumDeltaLoop += res.LoopClosureDelta
		sumBaseScore += res.BaseScore.Score
		sumFinalScore += res.FinalScore.Score
		sumDeltaScore += res.ScoreDelta
		sumBaseRenderSec += res.BaseRender.ElapsedSec
		sumFinalRenderSec += res.FinalRender.ElapsedSec
		sumFinalSize += float64(res.FinalRender.SizeBytes)
	}

	report := regressionReport{
		GeneratedAt:      time.Now().Format(time.RFC3339),
		InputDir:         *inputDir,
		Files:            len(files),
		Succeeded:        success,
		Failed:           failed,
		TunedApplied:     tunedApplied,
		EffectiveApplied: effectiveApplied,
		FallbackToBase:   fallbackUsed,
		RenderFailed:     renderFailed,
		ElapsedSec:       roundTo(time.Since(started).Seconds(), 3),
		Results:          results,
	}
	if success > 0 {
		denom := float64(success)
		report.AvgBaseLoopClosure = roundTo(sumBaseLoop/denom, 4)
		report.AvgFinalLoopClosure = roundTo(sumFinalLoop/denom, 4)
		report.AvgLoopClosureDelta = roundTo(sumDeltaLoop/denom, 4)
		report.AvgBaseScore = roundTo(sumBaseScore/denom, 4)
		report.AvgFinalScore = roundTo(sumFinalScore/denom, 4)
		report.AvgScoreDelta = roundTo(sumDeltaScore/denom, 4)
		report.AvgBaseRenderSec = roundTo(sumBaseRenderSec/denom, 4)
		report.AvgFinalRenderSec = roundTo(sumFinalRenderSec/denom, 4)
		report.AvgFinalSizeBytes = roundTo(sumFinalSize/denom, 1)
	}

	fmt.Printf("GIF loop regression | files=%d success=%d failed=%d\n", report.Files, report.Succeeded, report.Failed)
	fmt.Printf("tuned_applied=%d effective=%d fallback=%d render_failed=%d\n", report.TunedApplied, report.EffectiveApplied, report.FallbackToBase, report.RenderFailed)
	fmt.Printf("loop_closure base=%.3f final=%.3f delta=%.3f\n", report.AvgBaseLoopClosure, report.AvgFinalLoopClosure, report.AvgLoopClosureDelta)
	fmt.Printf("score        base=%.3f final=%.3f delta=%.3f\n", report.AvgBaseScore, report.AvgFinalScore, report.AvgScoreDelta)
	if opts.RenderOutputs {
		fmt.Printf("render_sec   base=%.3f final=%.3f avg_final_size=%.0fB\n", report.AvgBaseRenderSec, report.AvgFinalRenderSec, report.AvgFinalSizeBytes)
	}
	fmt.Println("")
	fmt.Printf("%-40s %-8s %-8s %-8s %-8s %-8s\n", "video", "applied", "effect", "fallback", "loopΔ", "scoreΔ")
	for _, item := range report.Results {
		name := trimString(filepath.Base(item.File), 40)
		if item.Error != "" {
			fmt.Printf("%-40s %-8s %-8s %-8s %-8s %-8s\n", name, "ERR", "-", "-", "-", "-")
			continue
		}
		fmt.Printf(
			"%-40s %-8v %-8v %-8v %-8.3f %-8.3f\n",
			name,
			item.Result.TuneApplied,
			item.Result.EffectiveApplied,
			item.Result.FallbackToBase,
			item.Result.LoopClosureDelta,
			item.Result.ScoreDelta,
		)
	}

	if strings.TrimSpace(*outJSON) != "" {
		if err := writeJSON(*outJSON, report); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: write json failed: %v\n", err)
			os.Exit(2)
		}
		fmt.Printf("\nreport saved: %s\n", *outJSON)
	}

	if report.Succeeded == 0 {
		os.Exit(3)
	}
}

func scanVideoFiles(root string) ([]string, error) {
	extAllow := map[string]struct{}{
		".mp4":  {},
		".mov":  {},
		".m4v":  {},
		".webm": {},
		".mkv":  {},
		".avi":  {},
		".flv":  {},
		".ts":   {},
		".mpeg": {},
		".mpg":  {},
	}
	out := make([]string, 0, 128)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := extAllow[ext]; !ok {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

func writeJSON(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func trimString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func roundTo(v float64, digits int) float64 {
	if digits <= 0 {
		return float64(int(v + 0.5))
	}
	base := 1.0
	for i := 0; i < digits; i++ {
		base *= 10
	}
	if v >= 0 {
		return float64(int(v*base+0.5)) / base
	}
	return float64(int(v*base-0.5)) / base
}

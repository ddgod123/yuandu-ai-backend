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
	File   string                              `json:"file"`
	Result videojobs.LiveCoverRegressionResult `json:"result"`
	Error  string                              `json:"error,omitempty"`
}

type regressionReport struct {
	GeneratedAt        string        `json:"generated_at"`
	InputDir           string        `json:"input_dir"`
	Files              int           `json:"files"`
	Succeeded          int           `json:"succeeded"`
	Failed             int           `json:"failed"`
	PortraitWeight     float64       `json:"portrait_weight"`
	AvgSelectedScore   float64       `json:"avg_selected_score"`
	AvgFirstFrameScore float64       `json:"avg_first_frame_score"`
	AvgDelta           float64       `json:"avg_delta"`
	ImprovedCount      int           `json:"improved_count"`
	ImprovedRate       float64       `json:"improved_rate"`
	ElapsedSec         float64       `json:"elapsed_sec"`
	Results            []videoResult `json:"results"`
}

func main() {
	inputDir := flag.String("input", "", "input directory containing test videos")
	outJSON := flag.String("out-json", "", "optional output json path")
	timeoutSec := flag.Int("timeout-sec", 90, "timeout per video analysis in seconds")
	portraitWeight := flag.Float64("portrait-weight", -1, "override live cover portrait weight (0~0.25), default uses quality default")
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

	settings := videojobs.DefaultQualitySettings()
	if *portraitWeight >= 0 {
		settings.LiveCoverPortraitWeight = *portraitWeight
	}
	settings = videojobs.NormalizeQualitySettings(settings)

	started := time.Now()
	results := make([]videoResult, 0, len(files))
	successCount := 0
	improvedCount := 0
	sumSelected := 0.0
	sumFirst := 0.0
	sumDelta := 0.0

	for _, filePath := range files {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSec)*time.Second)
		res, analyzeErr := videojobs.AnalyzeLiveCoverSelection(ctx, filePath, settings)
		cancel()

		item := videoResult{
			File: filePath,
		}
		if analyzeErr != nil {
			item.Error = analyzeErr.Error()
			results = append(results, item)
			continue
		}
		item.Result = res
		results = append(results, item)

		successCount++
		sumSelected += res.SelectedScore
		sumFirst += res.FirstFrameScore
		sumDelta += res.ScoreDelta
		if res.ScoreDelta > 0 {
			improvedCount++
		}
	}

	failed := len(files) - successCount
	avgSelected := 0.0
	avgFirst := 0.0
	avgDelta := 0.0
	improvedRate := 0.0
	if successCount > 0 {
		denom := float64(successCount)
		avgSelected = sumSelected / denom
		avgFirst = sumFirst / denom
		avgDelta = sumDelta / denom
		improvedRate = float64(improvedCount) / denom
	}

	report := regressionReport{
		GeneratedAt:        time.Now().Format(time.RFC3339),
		InputDir:           *inputDir,
		Files:              len(files),
		Succeeded:          successCount,
		Failed:             failed,
		PortraitWeight:     settings.LiveCoverPortraitWeight,
		AvgSelectedScore:   roundTo(avgSelected, 3),
		AvgFirstFrameScore: roundTo(avgFirst, 3),
		AvgDelta:           roundTo(avgDelta, 3),
		ImprovedCount:      improvedCount,
		ImprovedRate:       roundTo(improvedRate, 4),
		ElapsedSec:         roundTo(time.Since(started).Seconds(), 3),
		Results:            results,
	}

	fmt.Printf("Live cover regression | files=%d success=%d failed=%d portrait_weight=%.3f\n", report.Files, report.Succeeded, report.Failed, report.PortraitWeight)
	fmt.Printf("avg_selected=%.3f avg_first=%.3f avg_delta=%.3f improved=%d (%.1f%%)\n", report.AvgSelectedScore, report.AvgFirstFrameScore, report.AvgDelta, report.ImprovedCount, report.ImprovedRate*100)
	fmt.Println("")
	fmt.Printf("%-44s %-8s %-8s %-8s %-8s\n", "video", "selected", "first", "delta", "ts")
	for _, item := range report.Results {
		name := filepath.Base(item.File)
		if item.Error != "" {
			fmt.Printf("%-44s %-8s %-8s %-8s %-8s\n", trimString(name, 44), "ERR", "-", "-", "-")
			continue
		}
		fmt.Printf(
			"%-44s %-8.3f %-8.3f %-8.3f %-8.2f\n",
			trimString(name, 44),
			item.Result.SelectedScore,
			item.Result.FirstFrameScore,
			item.Result.ScoreDelta,
			item.Result.SelectedTimestamp,
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

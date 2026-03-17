package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type regressionReport struct {
	GeneratedAt         string  `json:"generated_at"`
	Files               int     `json:"files"`
	Succeeded           int     `json:"succeeded"`
	Failed              int     `json:"failed"`
	TunedApplied        int     `json:"tuned_applied"`
	EffectiveApplied    int     `json:"effective_applied"`
	FallbackToBase      int     `json:"fallback_to_base"`
	RenderFailed        int     `json:"render_failed"`
	AvgFinalLoopClosure float64 `json:"avg_final_loop_closure"`
	AvgScoreDelta       float64 `json:"avg_score_delta"`
}

type snapshot struct {
	Succeeded           int     `json:"succeeded"`
	EffectiveRate       float64 `json:"effective_rate"`
	FallbackRate        float64 `json:"fallback_rate"`
	RenderFailedRate    float64 `json:"render_failed_rate"`
	AvgFinalLoopClosure float64 `json:"avg_final_loop_closure"`
	AvgScoreDelta       float64 `json:"avg_score_delta"`
}

type diffDecision struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

type diffReport struct {
	GeneratedAt string       `json:"generated_at"`
	BaseFile    string       `json:"base_file"`
	TargetFile  string       `json:"target_file"`
	Base        snapshot     `json:"base"`
	Target      snapshot     `json:"target"`
	Delta       snapshot     `json:"delta"`
	Decision    diffDecision `json:"decision"`
}

func main() {
	basePath := flag.String("base", "", "base regression JSON path")
	targetPath := flag.String("target", "", "target regression JSON path")
	outJSON := flag.String("out-json", "", "optional output diff JSON path")
	minSamples := flag.Int("min-samples", 10, "minimum succeeded samples to make rollout decision")
	flag.Parse()

	if strings.TrimSpace(*basePath) == "" || strings.TrimSpace(*targetPath) == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --base and --target are required")
		os.Exit(1)
	}

	baseReport, err := loadReport(*basePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: load base report failed: %v\n", err)
		os.Exit(1)
	}
	targetReport, err := loadReport(*targetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: load target report failed: %v\n", err)
		os.Exit(1)
	}

	baseSnap := toSnapshot(baseReport)
	targetSnap := toSnapshot(targetReport)
	delta := snapshot{
		Succeeded:           targetSnap.Succeeded - baseSnap.Succeeded,
		EffectiveRate:       roundTo(targetSnap.EffectiveRate-baseSnap.EffectiveRate, 4),
		FallbackRate:        roundTo(targetSnap.FallbackRate-baseSnap.FallbackRate, 4),
		RenderFailedRate:    roundTo(targetSnap.RenderFailedRate-baseSnap.RenderFailedRate, 4),
		AvgFinalLoopClosure: roundTo(targetSnap.AvgFinalLoopClosure-baseSnap.AvgFinalLoopClosure, 4),
		AvgScoreDelta:       roundTo(targetSnap.AvgScoreDelta-baseSnap.AvgScoreDelta, 4),
	}
	decision := decideRollout(baseSnap, targetSnap, *minSamples)

	report := diffReport{
		GeneratedAt: time.Now().Format(time.RFC3339),
		BaseFile:    filepath.Clean(*basePath),
		TargetFile:  filepath.Clean(*targetPath),
		Base:        baseSnap,
		Target:      targetSnap,
		Delta:       delta,
		Decision:    decision,
	}

	fmt.Printf("GIF regression diff | base=%s target=%s\n", report.BaseFile, report.TargetFile)
	fmt.Printf("effective_rate: %.2f%% -> %.2f%% (Δ %.2f%%)\n", baseSnap.EffectiveRate*100, targetSnap.EffectiveRate*100, delta.EffectiveRate*100)
	fmt.Printf("fallback_rate : %.2f%% -> %.2f%% (Δ %.2f%%)\n", baseSnap.FallbackRate*100, targetSnap.FallbackRate*100, delta.FallbackRate*100)
	fmt.Printf("render_failed : %.2f%% -> %.2f%% (Δ %.2f%%)\n", baseSnap.RenderFailedRate*100, targetSnap.RenderFailedRate*100, delta.RenderFailedRate*100)
	fmt.Printf("loop_closure  : %.4f -> %.4f (Δ %.4f)\n", baseSnap.AvgFinalLoopClosure, targetSnap.AvgFinalLoopClosure, delta.AvgFinalLoopClosure)
	fmt.Printf("score_delta   : %.4f -> %.4f (Δ %.4f)\n", baseSnap.AvgScoreDelta, targetSnap.AvgScoreDelta, delta.AvgScoreDelta)
	fmt.Printf("decision      : %s (%s)\n", decision.State, decision.Reason)

	if strings.TrimSpace(*outJSON) != "" {
		if err := writeJSON(*outJSON, report); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: write json failed: %v\n", err)
			os.Exit(2)
		}
		fmt.Printf("diff report saved: %s\n", *outJSON)
	}
}

func loadReport(path string) (regressionReport, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return regressionReport{}, err
	}
	var report regressionReport
	if err := json.Unmarshal(content, &report); err != nil {
		return regressionReport{}, err
	}
	if report.Files <= 0 {
		return regressionReport{}, errors.New("invalid report: files must be > 0")
	}
	return report, nil
}

func toSnapshot(report regressionReport) snapshot {
	succeeded := report.Succeeded
	if succeeded < 0 {
		succeeded = 0
	}
	denom := float64(maxInt(succeeded, 1))
	return snapshot{
		Succeeded:           succeeded,
		EffectiveRate:       roundTo(float64(report.EffectiveApplied)/denom, 4),
		FallbackRate:        roundTo(float64(report.FallbackToBase)/denom, 4),
		RenderFailedRate:    roundTo(float64(report.RenderFailed)/denom, 4),
		AvgFinalLoopClosure: roundTo(report.AvgFinalLoopClosure, 4),
		AvgScoreDelta:       roundTo(report.AvgScoreDelta, 4),
	}
}

func decideRollout(base, target snapshot, minSamples int) diffDecision {
	if target.Succeeded < minSamples || base.Succeeded < minSamples {
		return diffDecision{State: "insufficient_data", Reason: "样本不足，建议继续采样"}
	}

	if target.RenderFailedRate > base.RenderFailedRate+0.02 && target.RenderFailedRate > 0.05 {
		return diffDecision{State: "scale_down", Reason: "渲染失败率明显上升"}
	}
	if target.FallbackRate > base.FallbackRate+0.05 || target.FallbackRate > 0.20 {
		return diffDecision{State: "scale_down", Reason: "回退率升高，稳定性风险增加"}
	}
	if target.AvgFinalLoopClosure < base.AvgFinalLoopClosure-0.03 {
		return diffDecision{State: "scale_down", Reason: "首尾闭合度下降明显"}
	}

	if target.EffectiveRate >= base.EffectiveRate+0.05 &&
		target.AvgFinalLoopClosure >= base.AvgFinalLoopClosure+0.02 &&
		target.RenderFailedRate <= base.RenderFailedRate+0.01 {
		return diffDecision{State: "scale_up", Reason: "生效率与闭合度提升且稳定性可控"}
	}

	return diffDecision{State: "hold", Reason: "指标变化有限，建议保持当前配置"}
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

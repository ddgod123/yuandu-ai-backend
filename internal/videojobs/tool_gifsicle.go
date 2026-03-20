package videojobs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultGifsicleTimeout = 20 * time.Second

type GIFOptimizationResult struct {
	Enabled         bool    `json:"enabled"`
	Attempted       bool    `json:"attempted"`
	Applied         bool    `json:"applied"`
	Tool            string  `json:"tool"`
	ToolPath        string  `json:"tool_path,omitempty"`
	Level           int     `json:"level"`
	SkipBelowKB     int     `json:"skip_below_kb,omitempty"`
	MinGainRatio    float64 `json:"min_gain_ratio,omitempty"`
	BeforeSizeBytes int64   `json:"before_size_bytes"`
	AfterSizeBytes  int64   `json:"after_size_bytes"`
	SavedBytes      int64   `json:"saved_bytes"`
	SavedRatio      float64 `json:"saved_ratio"`
	DurationMs      int64   `json:"duration_ms"`
	Reason          string  `json:"reason,omitempty"`
	Error           string  `json:"error,omitempty"`
}

func optimizeGIFWithGifsicle(
	ctx context.Context,
	outputPath string,
	qualitySettings QualitySettings,
	preferredBin string,
) GIFOptimizationResult {
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	result := GIFOptimizationResult{
		Enabled:      qualitySettings.GIFGifsicleEnabled,
		Applied:      false,
		Tool:         "gifsicle",
		Level:        qualitySettings.GIFGifsicleLevel,
		SkipBelowKB:  qualitySettings.GIFGifsicleSkipBelowKB,
		MinGainRatio: qualitySettings.GIFGifsicleMinGainRatio,
	}
	if !qualitySettings.GIFGifsicleEnabled {
		result.Reason = "disabled"
		return result
	}

	info, statErr := os.Stat(outputPath)
	if statErr != nil {
		result.Error = statErr.Error()
		result.Reason = "input_stat_failed"
		return result
	}
	result.BeforeSizeBytes = info.Size()
	if result.BeforeSizeBytes <= 0 {
		result.Reason = "empty_input"
		return result
	}

	if threshold := int64(qualitySettings.GIFGifsicleSkipBelowKB) * 1024; threshold > 0 && result.BeforeSizeBytes < threshold {
		result.Reason = "skip_below_threshold"
		return result
	}

	level := qualitySettings.GIFGifsicleLevel
	if level < 1 {
		level = 1
	}
	if level > 3 {
		level = 3
	}
	result.Level = level

	binPath, err := resolveGifsicleBinary(preferredBin)
	if err != nil {
		result.Error = err.Error()
		result.Reason = "tool_unavailable"
		return result
	}
	result.ToolPath = binPath

	tmpPath := outputPath + ".gifsicle.tmp"
	if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
		result.Error = removeErr.Error()
		result.Reason = "tmp_prepare_failed"
		return result
	}
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	runCtx := ctx
	cancel := func() {}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		runCtx, cancel = context.WithTimeout(ctx, defaultGifsicleTimeout)
	}
	defer cancel()

	result.Attempted = true
	started := time.Now()
	args := []string{
		"-O" + strconv.Itoa(level),
		"--careful",
		outputPath,
		"-o",
		tmpPath,
	}
	cmd := exec.CommandContext(runCtx, binPath, args...)
	out, runErr := cmd.CombinedOutput()
	result.DurationMs = clampDurationMillis(started)
	if runErr != nil {
		result.Error = strings.TrimSpace(string(out))
		if result.Error == "" {
			result.Error = runErr.Error()
		}
		if runCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
			result.Reason = "timeout"
		} else {
			result.Reason = "command_failed"
		}
		return result
	}

	optimized, optErr := os.Stat(tmpPath)
	if optErr != nil {
		result.Error = optErr.Error()
		result.Reason = "optimized_stat_failed"
		return result
	}
	result.AfterSizeBytes = optimized.Size()
	result.SavedBytes = result.BeforeSizeBytes - result.AfterSizeBytes
	if result.BeforeSizeBytes > 0 {
		result.SavedRatio = roundTo(float64(result.SavedBytes)/float64(result.BeforeSizeBytes), 6)
	}

	if result.AfterSizeBytes <= 0 {
		result.Reason = "optimized_empty"
		return result
	}
	if result.AfterSizeBytes >= result.BeforeSizeBytes {
		result.Reason = "no_gain"
		return result
	}
	if result.SavedRatio < qualitySettings.GIFGifsicleMinGainRatio {
		result.Reason = "gain_below_threshold"
		return result
	}

	if err := os.Rename(tmpPath, outputPath); err != nil {
		result.Error = err.Error()
		result.Reason = "replace_failed"
		return result
	}
	result.Applied = true
	result.Reason = "applied"
	return result
}

func resolveGifsicleBinary(preferred string) (string, error) {
	path := strings.TrimSpace(preferred)
	if path != "" {
		if filepath.IsAbs(path) {
			if _, err := os.Stat(path); err != nil {
				return "", fmt.Errorf("gifsicle bin not found: %w", err)
			}
			return path, nil
		}
		if resolved, err := exec.LookPath(path); err == nil {
			return resolved, nil
		}
	}
	resolved, err := exec.LookPath("gifsicle")
	if err != nil {
		return "", err
	}
	return resolved, nil
}

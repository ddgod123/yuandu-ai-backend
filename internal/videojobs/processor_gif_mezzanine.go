package videojobs

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func buildGIFBundleMezzanine(
	ctx context.Context,
	sourcePath string,
	outputDir string,
	bundle gifRenderBundlePlan,
	config gifBundleRuntimeConfig,
) (string, int64, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", 0, fmt.Errorf("empty source path")
	}
	durationSec := bundle.EndSec - bundle.StartSec
	if durationSec <= 0 {
		return "", 0, fmt.Errorf("invalid bundle duration")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("create mezzanine dir: %w", err)
	}

	fileName := fmt.Sprintf("%s.mp4", strings.TrimSpace(bundle.BundleID))
	targetPath := filepath.Join(outputDir, fileName)
	_ = os.Remove(targetPath)

	timeout := time.Duration(durationSec*4)*time.Second + 8*time.Second
	if timeout < 12*time.Second {
		timeout = 12 * time.Second
	}
	if timeout > 120*time.Second {
		timeout = 120 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
	}
	if bundle.StartSec > 0 {
		args = append(args, "-ss", formatFFmpegNumber(bundle.StartSec))
	}
	args = append(args,
		"-i", sourcePath,
		"-t", formatFFmpegNumber(durationSec),
		"-an",
		"-c:v", "libx264",
		"-preset", strings.TrimSpace(config.MezzaninePreset),
		"-crf", fmt.Sprintf("%d", config.MezzanineCRF),
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		targetPath,
	)

	startedAt := time.Now()
	cmd := exec.CommandContext(runCtx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	elapsedMs := time.Since(startedAt).Milliseconds()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		if runCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
			return "", elapsedMs, fmt.Errorf("mezzanine timeout after %s: %s", timeout.String(), msg)
		}
		return "", elapsedMs, fmt.Errorf("mezzanine render failed: %s", msg)
	}
	return targetPath, elapsedMs, nil
}

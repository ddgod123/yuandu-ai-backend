package videojobs

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type aiDirectorFrameSample struct {
	Index        int
	TimestampSec float64
	Bytes        int
	DataURL      string
}

func buildAIDirectorFrameSamplePreviews(samples []aiDirectorFrameSample, maxN int) []map[string]interface{} {
	if len(samples) == 0 {
		return nil
	}
	if maxN <= 0 {
		maxN = 6
	}
	out := make([]map[string]interface{}, 0, minInt(len(samples), maxN))
	for _, item := range samples {
		if len(out) >= maxN {
			break
		}
		dataURL := strings.TrimSpace(item.DataURL)
		if dataURL == "" {
			continue
		}
		out = append(out, map[string]interface{}{
			"index":          item.Index,
			"timestamp_sec":  roundTo(item.TimestampSec, 3),
			"bytes":          item.Bytes,
			"image_data_url": dataURL,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sampleAIDirectorFrames(ctx context.Context, sourcePath string, meta videoProbeMeta, maxFrames int) ([]aiDirectorFrameSample, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return nil, fmt.Errorf("empty source path")
	}
	if maxFrames <= 0 {
		maxFrames = 6
	}
	if maxFrames > 12 {
		maxFrames = 12
	}

	durationSec := meta.DurationSec
	if durationSec <= 0 {
		durationSec = 30
	}
	sampleFPS := clampFloat(float64(maxFrames)/durationSec, 0.05, 1.2)
	if durationSec < 10 && maxFrames > 4 {
		maxFrames = 4
	}

	tmpDir, err := os.MkdirTemp("", "emoji_ai1_frames_*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	outputPattern := filepath.Join(tmpDir, "frame_%03d.jpg")
	filter := fmt.Sprintf("fps=%s,scale=640:-2:flags=lanczos", formatFFmpegNumber(sampleFPS))
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", sourcePath,
		"-vf", filter,
		"-frames:v", strconv.Itoa(maxFrames),
		"-q:v", "8",
		outputPattern,
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sample ai1 frames failed: %w: %s", err, strings.TrimSpace(string(raw)))
	}

	files, err := filepath.Glob(filepath.Join(tmpDir, "frame_*.jpg"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no frame extracted for ai1")
	}

	timestamps := estimateAIDirectorFrameTimestamps(meta.DurationSec, len(files))
	samples := make([]aiDirectorFrameSample, 0, len(files))
	for idx, filePath := range files {
		buf, readErr := os.ReadFile(filePath)
		if readErr != nil || len(buf) == 0 {
			continue
		}
		timestamp := 0.0
		if idx < len(timestamps) {
			timestamp = timestamps[idx]
		}
		dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf)
		samples = append(samples, aiDirectorFrameSample{
			Index:        idx + 1,
			TimestampSec: roundTo(timestamp, 3),
			Bytes:        len(buf),
			DataURL:      dataURL,
		})
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("no readable frame extracted for ai1")
	}
	return samples, nil
}

func estimateAIDirectorFrameTimestamps(durationSec float64, count int) []float64 {
	if count <= 0 {
		return nil
	}
	if durationSec <= 0 {
		out := make([]float64, count)
		for i := 0; i < count; i++ {
			out[i] = float64(i)
		}
		return out
	}
	if count == 1 {
		return []float64{roundTo(durationSec*0.5, 3)}
	}
	start := durationSec * 0.08
	end := durationSec * 0.92
	if end <= start {
		start = 0
		end = durationSec
	}
	step := (end - start) / float64(count-1)
	out := make([]float64, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, roundTo(start+float64(i)*step, 3))
	}
	return out
}

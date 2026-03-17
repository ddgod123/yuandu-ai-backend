package videojobs

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrVideoStreamNotFound     = errors.New("video stream not found")
	ErrVideoDurationMissing    = errors.New("video duration unavailable")
	ErrVideoDurationTooLong    = errors.New("video duration too long")
	MaxAllowedProbeDurationSec = 4 * 60 * 60
)

// ProbeMeta is a safe-to-expose probe summary for preflight validation.
type ProbeMeta struct {
	DurationSec float64 `json:"duration_sec"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FPS         float64 `json:"fps"`
}

// ProbeVideoSource probes a video source (local path or URL) and validates core
// dimensions so the API can fail fast before queueing a job.
func ProbeVideoSource(ctx context.Context, sourcePath string) (ProbeMeta, error) {
	meta, err := probeVideo(ctx, sourcePath)
	if err != nil {
		return ProbeMeta{}, err
	}
	if err := validateProbeMeta(meta); err != nil {
		return ProbeMeta{}, err
	}
	return ProbeMeta{
		DurationSec: meta.DurationSec,
		Width:       meta.Width,
		Height:      meta.Height,
		FPS:         meta.FPS,
	}, nil
}

func validateProbeMeta(meta videoProbeMeta) error {
	if meta.Width <= 0 || meta.Height <= 0 {
		return ErrVideoStreamNotFound
	}
	if meta.DurationSec <= 0 {
		return ErrVideoDurationMissing
	}
	if meta.DurationSec > float64(MaxAllowedProbeDurationSec) {
		return fmt.Errorf("%w: %.1fs", ErrVideoDurationTooLong, meta.DurationSec)
	}
	return nil
}

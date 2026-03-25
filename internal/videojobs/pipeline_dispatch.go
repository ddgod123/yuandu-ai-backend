package videojobs

import (
	"context"

	"emoji/internal/models"
)

func (p *Processor) process(ctx context.Context, jobID uint64) error {
	format, err := p.resolvePrimaryRequestedFormat(jobID)
	if err != nil {
		return err
	}

	switch format {
	case "gif":
		return p.processGIFPipeline(ctx, jobID)
	case "png", "jpg", "webp", "live", "mp4":
		return p.processImagePipeline(ctx, jobID)
	default:
		return p.processUnified(ctx, jobID)
	}
}

func (p *Processor) resolvePrimaryRequestedFormat(jobID uint64) (string, error) {
	if p == nil || p.db == nil || jobID == 0 {
		return "", nil
	}
	var job models.VideoJob
	if err := p.db.Select("output_formats").First(&job, jobID).Error; err != nil {
		return "", err
	}
	return PrimaryRequestedFormat(job.OutputFormats), nil
}

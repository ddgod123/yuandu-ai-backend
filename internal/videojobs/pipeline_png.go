package videojobs

import "context"

// processImagePipeline is the dedicated execution lane for still-image jobs
// (png/jpg/webp/live/mp4 first-frame path). It currently reuses processUnified
// and serves as the split seam for future format-specific AI stages.
func (p *Processor) processImagePipeline(ctx context.Context, jobID uint64) error {
	return p.processUnified(ctx, jobID)
}

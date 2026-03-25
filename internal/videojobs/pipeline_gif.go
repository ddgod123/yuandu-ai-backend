package videojobs

import "context"

// processGIFPipeline is the dedicated execution lane for GIF jobs.
// Stage-2 refactor keeps the heavy logic in processUnified and isolates the
// entrypoint so GIF-specific orchestration can evolve independently.
func (p *Processor) processGIFPipeline(ctx context.Context, jobID uint64) error {
	return p.processUnified(ctx, jobID)
}

package videojobs

import (
	"errors"
)

var errGIFPipelineHalt = errors.New("gif pipeline halted")

func newGIFPipelineStageRegistry() *pipelineStageRegistry {
	return newPipelineStageRegistry(8)
}

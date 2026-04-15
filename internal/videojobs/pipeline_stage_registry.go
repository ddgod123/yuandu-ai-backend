package videojobs

import "strings"

type pipelineStage struct {
	Name string
	Run  func() error
}

type pipelineStageRegistry struct {
	stages []pipelineStage
}

func newPipelineStageRegistry(capacity int) *pipelineStageRegistry {
	if capacity <= 0 {
		capacity = 8
	}
	return &pipelineStageRegistry{
		stages: make([]pipelineStage, 0, capacity),
	}
}

func (r *pipelineStageRegistry) Register(name string, run func() error) {
	if r == nil || run == nil {
		return
	}
	r.stages = append(r.stages, pipelineStage{
		Name: strings.TrimSpace(name),
		Run:  run,
	})
}

func (r *pipelineStageRegistry) Execute() error {
	if r == nil {
		return nil
	}
	for _, stage := range r.stages {
		if stage.Run == nil {
			continue
		}
		if err := stage.Run(); err != nil {
			return err
		}
	}
	return nil
}

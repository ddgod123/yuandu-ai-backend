package videojobs

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	TaskTypeProcessVideoJob = "video_jobs:process"
)

type ProcessVideoJobPayload struct {
	JobID uint64 `json:"job_id"`
}

func NewProcessVideoJobTask(jobID uint64) (*asynq.Task, error) {
	if jobID == 0 {
		return nil, fmt.Errorf("invalid job id")
	}
	payload, err := json.Marshal(ProcessVideoJobPayload{JobID: jobID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskTypeProcessVideoJob, payload), nil
}

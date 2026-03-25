package copyrightjobs

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	TaskTypeProcessCollectionCopyright = "copyright:process_collection_task"
)

type ProcessCollectionCopyrightPayload struct {
	TaskID uint64 `json:"task_id"`
}

func NewProcessCollectionCopyrightTask(taskID uint64) (*asynq.Task, error) {
	if taskID == 0 {
		return nil, fmt.Errorf("invalid task id")
	}
	payload, err := json.Marshal(ProcessCollectionCopyrightPayload{TaskID: taskID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskTypeProcessCollectionCopyright, payload), nil
}

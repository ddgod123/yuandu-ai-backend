package feishujobs

import (
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

const (
	TaskTypeIngestFeishuMessage = "feishu:ingest_message"
	TaskTypeNotifyFeishuResult  = "feishu:notify_result"
)

type IngestFeishuMessagePayload struct {
	MessageJobID uint64 `json:"message_job_id"`
}

type NotifyFeishuResultPayload struct {
	MessageJobID uint64 `json:"message_job_id"`
	Attempt      int    `json:"attempt"`
}

func NewIngestFeishuMessageTask(messageJobID uint64) (*asynq.Task, error) {
	if messageJobID == 0 {
		return nil, fmt.Errorf("invalid message_job_id")
	}
	payload, err := json.Marshal(IngestFeishuMessagePayload{MessageJobID: messageJobID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskTypeIngestFeishuMessage, payload), nil
}

func NewNotifyFeishuResultTask(messageJobID uint64, attempt int) (*asynq.Task, error) {
	if messageJobID == 0 {
		return nil, fmt.Errorf("invalid message_job_id")
	}
	if attempt < 0 {
		attempt = 0
	}
	payload, err := json.Marshal(NotifyFeishuResultPayload{MessageJobID: messageJobID, Attempt: attempt})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskTypeNotifyFeishuResult, payload), nil
}

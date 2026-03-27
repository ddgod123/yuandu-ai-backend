package videojobs

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hibiken/asynq"
)

const (
	TaskTypeProcessVideoJob     = "video_jobs:process"
	TaskTypeProcessVideoJobGIF  = "video_jobs:process:gif"
	TaskTypeProcessVideoJobPNG  = "video_jobs:process:png"
	TaskTypeProcessVideoJobJPG  = "video_jobs:process:jpg"
	TaskTypeProcessVideoJobWEBP = "video_jobs:process:webp"
	TaskTypeProcessVideoJobLIVE = "video_jobs:process:live"
	TaskTypeProcessVideoJobMP4  = "video_jobs:process:mp4"

	QueueVideoJobMedia = "media"
	QueueVideoJobGIF   = "video_gif"
	QueueVideoJobPNG   = "video_png"
	QueueVideoJobJPG   = "video_jpg"
	QueueVideoJobWEBP  = "video_webp"
	QueueVideoJobLIVE  = "video_live"
	QueueVideoJobMP4   = "video_mp4"
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

func NormalizeRequestedFormat(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "jpeg" {
		return "jpg"
	}
	return value
}

func PrimaryRequestedFormat(rawOutputFormats string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(rawOutputFormats)), ",")
	for _, item := range parts {
		format := NormalizeRequestedFormat(item)
		if format == "" {
			continue
		}
		return format
	}
	return ""
}

func ResolveVideoJobExecutionTarget(rawOutputFormats string) (queue string, taskType string, primaryFormat string) {
	primaryFormat = PrimaryRequestedFormat(rawOutputFormats)
	switch primaryFormat {
	case "gif":
		return QueueVideoJobGIF, TaskTypeProcessVideoJobGIF, primaryFormat
	case "png":
		return QueueVideoJobPNG, TaskTypeProcessVideoJobPNG, primaryFormat
	case "jpg":
		return QueueVideoJobJPG, TaskTypeProcessVideoJobJPG, primaryFormat
	case "webp":
		return QueueVideoJobWEBP, TaskTypeProcessVideoJobWEBP, primaryFormat
	case "live":
		return QueueVideoJobLIVE, TaskTypeProcessVideoJobLIVE, primaryFormat
	case "mp4":
		return QueueVideoJobMP4, TaskTypeProcessVideoJobMP4, primaryFormat
	default:
		return QueueVideoJobMedia, TaskTypeProcessVideoJob, primaryFormat
	}
}

func NewProcessVideoJobTaskByFormat(jobID uint64, rawOutputFormats string) (*asynq.Task, string, string, error) {
	if jobID == 0 {
		return nil, "", "", fmt.Errorf("invalid job id")
	}
	queue, taskType, primaryFormat := ResolveVideoJobExecutionTarget(rawOutputFormats)
	payload, err := json.Marshal(ProcessVideoJobPayload{JobID: jobID})
	if err != nil {
		return nil, "", "", err
	}
	return asynq.NewTask(taskType, payload), queue, primaryFormat, nil
}

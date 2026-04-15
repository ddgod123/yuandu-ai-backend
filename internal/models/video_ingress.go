package models

import (
	"time"

	"gorm.io/datatypes"
)

const (
	VideoIngressProviderWeb    = "web"
	VideoIngressProviderFeishu = "feishu"
	VideoIngressProviderQQ     = "qq"
	VideoIngressProviderWeCom  = "wecom"
)

const (
	VideoIngressStatusQueued      = "queued"
	VideoIngressStatusProcessing  = "processing"
	VideoIngressStatusWaitingBind = "waiting_bind"
	VideoIngressStatusJobQueued   = "job_queued"
	VideoIngressStatusDone        = "done"
	VideoIngressStatusFailed      = "failed"
)

type VideoIngressJob struct {
	ID                uint64         `gorm:"primaryKey;autoIncrement"`
	Provider          string         `gorm:"column:provider;size:32;not null;index"`
	TenantKey         string         `gorm:"column:tenant_key;size:128;index"`
	Channel           string         `gorm:"column:channel;size:64;index"`
	ChatID            string         `gorm:"column:chat_id;size:128;index"`
	SessionID         string         `gorm:"column:session_id;size:128;index"`
	ExternalUserID    string         `gorm:"column:external_user_id;size:128;index"`
	ExternalOpenID    string         `gorm:"column:external_open_id;size:128;index"`
	ExternalUnionID   string         `gorm:"column:external_union_id;size:128;index"`
	BoundUserID       *uint64        `gorm:"column:bound_user_id;index"`
	SourceMessageID   string         `gorm:"column:source_message_id;size:128;index"`
	SourceResourceKey string         `gorm:"column:source_resource_key;size:256;index"`
	SourceVideoKey    string         `gorm:"column:source_video_key;size:512;index"`
	SourceVideoURL    string         `gorm:"column:source_video_url;type:text"`
	SourceFileName    string         `gorm:"column:source_file_name;size:512"`
	SourceSizeBytes   int64          `gorm:"column:source_size_bytes"`
	VideoJobID        *uint64        `gorm:"column:video_job_id;index"`
	Status            string         `gorm:"column:status;size:32;not null;index"`
	ErrorMessage      string         `gorm:"column:error_message;type:text"`
	RequestPayload    datatypes.JSON `gorm:"column:request_payload;type:jsonb"`
	ResultPayload     datatypes.JSON `gorm:"column:result_payload;type:jsonb"`
	FinishedAt        *time.Time     `gorm:"column:finished_at;index"`
	CreatedAt         time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt         time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (VideoIngressJob) TableName() string {
	return "archive.video_ingress_jobs"
}

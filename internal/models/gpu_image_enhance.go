package models

import (
	"time"

	"gorm.io/datatypes"
)

const (
	GPUImageEnhanceStatusQueued    = "queued"
	GPUImageEnhanceStatusRunning   = "running"
	GPUImageEnhanceStatusSucceeded = "succeeded"
	GPUImageEnhanceStatusFailed    = "failed"
	GPUImageEnhanceStatusCancelled = "cancelled"
)

const (
	GPUImageEnhanceAssetRoleSource = "source"
	GPUImageEnhanceAssetRoleResult = "result"
)

type GPUImageEnhanceJob struct {
	ID              uint64         `gorm:"primaryKey;autoIncrement"`
	UserID          uint64         `gorm:"column:user_id;index"`
	Title           string         `gorm:"column:title;size:255"`
	Provider        string         `gorm:"column:provider;size:64"`
	Model           string         `gorm:"column:model;size:128"`
	Scale           int            `gorm:"column:scale"`
	SourceObjectKey string         `gorm:"column:source_object_key;size:768;index"`
	SourceMimeType  string         `gorm:"column:source_mime_type;size:128"`
	SourceSizeBytes int64          `gorm:"column:source_size_bytes"`
	ResultObjectKey string         `gorm:"column:result_object_key;size:768"`
	ResultMimeType  string         `gorm:"column:result_mime_type;size:128"`
	ResultSizeBytes int64          `gorm:"column:result_size_bytes"`
	Status          string         `gorm:"column:status;size:32;index"`
	Stage           string         `gorm:"column:stage;size:32;index"`
	Progress        int            `gorm:"column:progress"`
	ErrorMessage    string         `gorm:"column:error_message;type:text"`
	RequestPayload  datatypes.JSON `gorm:"column:request_payload;type:jsonb"`
	CallbackPayload datatypes.JSON `gorm:"column:callback_payload;type:jsonb"`
	Metadata        datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	QueuedAt        time.Time      `gorm:"column:queued_at;autoCreateTime"`
	StartedAt       *time.Time     `gorm:"column:started_at;index"`
	FinishedAt      *time.Time     `gorm:"column:finished_at;index"`
	CreatedAt       time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (GPUImageEnhanceJob) TableName() string {
	return "archive.gpu_image_enhance_jobs"
}

type GPUImageEnhanceAsset struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	JobID     uint64         `gorm:"column:job_id;index"`
	UserID    uint64         `gorm:"column:user_id;index"`
	AssetRole string         `gorm:"column:asset_role;size:32;index"`
	ObjectKey string         `gorm:"column:object_key;size:768"`
	MimeType  string         `gorm:"column:mime_type;size:128"`
	SizeBytes int64          `gorm:"column:size_bytes"`
	Width     int            `gorm:"column:width"`
	Height    int            `gorm:"column:height"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime"`
}

func (GPUImageEnhanceAsset) TableName() string {
	return "archive.gpu_image_enhance_assets"
}

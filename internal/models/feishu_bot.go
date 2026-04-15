package models

import (
	"time"

	"gorm.io/datatypes"
)

const (
	ExternalAccountProviderFeishu = "feishu"
	ExternalAccountProviderQQ     = "qq"
	ExternalAccountProviderWeCom  = "wecom"

	ExternalAccountStatusActive   = "active"
	ExternalAccountStatusDisabled = "disabled"

	FeishuEventStatusReceived = "received"
	FeishuEventStatusQueued   = "queued"
	FeishuEventStatusIgnored  = "ignored"
	FeishuEventStatusFailed   = "failed"

	FeishuMessageJobStatusQueued      = "queued"
	FeishuMessageJobStatusProcessing  = "processing"
	FeishuMessageJobStatusRetrying    = "retrying"
	FeishuMessageJobStatusWaitingBind = "waiting_bind"
	FeishuMessageJobStatusJobQueued   = "job_queued"
	FeishuMessageJobStatusDone        = "done"
	FeishuMessageJobStatusFailed      = "failed"

	FeishuBindCodeStatusActive  = "active"
	FeishuBindCodeStatusUsed    = "used"
	FeishuBindCodeStatusExpired = "expired"
)

type ExternalAccount struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	Provider  string         `gorm:"column:provider;size:32;not null;index;uniqueIndex:uniq_external_provider_tenant_open"`
	TenantKey string         `gorm:"column:tenant_key;size:128;not null;index;uniqueIndex:uniq_external_provider_tenant_open"`
	OpenID    string         `gorm:"column:open_id;size:128;not null;index;uniqueIndex:uniq_external_provider_tenant_open"`
	UnionID   string         `gorm:"column:union_id;size:128;index"`
	UserID    uint64         `gorm:"column:user_id;not null;index"`
	Status    string         `gorm:"column:status;size:32;not null;index"`
	Metadata  datatypes.JSON `gorm:"column:metadata;type:jsonb"`
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
}

func (ExternalAccount) TableName() string {
	return "archive.external_accounts"
}

type FeishuEventLog struct {
	ID        uint64         `gorm:"primaryKey;autoIncrement"`
	EventID   string         `gorm:"column:event_id;size:128;not null;uniqueIndex"`
	EventType string         `gorm:"column:event_type;size:128;not null;index"`
	TenantKey string         `gorm:"column:tenant_key;size:128;index"`
	MessageID string         `gorm:"column:message_id;size:128;index"`
	Status    string         `gorm:"column:status;size:32;not null;index"`
	Payload   datatypes.JSON `gorm:"column:payload;type:jsonb"`
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
}

func (FeishuEventLog) TableName() string {
	return "archive.feishu_event_logs"
}

type FeishuMessageJob struct {
	ID             uint64         `gorm:"primaryKey;autoIncrement"`
	TenantKey      string         `gorm:"column:tenant_key;size:128;not null;index;uniqueIndex:uniq_feishu_message_resource"`
	ChatID         string         `gorm:"column:chat_id;size:128;not null;index"`
	MessageID      string         `gorm:"column:message_id;size:128;not null;index;uniqueIndex:uniq_feishu_message_resource"`
	MessageType    string         `gorm:"column:message_type;size:32;not null;index"`
	FileKey        string         `gorm:"column:file_key;size:256;not null;uniqueIndex:uniq_feishu_message_resource"`
	FileName       string         `gorm:"column:file_name;size:512"`
	OpenID         string         `gorm:"column:open_id;size:128;not null;index"`
	UnionID        string         `gorm:"column:union_id;size:128;index"`
	BindCode       string         `gorm:"column:bind_code;size:32"`
	UserID         *uint64        `gorm:"column:user_id;index"`
	VideoJobID     *uint64        `gorm:"column:video_job_id;index"`
	Status         string         `gorm:"column:status;size:32;not null;index"`
	ErrorMessage   string         `gorm:"column:error_message;type:text"`
	NotifyAttempts int            `gorm:"column:notify_attempts;not null;default:0"`
	NotifiedAt     *time.Time     `gorm:"column:notified_at;index"`
	RequestPayload datatypes.JSON `gorm:"column:request_payload;type:jsonb"`
	ResultPayload  datatypes.JSON `gorm:"column:result_payload;type:jsonb"`
	FinishedAt     *time.Time     `gorm:"column:finished_at;index"`
	CreatedAt      time.Time      `gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime"`
}

func (FeishuMessageJob) TableName() string {
	return "archive.feishu_message_jobs"
}

type FeishuBindCode struct {
	ID        uint64     `gorm:"primaryKey;autoIncrement"`
	Code      string     `gorm:"column:code;size:32;not null;uniqueIndex"`
	TenantKey string     `gorm:"column:tenant_key;size:128;not null;index"`
	ChatID    string     `gorm:"column:chat_id;size:128;not null"`
	OpenID    string     `gorm:"column:open_id;size:128;not null;index"`
	UnionID   string     `gorm:"column:union_id;size:128;index"`
	UserID    *uint64    `gorm:"column:user_id;index"`
	Status    string     `gorm:"column:status;size:32;not null;index"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null;index"`
	UsedAt    *time.Time `gorm:"column:used_at;index"`
	CreatedAt time.Time  `gorm:"autoCreateTime"`
	UpdatedAt time.Time  `gorm:"autoUpdateTime"`
}

func (FeishuBindCode) TableName() string {
	return "archive.feishu_bind_codes"
}

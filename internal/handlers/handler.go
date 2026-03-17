package handlers

import (
	"emoji/internal/config"
	"emoji/internal/queue"
	"emoji/internal/service"
	"emoji/internal/storage"
	"emoji/pkg/oss"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

type Handler struct {
	db         *gorm.DB
	cfg        config.Config
	qiniu      *storage.QiniuClient
	smsLimiter SMSLimiter
	ai         *service.AIService
	compose    *service.ComposeService
	ossClient  *oss.Client
	queue      *asynq.Client
}

func New(db *gorm.DB, cfg config.Config, qiniu *storage.QiniuClient, ossClient *oss.Client, ai *service.AIService, compose *service.ComposeService) *Handler {
	return &Handler{
		db:         db,
		cfg:        cfg,
		qiniu:      qiniu,
		smsLimiter: newSMSLimiter(cfg),
		ai:         ai,
		compose:    compose,
		ossClient:  ossClient,
		queue:      queue.NewClient(cfg),
	}
}

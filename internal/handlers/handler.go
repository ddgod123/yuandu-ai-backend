package handlers

import (
	"emoji/internal/config"
	"emoji/internal/service"
	"emoji/internal/storage"
	"emoji/pkg/oss"
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
	}
}

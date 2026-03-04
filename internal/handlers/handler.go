package handlers

import (
	"emoji/internal/config"
	"emoji/internal/storage"
	"gorm.io/gorm"
)

type Handler struct {
	db         *gorm.DB
	cfg        config.Config
	qiniu      *storage.QiniuClient
	smsLimiter SMSLimiter
}

func New(db *gorm.DB, cfg config.Config, qiniu *storage.QiniuClient) *Handler {
	return &Handler{
		db:         db,
		cfg:        cfg,
		qiniu:      qiniu,
		smsLimiter: newSMSLimiter(cfg),
	}
}

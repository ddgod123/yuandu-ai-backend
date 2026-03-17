package queue

import (
	"emoji/internal/config"

	"github.com/hibiken/asynq"
)

func NewClient(cfg config.Config) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.AsynqRedisAddr,
		Password: cfg.AsynqRedisPassword,
		DB:       cfg.AsynqRedisDB,
	})
}

func NewServer(cfg config.Config) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.AsynqRedisAddr,
			Password: cfg.AsynqRedisPassword,
			DB:       cfg.AsynqRedisDB,
		},
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"default": 6,
				"media":   4,
			},
		},
	)
}

func NewInspector(cfg config.Config) *asynq.Inspector {
	return asynq.NewInspector(asynq.RedisClientOpt{
		Addr:     cfg.AsynqRedisAddr,
		Password: cfg.AsynqRedisPassword,
		DB:       cfg.AsynqRedisDB,
	})
}

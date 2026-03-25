package queue

import (
	"emoji/internal/config"
	"emoji/internal/videojobs"
	"os"
	"strconv"
	"strings"

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
	concurrency := parseAsynqConcurrency(10)
	queues := defaultAsynqQueueWeights()
	if override := parseAsynqQueueWeightsEnv(strings.TrimSpace(os.Getenv("ASYNQ_QUEUE_WEIGHTS"))); len(override) > 0 {
		queues = override
	}

	return asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.AsynqRedisAddr,
			Password: cfg.AsynqRedisPassword,
			DB:       cfg.AsynqRedisDB,
		},
		asynq.Config{
			Concurrency: concurrency,
			Queues:      queues,
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

func defaultAsynqQueueWeights() map[string]int {
	return map[string]int{
		"default":                    4,
		videojobs.QueueVideoJobGIF:   4,
		videojobs.QueueVideoJobPNG:   4,
		videojobs.QueueVideoJobMedia: 2,
	}
}

func parseAsynqConcurrency(fallback int) int {
	raw := strings.TrimSpace(os.Getenv("ASYNQ_CONCURRENCY"))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func parseAsynqQueueWeightsEnv(raw string) map[string]int {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := map[string]int{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 {
			continue
		}
		name := strings.TrimSpace(kv[0])
		if name == "" {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil || value <= 0 {
			continue
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

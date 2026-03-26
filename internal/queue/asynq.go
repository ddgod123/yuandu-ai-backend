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
	queues := ResolveAsynqQueueWeightsFromEnv()

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

func ResolveAsynqQueueWeightsFromEnv() map[string]int {
	queues := defaultAsynqQueueWeights()
	if override := parseAsynqQueueWeightsEnv(strings.TrimSpace(os.Getenv("ASYNQ_QUEUE_WEIGHTS"))); len(override) > 0 {
		queues = override
	}
	role := resolveVideoWorkerRole(strings.TrimSpace(os.Getenv("VIDEO_WORKER_ROLE")))
	return applyVideoWorkerRoleFilter(queues, role)
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

func resolveVideoWorkerRole(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "all", "default":
		return "all"
	case "gif":
		return "gif"
	case "png", "image":
		return "png"
	case "media":
		return "media"
	default:
		return "all"
	}
}

func applyVideoWorkerRoleFilter(queues map[string]int, role string) map[string]int {
	role = resolveVideoWorkerRole(role)
	if role == "all" {
		return queues
	}

	pick := func(name string) map[string]int {
		weight := 1
		if queues != nil {
			if value, ok := queues[name]; ok && value > 0 {
				weight = value
			}
		}
		return map[string]int{name: weight}
	}

	switch role {
	case "gif":
		return pick(videojobs.QueueVideoJobGIF)
	case "png":
		return pick(videojobs.QueueVideoJobPNG)
	case "media":
		return pick(videojobs.QueueVideoJobMedia)
	default:
		return queues
	}
}

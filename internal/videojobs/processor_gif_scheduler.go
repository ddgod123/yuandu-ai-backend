package videojobs

import (
	"context"
	"math"
	"sort"
	"strings"

	"golang.org/x/sync/semaphore"
)

const gifRenderCostTokenScale = 10.0

type gifRenderScheduleItem struct {
	TaskIndex int
	CostUnits float64
}

type gifRenderScheduler struct {
	workerSem *semaphore.Weighted
	costSem   *semaphore.Weighted
	maxTokens int64
}

func newGIFRenderScheduler(maxWorkers int, maxCostUnits float64) *gifRenderScheduler {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	maxTokens := int(math.Ceil(maxCostUnits * gifRenderCostTokenScale))
	if maxTokens < 1 {
		maxTokens = 1
	}
	return &gifRenderScheduler{
		workerSem: semaphore.NewWeighted(int64(maxWorkers)),
		costSem:   semaphore.NewWeighted(int64(maxTokens)),
		maxTokens: int64(maxTokens),
	}
}

func (s *gifRenderScheduler) acquire(ctx context.Context, costUnits float64) (func(), error) {
	if s == nil {
		return func() {}, nil
	}
	if err := s.workerSem.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	needTokens := int64(math.Ceil(costUnits * gifRenderCostTokenScale))
	if needTokens < 1 {
		needTokens = 1
	}
	if needTokens > s.maxTokens {
		needTokens = s.maxTokens
	}
	if err := s.costSem.Acquire(ctx, needTokens); err != nil {
		s.workerSem.Release(1)
		return nil, err
	}
	release := func() {
		s.costSem.Release(needTokens)
		s.workerSem.Release(1)
	}
	return release, nil
}

func buildGIFRenderSchedule(tasks []animatedTask) []gifRenderScheduleItem {
	items := make([]gifRenderScheduleItem, 0, len(tasks))
	bundleSeen := make(map[string]int, len(tasks))
	for idx, task := range tasks {
		costUnits := 0.5
		if strings.EqualFold(strings.TrimSpace(task.Format), "gif") {
			bundleID := strings.TrimSpace(task.BundleID)
			costUnits = estimateGIFRenderScheduleCost(task, bundleSeen[bundleID])
			if bundleID != "" {
				bundleSeen[bundleID]++
			}
		}
		items = append(items, gifRenderScheduleItem{
			TaskIndex: idx,
			CostUnits: costUnits,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if math.Abs(items[i].CostUnits-items[j].CostUnits) < 1e-6 {
			return items[i].TaskIndex < items[j].TaskIndex
		}
		return items[i].CostUnits > items[j].CostUnits
	})
	return items
}

func estimateGIFRenderScheduleCost(task animatedTask, bundleOrder int) float64 {
	costUnits := task.RenderCostUnits
	if costUnits <= 0 {
		costUnits = 1.0
	}
	if !strings.EqualFold(strings.TrimSpace(task.Format), "gif") {
		return 0.5
	}
	bundleID := strings.TrimSpace(task.BundleID)
	if bundleID != "" {
		// 同 bundle 下窗口复用率更高，调度阶段按较低成本估算，避免 token 限流过早串行化。
		costUnits *= 0.85
		if bundleOrder > 0 {
			costUnits *= 0.7
		}
	}
	if strings.TrimSpace(task.MezzaninePath) != "" {
		// 使用 mezzanine 的窗口通常解码更轻，进一步下调调度成本估算。
		costUnits *= 0.75
		if bundleOrder > 0 {
			costUnits *= 0.8
		}
	}
	return clampFloat(costUnits, 0.35, 8.0)
}

func resolveGIFRenderMaxCostUnits(
	meta videoProbeMeta,
	tasks []animatedTask,
	qualitySettings QualitySettings,
	workers int,
) float64 {
	if workers < 1 {
		workers = 1
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	baseMultiplier := qualitySettings.GIFRenderBudgetNormalMultiplier * 1.4
	baseMultiplier = clampFloat(baseMultiplier, 2.2, 3.6)
	maxUnits := float64(workers) * baseMultiplier
	if maxUnits < 1 {
		maxUnits = 1
	}
	longSide := meta.Width
	if meta.Height > longSide {
		longSide = meta.Height
	}

	highResThreshold := qualitySettings.GIFDownshiftHighResLongSideThreshold
	if highResThreshold <= 0 {
		highResThreshold = 1600
	}
	longDurationThreshold := qualitySettings.GIFDurationTierLongSec
	if longDurationThreshold <= 0 {
		longDurationThreshold = 120
	}
	earlyDurationThreshold := qualitySettings.GIFDownshiftEarlyDurationSec
	if earlyDurationThreshold <= 0 {
		earlyDurationThreshold = 45
	}
	if longSide >= highResThreshold || meta.DurationSec >= longDurationThreshold {
		// 高分辨率/长视频优先稳定，减少同时在飞的总成本。
		stableMultiplier := clampFloat(qualitySettings.GIFRenderBudgetLongMultiplier*1.15, 1.0, 2.2)
		maxUnits = math.Max(1, float64(workers)*stableMultiplier)
	}

	gifTasks := 0
	for _, task := range tasks {
		if strings.EqualFold(strings.TrimSpace(task.Format), "gif") {
			gifTasks++
		}
	}
	if gifTasks >= 3 && meta.DurationSec > 0 && meta.DurationSec < earlyDurationThreshold && longSide < highResThreshold {
		// 短视频优先吞吐：放宽 token 上限，避免 4 worker 仅跑成 2 并发。
		maxUnits *= 1.25
	}
	if gifTasks <= 1 {
		maxUnits = math.Max(maxUnits, 1)
	}
	maxUnits = clampFloat(maxUnits, 1, float64(workers)*4.0)
	return roundTo(maxUnits, 3)
}

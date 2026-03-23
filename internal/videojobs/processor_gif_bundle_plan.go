package videojobs

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type gifRenderBundlePlan struct {
	BundleID    string
	StartSec    float64
	EndSec      float64
	TaskIndexes []int
}

type gifBundleRuntimeConfig struct {
	BundleEnabled       bool
	MezzanineEnabled    bool
	BundleMergeGapSec   float64
	BundleMaxSpanSec    float64
	MezzanineMinWindows int
	MezzanineCRF        int
	MezzaninePreset     string
}

func (p *Processor) resolveGIFBundleRuntimeConfig() gifBundleRuntimeConfig {
	cfg := gifBundleRuntimeConfig{
		BundleEnabled:       false,
		MezzanineEnabled:    false,
		BundleMergeGapSec:   0.8,
		BundleMaxSpanSec:    12,
		MezzanineMinWindows: 2,
		MezzanineCRF:        18,
		MezzaninePreset:     "veryfast",
	}
	if p == nil {
		return cfg
	}
	cfg.BundleEnabled = p.cfg.GIFBundleEnabled
	cfg.MezzanineEnabled = p.cfg.GIFMezzanineEnabled
	if p.cfg.GIFBundleMergeGapMS > 0 {
		cfg.BundleMergeGapSec = float64(p.cfg.GIFBundleMergeGapMS) / 1000.0
	}
	if p.cfg.GIFBundleMaxSpanSec > 0 {
		cfg.BundleMaxSpanSec = float64(p.cfg.GIFBundleMaxSpanSec)
	}
	if p.cfg.GIFMezzanineMinWindows > 0 {
		cfg.MezzanineMinWindows = p.cfg.GIFMezzanineMinWindows
	}
	if p.cfg.GIFMezzanineCRF > 0 {
		cfg.MezzanineCRF = p.cfg.GIFMezzanineCRF
	}
	if strings.TrimSpace(p.cfg.GIFMezzaninePreset) != "" {
		cfg.MezzaninePreset = strings.TrimSpace(p.cfg.GIFMezzaninePreset)
	}

	cfg.BundleMergeGapSec = clampFloat(cfg.BundleMergeGapSec, 0, 3)
	cfg.BundleMaxSpanSec = clampFloat(cfg.BundleMaxSpanSec, 3, 30)
	if cfg.MezzanineMinWindows < 2 {
		cfg.MezzanineMinWindows = 2
	}
	if cfg.MezzanineMinWindows > 10 {
		cfg.MezzanineMinWindows = 10
	}
	if cfg.MezzanineCRF < 12 {
		cfg.MezzanineCRF = 12
	}
	if cfg.MezzanineCRF > 30 {
		cfg.MezzanineCRF = 30
	}
	return cfg
}

func buildGIFRenderBundlePlan(tasks []animatedTask, config gifBundleRuntimeConfig) []gifRenderBundlePlan {
	if !config.BundleEnabled || len(tasks) < 2 {
		return nil
	}

	type bundleTaskRef struct {
		TaskIndex int
		StartSec  float64
		EndSec    float64
	}
	refs := make([]bundleTaskRef, 0, len(tasks))
	for idx, task := range tasks {
		if !strings.EqualFold(strings.TrimSpace(task.Format), "gif") {
			continue
		}
		startSec := task.Window.StartSec
		endSec := task.Window.EndSec
		if endSec <= startSec {
			continue
		}
		refs = append(refs, bundleTaskRef{
			TaskIndex: idx,
			StartSec:  startSec,
			EndSec:    endSec,
		})
	}
	if len(refs) < 2 {
		return nil
	}

	sort.SliceStable(refs, func(i, j int) bool {
		if math.Abs(refs[i].StartSec-refs[j].StartSec) < 1e-6 {
			if math.Abs(refs[i].EndSec-refs[j].EndSec) < 1e-6 {
				return refs[i].TaskIndex < refs[j].TaskIndex
			}
			return refs[i].EndSec < refs[j].EndSec
		}
		return refs[i].StartSec < refs[j].StartSec
	})

	bundles := make([]gifRenderBundlePlan, 0, len(refs)/2+1)
	current := gifRenderBundlePlan{
		StartSec:    refs[0].StartSec,
		EndSec:      refs[0].EndSec,
		TaskIndexes: []int{refs[0].TaskIndex},
	}
	for i := 1; i < len(refs); i++ {
		item := refs[i]
		gapSec := item.StartSec - current.EndSec
		if gapSec < 0 {
			gapSec = 0
		}
		nextStart := current.StartSec
		nextEnd := current.EndSec
		if item.EndSec > nextEnd {
			nextEnd = item.EndSec
		}
		spanSec := nextEnd - nextStart
		if gapSec <= config.BundleMergeGapSec && spanSec <= config.BundleMaxSpanSec {
			current.EndSec = nextEnd
			current.TaskIndexes = append(current.TaskIndexes, item.TaskIndex)
			continue
		}
		if len(current.TaskIndexes) >= 2 {
			bundles = append(bundles, current)
		}
		current = gifRenderBundlePlan{
			StartSec:    item.StartSec,
			EndSec:      item.EndSec,
			TaskIndexes: []int{item.TaskIndex},
		}
	}
	if len(current.TaskIndexes) >= 2 {
		bundles = append(bundles, current)
	}

	for idx := range bundles {
		bundles[idx].BundleID = fmt.Sprintf("gif_bundle_%03d", idx+1)
	}
	return bundles
}

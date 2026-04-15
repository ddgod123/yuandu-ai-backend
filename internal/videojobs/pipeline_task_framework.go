package videojobs

import (
	"strings"
	"time"
)

type pipelineTaskFramework struct {
	p                 *Processor
	jobID             uint64
	metrics           map[string]interface{}
	stageStatusMetric string
	stageStatus       map[string]string
	observability     *pipelineObservability
}

type pipelineTaskStageOptions struct {
	StageNames     []string
	JobStage       string
	Progress       int
	EventStage     string
	StartMessage   string
	StartLevel     string
	StartMetadata  map[string]interface{}
	FailureStage   string
	FailureMessage string
	FailureLevel   string
}

func newPipelineTaskFramework(
	p *Processor,
	jobID uint64,
	metrics map[string]interface{},
	stageStatusMetric string,
	stageStatus map[string]string,
) *pipelineTaskFramework {
	fw := &pipelineTaskFramework{
		p:                 p,
		jobID:             jobID,
		metrics:           metrics,
		stageStatusMetric: strings.TrimSpace(stageStatusMetric),
		stageStatus:       stageStatus,
	}
	if fw.metrics == nil {
		fw.metrics = map[string]interface{}{}
	}
	fw.observability = newPipelineObservability(fw.metrics)
	if fw.stageStatus != nil && fw.stageStatusMetric != "" {
		fw.metrics[fw.stageStatusMetric] = fw.stageStatus
	}
	return fw
}

func (f *pipelineTaskFramework) setMetric(key string, value interface{}) {
	if f == nil || f.metrics == nil {
		return
	}
	k := strings.TrimSpace(key)
	if k == "" {
		return
	}
	f.metrics[k] = value
}

func (f *pipelineTaskFramework) mergeMetrics(values map[string]interface{}) {
	if f == nil || f.metrics == nil || len(values) == 0 {
		return
	}
	for key, value := range values {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		f.metrics[k] = value
	}
}

func (f *pipelineTaskFramework) markStageStatus(stage, status string) {
	if f == nil || f.stageStatus == nil {
		return
	}
	stageKey := strings.TrimSpace(stage)
	if stageKey == "" {
		return
	}
	statusValue := strings.ToLower(strings.TrimSpace(status))
	if statusValue == "" {
		statusValue = "pending"
	}
	f.stageStatus[stageKey] = statusValue
	if f.metrics != nil && f.stageStatusMetric != "" {
		f.metrics[f.stageStatusMetric] = f.stageStatus
	}
}

func (f *pipelineTaskFramework) markStageStatuses(status string, stages ...string) {
	if f == nil || len(stages) == 0 {
		return
	}
	for _, stage := range stages {
		f.markStageStatus(stage, status)
	}
}

func (f *pipelineTaskFramework) updateStageProgress(stage string, progress int, extra map[string]interface{}) {
	if f == nil || f.p == nil || f.jobID == 0 {
		return
	}
	updates := map[string]interface{}{}
	if stageText := strings.TrimSpace(stage); stageText != "" {
		updates["stage"] = stageText
	}
	if progress >= 0 {
		updates["progress"] = progress
	}
	for key, value := range extra {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		updates[k] = value
	}
	if f.metrics != nil {
		updates["metrics"] = mustJSON(f.metrics)
	}
	f.p.updateVideoJob(f.jobID, updates)
}

func (f *pipelineTaskFramework) appendEvent(stage, level, message string, metadata map[string]interface{}) {
	if f == nil || f.p == nil || f.jobID == 0 {
		return
	}
	lv := strings.TrimSpace(strings.ToLower(level))
	if lv == "" {
		lv = "info"
	}
	f.p.appendJobEvent(
		f.jobID,
		strings.TrimSpace(stage),
		lv,
		strings.TrimSpace(message),
		cloneMapStringKey(metadata),
	)
}

func (f *pipelineTaskFramework) recordFallback(key string, hit bool, reason string) {
	if f == nil || f.observability == nil {
		return
	}
	f.observability.recordFallback(key, hit, reason)
}

func (f *pipelineTaskFramework) recordFailure(stage, reason string, err error) {
	if f == nil || f.observability == nil {
		return
	}
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	f.observability.recordFailure(stage, reason, errText)
}

func (f *pipelineTaskFramework) runStage(options pipelineTaskStageOptions, run func() error) error {
	if f == nil {
		if run == nil {
			return nil
		}
		return run()
	}
	stageNames := normalizePipelineStageNames(options.StageNames)
	if len(stageNames) > 0 {
		f.markStageStatuses("running", stageNames...)
	}
	if strings.TrimSpace(options.JobStage) != "" || options.Progress >= 0 {
		f.updateStageProgress(options.JobStage, options.Progress, nil)
	}
	if strings.TrimSpace(options.StartMessage) != "" {
		f.appendEvent(
			firstNonEmptyString(options.EventStage, options.JobStage),
			firstNonEmptyString(options.StartLevel, "info"),
			options.StartMessage,
			options.StartMetadata,
		)
	}

	if run == nil {
		for _, stage := range stageNames {
			if strings.EqualFold(strings.TrimSpace(f.stageStatus[stage]), "running") {
				f.markStageStatus(stage, "done")
			}
			if f.observability != nil {
				f.observability.recordStage(stage, time.Now(), "done")
			}
		}
		return nil
	}

	started := time.Now()
	observedStages := append([]string{}, stageNames...)
	if len(observedStages) == 0 {
		if stageText := strings.TrimSpace(options.JobStage); stageText != "" {
			observedStages = append(observedStages, stageText)
		}
	}
	if err := run(); err != nil {
		for _, stage := range stageNames {
			f.markStageStatus(stage, "failed")
		}
		stageNameText := strings.Join(stageNames, ",")
		failureMetadata := cloneMapStringKey(options.StartMetadata)
		if failureMetadata == nil {
			failureMetadata = map[string]interface{}{}
		}
		failureMetadata["error"] = err.Error()
		failureMetadata["duration_ms"] = clampDurationMillis(started)
		if stageNameText != "" {
			failureMetadata["sub_stage"] = stageNameText
		}
		failMsg := strings.TrimSpace(options.FailureMessage)
		if failMsg == "" {
			if stageNameText != "" {
				failMsg = stageNameText + " failed"
			} else {
				failMsg = "pipeline stage failed"
			}
		}
		f.appendEvent(
			firstNonEmptyString(options.FailureStage, options.EventStage, options.JobStage),
			firstNonEmptyString(options.FailureLevel, "warn"),
			failMsg,
			failureMetadata,
		)
		for _, stage := range observedStages {
			if f.observability != nil {
				f.observability.recordStage(stage, started, "failed")
			}
			f.recordFailure(stage, classifyPipelineFailureReason(err.Error()), err)
		}
		return err
	}

	for _, stage := range stageNames {
		if strings.EqualFold(strings.TrimSpace(f.stageStatus[stage]), "running") {
			f.markStageStatus(stage, "done")
		}
		if f.observability != nil {
			f.observability.recordStage(stage, started, "done")
		}
	}
	if len(stageNames) == 0 {
		for _, stage := range observedStages {
			if f.observability != nil {
				f.observability.recordStage(stage, started, "done")
			}
		}
	}
	return nil
}

func normalizePipelineStageNames(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		key := strings.TrimSpace(item)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

type pipelineSubStageTracker struct {
	metrics       map[string]interface{}
	metricKey     string
	stageDetails  map[string]map[string]interface{}
	observability *pipelineObservability
}

func newPipelineSubStageTracker(
	metrics map[string]interface{},
	metricKey string,
	seed map[string]map[string]interface{},
) *pipelineSubStageTracker {
	stageDetails := seed
	if stageDetails == nil {
		stageDetails = map[string]map[string]interface{}{}
	}
	tracker := &pipelineSubStageTracker{
		metrics:       metrics,
		metricKey:     strings.TrimSpace(metricKey),
		stageDetails:  stageDetails,
		observability: newPipelineObservability(metrics),
	}
	tracker.syncMetric()
	return tracker
}

func (t *pipelineSubStageTracker) syncMetric() {
	if t == nil || t.metrics == nil || t.metricKey == "" {
		return
	}
	t.metrics[t.metricKey] = t.stageDetails
}

func (t *pipelineSubStageTracker) Start(name string, detail map[string]interface{}) time.Time {
	started := time.Now()
	if t == nil {
		return started
	}
	stageName := strings.TrimSpace(name)
	if stageName == "" {
		return started
	}
	stageDetail := map[string]interface{}{
		"status":      "running",
		"started_at":  started.Format(time.RFC3339),
		"finished_at": "",
		"duration_ms": int64(0),
	}
	for key, value := range detail {
		stageDetail[key] = value
	}
	t.stageDetails[stageName] = stageDetail
	t.syncMetric()
	return started
}

func (t *pipelineSubStageTracker) Done(name string, started time.Time, status string, detail map[string]interface{}) {
	if t == nil {
		return
	}
	stageName := strings.TrimSpace(name)
	if stageName == "" {
		return
	}
	finalStatus := strings.ToLower(strings.TrimSpace(status))
	if finalStatus == "" {
		finalStatus = "done"
	}
	stageDetail := t.stageDetails[stageName]
	if stageDetail == nil {
		stageDetail = map[string]interface{}{}
	}
	if _, ok := stageDetail["started_at"]; !ok {
		if !started.IsZero() {
			stageDetail["started_at"] = started.Format(time.RFC3339)
		} else {
			stageDetail["started_at"] = time.Now().Format(time.RFC3339)
		}
	}
	stageDetail["status"] = finalStatus
	stageDetail["finished_at"] = time.Now().Format(time.RFC3339)
	if !started.IsZero() {
		stageDetail["duration_ms"] = clampDurationMillis(started)
	}
	for key, value := range detail {
		stageDetail[key] = value
	}
	t.stageDetails[stageName] = stageDetail
	t.syncMetric()
	if t.observability != nil {
		t.observability.recordStage(stageName, started, finalStatus)
		switch finalStatus {
		case "degraded":
			reason := strings.TrimSpace(stringFromAny(mapFromAny(detail)["reason"]))
			if reason == "" {
				reason = "degraded"
			}
			t.observability.recordFallback("gif_sub_stage_"+stageName, true, reason)
		case "done":
			t.observability.recordFallback("gif_sub_stage_"+stageName, false, "")
		case "failed":
			errText := strings.TrimSpace(stringFromAny(mapFromAny(detail)["error"]))
			t.observability.recordFailure(stageName, classifyPipelineFailureReason(errText), errText)
		}
	}
}

func (t *pipelineSubStageTracker) Skip(name, reason string) {
	if t == nil {
		return
	}
	stageName := strings.TrimSpace(name)
	if stageName == "" {
		return
	}
	stageDetail := t.stageDetails[stageName]
	if stageDetail == nil {
		stageDetail = map[string]interface{}{}
	}
	stageDetail["status"] = "skipped"
	stageDetail["reason"] = strings.TrimSpace(reason)
	if _, ok := stageDetail["started_at"]; !ok {
		stageDetail["started_at"] = ""
	}
	if _, ok := stageDetail["finished_at"]; !ok {
		stageDetail["finished_at"] = ""
	}
	if _, ok := stageDetail["duration_ms"]; !ok {
		stageDetail["duration_ms"] = int64(0)
	}
	t.stageDetails[stageName] = stageDetail
	t.syncMetric()
}

func (t *pipelineSubStageTracker) HasFinalStatus(name string) bool {
	if t == nil {
		return false
	}
	stageName := strings.TrimSpace(name)
	if stageName == "" {
		return false
	}
	stageDetail := t.stageDetails[stageName]
	if stageDetail == nil {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(stringFromAny(stageDetail["status"])))
	switch status {
	case "done", "degraded", "failed", "skipped":
		return true
	default:
		return false
	}
}

func (t *pipelineSubStageTracker) StatusSnapshot() map[string]string {
	out := map[string]string{}
	if t == nil {
		return out
	}
	for name, detail := range t.stageDetails {
		key := strings.TrimSpace(name)
		if key == "" {
			continue
		}
		out[key] = strings.ToLower(strings.TrimSpace(stringFromAny(mapFromAny(detail)["status"])))
	}
	return out
}

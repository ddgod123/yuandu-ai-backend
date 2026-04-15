package videojobs

import (
	"strings"
	"time"
)

const pipelineObservabilityMetricKey = "pipeline_observability_v1"

type pipelineObservability struct {
	metrics map[string]interface{}
}

func newPipelineObservability(metrics map[string]interface{}) *pipelineObservability {
	if metrics == nil {
		return nil
	}
	obs := &pipelineObservability{metrics: metrics}
	_ = obs.ensureRoot()
	return obs
}

func (o *pipelineObservability) ensureRoot() map[string]interface{} {
	if o == nil || o.metrics == nil {
		return nil
	}
	root := mapFromAny(o.metrics[pipelineObservabilityMetricKey])
	if root == nil {
		root = map[string]interface{}{}
	}
	if strings.TrimSpace(stringFromAny(root["schema_version"])) == "" {
		root["schema_version"] = "v1"
	}
	ensureObservabilityMap(root, "stage_duration_ms")
	ensureObservabilityMap(root, "stage_runs")
	ensureObservabilityMap(root, "stage_failures")
	ensureObservabilityMap(root, "stage_last_status")
	ensureObservabilityMap(root, "stage_last_duration_ms")
	fallback := ensureObservabilityMap(root, "fallback")
	if intFromAny(fallback["total"]) < 0 {
		fallback["total"] = 0
	}
	if intFromAny(fallback["hit"]) < 0 {
		fallback["hit"] = 0
	}
	ensureObservabilityMap(fallback, "by_key")
	failure := ensureObservabilityMap(root, "failure_profile")
	if intFromAny(failure["total"]) < 0 {
		failure["total"] = 0
	}
	ensureObservabilityMap(failure, "by_stage")
	ensureObservabilityMap(failure, "by_reason")
	ensureObservabilityMap(failure, "by_stage_reason")
	root["updated_at"] = time.Now().Format(time.RFC3339)
	o.metrics[pipelineObservabilityMetricKey] = root
	return root
}

func ensureObservabilityMap(parent map[string]interface{}, key string) map[string]interface{} {
	if parent == nil {
		return nil
	}
	k := strings.TrimSpace(key)
	if k == "" {
		return nil
	}
	out := mapFromAny(parent[k])
	if out == nil {
		out = map[string]interface{}{}
	}
	parent[k] = out
	return out
}

func (o *pipelineObservability) recordStage(stage string, started time.Time, status string) {
	root := o.ensureRoot()
	if root == nil {
		return
	}
	stage = strings.TrimSpace(stage)
	if stage == "" {
		return
	}
	status = strings.ToLower(strings.TrimSpace(status))
	if status == "" {
		status = "done"
	}
	durationMs := int64(0)
	if !started.IsZero() {
		durationMs = clampDurationMillis(started)
	}

	stageDurations := ensureObservabilityMap(root, "stage_duration_ms")
	stageDurations[stage] = sourceInt64FromAny(stageDurations[stage]) + durationMs

	stageRuns := ensureObservabilityMap(root, "stage_runs")
	stageRuns[stage] = intFromAny(stageRuns[stage]) + 1

	stageLastStatus := ensureObservabilityMap(root, "stage_last_status")
	stageLastStatus[stage] = status

	stageLastDuration := ensureObservabilityMap(root, "stage_last_duration_ms")
	stageLastDuration[stage] = durationMs

	if status == "failed" {
		stageFailures := ensureObservabilityMap(root, "stage_failures")
		stageFailures[stage] = intFromAny(stageFailures[stage]) + 1
	}

	root["updated_at"] = time.Now().Format(time.RFC3339)
	o.metrics[pipelineObservabilityMetricKey] = root
}

func (o *pipelineObservability) recordFallback(key string, hit bool, reason string) {
	root := o.ensureRoot()
	if root == nil {
		return
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	reason = strings.TrimSpace(reason)

	fallback := ensureObservabilityMap(root, "fallback")
	total := intFromAny(fallback["total"]) + 1
	hits := intFromAny(fallback["hit"])
	if hit {
		hits++
	}
	fallback["total"] = total
	fallback["hit"] = hits
	if total > 0 {
		fallback["rate"] = roundTo(float64(hits)/float64(total), 4)
	}

	byKey := ensureObservabilityMap(fallback, "by_key")
	entry := ensureObservabilityMap(byKey, key)
	entryTotal := intFromAny(entry["total"]) + 1
	entryHit := intFromAny(entry["hit"])
	if hit {
		entryHit++
	}
	entry["total"] = entryTotal
	entry["hit"] = entryHit
	if entryTotal > 0 {
		entry["rate"] = roundTo(float64(entryHit)/float64(entryTotal), 4)
	}
	entry["last_hit"] = hit
	if reason != "" {
		entry["last_reason"] = reason
	}
	entry["updated_at"] = time.Now().Format(time.RFC3339)
	byKey[key] = entry
	fallback["by_key"] = byKey
	root["fallback"] = fallback
	root["updated_at"] = time.Now().Format(time.RFC3339)
	o.metrics[pipelineObservabilityMetricKey] = root
}

func classifyPipelineFailureReason(errText string) string {
	msg := strings.ToLower(strings.TrimSpace(errText))
	if msg == "" {
		return "runtime_error"
	}
	switch {
	case strings.Contains(msg, "contract"):
		return "contract_invalid"
	case strings.Contains(msg, "parse"):
		return "parse_error"
	case strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "cancel"):
		return "cancelled"
	case strings.Contains(msg, "no frames"):
		return "empty_frames"
	case strings.Contains(msg, "not found"):
		return "not_found"
	case strings.Contains(msg, "ffmpeg"), strings.Contains(msg, "ffprobe"):
		return "tooling_error"
	case strings.Contains(msg, "download"):
		return "download_error"
	case strings.Contains(msg, "qiniu"), strings.Contains(msg, "upload"):
		return "storage_error"
	default:
		return "runtime_error"
	}
}

func (o *pipelineObservability) recordFailure(stage, reason, errText string) {
	root := o.ensureRoot()
	if root == nil {
		return
	}
	stage = strings.TrimSpace(stage)
	if stage == "" {
		stage = "unknown"
	}
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		reason = classifyPipelineFailureReason(errText)
	}
	failure := ensureObservabilityMap(root, "failure_profile")
	total := intFromAny(failure["total"]) + 1
	failure["total"] = total

	byStage := ensureObservabilityMap(failure, "by_stage")
	byStage[stage] = intFromAny(byStage[stage]) + 1

	byReason := ensureObservabilityMap(failure, "by_reason")
	byReason[reason] = intFromAny(byReason[reason]) + 1

	byStageReason := ensureObservabilityMap(failure, "by_stage_reason")
	composite := stage + ":" + reason
	byStageReason[composite] = intFromAny(byStageReason[composite]) + 1

	last := map[string]interface{}{
		"stage":       stage,
		"reason":      reason,
		"error":       strings.TrimSpace(errText),
		"occurred_at": time.Now().Format(time.RFC3339),
	}
	failure["last"] = last
	failure["by_stage"] = byStage
	failure["by_reason"] = byReason
	failure["by_stage_reason"] = byStageReason
	root["failure_profile"] = failure
	root["updated_at"] = time.Now().Format(time.RFC3339)
	o.metrics[pipelineObservabilityMetricKey] = root
}

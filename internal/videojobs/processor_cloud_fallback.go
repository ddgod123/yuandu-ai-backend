package videojobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func extractCloudHighlightUsage(parsed cloudHighlightResponse, body []byte) *cloudHighlightUsage {
	if parsed.Usage != nil {
		usage := normalizeCloudHighlightUsage(*parsed.Usage)
		return &usage
	}
	if len(body) == 0 {
		return nil
	}
	var raw cloudHighlightGenericResponse
	if err := json.Unmarshal(body, &raw); err != nil || raw.Usage == nil {
		return nil
	}
	usage := normalizeCloudHighlightUsage(*raw.Usage)
	return &usage
}

func (p *Processor) requestCloudHighlightFallback(
	ctx context.Context,
	jobID uint64,
	userID uint64,
	sourcePath string,
	meta videoProbeMeta,
	local highlightSuggestion,
	qualitySettings QualitySettings,
) (highlightSuggestion, error) {
	suggestion := highlightSuggestion{
		Version:  "cloud_v1",
		Strategy: "cloud_fallback_service",
	}
	qualitySettings = NormalizeQualitySettings(qualitySettings)
	cfg := p.loadCloudHighlightFallbackConfig()
	startedAt := time.Now()
	usageInput := videoJobAIUsageInput{
		JobID:    jobID,
		UserID:   userID,
		Stage:    "planner_fallback",
		Provider: detectCloudFallbackProvider(cfg.URL, p),
		Model:    detectCloudFallbackModel(p),
		Endpoint: cfg.URL,
		Metadata: map[string]interface{}{
			"top_n":            qualitySettings.GIFCandidateMaxOutputs,
			"scene_threshold":  cfg.Threshold,
			"local_candidates": len(local.Candidates),
		},
	}
	finishUsage := func(status, errMessage string, usage *cloudHighlightUsage, extra map[string]interface{}) {
		metadata := normalizeVideoJobAIUsageMetadata(usageInput.Metadata)
		for key, value := range extra {
			metadata[key] = value
		}
		usageInput.RequestDurationMs = clampDurationMillis(startedAt)
		usageInput.RequestStatus = strings.TrimSpace(status)
		usageInput.RequestError = strings.TrimSpace(errMessage)
		if usage != nil {
			normalizedUsage := normalizeCloudHighlightUsage(*usage)
			usageInput.InputTokens = normalizedUsage.InputTokens
			usageInput.OutputTokens = normalizedUsage.OutputTokens
			usageInput.CachedInputTokens = normalizedUsage.CachedInputTokens
			usageInput.ImageTokens = normalizedUsage.ImageTokens
			usageInput.VideoTokens = normalizedUsage.VideoTokens
			usageInput.AudioSeconds = normalizedUsage.AudioSeconds
			metadata["usage"] = map[string]interface{}{
				"input_tokens":        normalizedUsage.InputTokens,
				"output_tokens":       normalizedUsage.OutputTokens,
				"cached_input_tokens": normalizedUsage.CachedInputTokens,
				"image_tokens":        normalizedUsage.ImageTokens,
				"video_tokens":        normalizedUsage.VideoTokens,
				"audio_seconds":       roundTo(normalizedUsage.AudioSeconds, 3),
			}
		}
		usageInput.Metadata = metadata
		p.recordVideoJobAIUsage(usageInput)
	}

	if !cfg.Enabled {
		finishUsage("disabled", "cloud highlight fallback disabled", nil, nil)
		return suggestion, errors.New("cloud highlight fallback disabled")
	}

	topN := qualitySettings.GIFCandidateMaxOutputs
	if cfg.TopN > 0 {
		topN = cfg.TopN
	}
	if topN <= 0 {
		topN = defaultHighlightTopN
	}
	if topN > maxGIFCandidateOutputs {
		topN = maxGIFCandidateOutputs
	}

	scenePoints, err := detectScenePoints(ctx, sourcePath, cfg.Threshold)
	if err != nil {
		finishUsage("error", "collect scene points: "+err.Error(), nil, nil)
		return suggestion, fmt.Errorf("collect scene points: %w", err)
	}
	targetDuration := 2.6
	if local.Selected != nil {
		duration := local.Selected.EndSec - local.Selected.StartSec
		if duration > 0 {
			targetDuration = clampFloat(duration, 1.2, 4.0)
		}
	}

	reqPayload := cloudHighlightRequest{
		DurationSec:     roundTo(meta.DurationSec, 3),
		Width:           meta.Width,
		Height:          meta.Height,
		FPS:             roundTo(meta.FPS, 3),
		TargetDuration:  roundTo(targetDuration, 3),
		TopN:            topN,
		SceneThreshold:  cfg.Threshold,
		ScenePoints:     scenePoints,
		LocalCandidates: local.Candidates,
	}
	rawReq, _ := json.Marshal(reqPayload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(rawReq))
	if err != nil {
		finishUsage("error", err.Error(), nil, nil)
		return suggestion, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if cfg.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+cfg.Token)
	}

	client := p.httpClient
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	} else {
		client = &http.Client{
			Transport: client.Transport,
			Timeout:   cfg.Timeout,
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		finishUsage("error", err.Error(), nil, map[string]interface{}{"transport_error": true})
		return suggestion, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		finishUsage("error", fmt.Sprintf("http %d", resp.StatusCode), nil, map[string]interface{}{
			"http_status": resp.StatusCode,
			"response":    strings.TrimSpace(string(body)),
		})
		return suggestion, fmt.Errorf("cloud highlight http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed cloudHighlightResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		finishUsage("error", "decode response: "+err.Error(), nil, map[string]interface{}{
			"http_status": resp.StatusCode,
			"response":    strings.TrimSpace(string(body)),
		})
		return suggestion, fmt.Errorf("decode cloud highlight response: %w", err)
	}
	usage := extractCloudHighlightUsage(parsed, body)

	candidates := make([]highlightCandidate, 0, len(parsed.Candidates))
	for _, item := range parsed.Candidates {
		start, end := clampHighlightWindow(item.StartSec, item.EndSec, meta.DurationSec)
		if end <= start {
			continue
		}
		score := item.Score
		if score <= 0 {
			score = 0.4
		}
		if score > 1 {
			score = 1
		}
		reason := strings.TrimSpace(item.Reason)
		if reason == "" {
			reason = "cloud_fallback"
		}
		candidates = append(candidates, highlightCandidate{
			StartSec:   start,
			EndSec:     end,
			Score:      score,
			SceneScore: item.SceneScore,
			Reason:     reason,
		})
	}
	if len(candidates) == 0 {
		finishUsage("error", "cloud highlight response has no valid candidates", usage, map[string]interface{}{
			"http_status":      resp.StatusCode,
			"candidate_count":  0,
			"selected_present": parsed.Selected != nil,
		})
		return suggestion, errors.New("cloud highlight response has no valid candidates")
	}

	selected := pickNonOverlapCandidates(candidates, topN, qualitySettings.GIFCandidateDedupIOUThreshold)
	selected = applyGIFCandidateConfidenceThreshold(selected, candidates, qualitySettings.GIFCandidateConfidenceThreshold)
	if len(selected) == 0 {
		selected = candidates
	}
	if len(selected) > topN {
		selected = selected[:topN]
	}
	suggestion.Candidates = selected
	suggestion.All = candidates
	suggestion.Selected = &selected[0]
	finishUsage("ok", "", usage, map[string]interface{}{
		"http_status":      resp.StatusCode,
		"candidate_count":  len(candidates),
		"selected_count":   len(selected),
		"selected_present": suggestion.Selected != nil,
	})
	return suggestion, nil
}

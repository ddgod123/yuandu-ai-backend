package videojobs

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"sort"
	"strings"
)

func inferSceneTags(title, sourceKey string, formats []string) []string {
	text := strings.ToLower(strings.TrimSpace(title + " " + sourceKey))
	if text == "" && len(formats) == 0 {
		return nil
	}

	tagSet := map[string]struct{}{}
	addTag := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		tagSet[tag] = struct{}{}
	}

	containsAny := func(keywords ...string) bool {
		for _, kw := range keywords {
			kw = strings.TrimSpace(strings.ToLower(kw))
			if kw == "" {
				continue
			}
			if strings.Contains(text, kw) {
				return true
			}
		}
		return false
	}

	if containsAny("猫", "狗", "pet", "puppy", "kitten", "宠物", "dog", "cat") {
		addTag("pet")
	}
	if containsAny("探店", "店", "餐厅", "咖啡", "美食", "vlog", "store", "restaurant", "food") {
		addTag("explore")
	}
	if containsAny("搞笑", "笑", "funny", "meme", "reaction", "整活", "沙雕") {
		addTag("funny")
	}
	if containsAny("教程", "讲解", "教学", "lesson", "how to", "guide", "review") {
		addTag("knowledge")
	}
	if containsAny("纪录片", "采访", "新闻", "doc", "interview", "news") {
		addTag("documentary")
	}

	for _, format := range formats {
		switch strings.ToLower(strings.TrimSpace(format)) {
		case "gif", "webp":
			addTag("social")
		case "live":
			addTag("live_creator")
		case "png", "jpg":
			addTag("design")
		}
	}

	if len(tagSet) == 0 {
		addTag("general")
	}

	out := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func detectScenePoints(ctx context.Context, sourcePath string, threshold float64) ([]scenePoint, error) {
	if threshold <= 0 {
		threshold = 0.10
	}
	filter := fmt.Sprintf("select=gt(scene\\,%s),metadata=print:file=-", formatFFmpegNumber(threshold))
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-i", sourcePath,
		"-vf", filter,
		"-an",
		"-f", "null",
		"-",
	}
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	points := parseSceneMetadataOutput(out)
	if err != nil && len(points) == 0 {
		return nil, fmt.Errorf("scene scoring failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return points, nil
}

func parseSceneMetadataOutput(raw []byte) []scenePoint {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	points := make([]scenePoint, 0)
	pendingPts := -1.0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "frame:") {
			idx := strings.Index(line, "pts_time:")
			if idx >= 0 {
				pendingPts = parseLooseFloat(line[idx+len("pts_time:"):])
			}
			continue
		}
		if strings.HasPrefix(line, "lavfi.scene_score=") {
			score := parseLooseFloat(strings.TrimPrefix(line, "lavfi.scene_score="))
			if pendingPts >= 0 && score > 0 {
				points = append(points, scenePoint{
					PtsSec: pendingPts,
					Score:  score,
				})
			}
			pendingPts = -1
		}
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].Score == points[j].Score {
			return points[i].PtsSec < points[j].PtsSec
		}
		return points[i].Score > points[j].Score
	})
	return points
}

func buildFallbackHighlightCandidates(durationSec, targetDuration float64) []highlightCandidate {
	if durationSec <= 0 {
		return nil
	}
	if targetDuration <= 0 {
		targetDuration = chooseHighlightDuration(durationSec)
	}

	anchors := []float64{durationSec * 0.50, durationSec * 0.25, durationSec * 0.75}
	candidates := make([]highlightCandidate, 0, len(anchors))
	for idx, anchor := range anchors {
		start := anchor - targetDuration/2
		end := start + targetDuration
		start, end = clampHighlightWindow(start, end, durationSec)
		if end-start < 0.8 {
			continue
		}
		score := 0.45 - float64(idx)*0.05
		candidates = append(candidates, highlightCandidate{
			StartSec: start,
			EndSec:   end,
			Score:    roundTo(score, 4),
			Reason:   "fallback_uniform",
		})
	}
	return candidates
}

func chooseHighlightDuration(durationSec float64) float64 {
	if durationSec <= 0 {
		return 3
	}
	if durationSec < 2 {
		return durationSec
	}
	if durationSec <= 8 {
		return math.Max(1.6, durationSec*0.45)
	}
	if durationSec <= 30 {
		return 3.2
	}
	return 4.0
}

func clampHighlightWindow(startSec, endSec, durationSec float64) (float64, float64) {
	if durationSec <= 0 {
		return 0, 0
	}
	if startSec < 0 {
		startSec = 0
	}
	if endSec <= startSec {
		endSec = startSec + 1.2
	}
	if endSec > durationSec {
		overflow := endSec - durationSec
		endSec = durationSec
		startSec -= overflow
		if startSec < 0 {
			startSec = 0
		}
	}
	if endSec <= startSec {
		endSec = durationSec
	}
	return roundTo(startSec, 3), roundTo(endSec, 3)
}

func pickNonOverlapCandidates(candidates []highlightCandidate, topN int, iouThreshold float64) []highlightCandidate {
	if len(candidates) == 0 || topN <= 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score == candidates[j].Score {
			if candidates[i].StartSec == candidates[j].StartSec {
				return candidates[i].EndSec < candidates[j].EndSec
			}
			return candidates[i].StartSec < candidates[j].StartSec
		}
		return candidates[i].Score > candidates[j].Score
	})

	out := make([]highlightCandidate, 0, topN)
	for _, cand := range candidates {
		overlap := false
		for _, picked := range out {
			if windowIoU(cand.StartSec, cand.EndSec, picked.StartSec, picked.EndSec) > iouThreshold {
				overlap = true
				break
			}
		}
		if overlap {
			continue
		}
		out = append(out, cand)
		if len(out) >= topN {
			break
		}
	}
	return out
}

func windowIoU(startA, endA, startB, endB float64) float64 {
	interStart := math.Max(startA, startB)
	interEnd := math.Min(endA, endB)
	if interEnd <= interStart {
		return 0
	}
	intersection := interEnd - interStart
	union := (endA - startA) + (endB - startB) - intersection
	if union <= 0 {
		return 0
	}
	return intersection / union
}

func roundTo(v float64, digits int) float64 {
	if digits <= 0 {
		return math.Round(v)
	}
	base := math.Pow(10, float64(digits))
	return math.Round(v*base) / base
}

func roundFloatSlice(in []float64, digits int) []float64 {
	if len(in) == 0 {
		return nil
	}
	out := make([]float64, 0, len(in))
	for _, item := range in {
		out = append(out, roundTo(item, digits))
	}
	return out
}

func averageFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	count := 0
	for _, item := range values {
		if item <= 0 {
			continue
		}
		total += item
		count++
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

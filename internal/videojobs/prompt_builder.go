package videojobs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

func renderAIDirectorOperatorInstruction(raw string) (string, map[string]interface{}, string) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return "", nil, "empty"
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(text), &payload); err != nil || len(payload) == 0 {
		return text, nil, "text_passthrough"
	}

	knownKeys := []string{
		"operator_identity",
		"business_scene",
		"asset_goal",
		"delivery_goal",
		"success_definition",
		"must_capture_bias",
		"avoid_bias",
		"cost_mode",
		"candidate_strategy",
		"candidate_count_bias",
		"diversity_preference",
		"clarity_priority",
		"loop_priority",
		"standalone_priority",
		"emotion_priority",
		"risk_tolerance",
		"subtitle_dependency_tolerance",
		"quality_floor_note",
		"user_request_passthrough",
		"extra_business_note",
	}
	hasKnown := false
	for _, key := range knownKeys {
		if _, ok := payload[key]; ok {
			hasKnown = true
			break
		}
	}
	if !hasKnown {
		return text, payload, "json_passthrough"
	}

	strValue := func(key string) string {
		return strings.TrimSpace(stringFromAny(payload[key]))
	}
	listValue := func(key string) []string {
		values := stringSliceFromAny(payload[key])
		out := make([]string, 0, len(values))
		seen := map[string]struct{}{}
		for _, item := range values {
			v := strings.TrimSpace(item)
			if v == "" {
				continue
			}
			k := strings.ToLower(v)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, v)
			if len(out) >= 8 {
				break
			}
		}
		return out
	}
	appendSection := func(buf *bytes.Buffer, title string, lines []string) {
		buf.WriteString(title)
		buf.WriteString("\n")
		if len(lines) == 0 {
			buf.WriteString("- （未设置）\n\n")
			return
		}
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			buf.WriteString("- ")
			buf.WriteString(line)
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}

	var buf bytes.Buffer
	appendSection(&buf, "【运营身份】", []string{
		fmt.Sprintf("你当前站在“%s”的业务视角做判断。", firstNonEmpty(strValue("operator_identity"), "运营")),
	})
	appendSection(&buf, "【业务场景与交付目标】", []string{
		"本次业务场景：" + firstNonEmpty(strValue("business_scene"), "social_spread"),
		"本次资产目标：" + firstNonEmpty(strValue("asset_goal"), "gif_highlight"),
		"本次交付目标：" + firstNonEmpty(strValue("delivery_goal"), "standalone_shareable"),
	})
	appendSection(&buf, "【成功结果定义】", listValue("success_definition"))
	appendSection(&buf, "【优先抓取倾向】", listValue("must_capture_bias"))
	appendSection(&buf, "【尽量规避倾向】", listValue("avoid_bias"))
	appendSection(&buf, "【成本与候选策略】", []string{
		"当前成本策略：" + firstNonEmpty(strValue("cost_mode"), "balanced"),
		"当前候选策略：" + firstNonEmpty(strValue("candidate_strategy"), "balanced"),
		"候选数量偏好：" + firstNonEmpty(strValue("candidate_count_bias"), "medium"),
		"多样性偏好：" + firstNonEmpty(strValue("diversity_preference"), "medium"),
	})
	appendSection(&buf, "【质量偏好】", []string{
		"清晰度优先级：" + firstNonEmpty(strValue("clarity_priority"), "high"),
		"循环优先级：" + firstNonEmpty(strValue("loop_priority"), "medium"),
		"独立成立优先级：" + firstNonEmpty(strValue("standalone_priority"), "high"),
		"情绪/高光优先级：" + firstNonEmpty(strValue("emotion_priority"), "high"),
	})
	appendSection(&buf, "【风险与限制】", []string{
		"风险容忍度：" + firstNonEmpty(strValue("risk_tolerance"), "low"),
		"字幕/对白依赖容忍度：" + firstNonEmpty(strValue("subtitle_dependency_tolerance"), "low"),
		"质量底线说明：" + firstNonEmpty(strValue("quality_floor_note"), "不要为了凑数量而接受明显模糊或抖动严重的片段"),
	})
	appendSection(&buf, "【用户需求（如有）】", []string{
		firstNonEmpty(strValue("user_request_passthrough"), "无"),
	})
	appendSection(&buf, "【补充业务说明】", []string{
		firstNonEmpty(strValue("extra_business_note"), "无"),
	})
	rendered := strings.TrimSpace(buf.String())
	if rendered == "" {
		return text, payload, "json_passthrough"
	}
	return rendered, payload, "json_schema_rendered"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

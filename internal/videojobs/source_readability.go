package videojobs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var unexpectedHTTPStatusRegexp = regexp.MustCompile(`unexpected status\s+(\d+)`)

type sourceReadError struct {
	ReasonCode string
	Hint       string
	Permanent  bool
	Err        error
}

func (e *sourceReadError) Error() string {
	if e == nil {
		return ""
	}
	base := strings.TrimSpace(e.ReasonCode)
	if base == "" {
		base = "source_read_failed"
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", base, e.Err)
	}
	return base
}

func (e *sourceReadError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type sourceURLCandidate struct {
	step string
	kind string
	raw  string
}

func (p *Processor) downloadObjectByKeyWithReadability(ctx context.Context, key, outPath string, expectedSizeBytes int64) (map[string]interface{}, error) {
	startedAt := time.Now()
	cleanKey := strings.TrimLeft(strings.TrimSpace(key), "/")
	attempts := make([]map[string]interface{}, 0, 8)
	expectedSizeBytes = normalizePositiveInt64(expectedSizeBytes)
	var lastDownloadedSize int64
	lastSizeMatch := expectedSizeBytes <= 0

	pushAttempt := func(step, kind, target string, started time.Time, success bool, err error, downloadedSize int64, sizeMatch bool) {
		row := map[string]interface{}{
			"step":                  strings.TrimSpace(step),
			"kind":                  strings.TrimSpace(kind),
			"duration_ms":           clampDurationMillis(started),
			"success":               success,
			"http_status":           int64(0),
			"error":                 "",
			"downloaded_size_bytes": normalizePositiveInt64(downloadedSize),
			"expected_size_bytes":   expectedSizeBytes,
			"size_match":            sizeMatch,
		}
		if host := sourceTargetHost(target); host != "" {
			row["target_host"] = host
		}
		if status, ok := sourceHTTPStatusFromErr(err); ok {
			row["http_status"] = int64(status)
		}
		if err != nil {
			row["error"] = strings.TrimSpace(err.Error())
		}
		lastDownloadedSize = normalizePositiveInt64(downloadedSize)
		lastSizeMatch = sizeMatch
		attempts = append(attempts, row)
	}

	buildDiagnostic := func(success bool, successStep string, reasonCode string, permanent bool, hint string, finalErr error) map[string]interface{} {
		diag := map[string]interface{}{
			"source_video_key":      strings.TrimSpace(cleanKey),
			"checked_at":            startedAt.Format(time.RFC3339),
			"duration_ms":           clampDurationMillis(startedAt),
			"success":               success,
			"success_step":          strings.TrimSpace(successStep),
			"used_fallback":         strings.TrimSpace(successStep) != "" && strings.TrimSpace(successStep) != "qiniu_sdk_get",
			"reason_code":           strings.TrimSpace(reasonCode),
			"permanent":             permanent,
			"hint":                  strings.TrimSpace(hint),
			"attempt_count":         len(attempts),
			"attempts":              attempts,
			"error":                 "",
			"downloaded_size_bytes": normalizePositiveInt64(lastDownloadedSize),
			"expected_size_bytes":   expectedSizeBytes,
			"size_match":            lastSizeMatch,
		}
		if finalErr != nil {
			diag["error"] = strings.TrimSpace(finalErr.Error())
		}
		return diag
	}

	if cleanKey == "" {
		err := &sourceReadError{
			ReasonCode: "source_video_key_empty",
			Hint:       "任务缺少 source_video_key，请重新上传视频并创建任务。",
			Permanent:  true,
			Err:        errors.New("empty source video key"),
		}
		pushAttempt("source_key_check", "local", cleanKey, startedAt, false, err.Err, 0, false)
		return buildDiagnostic(false, "", err.ReasonCode, err.Permanent, err.Hint, err), err
	}

	// Layer 1: 七牛 SDK 直连读取
	sdkStarted := time.Now()
	_ = os.Remove(outPath)
	sdkErr := p.downloadObjectByBucketManager(ctx, cleanKey, outPath)
	if sdkErr == nil {
		downloadedSize, sizeMatch, verifyErr := validateDownloadedSourceFile(outPath, expectedSizeBytes)
		if verifyErr == nil {
			pushAttempt("qiniu_sdk_get", "sdk", cleanKey, sdkStarted, true, nil, downloadedSize, sizeMatch)
			return buildDiagnostic(true, "qiniu_sdk_get", "ok", false, "", nil), nil
		}
		sdkErr = verifyErr
		pushAttempt("qiniu_sdk_get", "sdk", cleanKey, sdkStarted, false, sdkErr, downloadedSize, sizeMatch)
	} else {
		pushAttempt("qiniu_sdk_get", "sdk", cleanKey, sdkStarted, false, sdkErr, 0, false)
	}

	// Layer 2/3: URL 读取（签名 URL -> 公共 URL）
	candidates := make([]sourceURLCandidate, 0, 2)
	if signed, err := p.buildSignedObjectReadURL(cleanKey); err == nil {
		candidates = append(candidates, sourceURLCandidate{
			step: "qiniu_signed_url",
			kind: "signed_url",
			raw:  signed,
		})
	} else {
		pushAttempt("qiniu_signed_url_prepare", "signed_url", cleanKey, time.Now(), false, err, 0, false)
	}

	if publicURL, err := p.buildPublicObjectReadURL(cleanKey); err == nil {
		candidates = append(candidates, sourceURLCandidate{
			step: "qiniu_public_url",
			kind: "public_url",
			raw:  publicURL,
		})
	} else {
		pushAttempt("qiniu_public_url_prepare", "public_url", cleanKey, time.Now(), false, err, 0, false)
	}

	used := map[string]struct{}{}
	var lastErr error
	successStep := ""
	for _, candidate := range candidates {
		u := strings.TrimSpace(candidate.raw)
		if u == "" {
			continue
		}
		dedup := strings.ToLower(candidate.kind + "|" + u)
		if _, ok := used[dedup]; ok {
			continue
		}
		used[dedup] = struct{}{}
		started := time.Now()
		_ = os.Remove(outPath)
		err := p.downloadObject(ctx, u, outPath)
		if err == nil {
			downloadedSize, sizeMatch, verifyErr := validateDownloadedSourceFile(outPath, expectedSizeBytes)
			if verifyErr == nil {
				pushAttempt(candidate.step, candidate.kind, u, started, true, nil, downloadedSize, sizeMatch)
				successStep = candidate.step
				return buildDiagnostic(true, successStep, "ok", false, "", nil), nil
			}
			lastErr = verifyErr
			pushAttempt(candidate.step, candidate.kind, u, started, false, verifyErr, downloadedSize, sizeMatch)
			continue
		}
		lastErr = err
		pushAttempt(candidate.step, candidate.kind, u, started, false, err, 0, false)
	}

	combinedErr := lastErr
	if combinedErr == nil {
		combinedErr = sdkErr
	}
	reasonCode, hint, permanent := classifySourceReadabilityFailure(sdkErr, attempts)
	readErr := &sourceReadError{
		ReasonCode: reasonCode,
		Hint:       hint,
		Permanent:  permanent,
		Err:        combinedErr,
	}
	return buildDiagnostic(false, "", reasonCode, permanent, hint, readErr), readErr
}

func validateDownloadedSourceFile(path string, expectedSizeBytes int64) (downloadedSize int64, sizeMatch bool, err error) {
	info, statErr := os.Stat(strings.TrimSpace(path))
	if statErr != nil {
		return 0, false, fmt.Errorf("downloaded source stat failed: %w", statErr)
	}
	downloadedSize = info.Size()
	if downloadedSize <= 0 {
		return 0, false, errors.New("downloaded source size non_positive")
	}
	expectedSizeBytes = normalizePositiveInt64(expectedSizeBytes)
	if expectedSizeBytes > 0 && downloadedSize != expectedSizeBytes {
		return downloadedSize, false, fmt.Errorf("downloaded source size mismatch: expected=%d got=%d", expectedSizeBytes, downloadedSize)
	}
	return downloadedSize, true, nil
}

func normalizePositiveInt64(v int64) int64 {
	if v <= 0 {
		return 0
	}
	return v
}

func (p *Processor) buildSignedObjectReadURL(key string) (string, error) {
	if p == nil || p.qiniu == nil {
		return "", errors.New("qiniu client not configured")
	}
	cleanKey := strings.TrimLeft(strings.TrimSpace(key), "/")
	if cleanKey == "" {
		return "", errors.New("empty source video key")
	}
	signed, err := p.qiniu.SignedURL(cleanKey, 3600)
	if err != nil {
		return "", err
	}
	signed = strings.TrimSpace(signed)
	if strings.HasPrefix(strings.ToLower(signed), "http://") || strings.HasPrefix(strings.ToLower(signed), "https://") {
		return signed, nil
	}
	return "", errors.New("qiniu signed url unavailable")
}

func (p *Processor) buildPublicObjectReadURL(key string) (string, error) {
	if p == nil || p.qiniu == nil {
		return "", errors.New("qiniu client not configured")
	}
	cleanKey := strings.TrimLeft(strings.TrimSpace(key), "/")
	if cleanKey == "" {
		return "", errors.New("empty source video key")
	}
	publicURL := strings.TrimSpace(p.qiniu.PublicURL(cleanKey))
	if strings.HasPrefix(strings.ToLower(publicURL), "http://") || strings.HasPrefix(strings.ToLower(publicURL), "https://") {
		return publicURL, nil
	}
	return "", errors.New("qiniu public url unavailable")
}

func sourceTargetHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Host)
}

func sourceHTTPStatusFromErr(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	match := unexpectedHTTPStatusRegexp.FindStringSubmatch(strings.ToLower(strings.TrimSpace(err.Error())))
	if len(match) != 2 {
		return 0, false
	}
	code, convErr := strconv.Atoi(match[1])
	if convErr != nil || code <= 0 {
		return 0, false
	}
	return code, true
}

func classifySourceReadabilityFailure(sdkErr error, attempts []map[string]interface{}) (reasonCode string, hint string, permanent bool) {
	has404 := false
	hasAuth := false
	has5xx := false
	hasTimeout := false
	hasNetwork := false
	hasURLPrepareFailure := false
	hasIntegrityMismatch := false

	for _, item := range attempts {
		step := strings.TrimSpace(strings.ToLower(fmt.Sprint(item["step"])))
		if strings.Contains(step, "_prepare") {
			hasURLPrepareFailure = true
		}
		status := int(sourceInt64FromAny(item["http_status"]))
		if status == 404 {
			has404 = true
		}
		if status == 401 || status == 403 {
			hasAuth = true
		}
		if status >= 500 {
			has5xx = true
		}
		downloadedSize := sourceInt64FromAny(item["downloaded_size_bytes"])
		expectedSize := sourceInt64FromAny(item["expected_size_bytes"])
		sizeMatchValue := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["size_match"])))
		if expectedSize > 0 && (downloadedSize != expectedSize || sizeMatchValue == "false") {
			hasIntegrityMismatch = true
		}
		errText := strings.ToLower(strings.TrimSpace(fmt.Sprint(item["error"])))
		if errText == "" {
			continue
		}
		if strings.Contains(errText, "size mismatch") || strings.Contains(errText, "size non_positive") {
			hasIntegrityMismatch = true
		}
		if strings.Contains(errText, "timeout") || strings.Contains(errText, "deadline exceeded") {
			hasTimeout = true
		}
		if strings.Contains(errText, "connection refused") ||
			strings.Contains(errText, "no such host") ||
			strings.Contains(errText, "network is unreachable") ||
			strings.Contains(errText, "tls") ||
			strings.Contains(errText, "x509") {
			hasNetwork = true
		}
	}

	if has404 {
		return "source_video_not_found", "源视频对象不存在或已被删除，请重新上传后再创建任务。", true
	}
	if hasAuth {
		return "source_video_forbidden", "源视频读取被拒绝，请检查七牛私有读权限、签名域名和 token 配置。", true
	}
	if hasIntegrityMismatch {
		return "source_video_integrity_mismatch", "源视频完整性校验失败（大小不一致/空文件），请稍后重试或重新上传源视频。", false
	}
	if has5xx {
		return "source_video_storage_5xx", "存储服务异常（5xx），建议稍后自动重试。", false
	}
	if hasTimeout || hasNetwork {
		return "source_video_network_unstable", "源视频读取网络不稳定，请稍后重试。", false
	}
	if hasURLPrepareFailure && sdkErr != nil {
		return "source_video_url_unavailable", "未能生成可访问的视频读取 URL，请检查七牛下载域名配置。", true
	}
	if sdkErr != nil {
		msg := strings.ToLower(strings.TrimSpace(sdkErr.Error()))
		if strings.Contains(msg, "qiniu not configured") || strings.Contains(msg, "not configured") {
			return "source_video_storage_unconfigured", "存储配置缺失，请联系管理员检查七牛配置。", true
		}
	}
	return "source_video_read_failed", "源视频读取失败，请稍后重试或重新上传。", false
}

func sourceInt64FromAny(val interface{}) int64 {
	switch v := val.(type) {
	case int:
		return int64(v)
	case int8:
		return int64(v)
	case int16:
		return int64(v)
	case int32:
		return int64(v)
	case int64:
		return v
	case uint:
		return int64(v)
	case uint8:
		return int64(v)
	case uint16:
		return int64(v)
	case uint32:
		return int64(v)
	case uint64:
		if v > uint64(^uint64(0)>>1) {
			return int64(^uint64(0) >> 1)
		}
		return int64(v)
	case float32:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

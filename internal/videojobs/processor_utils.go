package videojobs

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	"emoji/internal/models"
	"emoji/internal/storage"

	qiniustorage "github.com/qiniu/go-sdk/v7/storage"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxIntValue(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func pickFramePathSample(paths []string, limit int) []string {
	if len(paths) == 0 || limit <= 0 {
		return nil
	}
	if len(paths) <= limit {
		return append([]string{}, paths...)
	}
	if limit == 1 {
		return []string{paths[0]}
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		idx := int(math.Round(float64(i) * float64(len(paths)-1) / float64(limit-1)))
		out = append(out, paths[idx])
	}
	return out
}

func readImageInfo(filePath string) (int64, int, int) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, 0, 0
	}
	size := info.Size()
	f, err := os.Open(filePath)
	if err != nil {
		return size, 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return size, 0, 0
	}
	return size, cfg.Width, cfg.Height
}

func uploadFileToQiniu(uploader *qiniustorage.FormUploader, q *storage.QiniuClient, key, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	putPolicy := qiniustorage.PutPolicy{Scope: q.Bucket + ":" + key}
	upToken := putPolicy.UploadToken(q.Mac)
	var ret qiniustorage.PutRet
	return uploader.Put(context.Background(), &ret, upToken, key, f, info.Size(), &qiniustorage.PutExtra{})
}

func deleteQiniuKeysByPrefix(q *storage.QiniuClient, keys []string) {
	if q == nil || len(keys) == 0 {
		return
	}
	ops := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimLeft(strings.TrimSpace(key), "/")
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ops = append(ops, qiniustorage.URIDelete(q.Bucket, key))
	}
	if len(ops) == 0 {
		return
	}
	_, _ = q.BucketManager().Batch(ops)
}

func deleteQiniuKey(q *storage.QiniuClient, key string) error {
	if q == nil {
		return nil
	}
	key = strings.TrimLeft(strings.TrimSpace(key), "/")
	if key == "" {
		return nil
	}
	if err := q.BucketManager().Delete(q.Bucket, key); err != nil && !isQiniuNotFoundErr(err) {
		return err
	}
	return nil
}

func isQiniuNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "no such file") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "612")
}

func slugify(input string) string {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return fmt.Sprintf("video-job-%d", time.Now().Unix())
	}
	var b strings.Builder
	lastDash := false
	for _, r := range input {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '_' || r == ' ' {
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fmt.Sprintf("video-job-%d", time.Now().Unix())
	}
	return out
}

func ensureUniqueSlug(db *gorm.DB, slug string) string {
	base := strings.TrimSpace(slug)
	if base == "" {
		base = fmt.Sprintf("video-job-%d", time.Now().Unix())
	}
	candidate := base
	for i := 0; i < 100; i++ {
		var count int64
		_ = db.Model(&models.Collection{}).Where("slug = ?", candidate).Count(&count).Error
		if count == 0 {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, i+1)
	}
	return fmt.Sprintf("%s-%d", base, time.Now().Unix())
}

func ensureUniqueDownloadCode(db *gorm.DB) (string, error) {
	for i := 0; i < 10; i++ {
		code, err := randomDownloadCode(downloadCodeLength)
		if err != nil {
			return "", err
		}
		var count int64
		if err := db.Model(&models.Collection{}).Where("download_code = ?", code).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", errors.New("failed to generate unique download code")
}

func randomDownloadCode(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("invalid code length")
	}
	max := big.NewInt(int64(len(downloadCodeAlphabet)))
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = downloadCodeAlphabet[n.Int64()]
	}
	return string(out), nil
}

func parseFPS(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if strings.Contains(raw, "/") {
		parts := strings.SplitN(raw, "/", 2)
		num := parseFloat(parts[0])
		den := parseFloat(parts[1])
		if den <= 0 {
			return 0
		}
		return num / den
	}
	return parseFloat(raw)
}

func parseFloat(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return f
}

func parseLooseFloat(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if idx := strings.IndexAny(raw, " \t\r\n"); idx >= 0 {
		raw = raw[:idx]
	}
	return parseFloat(raw)
}

func mapFromAny(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case map[string]interface{}:
		return value
	default:
		return map[string]interface{}{}
	}
}

func stringSliceFromAny(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok {
		if values, ok2 := raw.([]string); ok2 {
			out := make([]string, 0, len(values))
			for _, value := range values {
				value = strings.TrimSpace(strings.ToLower(value))
				if value == "" {
					continue
				}
				out = append(out, value)
			}
			return out
		}
		return nil
	}

	out := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(strings.ToLower(stringFromAny(item)))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func stringFromAny(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return value
	default:
		return ""
	}
}

func floatFromAny(raw interface{}) float64 {
	switch value := raw.(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case int32:
		return float64(value)
	case uint64:
		return float64(value)
	case json.Number:
		f, _ := value.Float64()
		return f
	case string:
		return parseFloat(value)
	default:
		return 0
	}
}

func intFromAny(raw interface{}) int {
	return int(math.Round(floatFromAny(raw)))
}

func boolFromAny(raw interface{}) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case int:
		return value > 0
	case int64:
		return value > 0
	case float64:
		return value > 0
	case string:
		normalized := strings.ToLower(strings.TrimSpace(value))
		return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "y"
	default:
		return false
	}
}

func valueOrNilUint64(raw *uint64) interface{} {
	if raw == nil || *raw == 0 {
		return nil
	}
	return *raw
}

func normalizeReason(raw interface{}) string {
	return strings.ToLower(strings.TrimSpace(stringFromAny(raw)))
}

func clampZeroOne(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

type jobOptions struct {
	AutoHighlight    bool    `json:"auto_highlight"`
	MaxStatic        int     `json:"max_static"`
	FrameIntervalSec float64 `json:"frame_interval_sec"`
	StartSec         float64 `json:"start_sec"`
	EndSec           float64 `json:"end_sec"`
	CropX            int     `json:"crop_x"`
	CropY            int     `json:"crop_y"`
	CropW            int     `json:"crop_w"`
	CropH            int     `json:"crop_h"`
	Speed            float64 `json:"speed"`
	FPS              int     `json:"fps"`
	Width            int     `json:"width"`
	MaxColors        int     `json:"max_colors"`
	RequestedJPG     bool    `json:"-"`
	RequestedPNG     bool    `json:"-"`
}

func parseJobOptions(raw datatypes.JSON) jobOptions {
	out := jobOptions{
		AutoHighlight:    true,
		MaxStatic:        24,
		FrameIntervalSec: 0,
		Speed:            1,
	}
	if len(raw) == 0 || string(raw) == "null" {
		return out
	}
	var payload struct {
		AutoHighlight    *bool   `json:"auto_highlight"`
		MaxStatic        int     `json:"max_static"`
		FrameIntervalSec float64 `json:"frame_interval_sec"`
		StartSec         float64 `json:"start_sec"`
		EndSec           float64 `json:"end_sec"`
		CropX            int     `json:"crop_x"`
		CropY            int     `json:"crop_y"`
		CropW            int     `json:"crop_w"`
		CropH            int     `json:"crop_h"`
		Speed            float64 `json:"speed"`
		FPS              int     `json:"fps"`
		Width            int     `json:"width"`
		MaxColors        int     `json:"max_colors"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return out
	}
	if payload.AutoHighlight != nil {
		out.AutoHighlight = *payload.AutoHighlight
	}
	out.MaxStatic = payload.MaxStatic
	out.FrameIntervalSec = payload.FrameIntervalSec
	out.StartSec = payload.StartSec
	out.EndSec = payload.EndSec
	out.CropX = payload.CropX
	out.CropY = payload.CropY
	out.CropW = payload.CropW
	out.CropH = payload.CropH
	out.Speed = payload.Speed
	out.FPS = payload.FPS
	out.Width = payload.Width
	out.MaxColors = payload.MaxColors
	if out.MaxStatic <= 0 {
		out.MaxStatic = 24
	}
	if out.MaxStatic > 80 {
		out.MaxStatic = 80
	}
	if out.FrameIntervalSec < 0 {
		out.FrameIntervalSec = 0
	}
	if out.StartSec < 0 {
		out.StartSec = 0
	}
	if out.EndSec < 0 {
		out.EndSec = 0
	}
	if out.EndSec > 0 && out.EndSec <= out.StartSec {
		out.EndSec = 0
	}

	if out.CropX < 0 {
		out.CropX = 0
	}
	if out.CropY < 0 {
		out.CropY = 0
	}
	if out.CropW < 0 {
		out.CropW = 0
	}
	if out.CropH < 0 {
		out.CropH = 0
	}

	if out.Speed <= 0 {
		out.Speed = 1
	}
	if out.Speed < 0.5 {
		out.Speed = 0.5
	}
	if out.Speed > 2.0 {
		out.Speed = 2.0
	}

	if out.FPS < 0 {
		out.FPS = 0
	}
	if out.FPS > 30 {
		out.FPS = 30
	}
	if out.FPS > 0 && out.FPS < 4 {
		out.FPS = 4
	}

	if out.Width < 0 {
		out.Width = 0
	}
	if out.Width > 1280 {
		out.Width = 1280
	}
	if out.Width > 0 && out.Width < 120 {
		out.Width = 120
	}

	if out.MaxColors < 0 {
		out.MaxColors = 0
	}
	if out.MaxColors > 256 {
		out.MaxColors = 256
	}
	if out.MaxColors > 0 && out.MaxColors < 16 {
		out.MaxColors = 16
	}
	return out
}

func applyQualityProfileOverridesFromOptions(
	settings QualitySettings,
	options map[string]interface{},
	requestedFormats []string,
) (QualitySettings, map[string]string) {
	settings = NormalizeQualitySettings(settings)
	if len(options) == 0 {
		return settings, nil
	}
	raw, ok := options["quality_profile_overrides"]
	if !ok || raw == nil {
		return settings, nil
	}
	overrides, ok := raw.(map[string]interface{})
	if !ok || len(overrides) == 0 {
		return settings, nil
	}

	requested := map[string]struct{}{}
	for _, format := range requestedFormats {
		format = strings.ToLower(strings.TrimSpace(format))
		if format == "" {
			continue
		}
		requested[format] = struct{}{}
	}

	normalizeProfile := func(value interface{}) (string, bool) {
		profile := strings.ToLower(strings.TrimSpace(stringFromAny(value)))
		switch profile {
		case QualityProfileClarity, QualityProfileSize:
			return profile, true
		default:
			return "", false
		}
	}

	applied := map[string]string{}
	setIfRequested := func(format string) (string, bool) {
		if _, ok := requested[format]; !ok {
			return "", false
		}
		value, exists := overrides[format]
		if !exists {
			return "", false
		}
		return normalizeProfile(value)
	}

	if profile, ok := setIfRequested("gif"); ok {
		settings.GIFProfile = profile
		applied["gif"] = profile
	}
	if profile, ok := setIfRequested("webp"); ok {
		settings.WebPProfile = profile
		applied["webp"] = profile
	}
	liveProfile := ""
	if profile, ok := setIfRequested("live"); ok {
		liveProfile = profile
		applied["live"] = profile
	}
	if profile, ok := setIfRequested("mp4"); ok {
		if liveProfile == "" {
			liveProfile = profile
		}
		applied["mp4"] = profile
	}
	if liveProfile != "" {
		settings.LiveProfile = liveProfile
	}
	if profile, ok := setIfRequested("jpg"); ok {
		settings.JPGProfile = profile
		applied["jpg"] = profile
	}
	if profile, ok := setIfRequested("png"); ok {
		settings.PNGProfile = profile
		applied["png"] = profile
	}
	if len(applied) == 0 {
		return settings, nil
	}
	return NormalizeQualitySettings(settings), applied
}

func jobOptionsMetrics(options jobOptions, intervalSec float64) map[string]interface{} {
	out := map[string]interface{}{
		"auto_highlight":     options.AutoHighlight,
		"max_static":         options.MaxStatic,
		"frame_interval_sec": intervalSec,
	}
	if options.StartSec > 0 {
		out["start_sec"] = options.StartSec
	}
	if options.EndSec > 0 {
		out["end_sec"] = options.EndSec
	}
	if options.CropW > 0 && options.CropH > 0 {
		out["crop"] = map[string]interface{}{
			"x": options.CropX,
			"y": options.CropY,
			"w": options.CropW,
			"h": options.CropH,
		}
	}
	if options.Speed > 0 && math.Abs(options.Speed-1.0) > 0.001 {
		out["speed"] = options.Speed
	}
	if options.FPS > 0 {
		out["fps"] = options.FPS
	}
	if options.Width > 0 {
		out["width"] = options.Width
	}
	if options.MaxColors > 0 {
		out["max_colors"] = options.MaxColors
	}
	return out
}

func mustJSON(v interface{}) datatypes.JSON {
	if v == nil {
		return datatypes.JSON([]byte("{}"))
	}
	b, err := json.Marshal(v)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	if len(b) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(b)
}

func parseJSONMap(raw datatypes.JSON) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func sourceVideoDeleted(metrics map[string]interface{}) bool {
	if len(metrics) == 0 {
		return false
	}
	raw, ok := metrics["source_video_deleted"]
	if !ok {
		return false
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true")
	default:
		return false
	}
}

func fallbackTitle(title string) string {
	title = strings.TrimSpace(title)
	if title != "" {
		return title
	}
	return fmt.Sprintf("视频表情包-%s", time.Now().Format("20060102150405"))
}

func normalizeOutputFormats(raw string) []string {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(raw)), ",")
	if len(parts) == 0 {
		return []string{"jpg", "gif"}
	}
	allow := map[string]struct{}{
		"jpg":  {},
		"jpeg": {},
		"png":  {},
		"gif":  {},
		"webp": {},
		"mp4":  {},
		"live": {},
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if p == "jpeg" {
			p = "jpg"
		}
		if _, ok := allow[p]; !ok {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		result = append(result, p)
	}
	if len(result) == 0 {
		return []string{"jpg", "gif"}
	}
	return result
}

# Go struct 草案

> 说明：本文件给的是**与当前仓库风格对齐的 struct 草案**，目标是帮助你们后续直接落代码。  
> 范围：只覆盖第一期最需要的 struct 增量，不尝试一次性改全。

---

## 1. `internal/config/config.go` 草案

### 1.1 `Config` 新增字段建议

```go
type Config struct {
    // ...existing fields...

    GIFSICLEBin                string
    PySceneDetectPythonBin     string
    PySceneDetectBin           string
    VMAFEnabled                bool
    LIBVMAFModelPath           string
    EnableExternalToolFallback bool

    AsynqmonEnabled        bool
    OTelEnabled            bool
    OTelExporterOTLPEndpoint string
    OTelServiceName        string
}
```

### 1.2 `Load()` 默认值建议

```go
cfg.GIFSICLEBin = getEnv("GIFSICLE_BIN", "")
cfg.PySceneDetectPythonBin = getEnv("PYSCENEDETECT_PYTHON_BIN", "python3")
cfg.PySceneDetectBin = getEnv("PYSCENEDETECT_BIN", "")
cfg.VMAFEnabled = getEnvAsBool("VMAF_ENABLED", false)
cfg.LIBVMAFModelPath = getEnv("LIBVMAF_MODEL_PATH", "")
cfg.EnableExternalToolFallback = getEnvAsBool("ENABLE_EXTERNAL_TOOL_FALLBACK", true)

cfg.AsynqmonEnabled = getEnvAsBool("ASYNQMON_ENABLED", false)
cfg.OTelEnabled = getEnvAsBool("OTEL_ENABLED", false)
cfg.OTelExporterOTLPEndpoint = getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
cfg.OTelServiceName = getEnv("OTEL_SERVICE_NAME", "emoji-backend")
```

---

## 2. `internal/videojobs/quality_settings.go` 草案

### 2.1 `QualitySettings` 新增字段建议

```go
type QualitySettings struct {
    // ...existing fields...

    GIFGifsicleEnabled      bool    `json:"gif_gifsicle_enabled"`
    GIFGifsicleLevel        int     `json:"gif_gifsicle_level"`
    GIFGifsicleSkipBelowKB  int     `json:"gif_gifsicle_skip_below_kb"`
    GIFGifsicleMinGainRatio float64 `json:"gif_gifsicle_min_gain_ratio"`

    SceneDetectEnabled        bool    `json:"scene_detect_enabled"`
    SceneDetectProvider       string  `json:"scene_detect_provider"`
    SceneDetectDetector       string  `json:"scene_detect_detector"`
    SceneDetectThreshold      float64 `json:"scene_detect_threshold"`
    SceneDetectMinSceneLenSec float64 `json:"scene_detect_min_scene_len_sec"`

    VMAFBenchmarkEnabled bool    `json:"vmaf_benchmark_enabled"`
    VMAFSampleRate       int     `json:"vmaf_sample_rate"`
    VMAFMinClipSec       float64 `json:"vmaf_min_clip_sec"`
}
```

### 2.2 `DefaultQualitySettings()` 建议补值

```go
GIFGifsicleEnabled:      true,
GIFGifsicleLevel:        2,
GIFGifsicleSkipBelowKB:  256,
GIFGifsicleMinGainRatio: 0.03,

SceneDetectEnabled:        false,
SceneDetectProvider:       "ffmpeg",
SceneDetectDetector:       "adaptive",
SceneDetectThreshold:      27,
SceneDetectMinSceneLenSec: 0.8,

VMAFBenchmarkEnabled: false,
VMAFSampleRate:       10,
VMAFMinClipSec:       1.2,
```

### 2.3 `NormalizeQualitySettings()` 建议补逻辑

```go
if out.GIFGifsicleLevel < 1 {
    out.GIFGifsicleLevel = def.GIFGifsicleLevel
}
if out.GIFGifsicleLevel > 3 {
    out.GIFGifsicleLevel = 3
}
if out.GIFGifsicleSkipBelowKB < 0 {
    out.GIFGifsicleSkipBelowKB = 0
}
if out.GIFGifsicleSkipBelowKB > 4096 {
    out.GIFGifsicleSkipBelowKB = 4096
}
out.GIFGifsicleMinGainRatio = clampFloat(out.GIFGifsicleMinGainRatio, 0, 0.50)

switch strings.ToLower(strings.TrimSpace(out.SceneDetectProvider)) {
case "ffmpeg", "pyscenedetect":
    out.SceneDetectProvider = strings.ToLower(strings.TrimSpace(out.SceneDetectProvider))
default:
    out.SceneDetectProvider = def.SceneDetectProvider
}

switch strings.ToLower(strings.TrimSpace(out.SceneDetectDetector)) {
case "content", "adaptive", "threshold":
    out.SceneDetectDetector = strings.ToLower(strings.TrimSpace(out.SceneDetectDetector))
default:
    out.SceneDetectDetector = def.SceneDetectDetector
}

out.SceneDetectThreshold = clampFloat(out.SceneDetectThreshold, 1, 100)
out.SceneDetectMinSceneLenSec = clampFloat(out.SceneDetectMinSceneLenSec, 0.2, 10)

if out.VMAFSampleRate < 1 {
    out.VMAFSampleRate = def.VMAFSampleRate
}
if out.VMAFSampleRate > 30 {
    out.VMAFSampleRate = 30
}
out.VMAFMinClipSec = clampFloat(out.VMAFMinClipSec, 0.5, 10)
```

---

## 3. `internal/models/video_jobs.go` 草案

### 3.1 `VideoQualitySetting` 新增字段建议

放在 `AIDirectorOperatorEnabled` 前后都可以，但建议仍然放在同类配置区域附近，便于维护：

```go
type VideoQualitySetting struct {
    // ...existing fields...

    GIFGifsicleEnabled      bool    `gorm:"column:gif_gifsicle_enabled"`
    GIFGifsicleLevel        int     `gorm:"column:gif_gifsicle_level"`
    GIFGifsicleSkipBelowKB  int     `gorm:"column:gif_gifsicle_skip_below_kb"`
    GIFGifsicleMinGainRatio float64 `gorm:"column:gif_gifsicle_min_gain_ratio"`

    SceneDetectEnabled        bool    `gorm:"column:scene_detect_enabled"`
    SceneDetectProvider       string  `gorm:"column:scene_detect_provider;size:32"`
    SceneDetectDetector       string  `gorm:"column:scene_detect_detector;size:32"`
    SceneDetectThreshold      float64 `gorm:"column:scene_detect_threshold"`
    SceneDetectMinSceneLenSec float64 `gorm:"column:scene_detect_min_scene_len_sec"`

    VMAFBenchmarkEnabled bool    `gorm:"column:vmaf_benchmark_enabled"`
    VMAFSampleRate       int     `gorm:"column:vmaf_sample_rate"`
    VMAFMinClipSec       float64 `gorm:"column:vmaf_min_clip_sec"`

    // ...existing fields...
}
```

---

## 4. `internal/handlers/video_quality_settings.go` 草案

### 4.1 `VideoQualitySettingRequest` 新增字段

```go
type VideoQualitySettingRequest struct {
    // ...existing fields...

    GIFGifsicleEnabled      bool    `json:"gif_gifsicle_enabled"`
    GIFGifsicleLevel        int     `json:"gif_gifsicle_level"`
    GIFGifsicleSkipBelowKB  int     `json:"gif_gifsicle_skip_below_kb"`
    GIFGifsicleMinGainRatio float64 `json:"gif_gifsicle_min_gain_ratio"`

    SceneDetectEnabled        bool    `json:"scene_detect_enabled"`
    SceneDetectProvider       string  `json:"scene_detect_provider"`
    SceneDetectDetector       string  `json:"scene_detect_detector"`
    SceneDetectThreshold      float64 `json:"scene_detect_threshold"`
    SceneDetectMinSceneLenSec float64 `json:"scene_detect_min_scene_len_sec"`

    VMAFBenchmarkEnabled bool    `json:"vmaf_benchmark_enabled"`
    VMAFSampleRate       int     `json:"vmaf_sample_rate"`
    VMAFMinClipSec       float64 `json:"vmaf_min_clip_sec"`
}
```

### 4.2 `validateVideoQualitySettingRequest` 建议补校验

```go
if req.GIFGifsicleLevel < 1 || req.GIFGifsicleLevel > 3 {
    return errors.New("invalid gif_gifsicle_level: expected 1..3")
}
if req.GIFGifsicleSkipBelowKB < 0 || req.GIFGifsicleSkipBelowKB > 4096 {
    return errors.New("invalid gif_gifsicle_skip_below_kb: expected 0..4096")
}
if req.GIFGifsicleMinGainRatio < 0 || req.GIFGifsicleMinGainRatio > 0.50 {
    return errors.New("invalid gif_gifsicle_min_gain_ratio: expected 0..0.50")
}

switch strings.TrimSpace(strings.ToLower(req.SceneDetectProvider)) {
case "ffmpeg", "pyscenedetect":
default:
    return errors.New("invalid scene_detect_provider: expected ffmpeg|pyscenedetect")
}

switch strings.TrimSpace(strings.ToLower(req.SceneDetectDetector)) {
case "content", "adaptive", "threshold":
default:
    return errors.New("invalid scene_detect_detector: expected content|adaptive|threshold")
}

if req.SceneDetectThreshold < 1 || req.SceneDetectThreshold > 100 {
    return errors.New("invalid scene_detect_threshold: expected 1..100")
}
if req.SceneDetectMinSceneLenSec < 0.2 || req.SceneDetectMinSceneLenSec > 10 {
    return errors.New("invalid scene_detect_min_scene_len_sec: expected 0.2..10")
}
if req.VMAFSampleRate < 1 || req.VMAFSampleRate > 30 {
    return errors.New("invalid vmaf_sample_rate: expected 1..30")
}
if req.VMAFMinClipSec < 0.5 || req.VMAFMinClipSec > 10 {
    return errors.New("invalid vmaf_min_clip_sec: expected 0.5..10")
}
```

### 4.3 `applyQualitySettingsToModel(...)` 建议补映射

```go
dst.GIFGifsicleEnabled = settings.GIFGifsicleEnabled
dst.GIFGifsicleLevel = settings.GIFGifsicleLevel
dst.GIFGifsicleSkipBelowKB = settings.GIFGifsicleSkipBelowKB
dst.GIFGifsicleMinGainRatio = settings.GIFGifsicleMinGainRatio

dst.SceneDetectEnabled = settings.SceneDetectEnabled
dst.SceneDetectProvider = strings.TrimSpace(settings.SceneDetectProvider)
dst.SceneDetectDetector = strings.TrimSpace(settings.SceneDetectDetector)
dst.SceneDetectThreshold = settings.SceneDetectThreshold
dst.SceneDetectMinSceneLenSec = settings.SceneDetectMinSceneLenSec

dst.VMAFBenchmarkEnabled = settings.VMAFBenchmarkEnabled
dst.VMAFSampleRate = settings.VMAFSampleRate
dst.VMAFMinClipSec = settings.VMAFMinClipSec
```

### 4.4 `qualitySettingsFromModel(...)` / `loadQualitySettings()` 建议补映射

```go
GIFGifsicleEnabled:      row.GIFGifsicleEnabled,
GIFGifsicleLevel:        row.GIFGifsicleLevel,
GIFGifsicleSkipBelowKB:  row.GIFGifsicleSkipBelowKB,
GIFGifsicleMinGainRatio: row.GIFGifsicleMinGainRatio,

SceneDetectEnabled:        row.SceneDetectEnabled,
SceneDetectProvider:       row.SceneDetectProvider,
SceneDetectDetector:       row.SceneDetectDetector,
SceneDetectThreshold:      row.SceneDetectThreshold,
SceneDetectMinSceneLenSec: row.SceneDetectMinSceneLenSec,

VMAFBenchmarkEnabled: row.VMAFBenchmarkEnabled,
VMAFSampleRate:       row.VMAFSampleRate,
VMAFMinClipSec:       row.VMAFMinClipSec,
```

---

## 5. `internal/videojobs/capabilities.go` 草案

### 5.1 新增 capability struct

```go
type ExternalToolCapability struct {
    Name      string `json:"name"`
    Available bool   `json:"available"`
    Path      string `json:"path,omitempty"`
    Version   string `json:"version,omitempty"`
    Reason    string `json:"reason,omitempty"`
}
```

### 5.2 扩 `RuntimeCapabilities`

```go
type RuntimeCapabilities struct {
    FFmpegAvailable        bool                    `json:"ffmpeg_available"`
    FFprobeAvailable       bool                    `json:"ffprobe_available"`
    GifsicleAvailable      bool                    `json:"gifsicle_available"`
    PySceneDetectAvailable bool                    `json:"pyscenedetect_available"`
    VMAFAvailable          bool                    `json:"vmaf_available"`
    SupportedFormats       []string                `json:"supported_formats"`
    UnsupportedFormats     []string                `json:"unsupported_formats"`
    Formats                []FormatCapability      `json:"formats"`
    ExternalTools          []ExternalToolCapability `json:"external_tools,omitempty"`
}
```

### 5.3 `VideoJobCapabilitiesResponse` 同步扩字段

```go
type VideoJobCapabilitiesResponse struct {
    FFmpegAvailable        bool                         `json:"ffmpeg_available"`
    FFprobeAvailable       bool                         `json:"ffprobe_available"`
    GifsicleAvailable      bool                         `json:"gifsicle_available"`
    PySceneDetectAvailable bool                         `json:"pyscenedetect_available"`
    VMAFAvailable          bool                         `json:"vmaf_available"`
    SupportedFormats       []string                     `json:"supported_formats"`
    UnsupportedFormats     []string                     `json:"unsupported_formats"`
    Formats                []videojobs.FormatCapability `json:"formats"`
    ExternalTools          []videojobs.ExternalToolCapability `json:"external_tools,omitempty"`
}
```

---

## 6. `internal/videojobs/tool_gifsicle.go` 草案

```go
type GIFOptimizationResult struct {
    Applied         bool    `json:"applied"`
    Tool            string  `json:"tool"`
    Level           int     `json:"level,omitempty"`
    BeforeSizeBytes int64   `json:"before_size_bytes,omitempty"`
    AfterSizeBytes  int64   `json:"after_size_bytes,omitempty"`
    SavedBytes      int64   `json:"saved_bytes,omitempty"`
    SavedRatio      float64 `json:"saved_ratio,omitempty"`
    DurationMs      int64   `json:"duration_ms,omitempty"`
    Error           string  `json:"error,omitempty"`
}
```

---

## 7. `internal/videojobs/tool_scenedetect.go` 草案

```go
type SceneBoundary struct {
    StartSec float64 `json:"start_sec"`
    EndSec   float64 `json:"end_sec"`
}

type SceneDetectResult struct {
    Applied      bool            `json:"applied"`
    Provider     string          `json:"provider,omitempty"`
    Detector     string          `json:"detector,omitempty"`
    SceneCount   int             `json:"scene_count,omitempty"`
    AvgSceneSec  float64         `json:"avg_scene_sec,omitempty"`
    FallbackUsed bool            `json:"fallback_used,omitempty"`
    Boundaries   []SceneBoundary `json:"boundaries,omitempty"`
    DurationMs   int64           `json:"duration_ms,omitempty"`
    Error        string          `json:"error,omitempty"`
}
```

---

## 8. `internal/videojobs/tool_vmaf.go` / `tasks.go` 草案

### 8.1 benchmark result

```go
type VMAFBenchmarkResult struct {
    Applied    bool    `json:"applied"`
    MeanScore  float64 `json:"mean_score,omitempty"`
    P5Score    float64 `json:"p5_score,omitempty"`
    FrameCount int     `json:"frame_count,omitempty"`
    DurationMs int64   `json:"duration_ms,omitempty"`
    Error      string  `json:"error,omitempty"`
}
```

### 8.2 task payload

```go
const TaskTypeBenchmarkVMAF = "video_jobs:benchmark_vmaf"

type BenchmarkVMAFPayload struct {
    JobID            uint64 `json:"job_id"`
    OutputID         uint64 `json:"output_id"`
    BaselineOutputID uint64 `json:"baseline_output_id"`
    SampleRate       int    `json:"sample_rate"`
    Reason           string `json:"reason"`
}
```

---

## 9. `internal/handlers` admin DTO 草案

### 9.1 benchmark request

```go
type AdminBenchmarkVMAFRequest struct {
    OutputID         uint64 `json:"output_id"`
    BaselineOutputID uint64 `json:"baseline_output_id"`
    SampleRate       int    `json:"sample_rate"`
    Reason           string `json:"reason"`
}
```

### 9.2 scene detect preview request

```go
type AdminSceneDetectPreviewRequest struct {
    Provider        string  `json:"provider"`
    Detector        string  `json:"detector"`
    Threshold       float64 `json:"threshold"`
    MinSceneLenSec  float64 `json:"min_scene_len_sec"`
}
```

### 9.3 audit chain summary 增量字段

```go
type AdminVideoJobGIFAuditChainSummary struct {
    // ...existing fields...
    SceneDetectProvider string  `json:"scene_detect_provider,omitempty"`
    SceneCount          int     `json:"scene_count,omitempty"`
    GIFOptimizer        string  `json:"gif_optimizer,omitempty"`
    GIFSizeSavedRatio   float64 `json:"gif_size_saved_ratio,omitempty"`
    VMAFMean            float64 `json:"vmaf_mean,omitempty"`
}
```

---

## 10. 我建议你们先真正落的 struct 顺序

如果现在就开始改代码，最先落这几个：

1. `config.Config`
2. `videojobs.QualitySettings`
3. `models.VideoQualitySetting`
4. `handlers.VideoQualitySettingRequest`
5. `videojobs.RuntimeCapabilities`
6. `videojobs.BenchmarkVMAFPayload`

这 6 组 struct 落稳以后，后面的 handler、migration、tool 封装就能顺着写。

# 代码落点与 Patch 方案

> 目标：按现有代码结构，明确每个工具的落点、函数签名建议、改动顺序与最小 patch 范围。

---

## 1. 总体原则

第一期 patch 方案遵循 4 条原则：

1. **新工具尽量封装成独立文件**，不要把外部命令调用散落到各个逻辑文件
2. **主链路只插点，不重写编排方式**
3. **所有工具都必须有 fallback**
4. **每个工具结果都要能进入 metrics / metadata / audit chain**

---

## 2. Runtime 能力探测 patch

## 2.1 新增文件

### `internal/videojobs/tool_runtime.go`

建议提供：

```go
type ExternalToolCapability struct {
    Name      string `json:"name"`
    Available bool   `json:"available"`
    Path      string `json:"path,omitempty"`
    Version   string `json:"version,omitempty"`
    Reason    string `json:"reason,omitempty"`
}

func detectExternalTool(name string, versionArgs ...string) ExternalToolCapability
func detectExternalToolSet(cfg config.Config) []ExternalToolCapability
func findExecutable(candidates ...string) (string, error)
```

## 2.2 修改文件

### `internal/videojobs/capabilities.go`

当前 `RuntimeCapabilities` 建议扩成：

```go
type RuntimeCapabilities struct {
    FFmpegAvailable    bool
    FFprobeAvailable   bool
    GifsicleAvailable  bool
    PySceneDetectAvailable bool
    VMAFAvailable      bool
    ExternalTools      []ExternalToolCapability
    ...
}
```

### `internal/handlers/video_jobs.go`

扩 `VideoJobCapabilitiesResponse`，直接透出上述字段。

---

## 3. Gifsicle patch

## 3.1 新增文件

### `internal/videojobs/tool_gifsicle.go`

建议函数：

```go
type GIFOptimizationResult struct {
    Applied         bool
    Tool            string
    Level           int
    BeforeSizeBytes int64
    AfterSizeBytes  int64
    SavedBytes      int64
    SavedRatio      float64
    DurationMs      int64
    Error           string
}

func optimizeGIFWithGifsicle(ctx context.Context, inputPath string, level int) (GIFOptimizationResult, error)
```

### 设计要点

- 用临时文件输出，不直接覆盖原文件
- 只有优化成功且文件可读，才替换原文件
- 如果优化后比原文件更大，允许保留原文件

## 3.2 修改 `internal/videojobs/processor_gif_render.go`

### 推荐插点

在 `renderGIFOutput(...)` 内，`ffmpeg` 成功产出文件后插入：

```go
if qualitySettings.GIFGifsicleEnabled && toolAvailable {
    opt, err := optimizeGIFWithGifsicle(...)
    // 记录结果；失败则 fallback 原文件
}
```

### 不建议做的事

- 不要在每次 size shrink attempt 中都跑 gifsicle
- 不要把 gifsicle 混进 ffmpeg 重试循环里

正确顺序应该是：

```text
ffmpeg 完成最终产物 -> gifsicle 单次优化 -> 记录结果
```

## 3.3 修改 `internal/videojobs/gif_rerender.go`

让 admin rerender 路径和主路径保持一致，否则会出现：

- 正式产物有优化
- rerender 产物无优化

导致后台比较失真。

## 3.4 修改 `internal/videojobs/gif_evaluation.go`

在 `featureJSON` 填充时，增加：

```go
featureJSON["gif_optimization_v1"] = ...
```

这样后续导出与报表不需要二次 join output metadata。

---

## 4. PySceneDetect patch

## 4.1 新增文件

### `internal/videojobs/tool_scenedetect.go`

建议提供：

```go
type SceneBoundary struct {
    StartSec float64 `json:"start_sec"`
    EndSec   float64 `json:"end_sec"`
}

type SceneDetectResult struct {
    Applied        bool
    Provider       string
    Detector       string
    SceneCount     int
    AvgSceneSec    float64
    FallbackUsed   bool
    Boundaries     []SceneBoundary
    DurationMs     int64
    Error          string
}

func runPySceneDetect(ctx context.Context, sourcePath string, detector string, threshold float64, minSceneLenSec float64) (SceneDetectResult, error)
```

## 4.2 修改 `internal/videojobs/processor_scene_selection.go`

当前已有：

- `detectScenePoints(...)`
- `buildFallbackHighlightCandidates(...)`

建议补一层统一入口：

```go
func detectSceneStructure(ctx context.Context, sourcePath string, settings QualitySettings, caps RuntimeCapabilities) SceneDetectResult
```

逻辑：

1. 若 `SceneDetectEnabled=false` -> 旧逻辑
2. 若 provider=`pyscenedetect` 且工具可用 -> 跑新逻辑
3. 失败 -> fallback 到 ffmpeg scene

## 4.3 修改 `internal/videojobs/processor_pipeline.go`

在 `probeVideo(...)` 后、candidate build 前，增加：

- 解析 runtime capability
- 跑 `detectSceneStructure`
- 把结果写入 `metrics["scene_detect_v1"]`

## 4.4 修改 `internal/videojobs/processor_gif_candidates.go`

第一期只做两步：

### 第一步

- 读取 scene detect 结果
- 仅用于辅助 candidate 去重和边界裁切

### 第二步

- 再决定是否让 candidate 直接按 scene 来构造

这样风险更低。

---

## 5. VMAF patch

## 5.1 新增文件

### `internal/videojobs/tool_vmaf.go`

建议：

```go
type VMAFBenchmarkResult struct {
    Applied     bool
    MeanScore   float64
    P5Score     float64
    FrameCount  int
    DurationMs  int64
    Error       string
}

func runVMAFBenchmark(ctx context.Context, refPath, distPath string, sampleRate int, modelPath string) (VMAFBenchmarkResult, error)
```

## 5.2 新增文件

### `internal/videojobs/benchmark_vmaf.go`

负责：

- 下载参考素材/候选素材
- 调 `runVMAFBenchmark`
- 落地 JSON / audit

## 5.3 修改 `internal/videojobs/tasks.go`

新增：

```go
const TaskTypeBenchmarkVMAF = "video_jobs:benchmark_vmaf"
```

并增加 payload 结构。

## 5.4 修改 `cmd/worker/main.go`

给 mux 注册 benchmark handler。

---

## 6. quality settings patch

## 6.1 修改 `internal/videojobs/quality_settings.go`

要补三类内容：

1. 结构体字段
2. 默认值
3. normalize 逻辑

## 6.2 修改 `internal/handlers/video_quality_settings.go`

要补四类内容：

1. request/response JSON 字段
2. validate 规则
3. `applyQualitySettingsToModel(...)`
4. `toVideoQualitySettingResponse(...)`

## 6.3 修改 `internal/models/video_jobs.go`

同步给 `VideoQualitySetting` model 加列映射。

---

## 7. audit chain patch

## 7.1 修改 `internal/handlers/admin_video_jobs_audit_chain.go`

建议新增字段：

```go
SceneDetectProvider string  `json:"scene_detect_provider,omitempty"`
SceneCount          int     `json:"scene_count,omitempty"`
GIFOptimizer        string  `json:"gif_optimizer,omitempty"`
GIFSizeSavedRatio   float64 `json:"gif_size_saved_ratio,omitempty"`
VMAFMean            float64 `json:"vmaf_mean,omitempty"`
```

### 数据来源

- `job.metrics.scene_detect_v1`
- `output.metadata.gif_optimization_v1`
- `output.metadata.vmaf_benchmark_v1`

目标是让后台一页能看出：

- 用了哪个 scene detect
- GIF 是否被优化
- 优化节省了多少
- benchmark 指标如何

---

## 8. 路由 patch

## 8.1 修改 `internal/router/router.go`

第一期建议：

### 保持不新增用户侧接口

只扩：

- `GET /api/video-jobs/capabilities`

### 后台侧新增 1~2 个内部接口即可

建议：

- `POST /api/admin/video-jobs/:id/benchmark-vmaf`
- 可选：`POST /api/admin/video-jobs/:id/scene-detect-preview`

其余能力优先塞进已有：

- `quality-settings`
- `gif-audit-chain`
- `rerender-gif`

---

## 9. 脚本 patch

## 9.1 新增脚本

### Gifsicle

- `scripts/check_gif_optimizer_gain.sql`
- `scripts/check_gif_optimizer_gain.sh`
- `scripts/smoke_gifsicle_release_gate.sh`

### Scene detect

- `scripts/check_scene_detect_quality.sql`
- `scripts/check_scene_detect_quality.sh`
- `scripts/run_scene_detect_baseline_flow.sh`

### VMAF

- `scripts/check_vmaf_regression.sql`
- `scripts/check_vmaf_regression.sh`
- `scripts/run_vmaf_regression_flow.sh`

---

## 10. 最小 patch 顺序

建议按下面顺序打 patch，冲突最小：

1. `config.go`
2. `quality_settings.go`
3. `models/video_jobs.go`
4. migration SQL
5. `capabilities.go`
6. `video_jobs.go` capabilities 响应
7. `tool_runtime.go`
8. `tool_gifsicle.go`
9. `processor_gif_render.go`
10. `gif_rerender.go`
11. `gif_evaluation.go`
12. `admin_video_jobs_audit_chain.go`
13. scripts
14. `tool_scenedetect.go`
15. `processor_scene_selection.go`
16. `processor_pipeline.go`
17. `processor_gif_candidates.go`
18. `tool_vmaf.go`
19. `benchmark_vmaf.go`
20. `tasks.go` / worker 注册

---

## 11. 一句话建议

如果你准备开始真正改代码，**第一刀就从 Gifsicle 开始**。因为它对当前代码侵入最小、收益最直观、最容易量化和回滚。

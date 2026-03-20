# AI1 文本 JSON v2 字段清单

## 1. 文档目的

本文档用于把 AI1 文本 JSON v2 拆成可以直接落地的“字段清单表”。

目标不是讨论抽象概念，而是明确：

- 字段名是什么
- 类型是什么
- 是否必填
- 示例值是什么
- 谁来生产
- 谁来消费
- 是否进入模型输入
- 它对质量 / 效率有什么作用

适用范围：

- 当前 `AI1 -> AI2 -> worker&打分 -> AI3` GIF/视频转图片链路
- 重点是 **AI1 / Director** 输入中的文本 JSON

---

## 2. v2 设计原则

AI1 升级到 `full_video` 后，文本 JSON 的职责必须收敛为：

1. **任务定义**
2. **业务约束**
3. **低成本导航提示**
4. **成本边界**

一句话原则：

> **视频负责感知，JSON 负责任务合同。**

---

## 3. 推荐的 v2 结构

```json
{
  "schema_version": "ai1_input_v2",
  "task": {
    "asset_goal": "gif_highlight",
    "business_scene": "social_spread",
    "delivery_goal": "standalone_shareable",
    "optimization_target": "clarity_first",
    "cost_sensitivity": "normal",
    "hard_constraints": {
      "target_count_min": 3,
      "target_count_max": 6,
      "duration_sec_min": 1.4,
      "duration_sec_max": 3.2
    }
  },
  "source": {
    "title": "示例标题",
    "duration_sec": 128.4,
    "width": 1920,
    "height": 1080,
    "fps": 30,
    "aspect_ratio": "16:9",
    "orientation": "landscape",
    "input_mode": "full_video"
  },
  "navigation_hints": {
    "scene_count_est": 12,
    "motion_level": "medium",
    "motion_peaks_sec": [18.2, 46.7, 88.4],
    "subtitle_density": "high",
    "ocr_hints": ["subtitle_overlay"]
  },
  "risk_hints": ["low_light", "fast_motion"]
}
```

---

## 4. P0：第一阶段必须落地的字段

这部分字段建议作为 **AI1 v2 第一阶段正式字段集**，优先进入开发。

### 4.1 根节点字段

| 字段 | 类型 | 必填 | 示例 | 生产方 | 消费方 | 是否进模型 | 作用 |
|---|---|---:|---|---|---|---:|---|
| `schema_version` | string | 是 | `ai1_input_v2` | 后端 `buildDirectorPayload()` | AI1、日志系统 | 是 | 标识 payload 版本，支持灰度与回放 |

---

### 4.2 `task` 字段组

| 字段 | 类型 | 必填 | 示例 | 生产方 | 消费方 | 是否进模型 | 作用 |
|---|---|---:|---|---|---|---:|---|
| `task.asset_goal` | string | 是 | `gif_highlight` | 产品配置 / 作业类型映射 | AI1，后续 AI2/AI3 | 是 | 明确产物类型，避免 AI1 默认只按 GIF 传播思路思考 |
| `task.business_scene` | string | 是 | `social_spread` | 产品场景配置 / 请求参数 | AI1，后续 AI3 | 是 | 决定“高价值片段”的业务标准 |
| `task.delivery_goal` | string | 是 | `standalone_shareable` | 产品策略配置 | AI1，后续 AI3 | 是 | 明确什么叫好结果，是独立传播、可点开、还是步骤完整 |
| `task.optimization_target` | string | 是 | `clarity_first` | 由 `gif_profile` 映射 | AI1，后续 worker/eval | 是 | 把内部质量枚举翻译成模型可理解目标 |
| `task.cost_sensitivity` | string | 是 | `normal` | 由预算/目标大小映射 | AI1，后续渲染层 | 是 | 让 AI1 知道成本约束的紧张程度 |
| `task.hard_constraints.target_count_min` | int | 是 | `3` | 产品配置 / 质量配置 | AI1，AI2 | 是 | 最少提名数量约束 |
| `task.hard_constraints.target_count_max` | int | 是 | `6` | 产品配置 / 质量配置 | AI1，AI2 | 是 | 最多提名数量约束 |
| `task.hard_constraints.duration_sec_min` | float | 是 | `1.4` | 产品配置 / 质量配置 | AI1，后续 worker | 是 | 候选窗口最短偏好 |
| `task.hard_constraints.duration_sec_max` | float | 是 | `3.2` | 产品配置 / 质量配置 | AI1，后续 worker | 是 | 候选窗口最长偏好 |

#### 建议枚举值

##### `task.asset_goal`

- `gif_highlight`
- `cover_image`
- `poster_frame`
- `keyframe_pack`

##### `task.business_scene`

- `social_spread`
- `ecommerce`
- `content_archive`
- `news`
- `training`

##### `task.delivery_goal`

- `standalone_shareable`
- `clickable_cover`
- `step_completeness`
- `emotion_peak`

##### `task.optimization_target`

- `clarity_first`
- `balanced`
- `size_first`
- `spread_first`

##### `task.cost_sensitivity`

- `strict`
- `normal`
- `loose`

---

### 4.3 `source` 字段组

| 字段 | 类型 | 必填 | 示例 | 生产方 | 消费方 | 是否进模型 | 作用 |
|---|---|---:|---|---|---|---:|---|
| `source.title` | string | 否 | `猫猫搞笑反应合集` | `job.Title` | AI1 | 是 | 弱业务提示，不可作为强事实 |
| `source.duration_sec` | float | 是 | `128.4` | `videoProbeMeta` | AI1、后续 AI2 | 是 | 判断长短视频策略与窗口数量 |
| `source.width` | int | 是 | `1920` | `videoProbeMeta` | AI1 | 是 | 画幅基础信息 |
| `source.height` | int | 是 | `1080` | `videoProbeMeta` | AI1 | 是 | 画幅基础信息 |
| `source.fps` | float | 是 | `30` | `videoProbeMeta` | AI1 | 是 | 运动密度与镜头节奏辅助信息 |
| `source.aspect_ratio` | string | 是 | `16:9` | 由 width/height 派生 | AI1 | 是 | 让模型快速理解画面构图属性 |
| `source.orientation` | string | 是 | `landscape` | 由 width/height 派生 | AI1 | 是 | 横屏/竖屏/方图场景判断 |
| `source.input_mode` | string | 是 | `full_video` | 当前 AI1 输入决策逻辑 | AI1、日志系统 | 是 | 告知模型当前主要感知方式 |
| `source.frame_refs` | array | 条件必填 | `[{"index":1,"timestamp_sec":3.2}]` | 帧采样逻辑 | AI1 | 是（仅 frames 模式） | 在帧模式下给图片与时间点建立映射 |

#### `source.input_mode` 建议取值

- `full_video`
- `frames`
- `hybrid_fallback_frames`

#### `source.frame_refs` 说明

仅在以下情况建议传：

- AI1 当前实际采用 `frames`
- 或 `hybrid` 回退到帧输入

建议结构：

```json
[
  {
    "index": 1,
    "timestamp_sec": 3.2
  }
]
```

注意：

- **只保留 `index` 和 `timestamp_sec`**
- 不要再传 `bytes`

---

### 4.4 `risk_hints` 字段组

| 字段 | 类型 | 必填 | 示例 | 生产方 | 消费方 | 是否进模型 | 作用 |
|---|---|---:|---|---|---|---:|---|
| `risk_hints` | string[] | 否 | `["low_light","fast_motion"]` | 本地规则 / 运营策略 / 预分析模块 | AI1，后续 AI2/AI3 | 是 | 让 AI1 更稳定地产出 `avoid` 与风险倾向 |

#### 建议枚举值

- `low_light`
- `fast_motion`
- `heavy_shake`
- `watermark`
- `dense_subtitles`
- `busy_background`

---

## 5. P1：第二阶段增强字段

这部分字段不要求第一阶段一次性做完，但对长视频、复杂视频、未来“视频流转图片”场景非常有价值。

### 5.1 `source` 增强字段

| 字段 | 类型 | 必填 | 示例 | 生产方 | 消费方 | 是否进模型 | 作用 |
|---|---|---:|---|---|---|---:|---|
| `source.has_audio` | bool | 否 | `true` | 扩展 ffprobe | AI1，后续 AI2 | 是 | 区分纯视觉素材与带语音/对白素材 |
| `source.audio_density` | string | 否 | `high` | 音频分析模块 | AI1 | 是 | 判断“是否强依赖上下文” |
| `source.language_hint` | string | 否 | `zh` | ASR / 请求上下文 | AI1 | 是 | 为字幕/对白理解提供弱提示 |

### 5.2 `navigation_hints` 字段组

| 字段 | 类型 | 必填 | 示例 | 生产方 | 消费方 | 是否进模型 | 作用 |
|---|---|---:|---|---|---|---:|---|
| `navigation_hints.scene_count_est` | int | 否 | `12` | 本地预分析 | AI1 | 是 | 提示镜头切换复杂度 |
| `navigation_hints.motion_level` | string | 否 | `medium` | 本地预分析 | AI1 | 是 | 给模型一个低成本整体运动印象 |
| `navigation_hints.motion_peaks_sec` | float[] | 否 | `[18.2,46.7,88.4]` | 预分析模块 | AI1 | 是 | 降低长视频中的搜索成本 |
| `navigation_hints.subtitle_density` | string | 否 | `high` | OCR/字幕分析 | AI1 | 是 | 判断片段能否脱离上下文单独成立 |
| `navigation_hints.ocr_hints` | string[] | 否 | `["subtitle_overlay","price_tag"]` | OCR/规则模块 | AI1 | 是 | 让模型知道画面里可能存在结构化文字 |
| `navigation_hints.scene_boundary_sample` | float[] | 否 | `[5.2,16.8,39.0]` | 场景切分模块 | AI1 | 是 | 给 AI1 低成本时序导航 |

#### `motion_level` 建议枚举值

- `low`
- `medium`
- `high`

#### `subtitle_density` 建议枚举值

- `none`
- `low`
- `medium`
- `high`

---

## 6. 不进入模型输入、但必须保留到日志/审计的字段

这部分字段建议从 AI1 文本 JSON 里拿掉，但继续保留到 usage metadata / persist context / 调试日志。

| 字段 | 当前来源 | 保留位置 | 为什么不进模型 |
|---|---|---|---|
| `job_id` | `job.ID` | usage metadata / persist context | 纯工程标识，对模型无语义价值 |
| `source_video_url_available` | URL 解析逻辑 | usage metadata | 工程状态位，不是任务语义 |
| `source_video_url_error` | URL 解析逻辑 | usage metadata / error log | 调试字段，不应污染模型注意力 |
| `source_video_url` | 对象存储 URL | 多模态 `video_url` part | 已通过多模态内容传入，文本重复无意义 |
| `source_input_mode_requested` | 质量配置 | usage metadata | 对模型无必要，内部决策信息 |
| `frame_manifest[].bytes` | 帧采样逻辑 | usage metadata | 文件大小不是内容语义 |
| `frame_count` | 帧采样逻辑 | usage metadata | full_video 模式下低价值 |
| `operator_instruction.text` | editable template | system prompt + persist context | 已进 system prompt，文本 JSON 再传会重复 |
| `operator_instruction.version` | editable template | persist context / logs | 审计有用，对模型可选 |
| `frame_sampling_error` | 帧采样异常 | error log | 调试字段 |

---

## 7. v1 到 v2 的字段映射建议

### 7.1 直接迁移

| v1 字段 | v2 字段 | 说明 |
|---|---|---|
| `title` | `source.title` | 直接迁移 |
| `duration_sec` | `source.duration_sec` | 直接迁移 |
| `width` | `source.width` | 直接迁移 |
| `height` | `source.height` | 直接迁移 |
| `fps` | `source.fps` | 直接迁移 |

### 7.2 语义升级迁移

| v1 字段 | v2 字段 | 升级方式 |
|---|---|---|
| `gif_profile` | `task.optimization_target` | 从工程枚举映射为模型可理解目标 |
| `gif_target_size_kb` | `task.cost_sensitivity` | 从具体 KB 预算映射为成本敏感度 |
| `source_input_mode_applied` + `source_input_type` | `source.input_mode` | 合并成一个模型侧字段 |
| `frame_manifest` | `source.frame_refs` | 保留 `index`/`timestamp_sec`，删除 `bytes` |

### 7.3 删除出模型、仅保留日志

| v1 字段 | v2 去向 |
|---|---|
| `job_id` | 日志保留 |
| `source_video_url_available` | 日志保留 |
| `source_video_url_error` | 日志保留 |
| `source_video_url` | 仅保留多模态 part |
| `source_input_mode_requested` | 日志保留 |
| `operator_instruction.text` | 仅保留 system prompt |
| `frame_manifest[].bytes` | 删除 |

---

## 8. 字段来源与当前代码落点建议

### 8.1 现有代码已具备的数据

| 字段 | 当前是否已有 | 当前来源代码 |
|---|---:|---|
| `source.title` | 是 | `job.Title` |
| `source.duration_sec` | 是 | `videoProbeMeta.DurationSec` |
| `source.width` | 是 | `videoProbeMeta.Width` |
| `source.height` | 是 | `videoProbeMeta.Height` |
| `source.fps` | 是 | `videoProbeMeta.FPS` |
| `source.input_mode` | 是 | `requestAIGIFPromptDirective()` |
| `source.frame_refs` | 是（需改名瘦身） | `frame_manifest` |
| `task.optimization_target` | 可由现有字段映射 | `qualitySettings.GIFProfile` |
| `task.cost_sensitivity` | 可由现有字段映射 | `qualitySettings.GIFTargetSizeKB` |

### 8.2 现有代码可低成本补充的数据

| 字段 | 当前是否易补 | 推荐来源 |
|---|---:|---|
| `source.aspect_ratio` | 是 | width/height 派生 |
| `source.orientation` | 是 | width/height 派生 |
| `navigation_hints.scene_count_est` | 是 | 复用现有 scene_count 统计 |
| `navigation_hints.motion_level` | 是 | 复用现有 motion_mean 分段映射 |
| `risk_hints` | 是 | 规则映射 + 运营配置 |

### 8.3 需要新增预处理能力的数据

| 字段 | 当前是否已有 | 说明 |
|---|---:|---|
| `source.has_audio` | 否 | 需扩 `ffprobeJSON` 与 `videoProbeMeta` |
| `navigation_hints.motion_peaks_sec` | 否 | 需新增预分析模块 |
| `navigation_hints.subtitle_density` | 否 | 需 OCR/字幕检测能力 |
| `navigation_hints.ocr_hints` | 否 | 需 OCR 摘要能力 |
| `navigation_hints.scene_boundary_sample` | 否 | 需场景切分摘要能力 |

---

## 9. 推荐开发实施顺序

### 第一步：改 AI1 payload 结构

代码落点：

- `internal/videojobs/ai_gif_pipeline.go:781`

动作：

1. 重构 `buildDirectorPayload()`
2. 从扁平结构改为：
   - `schema_version`
   - `task`
   - `source`
   - `risk_hints`
3. 在 `frames` 模式下保留 `source.frame_refs`
4. 从文本 JSON 中删掉日志类字段

### 第二步：补低成本派生字段

动作：

1. 加 `aspect_ratio`
2. 加 `orientation`
3. 从已有统计映射出：
   - `scene_count_est`
   - `motion_level`

### 第三步：补 AI1 导航增强能力

动作：

1. 加 `navigation_hints`
2. 逐步补：
   - `motion_peaks_sec`
   - `subtitle_density`
   - `ocr_hints`

### 第四步：让 v2 字段打到下游

动作：

1. AI2 接入更结构化的 task/context
2. worker/eval 接入 `optimization_target`
3. AI3 接入 `business_scene + delivery_goal`

---

## 10. 最终落地建议

如果从“能开发、能上线、能验证效果”的角度，我建议把字段分成三层推进：

### P0：马上改

- `schema_version`
- `task.*`
- `source.title/duration_sec/width/height/fps/aspect_ratio/orientation/input_mode`
- `source.frame_refs`（仅 frames）
- `risk_hints`

### P1：下个迭代补

- `navigation_hints.scene_count_est`
- `navigation_hints.motion_level`
- `source.has_audio`

### P2：做成长视频增强版

- `motion_peaks_sec`
- `subtitle_density`
- `ocr_hints`
- `scene_boundary_sample`

---

## 11. 一句话收口

这份字段清单的本质，不是让 AI1 拿到更多字段，而是：

> **让 AI1 拿到更少但更有决策价值的字段。**

字段越少越好，前提是这些字段真的能定义任务、约束成本、降低搜索熵，并且能继续传递到 AI2 / worker / AI3。

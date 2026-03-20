# AI1 文本 JSON 整改方案

## 1. 结论先行

当前项目里，AI1 已经从“看若干采样帧”升级为可支持 `frames | full_video | hybrid` 的多模态输入，其中在 `full_video` 模式下，模型直接接收整段视频 URL，而文本 JSON 继续作为同一请求中的文字上下文输入。

结合现有代码，我的核心结论是：

> **视频负责感知，JSON 负责任务定义、业务约束、导航提示、成本边界。**

因此，AI1 的文本 JSON 不应再承担“替视频补视觉”的职责，而应整改为：

- **任务合同（Task Contract）**
- **业务上下文（Business Context）**
- **低成本导航提示（Navigation Hints）**
- **成本与产出边界（Budget / Constraint Hints）**

---

## 2. 当前流程理解（基于代码）

### 2.1 当前链路顺序

当前 GIF AI 流程，按代码实际执行顺序，大致为：

1. **AI1 / Director**
   - 代码：`internal/videojobs/processor_pipeline.go:353`
   - 调用：`requestAIGIFPromptDirective(...)`
   - 作用：先生成结构化 `directive`

2. **AI2 / Planner**
   - 代码：`internal/videojobs/processor_pipeline.go:421`
   - 调用：`requestAIGIFPlannerSuggestion(...)`
   - 作用：基于 AI1 `directive` + 帧信息，产出 proposal

3. **worker + 本地打分 / 落库 / 渲染**
   - 关键逻辑：
     - `internal/videojobs/processor_pipeline.go:616`
     - `internal/videojobs/processor_gif_candidates.go`
     - `internal/videojobs/gif_evaluation.go`

4. **AI3 / Judge**
   - 代码：`internal/videojobs/processor_pipeline.go:910`
   - 调用：`runAIGIFJudgeReview(...)`
   - 作用：对 GIF 输出做最终复审

### 2.2 AI1 当前输入模式

AI1 当前支持三种输入模式：

- `frames`
- `full_video`
- `hybrid`

代码位置：`internal/videojobs/ai_gif_pipeline.go:705`

其中：

- `full_video` 成功时，模型直接接收 `video_url`
- 失败时，会降级到帧采样
- `hybrid` 本质上是“优先视频，失败回退帧”

### 2.3 AI1 当前输入组成

AI1 当前请求由三部分组成：

1. **固定模板 fixed prompt**
   - `internal/videojobs/ai_gif_pipeline.go:655`

2. **可编辑模板 editable prompt**
   - `internal/videojobs/ai_gif_pipeline.go:672`
   - 最终被拼接到 system prompt 中
   - `internal/videojobs/ai_gif_pipeline.go:843`

3. **用户侧多模态内容**
   - 文本 JSON
   - `video_url` 或若干 `image_url`
   - 构造位置：`internal/videojobs/ai_gif_pipeline.go:810`

---

## 3. 当前文本 JSON 的现状

### 3.1 当前 payload 字段

当前 AI1 文本 JSON 构造于：

- `internal/videojobs/ai_gif_pipeline.go:781`

当前主要字段包括：

```json
{
  "job_id": 0,
  "title": "",
  "duration_sec": 0,
  "width": 0,
  "height": 0,
  "fps": 0,
  "gif_profile": "",
  "gif_target_size_kb": 0,
  "source_input_mode_requested": "",
  "source_input_mode_applied": "",
  "source_input_type": "",
  "frame_count": 0,
  "frame_manifest": [
    {
      "index": 1,
      "timestamp_sec": 0.0,
      "bytes": 0
    }
  ],
  "source_video_url_available": false,
  "source_video_url_error": "",
  "operator_instruction": {
    "enabled": true,
    "version": "v1",
    "text": ""
  },
  "source_video_url": ""
}
```

### 3.2 当前主要问题

当前 payload 的问题不在于“字段少”，而在于：

1. **模型字段与日志字段混杂**
2. **一部分字段对模型低价值甚至是噪音**
3. **很多字段是工程视角，不是模型视角**
4. **缺少真正决定任务结果的业务目标字段**
5. **缺少对整段视频推理的导航提示字段**

换句话说：

> 现在这份 JSON，更像“工程调试包”，还不是“模型任务合同”。

---

## 4. 当前字段逐项整改建议

---

### 4.1 建议保留给模型的字段

这部分字段保留价值较高，应继续保留，但建议统一命名与语义：

#### 1）`title`

建议：**保留**

原因：

- 标题有时能提供弱业务上下文
- 例如能区分“搞笑片段 / 商品展示 / 访谈 / 新闻解说”

注意：

- 只能作为弱提示，不能让 AI1 过度依赖标题做语义判断

#### 2）`duration_sec`

建议：**保留**

原因：

- AI1 在做“候选片段数量、窗口长度倾向”时有价值
- 对长视频和短视频的策略不同

#### 3）`width` / `height`

建议：**保留**

原因：

- 可帮助模型理解视频画幅特征
- 后续可进一步派生出横竖屏、宽高比

#### 4）`fps`

建议：**保留**

原因：

- 可帮助模型判断运动密度、镜头切换密度
- 对“动作峰值 / 循环友好片段”的理解有辅助价值

#### 5）输入模式字段

当前字段：

- `source_input_mode_requested`
- `source_input_mode_applied`
- `source_input_type`

建议：

- 模型侧只保留一个更简洁字段，如：`input_mode`
- 值建议为：
  - `full_video`
  - `frames`
  - `hybrid_fallback_frames`

原因：

- 模型不关心工程内部的 requested/applied/source 三层细分
- 它只需要知道自己当前拿到的主要感知介质是什么

---

### 4.2 建议移出模型输入，仅保留到日志/持久化的字段

#### 1）`job_id`

建议：**从模型输入移除**

原因：

- 对模型无语义价值
- 纯工程标识

#### 2）`source_video_url_available`

建议：**从模型输入移除**

原因：

- 模型已经拿到 `video_url` part，本字段是工程状态位

#### 3）`source_video_url_error`

建议：**从模型输入移除**

原因：

- 属于调用侧调试信息
- 不应进入模型注意力空间

#### 4）`source_video_url`

建议：**不要放在文本 JSON 中**

原因：

- 已通过多模态 `video_url` 传入
- 文本 JSON 再放一遍，重复且浪费 token

#### 5）`frame_manifest[].bytes`

建议：**从模型输入移除**

原因：

- 帧文件大小对模型理解内容几乎无价值
- 这是工程质量与传输信息，不是语义信息

#### 6）`operator_instruction.text`

建议：**从文本 JSON 中移除**

原因：

- 当前 editable template 已被拼接进 `system prompt`
  - 代码：`internal/videojobs/ai_gif_pipeline.go:843`
- 再放入 JSON，会造成重复表达
- 容易让“业务任务字段”和“prompt 指令字段”混层

建议保留：

- `instruction_enabled`
- `instruction_version`

用于持久化或审计，不建议直接暴露正文给模型

#### 7）`frame_count`

建议：**在 full_video 模式下移除**

原因：

- full_video 模式下，模型感知主体是整段视频，不是这几个采样帧
- 保留它反而会误导模型把注意力放到采样视角

---

### 4.3 建议保留但要改“模型可理解表达”的字段

#### 1）`gif_profile`

建议：**不要直接给模型内部枚举**

当前问题：

- 这是渲染/画质策略枚举
- 工程语义较强，模型未必理解

建议替换为：

- `optimization_target`
  - `clarity_first`
  - `balanced`
  - `size_first`
  - `spread_first`

#### 2）`gif_target_size_kb`

建议：**不要原样直接暴露为主要字段**

当前问题：

- 这是渲染输出预算，不是 AI1 的主任务语义
- 模型看到具体 KB 数值，不一定能稳定映射到决策

建议替换为：

- `cost_sensitivity`
  - `strict`
  - `normal`
  - `loose`

或：

- `render_budget_level`
  - `tight`
  - `balanced`
  - `relaxed`

---

## 5. 建议立即新增的字段

这里是 AI1 文本 JSON 最应该补的部分。

### 5.1 schema 层

#### `schema_version`

建议：**立即新增**

作用：

- 方便后续灰度
- 兼容 v1/v2 payload
- 便于日志审计与回放

---

### 5.2 task 层：明确“你到底要模型干什么”

这是最重要的一组字段。

#### 1）`asset_goal`

示例：

- `gif_highlight`
- `cover_image`
- `poster_frame`
- `keyframe_pack`

作用：

- 告诉 AI1 当前要产出的资产类型
- 避免它默认只按“传播型 GIF”思路思考

#### 2）`business_scene`

示例：

- `social_spread`
- `ecommerce`
- `content_archive`
- `news`
- `training`

作用：

- 决定模型对“高价值片段”的判断标准

#### 3）`delivery_goal`

示例：

- `standalone_shareable`
- `clickable_cover`
- `step_completeness`
- `emotion_peak`

作用：

- 把“什么叫好结果”明确下来

#### 4）`hard_constraints`

建议包含：

- `target_count_min`
- `target_count_max`
- `duration_sec_min`
- `duration_sec_max`

作用：

- 让 AI1 在输出 `clip_count_*`、`duration_pref_*` 时更稳定
- 避免模型产出与系统产能/预算错位

---

### 5.3 source 层：补足结构化视频事实

建议新增：

#### 1）`aspect_ratio`

示例：

- `16:9`
- `9:16`
- `1:1`

#### 2）`orientation`

示例：

- `landscape`
- `portrait`
- `square`

#### 3）`has_audio`（建议下一步加）

说明：

- 当前 `videoProbeMeta` 仅有：
  - `duration`
  - `width`
  - `height`
  - `fps`
- 代码：`internal/videojobs/processor_gif_candidates.go:688`

因此如果要加 `has_audio`，需要先扩展 ffprobe 元数据结构。

---

### 5.4 navigation_hints 层：降低整段视频搜索熵

这是我认为对“全视频输入后 AI1 效率”最关键的一层。

AI1 已经能看整段视频，但整段视频越长，模型在时序里找重点的成本越高。因此建议额外给它一些**低成本导航提示**。

建议新增：

#### 1）`scene_count_est`

说明：

- 用于提示视频镜头切换复杂度
- 当前项目中，本地流程已有 `scene_count` 能力基础
  - `internal/videojobs/processor_gif_candidates.go:427`

#### 2）`motion_level`

示例：

- `low`
- `medium`
- `high`

或直接给：

- `motion_mean`

说明：

- 当前项目已有 `motion_mean`
  - `internal/videojobs/processor_gif_candidates.go:425`

#### 3）`motion_peaks_sec`

说明：

- 给出若干运动峰值大致时间点
- 可显著降低模型在长视频中的搜索成本

#### 4）`subtitle_density`

示例：

- `none`
- `low`
- `medium`
- `high`

作用：

- 对“适合做独立 GIF / 不适合脱离上下文”的判断很有帮助

#### 5）`ocr_hints`

示例：

- `subtitle_overlay`
- `price_tag`
- `headline_text`

作用：

- 后续如果引入 OCR/字幕摘要能力，这个字段非常有用

#### 6）`risk_hints`

示例：

- `low_light`
- `fast_motion`
- `heavy_shake`
- `watermark`
- `dense_subtitles`

作用：

- 让 AI1 更稳定地产出 `avoid` 与 `risk_flags`

---

## 6. 文本 JSON 对质量与效率的真实作用

### 6.1 对质量的作用

文本 JSON 最重要的质量价值，不是告诉模型“视频里看到了什么”，而是告诉模型：

1. **你要做的资产是什么**
2. **你要优先满足什么业务目标**
3. **你要规避什么风险**
4. **你要控制在什么边界内**

这会直接提升：

- AI1 输出 `directive` 的稳定性
- AI2 proposal 的业务贴合度
- worker 渲染结果的有效候选率
- AI3 复审结果的一致性

### 6.2 对效率的作用

文本 JSON 的效率价值体现在四点：

#### 1）减少无效 token

- 去掉调试类字段
- 去掉重复字段

#### 2）减少模型搜索空间

- 加导航提示字段
- 特别是在整段视频输入时有价值

#### 3）减少下游无效提名

- 提前明确 count/duration/scene 目标

#### 4）减少返工与回退

- AI1 的“任务定义”清晰，下游更不容易跑偏

---

## 7. 一个非常关键的现实问题：AI1 的很多信息还没真正打到下游

这是当前链路里最值得注意的地方。

### 7.1 当前 AI1 输出中，被程序强消费的字段并不多

当前比较明确的硬消费点主要是：

- `clip_count_min`
- `clip_count_max`

它们会影响 AI2 的 `targetTopN`

代码位置：

- `internal/videojobs/ai_gif_pipeline.go:1434`

### 7.2 其他大量字段目前主要是“prompt 语义消费”

例如：

- `must_capture`
- `avoid`
- `duration_pref_min_sec`
- `duration_pref_max_sec`
- `quality_weights`
- `risk_flags`
- `directive_text`

这些字段当前更多是通过 AI2 prompt 间接发挥作用，还没全面下沉为系统硬约束。

### 7.3 本地打分当前仍是固定权重

本地 evaluation 当前是固定权重：

- `internal/videojobs/gif_evaluation.go:71`

即：

- emotion
- clarity
- motion
- loop
- efficiency

目前并没有直接消费 AI1 的 `quality_weights`

### 7.4 AI3 当前也没有明确拿 AI1 的 directive 作为复审上下文

AI3 当前输入主要是：

- `job_id`
- `sample_size`
- `outputs`

代码位置：

- `internal/videojobs/ai_gif_pipeline.go:1645`

这意味着：

> AI1 的业务意图，还没有完整穿透到 AI3。

---

## 8. 整改方向：不只改 AI1 输入，还要把关键约束继续往后传

### 8.1 第一层整改：瘦身 AI1 文本 JSON

目标：

- 去掉噪音字段
- 引入任务字段
- 引入导航字段

### 8.2 第二层整改：把 AI1 的关键约束下沉到 AI2 / worker / AI3

建议优先打通三条：

#### 路径 A：`duration_pref_*` 下沉到候选过滤

作用：

- 让窗口时长不再只靠 prompt 软约束
- 减少不合格 proposal

#### 路径 B：`quality_weights` 下沉到本地 evaluation

作用：

- 让不同业务目标对应不同评分偏好
- 例如传播型更重 emotion/loop，封面型更重 clarity

#### 路径 C：`business_goal / delivery_goal` 传给 AI3

作用：

- 让复审标准和 AI1 的任务目标一致
- 避免 AI3 只按通用好坏判定

---

## 9. 推荐的 AI1 文本 JSON v2 结构

下面给出推荐的 v2 结构：

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

## 10. 建议的字段取舍清单（可直接用于开发）

### 10.1 模型输入保留

- `title`
- `duration_sec`
- `width`
- `height`
- `fps`
- `input_mode`
- `aspect_ratio`
- `orientation`
- `schema_version`
- `task.*`
- `navigation_hints.*`
- `risk_hints`

### 10.2 模型输入删除，日志保留

- `job_id`
- `source_video_url_available`
- `source_video_url_error`
- `source_video_url`
- `frame_manifest[].bytes`
- `source_input_mode_requested`
- `operator_instruction.text`
- `frame_count`（full_video 模式）

### 10.3 模型输入重命名/语义升级

- `gif_profile` → `optimization_target`
- `gif_target_size_kb` → `cost_sensitivity` / `render_budget_level`
- `source_input_mode_applied + source_input_type` → `input_mode`

---

## 11. 分阶段改造建议

### Phase 1：低风险快改

目标：

- 不动整体链路，只调整 AI1 payload

建议动作：

1. 改造 `buildDirectorPayload()`
   - 位置：`internal/videojobs/ai_gif_pipeline.go:781`
2. 精简模型输入字段
3. 增加 `schema_version`
4. 增加 `task` 与 `source` 结构分层
5. 保留旧字段到 usage metadata / persist context，便于审计

### Phase 2：引入导航提示

目标：

- 给 AI1 提供长视频导航能力

建议动作：

1. 新增 `navigation_hints`
2. 先复用已有本地统计能力：
   - `motion_mean`
   - `scene_count`
3. 后续再引入：
   - OCR/字幕摘要
   - 运动峰值检测
   - 场景切分摘要

### Phase 3：让 AI1 真正驱动全链路

目标：

- 从“AI1 产出一段提示词”升级为“AI1 产出全链路任务合同”

建议动作：

1. `duration_pref_*` 下沉到 proposal 过滤与候选约束
2. `quality_weights` 接入本地 evaluation
3. `business_goal / delivery_goal` 进入 AI3 judge 输入
4. 建立不同业务场景的 directive-to-score 映射策略

---

## 12. 代码改造落点建议

### AI1 输入整改

- `internal/videojobs/ai_gif_pipeline.go:781`
  - 重构 `buildDirectorPayload()`

- `internal/videojobs/ai_gif_pipeline.go:810`
  - 保持 `text + video_url/image_url` 多模态结构不变

- `internal/videojobs/ai_gif_pipeline.go:843`
  - editable template 继续走 system prompt，不建议再重复进 JSON

### AI2 协同

- `internal/videojobs/ai_gif_pipeline.go:1245`
  - 后续可把 AI1 新增 task 字段以更结构化方式传给 Planner

### worker / evaluation 协同

- `internal/videojobs/gif_evaluation.go:71`
  - 后续将固定权重改为“默认权重 + directive override”

### AI3 协同

- `internal/videojobs/ai_gif_pipeline.go:1645`
  - judge input 增加来自 AI1 的核心任务上下文

---

## 13. 最终建议

如果只给一句执行建议，就是：

> **先把 AI1 文本 JSON 从“工程调试包”改成“模型任务合同”，再把其中最关键的任务约束继续打通到 AI2 / worker / AI3。**

优先级上建议：

1. **先做字段瘦身**
2. **再补 task / navigation_hints**
3. **最后打通下游硬消费**

这样改，才能让“全视频 AI1”真正变成整个“视频转图片 / 视频转 GIF”链路的前置大脑，而不是只多看了视频、却没有真正改变系统决策质量。

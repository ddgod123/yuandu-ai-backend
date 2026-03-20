# 04-Processor重构方案

> 目标文件：`internal/videojobs/processor.go`  
> 当前规模：9816 行

## 1. 现状结论

`processor.go` 当前承担了几乎整个视频任务引擎的职责，包括：

- 任务生命周期控制
- job 抢占与幂等控制
- 源视频下载与探测
- 自动裁黑边
- 高光窗口选择
- GIF 候选与评分
- AI Director / Planner / Judge
- 抽帧与质量筛选
- 静态/动态渲染
- 七牛上传
- collection / emoji / artifact 持久化
- public 镜像同步
- zip 打包
- 成本/积分结算
- 临时文件清理
- 错误分类与重试策略

这会导致：

1. 文件过大，阅读成本极高
2. 单测难拆，新增逻辑容易回归
3. 任何一点改动都可能误伤整条链路
4. 运维逻辑、算法逻辑、持久化逻辑耦合严重

---

## 2. 重构目标

### 2.1 业务目标

- 不改变已有 API 契约
- 不改变 Asynq 任务类型
- 不改变产物 key 布局
- 不改变 public 镜像结构
- 不改变积分/成本结算结果

### 2.2 工程目标

- 将“总控”收敛到 800~1200 行以内
- 将算法逻辑、IO 逻辑、持久化逻辑拆开
- 让每个模块可单测、可替换、可观测
- 为后续引入多格式专属 pipeline 做准备

---

## 3. 建议的目标目录结构

> 这里不是强制一次性改完，而是建议的最终形态。

```text
internal/videojobs/
  processor.go                 # 仅保留 Processor 入口与 Register
  processor_run.go             # HandleProcessVideoJob / process 总控
  processor_lifecycle.go       # acquire / update / fail / retry / cancel
  processor_source.go          # 下载源视频、探测、裁黑边、meta 解析
  processor_highlight.go       # highlight window、feedback rerank、candidate persist
  processor_static.go          # 静态图抽帧、筛帧、静态输出
  processor_animated.go        # GIF / WebP / live 等动态输出
  processor_persist.go         # collection / emoji / artifact / package 落库
  processor_public_sync.go     # public.* 镜像同步封装
  processor_cleanup.go         # 临时目录、源视频清理
  processor_errors.go          # permanentError / retryable 分类
  processor_metrics.go         # metrics/event 组装与写入
  processor_types.go           # runtime context / render plan / persist result
```

---

## 4. 按职责拆分的建议

## 4.1 总控层：只负责编排

总控函数应只做：

1. 读 job
2. 抢占执行权
3. 构建运行上下文
4. 顺序调用各子阶段
5. 决定成功/失败/重试/取消
6. 收尾

也就是说，总控层应该像这样：

```text
load job
-> acquire run
-> prepare source
-> analyze video
-> select highlights
-> render outputs
-> persist outputs
-> post-review / package / sync
-> settle cost/points
-> cleanup
```

不再直接混入大量 ffmpeg 参数、SQL 细节、上传细节。

---

## 4.2 Source 阶段单独拆出

建议独立模块承担：

- 下载源视频
- ffprobe 探测
- 文件尺寸/时长收集
- 自动裁黑边
- 校验 source 是否已删除

输出一个稳定结构，例如：

- `SourceBundle`
  - `SourcePath`
  - `SourceMeta`
  - `SourceSizeBytes`
  - `CropSuggestion`

这样后续高光选择和渲染都只消费 `SourceBundle`，不直接碰原始 job/options。

---

## 4.3 Highlight 阶段拆成独立子流水线

建议把以下逻辑从总控拆出：

- `suggestHighlightWindow`
- AI Director
- AI Planner
- feedback rerank
- cloud fallback
- GIF candidate persist
- proposal/candidate 绑定

目标输出统一成一个 `HighlightPlan`：

- `Selected`
- `Candidates`
- `All`
- `DirectiveSnapshot`
- `PlannerSnapshot`
- `FeedbackSnapshot`
- `SubStageMetrics`

这样后面渲染层只接收“最终计划”，不关心计划怎么得来。

---

## 4.4 渲染层按“静态 / 动态”拆开

现在最值得拆的是：

### 静态渲染模块

负责：

- 抽帧
- 质量筛选
- JPG / PNG 生成
- live cover 相关静态选择

### 动态渲染模块

负责：

- GIF window 选择
- GIF loop tune
- WebP / MP4 / live 动态生成
- timeout fallback / emergency fallback

这样会比按“格式”拆更稳，因为你现在的 shared logic 很多。

---

## 4.5 持久化层独立

当前 `persistJobResults` 职责过重，建议拆为：

- `ensureResultCollection(...)`
- `persistEmojiRows(...)`
- `persistArtifactRows(...)`
- `persistPackage(...)`
- `syncPublicOutputs(...)`
- `refreshCollectionLatestZip(...)`

原则：

- SQL 写入放一层
- key 生成放一层
- 领域对象组装放一层

不要让一个函数既生成 key、又调 ffmpeg、又写多张表。

---

## 4.6 Metrics / Event 写入单独抽象

当前 `metrics` map 与 `appendJobEvent` 在流程中到处散落，建议统一为：

- `JobMetricsWriter`
- `JobEventWriter`

优点：

- 不容易漏字段
- 更容易保证 key 名兼容
- 能统一处理阶段状态与最终状态

这点尤其重要，因为后台统计很依赖这些 key。

---

## 5. 建议的分阶段重构路线

## Phase 0：冻结行为，先补护栏

在动结构前，先补：

- 针对 `process()` 的回归测试骨架
- 关键 metrics key 不变性测试
- 关键 event stage 顺序测试
- public mirror 一致性测试
- cost / point 结算测试

这一步的目标不是重构，而是“先把行为钉住”。

---

## Phase 1：只做文件搬家，不改逻辑

优先把**纯辅助函数**搬走：

- metrics 组装
- event 写入
- temp cleanup
- source download/probe helper
- key layout helper
- packaging helper

这一步尽量不改主流程，只减少 `processor.go` 体积。

---

## Phase 2：提炼运行时上下文

引入 `PipelineRuntime` / `RunContext`：

- job
- options
- qualitySettings
- source/meta
- metrics
- tempDir
- requestedFormats
- selectedWindows
- uploadedKeys

这样后面函数签名就不用到处传十几个参数。

---

## Phase 3：提炼 HighlightPipeline

把 GIF 相关的：

- director
- planner
- rerank
- candidate persist
- sub-stage metrics

从总控中完整剥离成：

- `RunHighlightPipeline(ctx, runtime) (*HighlightPlan, error)`

这是收益最大的一步。

---

## Phase 4：拆渲染层

拆成：

- `RunStaticRenderPipeline(...)`
- `RunAnimatedRenderPipeline(...)`

并让它们输出统一的 `RenderArtifacts`。

---

## Phase 5：拆持久化层

让渲染层只返回文件结果，不直接写数据库。  
数据库落库统一在 `PersistRenderResult(...)` 完成。

这样后续做“重渲”、“回填”、“离线导入产物”会容易很多。

---

## Phase 6：拆后台统计文件

虽然本文件是 Processor 方案，但强烈建议同步规划：

- `internal/handlers/admin_video_jobs.go` 同步模块化
- 按 overview / detail / exports / integrity / baseline / health / ai-template 分文件

否则 Processor 降复杂度后，后台统计仍会继续成为另一个瓶颈。

---

## 6. 推荐的中间数据结构

建议补这些中间结构：

### `PipelineRuntime`

统一承载一次任务执行的上下文。

### `SourceBundle`

统一承载源视频相关信息。

### `HighlightPlan`

统一承载 AI 与本地 scorer 的最终高光计划。

### `RenderArtifacts`

统一承载渲染产物，不区分静态/动态来源。

### `PersistOutcome`

统一承载落库结果：

- collectionID
- totalFrames
- generatedFormats
- packageOutcome
- publicSyncStats

---

## 7. 不要在第一轮做的事

以下内容第一轮重构**不要碰**：

1. 表结构改造
2. 产物 key 规则改造
3. API 返回字段改造
4. 指标 key 重命名
5. AI Prompt 模板协议改造
6. 全量“面向接口”抽象

先把结构拆清楚，再考虑抽象优雅化。

---

## 8. 推荐优先级

### 最高优先级

1. 提炼 `PipelineRuntime`
2. 拆 `HighlightPipeline`
3. 拆静态/动态渲染层
4. 拆持久化层

### 中优先级

5. 抽 metrics / events writer
6. 收敛错误分类与 retry 策略
7. 后台视频统计模块化

### 低优先级

8. 进一步细化 provider/renderer interface
9. 把 AI 配置管理抽成更独立 service

---

## 9. 预期收益

完成上述重构后，预期会得到：

- `processor.go` 回到“编排器”角色
- 单次改动影响面更小
- 重渲、离线回填、AB 实验更容易扩展
- 单测能覆盖真正的阶段级行为
- 新同事能在 1~2 天内理解主流程，而不是在 1 个大文件里找逻辑

---

## 10. 最终建议

如果只允许做一个最值的切分，我建议先做：

> **先把 GIF Highlight + AI 子流水线整体抽出去。**

原因：

- 它最复杂
- 演进最快
- 目前又承担了最多策略变化
- 与后续 Prompt Template / rerender / feedback learning 强相关

这是降低后续维护成本的第一刀。

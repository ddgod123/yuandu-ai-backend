# Prompt 与模型调用细化

## 1. 当前基础

当前代码已经支持：

- `AIDirector`
- `AIPlanner`
- `AIJudge`
- `ops.video_ai_prompt_templates`
- `audit.video_ai_prompt_template_audits`
- OpenAI-compatible chat completion 调用模式
- 按阶段独立配置 provider/model/endpoint/api_key/max_tokens/timeout

对应代码：

- `internal/videojobs/ai_gif_pipeline.go`
- `internal/config/config.go`
- `internal/handlers/video_ai_prompt_templates.go`

这说明系统已经具备 prompt 工程化治理的底座。

---

## 2. Prompt 设计原则

## 2.1 原则一：结构化优先

所有 AI 输出都应优先 JSON 结构，而不是自然语言大片说明。

### 原因

- 更容易解析
- 更容易校验
- 更容易落库
- 更容易回放

当前 Director / Planner / Judge 已经这样做，方向是对的。

---

## 2.2 原则二：Prompt 只表达角色，不重复系统硬规则

例如：

- 时间窗合法性
- 非重叠筛选
- size budget
- hard gate

这些规则不要试图全塞进 prompt，而应由系统后处理完成。

### 原因

模型适合做模糊判断，不适合承担全部工程硬规则。

---

## 2.3 原则三：每个阶段只做一件事

| 阶段 | 只做什么 |
|---|---|
| AI1 | 任务目标与权重定义 |
| AI2 | 候选上的语义提名与排序 |
| AI3 | 成品上的语义复审与建议 |

不要让某个 prompt 同时承担多个角色。

---

## 3. Director Prompt 细化

## 3.1 输入建议

建议 Director 输入包含：

- 视频 meta 摘要
- 时长/FPS/尺寸
- 当前目标格式（gif）
- 候选召回摘要
- 任务来源场景
- 运营层 instruction
- 输出数量上限

### 不建议输入

- 大量逐 candidate 细粒度详情
- 全部帧级信息
- 全部 output 评测信息

---

## 3.2 输出契约建议

当前字段已足够，建议固定为：

- `business_goal`
- `audience`
- `must_capture`
- `avoid`
- `clip_count_min`
- `clip_count_max`
- `duration_pref_min_sec`
- `duration_pref_max_sec`
- `loop_preference`
- `style_direction`
- `risk_flags`
- `quality_weights`
- `directive_text`

### 建议新增校验约束

- `quality_weights` 必须总和接近 1
- `clip_count_max <= GIFCandidateMaxOutputs * 2`
- `duration_pref_max_sec <= 5.0`

---

## 3.3 Director 最佳实践 Prompt 结构

推荐模板结构：

```text
System:
  你是 GIF 任务的需求定义器，只负责输出结构化任务 brief。
Developer/Fixed:
  解释字段含义、取值范围、业务目标。
Editable/Operator:
  近期运营重点，如“优先情绪爆发、反应强、适合传播”。
User/Input:
  视频 meta + recall 摘要 + 任务上下文。
Output:
  JSON only.
```

### 当前系统支持映射

- `fixed`：内建或 `ops.video_ai_prompt_templates`
- `editable`：ai1 的 operator layer

这点和最佳实践高度一致。

---

## 4. Planner Prompt 细化

## 4.1 Planner 是最关键的 AI Prompt

原因：

- 它直接影响最终试渲窗口
- 它影响 render 成本
- 它影响后续 Judge 看到的样本质量

### 输入应高度结构化

推荐每个 candidate 统一格式：

```json
{
  "candidate_id": 101,
  "start_sec": 12.3,
  "end_sec": 14.8,
  "base_score": 0.74,
  "confidence_score": 0.68,
  "reason": "emotion_peak",
  "scene_score": 0.57,
  "motion_estimate": 0.41,
  "loop_estimate": 0.62,
  "clarity_estimate": 0.71,
  "time_bucket": "middle"
}
```

### 目的

把模型的注意力放在“排序与选择”，而不是“猜字段是什么意思”。

---

## 4.2 Planner 输出契约建议

继续沿用当前结构：

- `proposal_rank`
- `start_sec`
- `end_sec`
- `score`
- `proposal_reason`
- `semantic_tags`
- `expected_value_level`
- `standalone_confidence`
- `loop_friendliness_hint`

### 建议额外增加可选字段

- `primary_candidate_id`
- `diversity_group`
- `risk_note`

注意：

这些可以先放 `metadata`，不一定非要改强结构。

---

## 4.3 Planner Prompt 的核心要求

Prompt 里要强调：

- 优先遵循 Director brief
- 只在候选集合内提名
- 避免重复与高度重叠提案
- 优先选择可脱离上下文成立的片段
- 不要为了“剧情完整”牺牲 GIF 的瞬时传播价值

### 建议少强调的内容

- 文件大小绝对值
- 最终 deliver/reject 结论

这些更适合系统层与 Judge 层处理。

---

## 5. Judge Prompt 细化

## 5.1 Judge 的输入必须是成品

Judge 应看到：

- output id
- window 区间
- technical evaluation
- proposal reason
- director brief 摘要
- 文件大小与时长
- loop 指标

### 不建议只给 Judge 技术分

因为那样 Judge 只能做“技术分解释器”，做不到真正的语义复审。

---

## 5.2 Judge Prompt 的核心规则

Prompt 中建议写清楚：

- 你输出的是 recommendation，不是最终系统裁决
- 若样本技术风险明显，应倾向 `manual_review` 或 `reject`
- 对边界 case 要优先解释“为什么不是明显 deliver”
- 输出简洁、可执行的 `suggested_action`

### 推荐判断顺序

1. 是否语义上成立
2. 是否脱离上下文也成立
3. 是否适合 GIF 这种短循环载体
4. 是否值得交付给用户

---

## 6. 模型调用分工建议

当前配置中各阶段可独立设置：

- provider
- model
- endpoint
- api key
- prompt version
- timeout
- max tokens

### 推荐分工原则

#### AI1 Director

- 目标：稳定、快、便宜
- 更适合轻量模型
- 因为它输出的是 task brief，而不是最终精细判断

#### AI2 Planner

- 目标：强排序能力 + 候选理解能力
- 是最值得投入推理预算的一层之一

#### AI3 Judge

- 目标：稳定、解释清楚、边界判断保守
- 更重视稳定性与一致性

### 推荐总体策略

```text
Director: 轻量快模型
Planner: 轻量多模态/强排序模型
Judge: 稳定文本推理模型
```

注意：这是一种职责建议，不是要求你固定某一家模型。

---

## 7. Timeout / Tokens 细化建议

当前配置已有：

- `AI_DIRECTOR_TIMEOUT_SECONDS`
- `AI_PLANNER_TIMEOUT_SECONDS`
- `AI_JUDGE_TIMEOUT_SECONDS`
- 对应 `MAX_TOKENS`

### 推荐原则

#### Director

- timeout 短
- max tokens 中等偏低
- 超时直接 fallback

#### Planner

- timeout 中等
- max tokens 取决于候选数
- 超时允许退本地 topN

#### Judge

- timeout 相对最长
- max tokens 适度更高
- 超时可以保守降级到 `manual_review/internal`

---

## 8. Prompt 版本治理建议

当前已有版本管理能力，建议补齐一套规则：

### 每次版本变更至少记录

- 版本号
- 作用范围（format/stage/layer）
- 变更目标
- 预期提升指标
- 风险说明
- 回滚方案
- 创建人与激活人

### 激活原则

- 先小样本/灰度
- 再扩大
- 不与 scorer/hard gate 阈值同时大改

---

## 9. Prompt A/B 实验建议

## 9.1 建议优先实验 Planner

因为 Planner 最直接影响候选命中率。

### 观察指标

- proposal accept rate
- render 后 deliver rate
- rerender 率
- judge reject 率
- avg cost per deliver

---

## 9.2 Director 适合低频小步实验

Director 变化太大容易导致全链路目标函数漂移。

### 更适合改什么

- quality weight phrasing
- must_capture/avoid 的强调方式
- 风险词表

---

## 9.3 Judge 更适合保守迭代

Judge 一旦激进，会造成：

- deliver 不稳定
- manual review 爆炸
- 与 hard gate 冲突率上升

因此建议 Judge prompt 调整尽量小步、保守。

---

## 10. Prompt 失败与降级策略

## 10.1 Director

- 模板读取失败 -> built-in
- 调用失败 -> fallback directive
- JSON 解析失败 -> fallback directive

## 10.2 Planner

- 模板读取失败 -> built-in
- 调用失败 -> local candidates
- JSON 解析失败 -> local candidates

## 10.3 Judge

- 模板读取失败 -> built-in
- 调用失败 -> 保守默认 recommendation
- JSON 解析失败 -> `manual_review/internal`

### 原则

AI 失败不能导致主链路不可用。

---

## 11. Prompt 与代码的实施建议

如果要继续细化实现，建议代码层遵循：

### 11.1 输入构造器独立

例如：

- `buildDirectorPromptInput()`
- `buildPlannerPromptInput()`
- `buildJudgePromptInput()`

### 11.2 输出校验器独立

例如：

- `validateDirectorResponse()`
- `validatePlannerResponse()`
- `validateJudgeResponse()`

### 11.3 Prompt Snapshot 统一持久化

每次调用都能追到：

- stage
- provider/model/endpoint
- prompt version
- resolved template version
- raw response 摘要
- fallback used

---

## 12. 最终建议

Prompt 这块最重要的不是“写得更华丽”，而是：

- 结构化
- 单一职责
- 可校验
- 可版本化
- 可回滚

你当前系统已经有了很好的基础，下一步只要把输入/输出契约再收紧，把实验流程再规范，Prompt 层就会非常稳。
